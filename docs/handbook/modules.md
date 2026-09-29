# Modules, components and plugins

Rho has three kinds of module. The distinction is what a module *is for*, and it
decides how the toolchain treats it.

Where a module *goes* — installed here, or published to a registry, and who may
change it there — is {ref}`handbook/registries`.

## A module is an application

An application has a `main` and generates a program:

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

deps:
  rho-log: ">=0.1.0"
```

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
