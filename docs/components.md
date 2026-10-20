# Components and plugins

A **component** is a module that publishes a command: `rho fmt`, `rho test`,
`rho doc`. The CLI does not know these commands — they arrive with the
component, which is why adding a tool does not mean rebuilding the toolchain.

A **plugin** is a module that extends a tool rather than adding a command. A
doc plugin adds a page to the site a doc build produces; a test plugin runs
suites the runner does not implement itself.

Both are published, installed and enabled the same way, and each documents
itself: every page below is generated from that module's own `docs/`, so this
index is a map rather than a second copy.

## Build and target

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-target <rho-target/index>` | `rho target` | Board and architecture specifications, and the CMake that builds for them |
| {ref}`rho-cimport <rho-cimport/index>` | `rho cimport` | Generates a Rho foreign interface from C headers |
| {ref}`rho-kconfig <rho-kconfig/index>` | `rho kconfig` | Configuration bridge between a project and the Kconfig tooling around it |

## Run, debug, flash

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-flash <rho-flash/index>` | `rho flash` | Puts a built binary on a board |
| {ref}`rho-deploy <rho-deploy/index>` | `rho deploy` | Puts a built program on a host, and starts it |
| {ref}`rho-dbg <rho-dbg/index>` | `rho dbg` | A debug adapter: DAP on one side, GDB on the other, a probe in between |
| {ref}`rho-monitor <rho-monitor/index>` | `rho monitor` | A target's console over UART, RTT or an ITM trace, alongside a debug session |
| {ref}`rho-test <rho-test/index>` | `rho test` | Discovers and runs a project's tests |

## Measure

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-mem <rho-mem/index>` | `rho mem` | Where a program's memory went, and whether it fits its budgets |
| {ref}`rho-bench <rho-bench/index>` | `rho bench` | How long code takes, what stack and heap it uses, and how that compares with a baseline |
| {ref}`rho-lint <rho-lint/index>` | `rho lint` | Every static analyzer over one compilation database, as SARIF |

## Toolchain and documentation

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-fmt <rho-fmt/index>` | `rho fmt` | The source formatter |
| {ref}`rho-doc <rho-doc/index>` | `rho doc` | The documentation generator that built this site |
| {ref}`rho-log <rho-log/index>` | `rho log` | Structured logging |
| {ref}`rho-lsp <rho-lsp/index>` | `rho lsp` | The language server |
| {ref}`rho-doctor <rho-doctor/index>` | `rho doctor` | What the installed modules need from this machine, and what it has |
| {ref}`rho-keeper <rho-keeper/index>` | `rho keeper` | A dashboard for the installation and its registry, and what it fixes |

## Services

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-router <rho-router/index>` | `rho route` | HTTP routing, parameter binding and API metadata |
| {ref}`rho-sql <rho-sql/index>` | `rho sql` | SQL a service declares, and the engine that runs it |
| {ref}`rho-openapi <rho-openapi/index>` | `rho openapi` | OpenAPI 3.1 descriptions and API references |
| {ref}`rho-auth <rho-auth/index>` | `rho auth` | Registry credentials |

## Plugins

A plugin extends a tool. It is enabled with `rho plugin enable <name>`, and a
tool that has one runs it wherever that tool offers the hook.

| Plugin | Extends | What it adds |
|---|---|---|
| {ref}`rho-doc-api <rho-doc-api/index>` | `rho-doc` | A service's HTTP API reference to its documentation site |
| {ref}`rho-doc-toc <rho-doc-toc/index>` | `rho-doc` | A table of contents and a generation notice |
| {ref}`rho-sql-sqlite <rho-sql-sqlite/index>` | `rho-sql` | Runs a service's SQL against a sqlite database |
| {ref}`rho-test-gtest <rho-test-gtest/index>` | `rho-test` | Runs a project's GoogleTest suites |
| {ref}`rho-test-junit <rho-test-junit/index>` | `rho-test` | Writes a JUnit report, which is what a CI system reads |
| {ref}`rho-robohub <rho-robohub/index>` | four components | Maps the RoboHub platform's target, configuration, C and flashing conventions onto the toolchain's contracts |

`rho-robohub` is the one to read if you are adding a platform: it implements one
contract across `rho-target`, `rho-kconfig`, `rho-cimport` and `rho-flash` at
once, and it runs only for a project whose module graph names the platform.

## Applications

Applications are written in Rho rather than extending the toolchain. They are
built and run like any other project.

| Application | What it is |
|---|---|
| {ref}`registry <registry/index>` | The remote module registry: what `rho module search` and `rho module install` talk to |

## Adding one

{ref}`handbook/modules` describes components and plugins, and what a manifest
declares for each. A component is a module with a `command`, a `main` that is a
binary in its own root, and its own `go.mod`; a plugin is a module with an
`extends` block naming the component it plugs into and the hooks it answers.

`rho-doc-toc` is the smallest plugin in the tree and the shortest thing to read
before writing one.
