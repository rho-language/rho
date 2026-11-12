# Modules: components and plugins

A **module** is either a **component** or a **plugin**. Its manifest says which,
and that decides how the toolchain treats it: a component publishes a command a
person types, and a plugin extends a component by answering its hooks. Both are
installed with `rho module install`, turned on with `rho module enable`, and
published to a registry.

Where a module *goes* — installed here, or published to a registry, and who may
change it there — is {ref}`handbook/registries`. An application is a different
thing, and is described in its own section at the end.

## A component is a tool

A component publishes a command and is dispatched to at run time. The CLI does
not know the command: `rho fmt` works because the formatter is installed and
because its manifest says `command: fmt`.

```yaml
module:
  name: rho-log
  type: component
  version: 0.1.0
  description: Structured logging
  command: log
  import_path: rho.log

main: rho-log

deps:
  rho-fmt: ">=0.1.0"
```

A component may also export a Rho library, which is what makes it usable as a
dependency. `import_path` is the dotted name an `import` uses, and the library
sources travel with the component. That is why `import rho.log` resolves in a
project that depends on `rho-log` without vendoring anything: the component is
already installed, and the compiler finds the library where the component
published it.

A component that declares no `import_path` is a command and nothing more, like
the test runner.

## A plugin is an extension

A plugin adds to a tool. It does not publish a command a person types; it
answers hooks a tool runs, which is why it is invoked by the tool rather than by
the CLI.

A plugin is not free-floating. It names the component it plugs into, and the
hooks of that component it answers:

```yaml
module:
  name: rho-doc-toc
  type: plugin
  version: 0.1.0
  description: Adds a table of contents to a documentation site

main: rho-doc-toc

# The component this plugin plugs into, and the hooks of it this plugin answers.
extends: rho-doc

# What this plugin *is*, as a contract of that component. A plugin says this
# rather than leaving a reader to infer it from the hooks: two plugins may
# answer the same hook and be unrelated things.
implements:
  - contents

hooks:
  - index
  - page

# The executables that answer those hooks.
commands:
  - name: table-of-contents
    run: rho-doc-toc
    hook: index
  - name: generation-notice
    run: rho-doc-toc
    hook: page
```

The component says what it offers, which is what the declaration above is
measured against:

```yaml
module:
  name: rho-doc
  type: component
  version: 0.1.0
  command: doc

main: rho-doc

provides:
  # What a plugin is for, and how many may be it.
  contracts:
    - name: contents
      description: Adds to a finished set of pages: a table of contents, an API reference, an index.
      one: false
      hooks: [index, page]
  hooks:
    - index
    - page
```

## Contracts

A hook says how a plugin is *called*. It does not say what the plugin is *for*,
and the two are different questions: `rho-doc` offers `index`, and both a table
of contents and an API reference answer it while being nothing alike. Reading
"which hook" off a plugin tells you neither what it does nor whether you want it.

A **contract** is the other half. A component declares the contracts it needs a
plugin to be, and each one says how many plugins may fill it:

| `one:` | What it means | Real case |
|---------|---------------|-----------|
| `true` | exactly one enabled plugin fills it | `rho-sql`'s `engine` — a service runs its statements against one database |
| `false` | every enabled plugin fills it | `rho-doc`'s `contents` — a set of pages is finished by what each adds |

A plugin names what it fills in `implements`. That is what lets a person decide
whether to turn it on without reading its source, and it is checkable: a contract
the component does not offer is a plugin written against a component that has
since changed, and `rho module validate` says so rather than letting it be
discovered from a pipeline that quietly did less than the plugin was written to
do.

`rho-test` declares two contracts rather than one, because discovering a
project's tests and reporting a finished run are unrelated jobs — and a plugin
that only reports is not a plugin that discovers:

```yaml
provides:
  contracts:
    - name: discover
      one: false
      hooks: [discover]
    - name: report
      one: false
      hooks: [run]
```

A plugin that names no contract is offered to every run of its component. That is
deliberate: a component whose plugins are all of one kind has no reason to name
them, and a rule with no failure behind it is only an obstacle.

## Module identity

`module.name` is what a person reads. `module.id` is what a registry, a lock file
and a saved plan refer to the module by, so it is declared by the module's author
and kept: two or more lowercase dotted segments, a namespace and a name.

```yaml
module:
  name: acme-driver-ds18b20
  id: acme.driver.ds18b20
  previous_ids:
    - acme-drivers.drivers.temperature.ds18b20
```

It is not the repository and folder the module lives in. Those are renamed for
reasons that have nothing to do with what the module is, and an id built from them
changes with them. A rename of the module itself is recorded in `previous_ids`, so
what refers to an old id still resolves. A previous id is only a clean token: it
was valid under the rule of its day.

`rho module ids [dir...]` reads every `module.yml` under the directories, lists the
ids, and fails on a malformed id, two modules with one id, or an id that is also
another module's previous id.

## Paths and the workspace

A manifest that has to reach a sibling checkout does not name a place on the
machine it was written on. It says where the sibling is relative to the
*workspace*, the directory the checkouts sit in:

```yaml
depends:
  - name: acme-common
    type: c
    path: ${ACME_WORKSPACE}/acme-common
    git: https://example.org/acme/acme-common
c_connection:
  include_dirs:
    - ${ACME_WORKSPACE}/acme-common/include
```

`${NAME}` and `${NAME:-default}` are expanded in the fields that are paths:
`depends[].path`, `build.include_dirs`, `build.source_dirs`,
`c_connection.path`, `c_connection.include_dirs` (and its variants') and
`exports.cmake.include_dirs`. A variable that is not set and has no default stops
the build and names it.

The workspace is `$RHO_WORKSPACE` when it is set. Otherwise it is the directory
that holds the repository the project is in. Any `<ECOSYSTEM>_WORKSPACE` that is
not set means the same directory, so an ecosystem writes its manifests in its own
terms without the toolchain knowing its name; set it to move that ecosystem's
checkouts elsewhere.

`rho env` shows what was resolved and where from:

```sh
$ rho env
RHO_HOME           /root/.rho  (default)
RHO_WORKSPACE      /work  (the parent of the repository at /work/app)
```

A path dependency that is not there is fetched into that path only when it is
inside the project or the workspace; any other absolute path is not written to.
`scripts/check-local-paths.sh` fails when a tracked file names a path under
`/home/<user>` or `/Users/<user>`, and is what a CI job runs.

## What a module gives, and what it keeps

A module that other code is meant to use says so in its manifest: the name it
is imported by, and the names that import may use.

```yaml
module:
  name: acme-forms
  type: module
  version: 0.2.0

main: src/forms.rho

provides:
  import: acme.forms
  exports:
    - open
    - parse
    - validate
```

An application, or another module, declares it and imports it:

```yaml
deps:
  acme:
    acme-forms: "^0.2"
```

```rho
import acme.forms

fn page() string {
    return forms.open("/save", "post")
}
```

The toolchain holds both sides to it:

- An import resolves through the dependencies the importing module's own
  manifest declares. A module that only something undeclared provides is
  refused, and the message names the dependency to add.
- Only exported names can be used from outside the module; anything else is
  the module's own. `forms.trim()` from an application is refused with the
  names the module does export, and a private helper never collides with an
  application function of the same name. The list covers types and constants
  too: an application that writes `forms.Field` needs `Field` under `exports`.
  A value of a type the module keeps can still be passed around, as what an
  exported function returns; it just cannot be named or built outside.
- Including another module by name, which pastes its whole source into the
  includer, is reported with the import that replaces it.

A module with no `exports` list lets every name through, which is how modules
written before export lists keep working.

## Interfaces, providers and bindings

An interface says what a family of providers can do, and an application chooses
which providers it uses. An interface is a module with an `interface` section:

```yaml
module:
  name: acme-alert
  type: module
  version: 1.0.0
main: src/alert.rho
provides:
  import: acme.alert
  exports: [raise]
interface:
  functions:
    send: "(level: string, message: string) u32"
```

A provider says it implements it, exports each function with exactly that
signature, and declares the settings it reads:

```yaml
module:
  name: acme-telegram
  type: provider
  version: 0.1.0
main: src/provider.rho
implements: [acme.alert]
provides:
  import: acme.alert_telegram
  exports: [send]
  settings: [bot_token, chat_id]
```

The application declares the interface and its providers as dependencies and
binds them:

```yaml
bind:
  acme.alert:
    providers: [acme-log, acme-telegram]
    config:
      acme-telegram:
        chat_id: "-1001"
        enabled: "false"
```

Only the bound providers are compiled in. The toolchain generates, inside the
interface, `send_to(provider, ...)` for each declared function,
`bound_providers()` and `bind_config(provider, key)`, so the interface's own code
decides how its providers are used: every one of them, as an alert does, or the
first, as a gateway would. A provider reads its settings with
`rho_bind_config(provider, key)`: the value in the application's `bind.conf`
(`acme-telegram.chat_id=...`) when there is one, so a running installation changes
a setting without a rebuild, otherwise the value in `bind:`. The file is in the
workspace a supervisor gives the application (`RHO_APP_WORKSPACE`), or in
`~/.rho/apps/<name>/` for a program started by hand, and only its owner can read
it, because a setting may be a secret. The
interface can also write one, through the generated
`bind_config_set(provider, key, value)`, which is how a settings page changes a
provider's configuration on a running installation.

A bound provider that is not a declared dependency, does not list the interface
under `implements`, is configured with a key it does not declare, or exports a
function with another signature is refused when the application is built. Every
provider accepts `enabled`, without declaring it, so an interface can let an
installation switch a provider off without removing it from the binding.

## A module's own files

A module that reads a template, a schema or a page of its own does not look for
its folder at run time: wherever a program runs, that folder is not beside it.
It declares the files instead, and every program that includes the module
carries them, embedded when it is built:

```yaml
assets:
  - templates            # a directory: every file under it
  - db/schema.sql
locales:
  - locale: en
    file: locales/en.json
```

```rho
extern fn rho_asset(module: string, path: string) string
extern fn rho_asset_dir(module: string) string

fn card() string {
    return rho_asset("forms", "templates/card.html")
}
```

`rho_asset` is one file's content, or `""` when the module declares none by that
path. `rho_asset_dir` is a directory holding all of them as files, for a reader
that takes a path — a YAML document read where it lies, a directory of steps;
it is written once per content under the user's cache. A module's locales are
merged into the program's catalogs before the program's own, so a program can
override any string. Both are the Go backends'; generated C carries no assets.

## What is installed and what is used

Installing a component or a plugin makes it available. Using it is a separate
decision, and a program that ran a module nobody chose would be running code its
author never asked for — on a machine whose state nobody wrote down.

So a module is **not** used until somebody enables it:

```sh
rho module list                  # what is installed, and which of it runs
rho module enable rho-sql         # use the newest installed version
rho module enable rho-log@0.1.0   # use this one, whichever else is installed
rho module disable rho-log        # installed, and not used
```

A build that needs a module which is installed and not enabled stops and says
which command fixes it:

```
rho build: the component rho-sql is installed but not enabled, and this is
needed to run; enable it with: rho module enable rho-sql@0.1.0
```

A module that was never installed gets a different message, because the fix is
different — there is nothing to turn on.

`rho plugin` is the same thing said about plugins, and `list` answers the
question contracts exist for:

```sh
rho plugin list
```

```
NAME            VERSION  STATE      EXTENDS   PROVIDES
rho-doc-api     0.1.0    installed  rho-doc   contents
rho-doc-toc     0.1.0    active     rho-doc   contents
rho-sql-sqlite  0.1.0    active     rho-sql   engine
rho-test-junit  0.1.0    installed  rho-test  report
```

Turning a plugin on says what it will now be asked for, so the decision is
made with the consequence in front of you:

```
$ rho plugin enable rho-sql-sqlite
enabled plugin rho-sql-sqlite at 0.1.0
  rho-sql.engine: Runs a service's SQL against a database. — exactly one plugin
  fills this (fills this)
```

## rho keeper

One component reads the installation and shows it, so that the answers to "what
have I got, and is it being used?" are somewhere to look rather than something to
reconstruct from a directory listing:

```sh
rho module enable rho-keeper
rho keeper
```

```
rho keeper: http://127.0.0.1:8477/
rho keeper: press ctrl-c to stop
```

It shows every module with its version, whether it is the one a run would use,
and what its own manifest says about it. It shows every version installed rather
than only the one in use, because showing one would imply there was only one. It
shows the contracts the installed components offer and which plugins fill them,
which is the question contracts exist for. And it shows what it could not read,
each with its reason — a list that quietly leaves things out is a list nobody
can trust to be complete.

It also acts on what it reads, and every action is the command a person would
type, run the same way and printed beside the thing it changes:

- enable or disable a module — the same `rho module enable` / `disable`;
- set a value a module declares in its manifest;
- install a module from the configured registry, or uninstall this machine's
  copy of one that has been turned off;
- install a system requirement `rho doctor` reports missing.

A change made through the page is therefore one that can be repeated, or undone,
in a terminal. It is not a daemon: it runs while you run it, and stopping it is
stopping the terminal. `rho keeper status` is the same reading without the page,
so a person in a terminal and a person with a browser are told the same thing;
that is why the reading is a library rather than something either reimplements.

## Several versions, one of them used

A machine may hold several versions of a module at once — a dependency brought in
at one version, an upgrade leaving the old one behind. Exactly one of them is the
one a build links, and it is not necessarily the largest number:

```sh
rho module enable rho-log@0.1.0   # pin to a version known to work
```

A build that picked the newest installed would depend on what was installed last
rather than on what anybody decided to use, and the two are not the same thing.
The decision is recorded in `~/.rho/modules/enabled.yml`, so it outlives the
command that made it and is readable by a person deciding what a machine is
doing.

Two components take plugins today: `rho-doc`, whose hooks are `index` over a
finished set of pages and `page` over one page, and `rho-test`, whose are
`discover` over the test files a run found and `run` over its results.
`rho-doc-toc` and `rho-test-junit` are the worked examples, one for each.

That pair is the whole extension contract, and it is checked rather than
assumed:

| Checked when | What |
|--------------|------|
| `rho module validate` | The named component exists and is dispatchable |
| `rho module validate` | Every hook the plugin declares is one the component offers |
| `rho module validate` | Every command answers a hook the plugin declared |
| `rho module validate` | Every contract the plugin claims is one the component offers |
| `rho module validate` | Every hook it answers is inside the contract it claimed |
| During a build | Only the enabled plugins that name *this* component are loaded |

The last one is the point of the relation: a tool loads the plugins attached to
it, so a plugin written for another tool is inert rather than being handed a
request it cannot answer. `rho doc -list-plugins` names those plugins and says
which component they belong to, so a plugin that never runs can be accounted
for:

```
1 plugin(s) extend the documentation generator:
  rho-doc-toc 0.1.0 hooks: index, page
  otherfmt 0.1.0 extends rho-fmt, so this generator does not load it
```

A component that offers no hooks offers no place for a plugin to stand, and a
plugin that names one is refused rather than installed.

## Applications

An **application** is the other thing a manifest can describe. It is a Rho
program: it has a `main`, it generates a program with `rho build`, and it is what
you build and ship.

It is not a component: the toolchain does not dispatch to it, and it publishes
no command. It can be published, though, which is how a machine that runs
applications gets one without its source tree. `rho module publish` builds it
with `rho build` and packs the program it made beside the sources, and the
registry installs it as a module. `applications/registry` and the projects under
`examples/` are applications.

A program that runs applications — a supervisor, a dashboard — finds the
published ones by asking the toolchain rather than reading the registry's
directories:

```sh
$ rho module list --format tsv
module	energy	0.1.0	~/.rho/modules/installed/module/energy@0.1.0	selected
```

One installed version per line: kind, name, version, where it is, and whether it
is the version the index selects.

```yaml
module:
  name: registry
  type: rho
  version: 0.1.0
  description: Remote module registry for Rho
main: src/main.rho

build:
  language: go
  targets:
    - go-server
```

## Capabilities and deployment

Beyond identity and dependencies, a module can describe how a deployment uses
it. The fields are optional and a module that only builds need not carry them.

A board says what roles it can take and what it has:

```yaml
capabilities:
  gateway: true
  relay: true
  node: true
  edge: true
  host: false
  wireless: true
  ethernet: false
  links: "uart console usb wifi socket"
  buses: "uart i2c spi onewire gpio"
  deploy: "flash"
```

`links` is the transports a deployment may put on that board; `buses` is what
the board's HAL carries. A host board is one that can run the gateway process.

A driver says what it is and what a deployment builds from it:

```yaml
capabilities:
  sensor: true
  buses: "onewire"
  measures: "temperature"

deployment:
  boards: "pico2w"
  example: "examples/temperature/ds18b20/c/telemetry"
  binary: "build/output/telemetry.elf"
  kconfig: "CONFIG_RH_HAL_ONEWIRE=y ..."
```

A board's `deployment` block says how a gateway is reached:

```yaml
deployment:
  method: "adb"          # adb, ssh, flash or process
  arch: "arm64"
  kconfig: "CONFIG_RH_BOARD_ARDUINO_UNO_Q=y"
  apps_dir: "/home/arduino/apps"
  user: "arduino"
  path: "/home/arduino/apps"
```

Both blocks are validated by the module schema. A deployment planner reads them
at runtime; nothing in the planner needs to know the board or driver by name.

## The `application` block

A module that is a web application rather than a library or a tool says so,
because an application has a listener the registry that runs it has to know
about:

```yaml
application:
  kind: keeper-hosted
  title: Acme Mesh
  description: Plans and runs Acme gateway deployments.
  route: /mesh
  port: "8478"
  command: mesh
```

`kind: keeper` is the dashboard itself; `kind: keeper-hosted` is an application
it can start and stop on its own port. A module without the block is not an
application — a documentation site or a component is built and run, but has no
listener of its own. The block is optional and is validated by the module
schema, so a consumer can list the applications on a machine without knowing
any of them by name.

## The `package` block

A module may say what a published package carries, so a consumer receives the
component rather than the whole working tree:

```yaml
package:
  kind: component
  binary: true
  import_path: rho.doc
  interface:
    - src/doc.rho
  include:
    - module.yml
    - README.md
    - src
  require:
    - module.yml
    - README.md
    - src
```

- `interface` is the part a dependency actually needs: the Rho library, and
  nothing else.
- `include` is what the package carries.
- `require` is what a consumer must have for it to work.

## Release history

A module's description says what it is and does not change between releases, so
two versions of one module would read alike. Each module carries a `CHANGELOG.md`
beside its manifest, one section per release:

```markdown
## [0.1.1] - 2026-10-12

Added a remote registry section.
```

It travels inside the package, and `rho module publish` reads the section for the
version being published and records it on the registry against that release. A
version's page, and a version list, then show what changed rather than two
identical descriptions; a version with no section is published without a note
rather than refused. The note is text a publisher wrote, so it is escaped
wherever it is shown and never treated as markup.

## Versioning

A component is versioned on its own, separately from the language. Its manifest
says which other modules it needs:

```yaml
deps:
  rho-log: ">=0.1.0"
  rho-fmt: ">=0.1.0"
```

The constraint is a range, as it is for a library: a project that needs the
logger and a project that needs a newer logger can both be satisfied by one
toolchain. The `>=` prefix and the `^` prefix mean the same thing; the
`>=`-style is what the ecosystem uses.

Dependencies may be grouped under a name, which is only for the reader: a group
is resolved exactly as the entries in it would be at the top level, and a
module in the same repository answers for one of its names before a registry is
asked.

```yaml
deps:
  rho-log: ">=0.1.0"
  acme:
    acme-forms: ">=0.1.0"
    acme-provider-mail: ">=0.1.0"
```
