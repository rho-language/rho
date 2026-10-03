/**
 * Debugging a Rho project.
 *
 * The adapter is `rho dbg --dap`: this extension does not implement the Debug
 * Adapter Protocol, it starts the program that does, and the protocol runs between
 * the editor and that program. What lives here is the three things an editor needs
 * around it and the component cannot have:
 *
 *   - a descriptor, so the editor knows how to start the adapter
 *   - a configuration provider, so F5 works in a project nobody has edited
 *   - the build a session depends on, run through the task system so its problems
 *     land in the Problems panel rather than in a terminal somebody has to watch
 *
 * The board's facts are not here. Which debugger, which probe, which register map
 * come from the target spec, through the adapter — so the launch configuration in
 * a repository and the session it produces cannot disagree about the board.
 */

import * as vscode from 'vscode';
import * as path from 'path';
import { Component, run, runJson, toolchainPath, workspaceRoot } from './toolchain';

/** The debug type this extension contributes. */
const DEBUG_TYPE = 'rho';

/** The task a session builds with, so its problems are navigable. */
const BUILD_TASK = 'rho: build';

/** The label the task carries, which is how a session refers to it. */
const BUILD_LABEL = 'rho build';

/** Configuration as this extension reads it. */
interface RhoDebugConfiguration extends vscode.DebugConfiguration {
    /** The built program. Filled in from the project when not given. */
    program?: string;
    /** The board, as its target spec names it. */
    target?: string;
    /** A debugger this machine has, when the board's is not the one wanted. */
    engine?: string;
    /** Where the chip's register map is, for a project that vendors its own. */
    svdFile?: string;
    /** The probe server, when this machine has it somewhere unusual. */
    serverpath?: string;
    /** Whether the session builds first. On unless the configuration says not to. */
    build?: boolean;
}

/** What `rho dbg engines` answers with. */
interface EnginesAnswer {
    engines?: { name: string; kind: string; comment?: string }[];
}

/** Registers the debug adapter, its configurations, and the build a session needs. */
export function registerDebugging(context: vscode.ExtensionContext): void {
    context.subscriptions.push(
        vscode.debug.registerDebugConfigurationProvider(DEBUG_TYPE, new ConfigurationProvider())
    );
    context.subscriptions.push(
        vscode.debug.registerDebugAdapterDescriptorFactory(DEBUG_TYPE, new DescriptorFactory())
    );

    // F5 without a configuration, and the two things a session needs that are not
    // debugging: the build it depends on, and the flash that puts a program on a
    // board.
    context.subscriptions.push(
        vscode.commands.registerCommand('rho.debug', async () => {
            const configuration = await new ConfigurationProvider().defaultConfiguration();
            await vscode.debug.startDebugging(vscode.workspace.workspaceFolders?.[0], configuration);
        })
    );
    context.subscriptions.push(
        vscode.commands.registerCommand('rho.debug.attach', async () => {
            const configuration = await new ConfigurationProvider().defaultConfiguration('attach');
            await vscode.debug.startDebugging(vscode.workspace.workspaceFolders?.[0], configuration);
        })
    );
    context.subscriptions.push(vscode.commands.registerCommand('rho.build', () => build()));
    context.subscriptions.push(vscode.commands.registerCommand('rho.clean', () => clean()));
    context.subscriptions.push(vscode.commands.registerCommand('rho.flash', () => flash()));
    context.subscriptions.push(
        vscode.commands.registerCommand('rho.debug.setup', () => configure())
    );
    context.subscriptions.push(
        vscode.commands.registerCommand('rho.debug.engines', async () => {
            const { out } = await run(['dbg', 'engines']);
            const document = await vscode.workspace.openTextDocument({
                language: 'markdown',
                content: out || 'The Rho toolchain was not found, so there is nothing to list.'
            });
            await vscode.window.showTextDocument(document, { preview: true });
        })
    );
}

/* -------------------------------------------------------------------------- */
/*  The adapter                                                                */
/* -------------------------------------------------------------------------- */

/**
 * How the editor starts the adapter.
 *
 * A command rather than a `program:` in the package manifest, because the adapter is
 * a component of a toolchain that is looked for on four different paths: naming a
 * program in the manifest would pin one of them, and a pinned path is wrong on
 * every machine that put the toolchain somewhere else.
 */
class DescriptorFactory implements vscode.DebugAdapterDescriptorFactory {
    createDebugAdapterDescriptor(session: vscode.DebugSession): vscode.ProviderResult<vscode.DebugAdapterDescriptor> {
        const toolchain = toolchainPath();
        if (!toolchain) {
            // Refused with a sentence, because the alternative is an editor that
            // reports a failed to launch and nothing else.
            void vscode.window.showErrorMessage(
                'Rho: the toolchain was not found, so no debugging session can start.'
            );
            return undefined;
        }
        const root = workspaceRoot() ?? process.cwd();
        const arguments_ = ['dbg', '--dap', '--cwd', root];
        return new vscode.DebugAdapterExecutable(toolchain, arguments_);
    }
}

/* -------------------------------------------------------------------------- */
/*  Configurations                                                             */
/* -------------------------------------------------------------------------- */

/**
 * Answers the editor's questions about a configuration.
 *
 * The one that matters is `resolveDebugConfiguration`: it is what makes F5 work in a
 * project nobody has edited. Given nothing, it fills in what the project declares —
 * the board from the manifest, the program from where the build writes it — and
 * answers with a complete configuration. Failing instead would make a person write
 * a launch.json by hand to find out what the project already says.
 */
class ConfigurationProvider implements vscode.DebugConfigurationProvider {
    /** What the project declares, read from the launch configuration `rho ide` wrote. */
    async project(): Promise<{ target?: string; program?: string }> {
        const root = workspaceRoot();
        if (!root) {
            return {};
        }
        // Read, not generated: writing a launch file because somebody pressed F5
        // would leave changes in their repository that they did not ask for. A
        // project with no launch configuration is offered `rho debug.setup` instead.
        const configuration = await readLaunchConfiguration(root);
        return {
            target: configuration?.target,
            program: resolveProgram(root, configuration?.program)
        };
    }

    /** A configuration for this project, with whatever the project says filled in. */
    async defaultConfiguration(request: string = 'launch'): Promise<RhoDebugConfiguration> {
        const kind: 'launch' | 'attach' = request === 'attach' ? 'attach' : 'launch';
        const root = workspaceRoot();
        const project = await this.project();
        const settings = vscode.workspace.getConfiguration('rho.debug');
        const target = settings.get<string>('target') ?? project.target;

        const configuration: RhoDebugConfiguration = {
            type: DEBUG_TYPE,
            request: kind,
            name: target ? `Debug ${target}` : `Debug ${path.basename(root ?? '.')}`,
            cwd: root ?? '${workspaceFolder}',
            preLaunchTask: BUILD_TASK
        };
        if (project.program) {
            configuration.program = project.program;
        }
        if (target) {
            configuration.target = target;
        }
        const engine = settings.get<string>('engine');
        if (engine) {
            configuration.engine = engine;
        }
        const svd = settings.get<string>('svdFile');
        if (svd) {
            configuration.svdFile = svd;
        }
        return configuration;
    }

    /** Fills in what a configuration left out, before the session starts. */
    async resolveDebugConfiguration(
        folder: vscode.WorkspaceFolder | undefined,
        configuration: RhoDebugConfiguration
    ): Promise<RhoDebugConfiguration | undefined> {
        const complete = await this.defaultConfiguration(configuration?.request ?? 'launch');

        // What the editor sent wins, field by field. A person who wrote a launch
        // configuration wrote it on purpose, and merging in the project's answers
        // would overwrite a value they chose to get another one they did not look
        // at.
        const merged: RhoDebugConfiguration = { ...complete, ...stripEmpty(configuration) };

        if (merged.build === false) {
            delete merged.preLaunchTask;
        } else if (merged.preLaunchTask === undefined) {
            merged.preLaunchTask = BUILD_TASK;
        }

        if (!merged.program) {
            // Said rather than started: a session with no program to load fails at
            // the debugger with a message about a file, which is a longer way of
            // saying this.
            const choice = await vscode.window.showErrorMessage(
                'Rho: this project has no built program to debug. Build it first, or set "program" in the launch configuration.',
                'Build now'
            );
            if (choice === 'Build now') {
                await build();
            }
            return undefined;
        }

        if (merged.request === 'launch' && !merged.target) {
            // A program on this machine needs no board. Said, because a person who
            // meant a board and got a host session would be debugging the wrong
            // thing without being told.
            vscode.window.showInformationMessage(
                'Rho: debugging the built program on this machine. Set "target" to debug a board.'
            );
        }
        return merged;
    }

    /** The configurations this project offers, for the dropdown. */
    async provideDebugConfigurations(
        folder: vscode.WorkspaceFolder | undefined,
        token?: vscode.CancellationToken
    ): Promise<RhoDebugConfiguration[]> {
        const launch = await this.defaultConfiguration('launch');
        const attach = await this.defaultConfiguration('attach');
        const target = launch.target;
        // What the machine can debug a board with, so the dropdown says which
        // engine a session will actually use rather than only the built-in one.
        const engines = await enginesHere();
        const offered: RhoDebugConfiguration[] = [launch, attach];
        for (const engine of engines) {
            if (engine.name === 'gdb') {
                continue;
            }
            offered.push({ ...launch, name: `${launch.name} (${engine.name})`, engine: engine.name });
        }
        if (target) {
            offered.push({ ...launch, name: `${target} (check)`, preLaunchTask: undefined, request: 'attach' });
        }
        return offered;
    }
}

/** stripEmpty is a configuration without the keys nobody set. */
function stripEmpty(configuration: RhoDebugConfiguration): RhoDebugConfiguration {
    const out: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(configuration ?? {})) {
        if (value === undefined || value === null || value === '') {
            continue;
        }
        out[key] = value;
    }
    return out as RhoDebugConfiguration;
}

/** readLaunchConfiguration is the target a project's launch configuration names. */
async function readLaunchConfiguration(root: string): Promise<{ target?: string; program?: string } | undefined> {
    const file = path.join(root, '.vscode', 'launch.json');
    try {
        const text = await vscode.workspace.fs.readFile(vscode.Uri.file(file));
        // A launch.json is JSON with comments in it, and a person writes comments
        // in it. Stripped rather than refused: a launch file this extension cannot
        // read is a launch file nobody can be helped with by this one.
        const parsed = JSON.parse(stripJsonComments(Buffer.from(text).toString('utf8')));
        const configurations: Record<string, unknown>[] = parsed?.configurations ?? [];
        const ours = configurations.find(
            entry => entry?.type === DEBUG_TYPE && (entry?.request === 'launch' || entry?.request === undefined)
        );
        const chosen = ours ?? configurations.find(entry => entry?.type === DEBUG_TYPE);
        if (!chosen) {
            return undefined;
        }
        return { target: chosen.target as string | undefined, program: chosen.program as string | undefined };
    } catch {
        return undefined;
    }
}

/** stripJsonComments removes `//` and comments, keeping what is inside strings. */
export function stripJsonComments(text: string): string {
    let out = '';
    let inString = false;
    let inLine = false;
    for (let index = 0; index < text.length; index++) {
        const c = text[index];
        const next = text[index + 1];
        if (inLine) {
            if (c === '\n') {
                inLine = false;
                out += c;
            }
            continue;
        }
        if (!inString && c === '/' && next === '/') {
            inLine = true;
            index++;
            continue;
        }
        if (!inString && c === '/' && next === '*') {
            // A block comment runs to its end, which is the other comment a person
            // writes in a configuration.
            const end = text.indexOf('*/', index + 2);
            index = end < 0 ? text.length : end + 1;
            continue;
        }
        if (c === '"' && text[index - 1] !== '\\') {
            inString = !inString;
        }
        out += c;
    }
    // Trailing commas are legal in JSONC and are what a person leaves behind when
    // they delete a line from a list.
    return out.replace(/,(\s*[}\]])/g, '$1');
}

/** resolveProgram is a launch configuration's program, with its variables expanded. */
function resolveProgram(root: string, program: string | undefined): string | undefined {
    if (!program) {
        return undefined;
    }
    return program
        .replace('${workspaceFolder}', root)
        .replace(/\$\{env:(\w+)\}/g, (_all, name: string) => process.env[name] ?? '')
        .replace(/^~(?=$|\/)/, require('os').homedir());
}

/* -------------------------------------------------------------------------- */
/*  Building, cleaning, flashing                                               */
/* -------------------------------------------------------------------------- */

/**
 * The build a session depends on, run as a task.
 *
 * As a task rather than in a terminal because a task's problems are navigable: a
 * compiler error in the Problems panel takes you to the line, and the same error in
 * a terminal is a sentence in a scrollback. The `$gcc` matcher is what makes that
 * work, and it is the same matcher `rho ide` writes into the project's tasks.
 */
async function build(): Promise<boolean> {
    const root = workspaceRoot();
    const task = new vscode.Task(
        { type: 'rho', task: 'build' },
        vscode.TaskScope.Workspace,
        BUILD_LABEL,
        'rho',
        new vscode.ShellExecution(toolchainPath() ?? 'rho', ['build', '.'], {
            cwd: root
        }),
        ['$gcc']
    );
    task.presentationOptions = {
        reveal: vscode.TaskRevealKind.Always,
        panel: vscode.TaskPanelKind.Dedicated,
        clear: true
    };
    const execution = await vscode.tasks.executeTask(task);
    return new Promise<boolean>(resolve => {
        // Resolved when the task ends rather than when it is handed over: a build
        // that has not finished is not a build that worked, and a session started
        // on a half-written program fails at the debugger with a message about a
        // file that is the wrong place to hear it.
        const ended = vscode.tasks.onDidEndTaskProcess(event => {
            if (event.execution.task === task) {
                ended.dispose();
                resolve(event.exitCode === 0);
            }
        });
    });
}

/** Clean removes everything a build generated, through the toolchain's own command. */
async function clean(): Promise<void> {
    const root = workspaceRoot();
    await run(['build', '.', '-clean'], root);
    vscode.window.showInformationMessage('Rho: removed the build directory.');
}

/**
 * Flash puts the built program on the board.
 *
 * Not offered as a guess: the board's flash command comes from its target spec, so
 * this asks for the board by name when the project does not say which one. Writing
 * to the wrong board is not a mistake worth a one-click button.
 */
async function flash(): Promise<void> {
    const root = workspaceRoot();
    const project = await new ConfigurationProvider().project();
    const target =
        vscode.workspace.getConfiguration('rho.debug').get<string>('target') ?? project.target;
    if (!target) {
        const chosen = await vscode.window.showQuickPick(
            (await targetsHere()),
            { title: 'Rho: flash which board?' }
        );
        if (!chosen) {
            return;
        }
        await flashTo(chosen, root, project.program);
        return;
    }
    await flashTo(target, root, project.program);
}

async function flashTo(target: string, root: string | undefined, program: string | undefined): Promise<void> {
    if (!root || !program) {
        vscode.window.showErrorMessage('Rho: build the project before flashing it.');
        return;
    }
    const { ok, out } = await run(['flash', '-target', target, program], root, 120_000);
    if (!ok) {
        vscode.window.showErrorMessage(`Rho: flashing ${target} failed. ${out}`);
        return;
    }
    vscode.window.showInformationMessage(`Rho: wrote the program to ${target}.`);
}

/** configure writes what an editor needs to read, from what the project declares. */
async function configure(): Promise<void> {
    const root = workspaceRoot();
    if (!root) {
        vscode.window.showErrorMessage('Rho: open a project folder first.');
        return;
    }
    const { ok, out } = await run(['ide', 'vscode', root], root);
    if (!ok) {
        vscode.window.showErrorMessage(`Rho: ${out}`);
        return;
    }
    vscode.window.showInformationMessage(
        'Rho: wrote .vscode/launch.json, .vscode/tasks.json and a workspace file. Reload the window to use them.'
    );
}

/* -------------------------------------------------------------------------- */
/*  What this machine has                                                      */
/* -------------------------------------------------------------------------- */

/** enginesHere are the debuggers this machine has, as `rho dbg engines` says. */
export async function enginesHere(): Promise<{ name: string; kind: string; comment?: string }[]> {
    const answer = await runJson<EnginesAnswer>(['dbg', 'engines'], workspaceRoot());
    return answer?.engines ?? [];
}

/** targetsHere are the boards this toolchain knows, for a list to choose from. */
export async function targetsHere(): Promise<string[]> {
    const { ok, out } = await run(['target', 'list'], workspaceRoot());
    if (!ok) {
        return [];
    }
    const names: string[] = [];
    for (const line of out.split('\n')) {
        const match = /^\s{2,}([a-z0-9][a-z0-9._-]*)\b/.exec(line);
        if (match) {
            names.push(match[1]);
        }
    }
    return names;
}