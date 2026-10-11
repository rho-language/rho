import * as vscode from 'vscode';
import * as os from 'os';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    State,
    TransportKind
} from 'vscode-languageclient/node';
import {
    Component,
    findToolchain,
    inTerminal,
    run,
    setToolchain,
    toolchainPath,
    toolchainSource,
    toolchainVersion,
    workspaceRoot
} from './toolchain';
import { registerDebugging } from './debugger';
import { registerPeripherals } from './peripherals';
import { ensureKeeper, registerSyntaxHelp } from './help';

let client: LanguageClient;
let status: vscode.StatusBarItem;

/** Server is the language server, and how to start it. */
interface Server extends Component {
    args: string[];
}

/**
 * The language server, which is a component the toolchain dispatches to.
 *
 * Nothing is found here beyond the toolchain: the server is not a separate program
 * with its own path, it is `rho lsp`, and the search for where the toolchain is
 * belongs to one place rather than to three of them.
 */
function findLanguageServer(): Server | undefined {
    const found = findToolchain();
    if (!found) {
        return undefined;
    }
    return { command: found.command, args: ['lsp'], source: found.source };
}

export async function activate(context: vscode.ExtensionContext) {
    status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
    status.command = 'rho.showStatus';
    context.subscriptions.push(status);
    showStatus('finding Rho', 'busy');

    const found = findLanguageServer();
    if (!found) {
        showStatus('Rho not installed', 'error');
        await offerInstall();
        return;
    }
    setToolchain(found);

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
            `  version: ${version ?? 'not reported'}`,
            `  toolchain: ${toolchainPath() ?? 'not found'}`,
            `  found in: ${toolchainSource() ?? 'nowhere'}`,
            `  server: ${found.command}`,
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

    // Debugging and the peripheral panel, both of which dispatch to components the
    // same way the language server does. Registered before the server starts so a
    // project whose sources are generated — an embedded one — has F5 and its
    // registers working anyway.
    registerDebugging(context);
    registerPeripherals(context);

    // Language help: the reference for the word under the cursor, served by the
    // keeper. The keeper is started here so the first hover is answered rather
    // than the one after a person has opened the dashboard themselves.
    registerSyntaxHelp(context);
    void ensureKeeper();

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
    // The tooltip says the toolchain and the debugger, not just the server: the two
    // failures a person meets first are "the toolchain is not installed" and "this
    // board has no debugger", and both are visible from here.
    status.tooltip = [
        toolchainPath() ?? 'Rho toolchain not found',
        toolchainSource() ? `found in ${toolchainSource()}` : undefined,
        found ? `language server: ${found.command}` : undefined,
        'debug adapter: rho dbg --dap',
        'peripherals: rho dbg registers'
    ]
        .filter(line => line !== undefined)
        .join('\n');
    status.backgroundColor = kind === 'error' ? new vscode.ThemeColor('statusBarItem.errorBackground') : undefined;
    status.show();
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    return client.stop();
}
