/**
 * Where the toolchain is, and how to run it.
 *
 * Extracted from the extension's activation because three things need it and none
 * of them is a language server: the language server, the debug adapter, and a
 * peripheral tree. Each of them is a component the toolchain dispatches to, so each
 * of them runs the same program with a different command — and a second copy of this
 * search would be a second thing to keep in step with the first.
 *
 * A Rho language server, a Rho debug adapter and a Rho peripheral view all run `rho`
 * with a different subcommand. Nothing else about them is different.
 */

import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import * as os from 'os';
import { execFile } from 'child_process';

/** The toolchain this session found, so every command runs the same one. */
let toolchain: string | undefined;

/** Where it was found, for the status bar and for a message that says so. */
let source: string | undefined;

/** Server is the toolchain and a subcommand: what a component is, in this toolchain. */
export interface Component {
    command: string;
    args: string[];
    source: string;
}

/** Where the Rho home is. */
export function rhoHome(): string {
    return process.env.RHO_HOME ?? path.join(os.homedir(), '.rho');
}

/**
 * Where the toolchain is, in the order a person would look.
 *
 * 1. The setting, because a person who set it meant it. A relative path is read
 *    from the Rho home, which is where a deployed toolchain puts itself.
 * 2. `RHO_BIN`, which the toolchain sets for the processes it launches, so a
 *    session started through `rho` uses that toolchain rather than a copy found by
 *    searching.
 * 3. The Rho home, which is where `rho install` puts the binary.
 * 4. The path, which is where it is once somebody has followed the install
 *    instructions.
 *
 * When none of those find it, the toolchain is not installed on this machine and
 * nothing the extension can do will change that — so the caller says so and offers
 * to install it, rather than failing with a path nobody chose.
 */
export function findToolchain(): Component | undefined {
    const configured = (vscode.workspace.getConfiguration('rho.lsp').get<string>('serverPath') ?? '').trim();
    if (configured !== '') {
        const candidate = path.isAbsolute(configured) ? configured : path.join(rhoHome(), configured);
        if (isRunnable(candidate)) {
            return { command: candidate, args: [], source: 'the setting' };
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
        return { command: fromEnv, args: [], source: 'RHO_BIN' };
    }

    const installed = path.join(rhoHome(), 'bin', 'rho');
    if (isRunnable(installed)) {
        return { command: installed, args: [], source: 'the Rho home' };
    }

    const found = which('rho');
    if (found) {
        return { command: found, args: [], source: 'PATH' };
    }
    return undefined;
}

/** isRunnable reports whether a path is a file that can be run. */
export function isRunnable(candidate: string): boolean {
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
export function which(program: string): string | undefined {
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

/** remember which toolchain this session found. */
export function setToolchain(found: Component | undefined): void {
    toolchain = found?.command;
    source = found?.source;
}

/** The toolchain this session found. */
export function toolchainPath(): string | undefined {
    return toolchain;
}

/** Where it was found. */
export function toolchainSource(): string | undefined {
    return source;
}

/** run runs the toolchain and answers with what it said, or why it could not. */
export function run(args: string[], cwd?: string, timeout = 60_000): Promise<{ ok: boolean; out: string }> {
    return new Promise(resolve => {
        if (!toolchain) {
            resolve({ ok: false, out: 'the Rho toolchain was not found' });
            return;
        }
        execFile(
            toolchain,
            args,
            { cwd, timeout, maxBuffer: 8 * 1024 * 1024 },
            (err, stdout, stderr) => {
                // A toolchain that answered with an error said something useful, so
                // its output is kept either way: the message is what a person needs
                // when a command fails, and discarding it is how a tool becomes
                // mysteriously unhelpful.
                resolve({ ok: !err, out: `${stdout ?? ''}${stderr ?? ''}`.trim() });
            }
        );
    });
}

/** runJson runs the toolchain and parses what it answered with. */
export async function runJson<T>(args: string[], cwd?: string): Promise<T | undefined> {
    const { ok, out } = await run(args, cwd);
    if (!ok || out.trim() === '') {
        return undefined;
    }
    try {
        return JSON.parse(out) as T;
    } catch {
        // A toolchain that answered with something else has still answered, and
        // showing its words beats showing nothing: the answer to a failed question
        // is often in the sentence rather than in the JSON.
        return undefined;
    }
}

/** inTerminal runs the toolchain where its output is visible and long-lived. */
export function inTerminal(args: string[], cwd?: string, name?: string): vscode.Terminal {
    const terminal = vscode.window.createTerminal({
        name: name ?? `rho ${args[0] ?? ''}`.trim(),
        cwd
    });
    terminal.show();
    terminal.sendText(toolchain ? `${toolchain} ${args.join(' ')}` : `rho ${args.join(' ')}`);
    return terminal;
}

/** workspaceRoot is the folder a project command runs in. */
export function workspaceRoot(): string | undefined {
    return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

/** toolchainVersion is what the toolchain says it is. */
export function toolchainVersion(): Promise<string | undefined> {
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