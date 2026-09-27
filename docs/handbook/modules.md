# Modules, components and plugins

Rho has three kinds of module. The distinction is what a module *is for*, and it
decides how the toolchain treats it.

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
  hooks:
    - index
    - page
```

That pair is the whole extension contract, and it is checked rather than
assumed:

| Checked when | What |
|--------------|------|
| `rho module validate` | The named component exists and is dispatchable |
| `rho module validate` | Every hook the plugin declares is one the component offers |
| `rho module validate` | Every command answers a hook the plugin declared |
| During a build | Only the plugins that name *this* component are loaded |

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
