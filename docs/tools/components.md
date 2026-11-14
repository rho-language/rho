# Components and plugins

A **component** is a module that publishes a command: `rho fmt`, `rho test`,
`rho doc`. The CLI does not know these commands. They arrive with the component,
which is a separate program with its own `go.mod`, so adding a tool does not mean
rebuilding the toolchain.

A **plugin** is a module that extends a component rather than adding a command.
A documentation plugin adds pages to a site; a test plugin runs suites the
runner does not implement itself; a SQL engine plugin runs a service's
statements.

Both are published, installed and enabled the same way — `rho module publish`,
`rho module install`, `rho module enable` — and both document themselves: every
page linked below is generated from that module's own `docs/`, so this index is
a map rather than a second copy. A first install has only `rho-lsp`;
{ref}`documentation/installing` says how to get the rest.

## Building for a target

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-target <rho-target/index>` | `rho target` | Board and architecture specifications, and the CMake and linker script a C build reads from them |
| {ref}`rho-cimport <rho-cimport/index>` | `rho cimport` | Generates a Rho foreign interface from C headers, and the C that Rho cannot express |
| {ref}`rho-kconfig <rho-kconfig/index>` | `rho kconfig` | Resolves a project's declared configuration into the form its toolchain reads |

## Putting a program somewhere

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-flash <rho-flash/index>` | `rho flash` | Writes a built binary to a board, with the command its target spec declares |
| {ref}`rho-deploy <rho-deploy/index>` | `rho deploy` | Copies a built program or container image to a host and starts it |
| {ref}`rho-artifact <rho-artifact/index>` | `rho artifact` | Packs and verifies signed host and firmware artifacts |
| {ref}`rho-bootstrap <rho-bootstrap/index>` | `rho bootstrap` | Creates a local instance identity from a provisioning contract, and enrolls it |

## Running, debugging, testing

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-test <rho-test/index>` | `rho test` | Runs the tests a project declares, and the suites its plugins resolve |
| {ref}`rho-dbg <rho-dbg/index>` | `rho dbg` | A debug adapter: DAP on one side, GDB on the other, the probe in between |
| {ref}`rho-monitor <rho-monitor/index>` | `rho monitor` | A target's console over UART, RTT or an ITM/SWO trace, alongside a debug session |

## Measuring and checking

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-mem <rho-mem/index>` | `rho mem` | Where a linked program's memory went, checked against the budgets a project declares |
| {ref}`rho-bench <rho-bench/index>` | `rho bench` | How long code takes, what stack and heap it uses, compared with a baseline |
| {ref}`rho-lint <rho-lint/index>` | `rho lint` | Every static analyzer over one compilation database, written as SARIF |

## Writing code

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-fmt <rho-fmt/index>` | `rho fmt` | The source formatter |
| {ref}`rho-lsp <rho-lsp/index>` | `rho lsp` | The language server; built and enabled by `rho install` |
| {ref}`rho-doc <rho-doc/index>` | `rho doc` | The documentation generator that built this site |
| {ref}`rho-log <rho-log/index>` | `rho log` | Structured logging: levels, text or JSON, rotating files, runtime configuration |

## The machine and the installation

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-doctor <rho-doctor/index>` | `rho doctor` | Reports what the installed modules need from this machine, and what it has |
| {ref}`rho-keeper <rho-keeper/index>` | `rho keeper` | A local dashboard for the installation and its registry, and the commands that fix what the doctor found |
| {ref}`rho-auth <rho-auth/index>` | `rho auth` | Registry tokens, signing keys, login tickets, and the user and client stores a service checks |

## Services

| Module | Command | What it does |
|---|---|---|
| {ref}`rho-router <rho-router/index>` | `rho route` | HTTP routing, parameter binding and API metadata |
| {ref}`rho-sql <rho-sql/index>` | `rho sql` | The SQL a service declares, and the engine that runs it |
| {ref}`rho-openapi <rho-openapi/index>` | `rho openapi` | OpenAPI 3.1 descriptions and API references |

## Plugins

A plugin names the component it extends in its manifest (`extends:`), with the
hooks it answers; the component runs it wherever it offers that hook. It is
turned on with `rho plugin enable <name>`.

| Plugin | Extends | What it adds |
|---|---|---|
| {ref}`rho-doc-api <rho-doc-api/index>` | `rho-doc` | A service's HTTP API reference, in its documentation site |
| {ref}`rho-doc-toc <rho-doc-toc/index>` | `rho-doc` | A table of contents and a generation notice |
| {ref}`rho-sql-sqlite <rho-sql-sqlite/index>` | `rho-sql` | A sqlite engine for a service's SQL |
| {ref}`rho-test-gtest <rho-test-gtest/index>` | `rho-test` | Runs a project's GoogleTest suites |
| {ref}`rho-test-ctest <rho-test-ctest/index>` | `rho-test` | Runs a project's CTest suites: configure, build, run, read the JUnit report |
| {ref}`rho-test-junit <rho-test-junit/index>` | `rho-test` | Writes a JUnit report of a run, which is what a CI system reads |
| {ref}`rho-robohub <rho-robohub/index>` | `rho-target`, `rho-kconfig`, `rho-cimport`, `rho-flash`, `rho-module` | An example integration plugin: one ecosystem's platform conventions, bindings and ports |

`rho-robohub` is an **integration plugin**: the single place one ecosystem meets
the toolchain. It fills the `platform` contract of four components and the
`module-manifest` contract of the module tools, declares the bindings and ports
it provides in an `integration:` section, and runs only for a project whose
module graph names that platform. It is the one to read when integrating another
ecosystem; {ref}`modules/integrations` is the contract.

`rho-doc-toc` is the smallest plugin in the tree, and the shortest thing to read
before writing one.

## Applications

Applications are written in Rho rather than extending the toolchain. They are
built and run like any other project, and both compile to a Go HTTP service.

| Application | What it is |
|---|---|
| {ref}`registry <registry/index>` | The remote module registry: what `rho module publish`, `search` and `install` talk to |
| {ref}`docsite <docsite/index>` | The documentation site: these pages, a menu and the release notes, served over HTTP |

## Adding one

{ref}`modules/plugins` describes components and plugins, and what a manifest
declares for each. A component is a module with a `command`, a `main` that names a
binary in its own root, and its own `go.mod`; a plugin is a module with an
`extends` block naming the component it plugs into and the hooks it answers.
