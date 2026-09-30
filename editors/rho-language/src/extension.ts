import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import * as os from 'os';
import { execFile } from 'child_process';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    State,
    TransportKind
} from 'vscode-languageclient/node';

let client: LanguageClient;
let status: vscode.StatusBarItem;
/** The toolchain this session found, so a command runs the same one. */
let toolchain: string | undefined;

/**
 * Where the Rho home is.
 *
 * The extension looks here rather than at the workspace because a toolchain is
 * deployed, not checked out: the sources it was built from are not on the machine
 * that runs it, so a path relative to a project would find nothing on exactly the
 * machines that installed Rho to use it.
 */
function rhoHome(): string {
    return process.env.RHO_HOME ?? path.join(os.homedir(), '.rho');
}

/** Server is the language server, and how to start it. */
interface Server {
    command: string;
    args: string[];
    source: string;
}

/**
 * Where the language server is, in the order a person would look.
 *
 * There is no standalone server binary: the server is a component the toolchain
 * dispatches to, the way `rho doc` dispatches to the generator. So what is found
 * here is the toolchain, and what is run is `rho lsp`.
 *
 * 1. The setting, because a person who set it meant it. A relative path is read
 *    from the Rho home, which is where a deployed toolchain puts itself.
 * 2. `RHO_BIN`, which the toolchain sets for the processes it launches, so a
 *    session started through `rho` uses that toolchain rather than a copy found
 *    by searching.
 * 3. The Rho home, which is where `rho install` puts the binary.
 * 4. The path, which is where it is once somebody has followed the install
 *    instructions.
 *
 * When none of those find it, the toolchain is not installed on this machine and
 * nothing the extension can do will change that — so it says so and offers to
 * install it, rather than failing with a path nobody chose.
 */
function findServer(): Server | undefined {
    const configured = (vscode.workspace.getConfiguration('rho.lsp').get<string>('serverPath') ?? '').trim();
    if (configured !== '') {
        const candidate = path.isAbsolute(configured) ? configured : path.join(rhoHome(), configured);
        if (isRunnable(candidate)) {
            return { command: candidate, args: ['lsp'], source: 'the setting' };
        }
        // A path that was set and is not there is worth saying out loud: the
        // fallback would find the toolchain anyway, and the person would never
        // learn the setting they wrote is doing nothing.
        vscode.window.showWarningMessage(
            `Rho: ${configured} is set as the toolchain but is not there. Using the one that is installed.`
        );
    }

    const fromEnv = process.env.RHO_BIN;
    if (fromEnv && isRunnable(fromEnv)) {
        return { command: fromEnv, args: ['lsp'], source: 'RHO_BIN' };
    }

    const installed = path.join(rhoHome(), 'bin', 'rho');
    if (isRunnable(installed)) {
        return { command: installed, args: ['lsp'], source: 'the Rho home' };
    }

    const found = which('rho');
    if (found) {
        return { command: found, args: ['lsp'], source: 'PATH' };
    }
    return undefined;
}

/** isRunnable reports whether a path is a file that can be run. */
function isRunnable(candidate: string): boolean {
    try {
        const info = fs.statSync(candidate);
        if (!info.isFile()) {
            return false;
        }
        // A file with no execute bit is not something to run, and reporting it as
        // found would fail later with a message about permissions rather than
        // about where the toolchain is.
        if (process.platform !== 'win32') {
            fs.accessSync(candidate, fs.constants.X_OK);
        }
        return true;
    } catch {
        return false;
    }
}

/** which finds a program on the path, without a shell. */
function which(program: string): string | undefined {
    const dirs = (process.env.PATH ?? '').split(path.delimiter).filter(Boolean);
    const suffixes = process.platform === 'win32' ? ['.exe', '.cmd', ''] : [''];
    for (const dir of dirs) {
        for (const suffix of suffixes) {
            const candidate = path.join(dir, program + suffix);
            if (isRunnable(candidate)) {
                return candidate;
            }
        }
    }
    return undefined;
}

/** run runs the toolchain and answers with what it said, or why it could not. */
function run(args: string[], cwd?: string): Promise<{ ok: boolean; out: string }> {
    return new Promise(resolve => {
        if (!toolchain) {
            resolve({ ok: false, out: 'the Rho toolchain was not found' });
            return;
        }
        execFile(toolchain, args, { cwd, timeout: 60_000, maxBuffer: 4 * 1024 * 1024 }, (err, stdout, stderr) => {
            // A toolchain that answered with an error said something useful, so
            // its output is kept either way: the message is what a person needs
            // when a build fails, and discarding it is how a tool becomes
            // mysteriously unhelpful.
            resolve({ ok: !err, out: `${stdout ?? ''}${stderr ?? ''}`.trim() });
        });
    });
}

/** inTerminal runs the toolchain where its output is visible and long-lived. */
function inTerminal(args: string[], cwd?: string) {
    const terminal = vscode.window.createTerminal({ name: `rho ${args[0] ?? ''}`.trim(), cwd });
    terminal.show();
    terminal.sendText(toolchain ? `${toolchain} ${args.join(' ')}` : `rho ${args.join(' ')}`);
}

/** workspaceRoot is the folder a project command runs in. */
function workspaceRoot(): string | undefined {
    return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

/** toolchainVersion is what the toolchain says it is. */
function toolchainVersion(): Promise<string | undefined> {
    return new Promise(resolve => {
        if (!toolchain) {
            resolve(undefined);
            return;
        }
        execFile(toolchain, ['--version'], { timeout: 4000 }, (err, stdout) => {
            if (err) {
                resolve(undefined);
                return;
            }
            const first = stdout.split('\n')[0]?.trim();
            resolve(first && /v?\d+\.\d+/.test(first) ? first : undefined);
        });
    });
}

export async function activate(context: vscode.ExtensionContext) {
    status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    status.command = 'rho.showStatus';
    context.subscriptions.push(status);
    showStatus('finding Rho', 'busy');

    const found = findServer();
    if (!found) {
        showStatus('Rho not installed', 'error');
        await offerInstall();
        return;
    }
    toolchain = found.command;

    const serverOptions: ServerOptions = {
        command: found.command,
        args: found.args,
        transport: TransportKind.stdio,
        options: { cwd: workspaceRoot() ?? os.homedir() }
    };

    const clientOptions: LanguageClientOptions = {
        documentSelector: [{ language: 'rho' }],
        synchronize: {
            fileEvents: vscode.workspace.createFileSystemWatcher('**/*.rho'),
            configurationSection: 'rho'
        },
        uriConverters: {
            // A path, not a file:// URI. The server normalises both, and sending a
            // bare path is what it is tested against.
            code2Protocol: (uri: vscode.Uri): string => uri.fsPath,
            protocol2Code: (str: string): vscode.Uri => vscode.Uri.file(str)
        }
    };

    client = new LanguageClient('rho-lsp', 'Rho Language Server', serverOptions, clientOptions);

    // The status bar follows the client's own state rather than a guess at it, so
    // it says stopped when the server stopped — which is the case a person is
    // usually looking at the bar to find out.
    client.onDidChangeState(e => {
        if (e.newState === State.Running) {
            showStatus('Rho: ready', 'ok', found);
        } else if (e.newState === State.Starting) {
            showStatus('Rho: starting', 'busy', found);
        } else {
            showStatus('Rho: stopped', 'error', found);
        }
    });

    register(context, 'rho.showStatus', async () => {
        const version = await toolchainVersion();
        const lines = [
            `Rho Toolchain Status`,
            `  toolchain: ${version ?? 'not reported'}`,
            `  server: ${found.command}`,
            `  found in: ${found.source}`,
            `  client: ${client ? client.state : State.Stopped}`,
        ];
        vscode.window.showInformationMessage(lines.join('\n'));
    });

    // The reference is read from the toolchain rather than written here, so it
    // lists the commands this machine has rather than the ones it had when the
    // extension was written.
    register(context, 'rho.cliReference', async () => {
        const { ok, out } = await run(['help']);
        const doc = await vscode.workspace.openTextDocument({
            language: 'markdown',
            content: out || 'The Rho toolchain was not found, so there is no reference to show.'
        });
        await vscode.window.showTextDocument(doc, { preview: true });
        if (!ok && out === '') {
            vscode.window.showErrorMessage('The Rho toolchain was not found.');
        }
    });

    register(context, 'rho.build', () => inTerminal(['build', '.'], workspaceRoot()));
    register(context, 'rho.run', async () => {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
            vscode.window.showInformationMessage('Open a file to run.');
            return;
        }
        inTerminal(['run', editor.document.uri.fsPath], workspaceRoot());
    });
    register(context, 'rho.test', () => inTerminal(['test'], workspaceRoot()));
    register(context, 'rho.doc', () => inTerminal(['doc', '.', '-out', 'build/docs'], workspaceRoot()));
    register(context, 'rho.modules', () => inTerminal(['module', 'list'], workspaceRoot()));

    try {
        await client.start();
    } catch (err) {
        showStatus('Rho: failed', 'error', found);
        // The server is a component, and a component that is installed and not
        // enabled is refused by name. That is a thing a person can put right in
        // one command, so it is offered rather than reported as a failure they
        // have to interpret.
        const choice = await vscode.window.showErrorMessage(
            `The Rho language server did not start: ${err}`,
            'Enable the server',
            'Show the toolchain'
        );
        if (choice === 'Enable the server') {
            const terminal = vscode.window.createTerminal({ name: 'Rho' });
            terminal.show();
            terminal.sendText('rho module enable rho-lsp');
        } else if (choice === 'Show the toolchain') {
            vscode.commands.executeCommand('rho.showStatus');
        }
    }
}

/**
 * Offer to install the language server, when there is no toolchain to dispatch
 * it.
 *
 * The server is a component, so "install it" is a component being installed and
 * turned on. Offering the command that does it, rather than a sentence about
 * where to get it, is the difference between a person who can act and one who has
 * to go and read something first.
 */
async function offerInstall() {
    const choice = await vscode.window.showWarningMessage(
        'Rho is not installed on this machine, so its language server cannot start.',
        'Install with the Rho CLI',
        'Set a path instead'
    );
    if (choice === 'Install with the Rho CLI') {
        const terminal = vscode.window.createTerminal({ name: 'Rho Installer' });
        terminal.show();
        terminal.sendText('rho install');
    } else if (choice === 'Set a path instead') {
        vscode.commands.executeCommand('workbench.action.openSettings', 'rho.lsp.serverPath');
    }
}

function register(context: vscode.ExtensionContext, command: string, handler: (...args: any[]) => any) {
    context.subscriptions.push(vscode.commands.registerCommand(command, handler));
}

/** showStatus is the bar, said the same way every time. */
function showStatus(text: string, kind: 'ok' | 'busy' | 'error', found?: { command: string; source: string }): void {
    if (!status) {
        return;
    }
    status.text = kind === 'busy' ? `$(sync~spin) ${text}` : `$(circuit-board) ${text}`;
    status.tooltip = found
        ? `Rho language server\n${found.command}\nfound in ${found.source}`
        : 'Rho language server';
    status.backgroundColor = kind === 'error' ? new vscode.ThemeColor('statusBarItem.errorBackground') : undefined;
    status.show();
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    return client.stop();
}
