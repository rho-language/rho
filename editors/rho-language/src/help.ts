//
// @file help.ts
// @brief Language help for the word under the cursor.
//
// The reference is a page, and a person reading code is not reading it. This
// asks `rho keeper` for the section that explains the word the cursor is on and
// shows it in a hover, with a link to the page for the rest. The keeper is
// started if it is not already running, because a hover that only works after
// someone has opened the dashboard is a hover that mostly does not work.
//
// @project Rho
// @license Apache-2.0
//

import * as vscode from 'vscode';
import * as cp from 'child_process';
import * as http from 'http';
import * as https from 'https';
import { findToolchain, workspaceRoot } from './toolchain';

const defaultKeeper = 'http://127.0.0.1:8477';
const wordPattern = /[A-Za-z_][A-Za-z0-9_]*/;

/**
 * keeperURL is where the dashboard is, configured or by its own default, and
 * only when it is an http(s) address.
 *
 * The setting is machine-scoped so a repository cannot set it, but a value that
 * is not an http address is refused all the same: the address is fetched and
 * opened in a browser, and a `file:` or `command:` URL is neither a keeper nor
 * something to open.
 */
function keeperURL(): string | undefined {
    const configured = vscode.workspace.getConfiguration('rho').get<string>('keeper.url');
    const candidate = configured && configured.trim() !== '' ? configured.trim() : defaultKeeper;
    try {
        const parsed = new URL(candidate);
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
            return undefined;
        }
    } catch {
        return undefined;
    }
    return candidate.replace(/\/+$/, '');
}

function delay(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
}

/** getJSON reads a URL, or gives undefined — an unreachable keeper is not an error to throw. */
function getJSON(url: string, timeoutMs = 2500): Promise<any | undefined> {
    return new Promise(resolve => {
        const client = url.startsWith('https') ? https : http;
        const request = client.get(url, response => {
            if (!response.statusCode || response.statusCode >= 400) {
                response.resume();
                resolve(undefined);
                return;
            }
            let body = '';
            response.on('data', chunk => (body += chunk));
            response.on('end', () => {
                try {
                    resolve(JSON.parse(body));
                } catch {
                    resolve(undefined);
                }
            });
        });
        request.on('error', () => resolve(undefined));
        request.setTimeout(timeoutMs, () => {
            request.destroy();
            resolve(undefined);
        });
    });
}

/**
 * ensureKeeper returns the dashboard's address, starting it when it is not
 * already running.
 *
 * A dashboard a person did not open is not running, so the extension starts
 * one: detached, so it outlives the window, and quiet, because it is a service
 * this asks rather than something to look at. When it is already there — a
 * person has the dashboard open — its address is used as it is.
 */
export async function ensureKeeper(): Promise<string | undefined> {
    const base = keeperURL();
    if (!base) {
        return undefined;
    }
    if (await getJSON(base + '/api/v1/system')) {
        return base;
    }
    const found = findToolchain();
    if (!found) {
        return undefined;
    }
    let host = '127.0.0.1:8477';
    try {
        host = new URL(base).host;
    } catch {
        // A malformed setting falls back to the default the dashboard uses.
    }
    try {
        const child = cp.spawn(found.command, ['keeper', 'start', '-addr', host], {
            detached: true,
            stdio: 'ignore',
            cwd: workspaceRoot() ?? undefined
        });
        child.unref();
    } catch {
        return undefined;
    }
    for (let attempt = 0; attempt < 40; attempt++) {
        await delay(250);
        if (await getJSON(base + '/api/v1/system')) {
            return base;
        }
    }
    return undefined;
}

/** registerSyntaxHelp adds the hover and the command that opens the full page. */
export function registerSyntaxHelp(context: vscode.ExtensionContext): void {
    const hover = vscode.languages.registerHoverProvider('rho', {
        async provideHover(document, position) {
            const range = document.getWordRangeAtPosition(position, wordPattern);
            if (!range) {
                return undefined;
            }
            const word = document.getText(range);
            const base = await ensureKeeper();
            if (!base) {
                return undefined;
            }
            const help = await getJSON(`${base}/api/v1/help/${encodeURIComponent(word)}`);
            if (!help) {
                return undefined;
            }
            const markdown = new vscode.MarkdownString();
            markdown.appendMarkdown(`**Rho: \`${help.keyword}\`** — _${help.title}_\n\n`);
            markdown.appendMarkdown(help.markdown);
            markdown.appendMarkdown(`\n\n[Open the full page](${base}/#/docs/${help.route})`);
            // A link a person can click, and no command: the content is the
            // reference's, but a hover that could run a command is a hover a
            // document could make run one, and the reference is a document.
            markdown.isTrusted = { enabledCommands: [] };
            return new vscode.Hover(markdown, range);
        }
    });
    context.subscriptions.push(hover);

    context.subscriptions.push(
        vscode.commands.registerCommand('rho.syntaxHelp', async () => {
            const base = await ensureKeeper();
            if (!base) {
                vscode.window.showInformationMessage('Rho keeper is not reachable, so there is no page to open.');
                return;
            }
            let route = 'language/syntax';
            const editor = vscode.window.activeTextEditor;
            if (editor) {
                const range = editor.document.getWordRangeAtPosition(editor.selection.active, wordPattern);
                if (range) {
                    const help = await getJSON(`${base}/api/v1/help/${encodeURIComponent(editor.document.getText(range))}`);
                    if (help && help.route) {
                        route = help.route;
                    }
                }
            }
            vscode.env.openExternal(vscode.Uri.parse(`${base}/#/docs/${route}`));
        })
    );
}
