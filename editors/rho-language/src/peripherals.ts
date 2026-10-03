/**
 * The peripheral view.
 *
 * A stopped microcontroller without its registers is half a machine: the program
 * is sitting on a line that reads a status register, the value is there, and there
 * is nothing to call it. This panel is that something — the chip's interfaces, their
 * registers and the bits in them, with their addresses.
 *
 * The map comes from `rho dbg registers`, not from parsing a CMSIS-SVD file here.
 * An SVD file is XML with three spellings for a field's bounds and clusters nested
 * inside clusters; reading it a second time in another language means a second set
 * of rules about all of that, and the one that is wrong shows a plausible value
 * where the truth is a different one. The component reads it once, with tests.
 *
 * No values are shown here. A register's contents are a fact about right now, and
 * reading them needs a live session: they are in the debug view's Peripherals
 * scope, where the debugger is the one reading them from the board.
 */

import * as vscode from 'vscode';
import { run, runJson, workspaceRoot } from './toolchain';

/** A register map, as `rho dbg registers` answers with. */
interface RegisterMap {
    device: string;
    source?: string;
    comment?: string;
    peripherals: Peripheral[];
}

interface Peripheral {
    name: string;
    description?: string;
    address: string;
    registers: Register[];
}

interface Register {
    name: string;
    description?: string;
    address: string;
    size: number;
    access?: string;
    readable: boolean;
    fields?: Field[];
}

interface Field {
    name: string;
    description?: string;
    offset: number;
    width: number;
    bits: string;
    mask: string;
}

/** The view's own state, so a refresh does not have to be told what it had. */
let cached: RegisterMap | undefined;

export const PERIPHERAL_VIEW = 'rho.peripherals';

/** Shows the chip's peripherals, from whatever the project is built for. */
export class PeripheralTree implements vscode.TreeDataProvider<Node> {
    private changed = new vscode.EventEmitter<Node | undefined>();
    readonly onDidChangeTreeData = this.changed.event;

    /** refresh re-reads the map, which is what a changed board spec should do. */
    refresh(): void {
        cached = undefined;
        this.changed.fire(undefined);
    }

    getTreeItem(element: Node): vscode.TreeItem {
        const item = new vscode.TreeItem(
            element.label,
            element.kind === 'map'
                ? vscode.TreeItemCollapsibleState.Expanded
                : element.kind === 'peripheral'
                  ? vscode.TreeItemCollapsibleState.Collapsed
                  : vscode.TreeItemCollapsibleState.None
        );
        if (element.description) {
            item.description = element.description;
        }
        item.tooltip = new vscode.MarkdownString(element.tooltip ?? element.label);
        if (element.kind === 'register') {
            item.iconPath = new vscode.ThemeIcon(element.readable ? 'symbol-constant' : 'circle-slash');
        } else if (element.kind === 'peripheral') {
            item.iconPath = new vscode.ThemeIcon('circuit-board');
        }
        return item;
    }

    async getChildren(element?: Node): Promise<Node[]> {
        const root = workspaceRoot();
        if (!root) {
            return [];
        }
        if (!element) {
            const map = await this.load(root);
            if (!map) {
                return [];
            }
            return [
                new Node({
                    kind: 'map',
                    label: map.device,
                    description: `${map.peripherals.length} peripherals`,
                    tooltip: map.comment ?? `From ${map.source ?? 'the vendor'}`
                })
            ];
        }

        const map = cached;
        if (!map) {
            return [];
        }
        if (element.kind === 'map') {
            return map.peripherals.map(
                peripheral =>
                    new Node({
                        kind: 'peripheral',
                        label: peripheral.name,
                        description: peripheral.address,
                        tooltip: peripheral.description ?? peripheral.name,
                        peripheral
                    })
            );
        }
        if (element.kind === 'peripheral' && element.peripheral) {
            return element.peripheral.registers.map(
                register =>
                    new Node({
                        kind: 'register',
                        label: register.name,
                        description: `${register.address} · ${register.size * 8} bits${
                            register.readable ? '' : ' · write-only'
                        }`,
                        tooltip: register.description ?? register.name,
                        register
                    })
            );
        }
        if (element.kind === 'register' && element.register) {
            return (element.register.fields ?? []).map(
                field =>
                    new Node({
                        kind: 'field',
                        label: field.name,
                        description: `bits ${field.bits} · ${field.mask}`,
                        tooltip: field.description ?? field.name
                    })
            );
        }
        return [];
    }

    /** load reads the map, once, and remembers it until something asks again. */
    private async load(root: string): Promise<RegisterMap | undefined> {
        if (cached) {
            return cached;
        }
        const target = vscode.workspace.getConfiguration('rho.debug').get<string>('target') ?? '';
        const args = ['dbg', 'registers', '-cwd', root];
        if (target) {
            args.push('-target', target);
        }
        const svd = vscode.workspace.getConfiguration('rho.debug').get<string>('svdFile');
        if (svd) {
            args.push('-svd', svd);
        }
        const { ok, out } = await run(args, root);
        if (!ok) {
            // Said where the panel is rather than as a modal: this is a panel that
            // updates on a workspace change, and a dialog every time the tree
            // refreshes would be one per keystroke.
            void vscode.window.setStatusBarMessage(`Rho: ${out.split('\n')[0]}`, 5000);
            return undefined;
        }
        cached = await runJson<RegisterMap>(args, root);
        return cached;
    }
}

/** One row of the tree. */
export class Node {
    readonly kind: 'map' | 'peripheral' | 'register' | 'field';
    readonly label: string;
    readonly description?: string;
    readonly tooltip?: string;
    readonly peripheral?: Peripheral;
    readonly register?: Register;
    readonly readable: boolean;

    constructor(node: {
        kind: 'map' | 'peripheral' | 'register' | 'field';
        label: string;
        description?: string;
        tooltip?: string;
        peripheral?: Peripheral;
        register?: Register;
        readable?: boolean;
    }) {
        this.kind = node.kind;
        this.label = node.label;
        this.description = node.description;
        this.tooltip = node.tooltip;
        this.peripheral = node.peripheral;
        this.register = node.register;
        this.readable = node.register?.readable ?? true;
    }
}

/** Registers the panel, and the refresh that goes with it. */
export function registerPeripherals(context: vscode.ExtensionContext): PeripheralTree {
    const tree = new PeripheralTree();
    context.subscriptions.push(
        vscode.window.registerTreeDataProvider(PERIPHERAL_VIEW, tree),
        vscode.commands.registerCommand('rho.peripherals.refresh', () => tree.refresh())
    );
    // A board spec changing changes the chip, and the panel would otherwise keep
    // showing the previous one's registers until the window was reloaded.
    context.subscriptions.push(
        vscode.workspace.onDidChangeConfiguration(event => {
            if (event.affectsConfiguration('rho.debug')) {
                tree.refresh();
            }
        })
    );
    return tree;
}