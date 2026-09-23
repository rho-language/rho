import * as vscode from 'vscode';
import * as path from 'path';
import * as fs from 'fs';
import {
    LanguageClient,
    LanguageClientOptions,
    ServerOptions,
    TransportKind
} from 'vscode-languageclient/node';

let client: LanguageClient;

export async function activate(context: vscode.ExtensionContext) {
    try {
        const workspaceFolders = vscode.workspace.workspaceFolders;
        const workspaceRoot = workspaceFolders?.[0]?.uri.fsPath;

        // 1. Resolve server path safely
        const serverPath = vscode.workspace.getConfiguration('rho.lsp').get<string>('serverPath') || 'bin/rho';
        const resolvedCommand = path.isAbsolute(serverPath)
            ? serverPath
            : path.join(workspaceRoot || process.cwd(), serverPath);

        // 2. Validate binary existence before spawning
        if (!fs.existsSync(resolvedCommand)) {
            const msg = `Rho LSP binary not found at: ${resolvedCommand}. Check your workspace or rho.lsp.serverPath setting.`;
            vscode.window.showErrorMessage(msg);
            return;
        }

        const serverOptions: ServerOptions = {
            command: resolvedCommand,
            args: ['lsp'],
            transport: TransportKind.stdio,
            options: {
                cwd: workspaceRoot
            }
        };

        const clientOptions: LanguageClientOptions = {
            documentSelector: [{ language: 'rho' }],
            synchronize: {
                fileEvents: vscode.workspace.createFileSystemWatcher('**/*.rho'),
                configurationSection: 'rho'
            },
            uriConverters: {
                code2Protocol: (uri: vscode.Uri): string => uri.fsPath,
                protocol2Code: (str: string): vscode.Uri => vscode.Uri.file(str)
            }
        };

        client = new LanguageClient(
            'rho-lsp',
            'Rho Language Server',
            serverOptions,
            clientOptions
        );

        context.subscriptions.push(client);
        await client.start();

        console.log('[Rho] LSP client started successfully');

    } catch (err) {
        vscode.window.showErrorMessage(`Rho activation error: ${err}`);
    }
}

export function deactivate(): Thenable<void> | undefined {
    if (!client) {
        return undefined;
    }
    return client.stop();
}