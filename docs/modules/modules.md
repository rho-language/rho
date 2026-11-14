# Modules

A **module** is a directory with a `module.yml` in it. The manifest says what the
module is, what it needs, and what it offers to other code, and everything the
toolchain does with a module — building it, resolving it as a dependency,
publishing it, dispatching a command to it — starts from that file.

This page is the manifest and what the toolchain holds a module to. Three pages
go further: {ref}`modules/plugins` covers components and the plugins that extend
them, {ref}`modules/interfaces` covers interfaces and the providers an application
binds to them, and {ref}`modules/registries` covers where a module is installed
and published.

## Kinds of module

`module.type` says which kind a module is, and that decides how the toolchain
treats it:

| `type` | What it is | Its `main` |
|---|---|---|
| `rho` | An application: a Rho program you build and ship | a Rho source file |
| `module` | A Rho library other code includes or imports | a Rho source file |
| `component` | A tool, reached as `rho <command>` | a binary in the module root |
| `plugin` | An extension of a component, run by it | a binary in the module root |
| `provider` | An implementation of an interface, chosen by an application | a Rho source file |

Those five are the toolchain's own. An ecosystem can add more — a board, a
driver — through a *module contract* that its integration plugin publishes, and
the toolchain validates them without knowing what they mean
(see {ref}`modules/integrations`). A module of such a type is installed as a
library.

## The manifest

A library needs very little:

```yaml
module:
  name: acme-forms
  id: acme.forms
  type: module
  version: 0.2.0
  description: HTML form helpers
  author: Ada Example
  license: Apache-2.0

main: src/forms.rho

provides:
  import: acme.forms
  exports: [open, parse, validate]

deps:
  rho-log: ">=0.1.0"
```

The `module:` block is identity and metadata:

| Field | What it is |
|---|---|
| `name` | What a person reads, and what `deps` and the registry name the module by. Required. |
| `id` | The stable identity a registry and a lock file refer to (below). |
| `previous_ids` | The ids the module was known by before a rename. |
| `type` | One of the kinds above, or an ecosystem's type. Required. |
| `version` | A semantic version, `MAJOR.MINOR.PATCH`. Required. |
| `description` | What the module is. It does not change between releases. |
| `author`, `maintainer`, `license` | The module's own claim about who wrote it and under what terms. |
| `visibility` | `public` (the default) or `private` on a remote registry. |
| `command` | For a component: the word after `rho` that reaches it. |
| `import_path` | For a component that also ships a Rho library: the name an `import` uses. |

The rest of the manifest is sections, each read by the part of the toolchain it
concerns: `deps` and `depends` by the dependency resolver, `provides` by the
compiler and the plugin runner, `build` by `rho build`, `package` by the
packager, `sql` by rho-sql, `config` by rho-kconfig, `system_deps` by
`rho doctor`, `docs` by rho-doc. `rho module validate [dir]` checks a manifest
against the schema; `rho module info [dir]` prints what it says.

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
what refers to an old id still resolves.

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
    path: ${RHO_WORKSPACE}/acme-common
    git: https://example.org/acme/acme-common
c_connection:
  include_dirs:
    - ${RHO_WORKSPACE}/acme-common/include
```

`${NAME}` and `${NAME:-default}` are expanded in the fields that are paths:
`depends[].path`, `build.include_dirs`, `build.source_dirs`,
`c_connection.path`, `c_connection.include_dirs` (and its variants') and
`exports.cmake.include_dirs`. A variable that is not set and has no default stops
the build and names it.

Two variables always have an answer:

- **`RHO_WORKSPACE`** is the environment variable when it is set. Otherwise it is
  the directory that holds the repository the project is in. Any other variable
  ending in `_WORKSPACE` that is not set means the same directory, so an
  ecosystem can write its manifests as `${ACME_WORKSPACE}/...` without the
  toolchain knowing its name; set it to move that ecosystem's checkouts.
- **`RHO_HOME`** is the installation, `~/.rho` unless the variable moves it.

`rho env` shows what was resolved and where from:

```console
$ rho env
RHO_HOME           /root/.rho  (default)
RHO_WORKSPACE      /work  (the parent of the repository at /work/my-app)
```

A path dependency that is not there is fetched into that path only when it is
inside the project or the workspace; any other absolute path is not written to.
`scripts/check-local-paths.sh` fails when a tracked file names a path under
`/home/<user>` or `/Users/<user>`, and is what a CI job runs.

## Dependencies

`deps` maps a module name to a version range:

```yaml
deps:
  rho-log: ">=0.1.0"
  acme:
    acme-forms: ">=0.2.0 <0.3.0"
    acme-alert: ">=1.0.0"
```

A range is one or more comparisons separated by spaces — `>=`, `>`, `<=`, `<`,
or an exact version — and a version satisfies it when it satisfies every one.
A map inside `deps` is a group, which is only for the reader: each entry in it is
resolved exactly as it would be at the top level.

A dependency is found in this order, and the first answer wins:

1. an environment variable the entry names (`depends[].env`);
2. a `path` the entry gives, when it exists;
3. a directory of that name inside the project;
4. an installed module — the version that is **enabled** in the local registry
   ({ref}`modules/registries`);
5. a module in the same repository or the same workspace group, so a checkout
   builds without an install step;
6. the global and the project's cache, which `rho module install` fills;
7. a `git` clone, when the entry names a repository.

`rho module install [dir]` resolves what a project declares, downloads it into
the cache, and writes `rho.lock` with each version and the digest of the package
it was; `rho module update` moves the lock forward and `rho module remove <name>`
drops a dependency. A package whose bytes do not match the digest the registry
recorded is refused.

## Include and import

A Rho file brings in another module in one of two ways, and they are different
relationships ({ref}`language/syntax` has the grammar):

- **`import acme.forms`** keeps the module's names its own. Its functions are
  called through the last segment of the name, `forms.open(...)`; two imported
  modules may both declare `init` and each calls its own.
- **`include "forms"`** splices the module's declarations into the including
  program. A bare name with no `/` and no `.rho` names a module rather than a
  file, and the entry is the `main:` its manifest declares. A module that calls
  functions the application defines — the host hooks a module documents — has to
  be included, because only then do the two share a namespace.

## What a module gives, and what it keeps

A module that other code is meant to use says so under `provides`: the name it is
imported by, and the names that import may use.

```yaml
provides:
  import: acme.forms
  exports:
    - open
    - parse
    - validate
    - Field
```

An application, or another module, declares it and imports it:

```yaml
deps:
  acme-forms: ">=0.2.0"
```

```rho
import acme.forms

fn page() string {
    return forms.open("/save", "post")
}
```

The toolchain holds both sides to it:

- **An import resolves through the importer's own `deps`.** A module that only
  something undeclared provides is refused, and the message names the dependency
  to add.
- **Only exported names can be used through an import.** `forms.trim()` from an
  application is refused with the names the module does export, and a private
  helper never collides with an application function of the same name. The list
  covers types and constants too: an application that writes `forms.Field` needs
  `Field` under `exports`. A value of a type the module keeps can still be passed
  around, as what an exported function returns; it just cannot be named outside.
- **A call across an include is reported.** An included module shares the
  program's namespace, so the language cannot keep its helpers private. Instead,
  once the whole program is loaded, the build warns about each call — from any
  file of the program, not only the one that includes the module — to a function
  of a module whose `provides.exports` does not list it:

  ```text
  warning: src/main.rho calls trim, which the acme-forms module does not export (provides.exports)
  ```

  A name the calling module defines itself is its own and is not reported. Each
  call is reported once per build.

The export list belongs with an import name: give `provides.import` beside
`provides.exports`, because the import is the path through which the list is
enforced, and without one only the include warning reads it. A module with no
`exports` list lets every name through, which is how modules written before
export lists keep working.

## What a version promises

A module's exports are what other modules build against, so its version says how
they changed. Publishing records each version's exported signatures — every
function's parameter and return types, every struct's fields in order, every
constant's type, and an interface's functions — in `exports.json` beside the
version in the local registry.

A release that **removes an export or changes one** needs a new major version, or
a new minor one before 1.0.0; `rho module publish` refuses it otherwise. An added
export, a renamed parameter or a constant's new value promises nothing new to
break.

`rho module check [dir]` says where a working tree stands before that:

```console
$ rho module check modules/acme-alert
acme-alert 0.2.0, compared with acme-alert 0.1.1 in git (3f9c2a7d41be^)
  removed  fn alert_append(config: string, line: string) u32
  added    fn send(level: string, message: string, metadata: string) u32
my-app (my-app) depends on it and uses none of what changed
0.2.0 may break what 0.1.1 promised, which its version says.
```

The previous release is the version recorded when it was published or, for a
module used from its repository, the module as it was before its manifest moved
to the current version, in git. The modules listed are the ones in the workspace
that depend on it or implement its interface, with the lines that use what a
breaking change touches. It exits 1 when the version the manifest declares cannot
carry the change.

## A module's own tables

A module that keeps records owns its tables and the statements over them, and
declares them in its own `sql:` block, with no driver:

```yaml
sql:
  schema: db/schema.sql
  queries: db/queries.sql
```

It generates nothing by itself. When an application that depends on it is built,
rho-sql puts every such module's tables and statements before the application's
own and generates them with the application's driver. A module that names a
driver is a project of its own and contributes nothing.

The module's definition is the one used. An application that still repeats a
module's table or statement is told which copies it can remove, and told when a
statement of the same name differs from the module's, in case it meant one of
its own. Two modules declaring one table, or one statement, is an error naming
both. A column a later version adds is in its table's `CREATE` and in an
`ALTER TABLE ... ADD COLUMN` after it; applying the schema passes over an `ALTER`
whose column is already there, so a new database and an older one end with the
same table. {ref}`rho-sql/index` has the rest.

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
that takes a path; it is written once per content under the user's cache. A
module's locales are merged into the program's catalogs before the program's own,
so a program can override any string. Both are the Go backends'; generated C
carries no assets.

## Applications

An application (`type: rho`) is a Rho program: it has a `main`, `rho build`
generates a project from it, and it is what you build and ship. The toolchain does
not dispatch to it and it publishes no command.

```yaml
module:
  name: my-app
  type: rho
  version: 0.1.0
  description: A small web service
main: src/main.rho

build:
  language: go
  targets:
    - go-server
```

It can be published, which is how a machine that runs applications gets one
without its source tree: `rho module publish` builds it and packs the program
beside the sources, and the registry installs it as a module. A program that runs
applications — a supervisor, a dashboard — finds the installed ones by asking the
toolchain rather than reading the registry's directories:

```console
$ rho module list --format tsv
module	my-app	0.1.0	/root/.rho/modules/installed/module/my-app@0.1.0	selected
```

One installed version per line: kind, name, version, where it is, and whether it
is the version the index selects. `applications/registry` and the projects under
`examples/` are applications.

## Sections the toolchain does not define

A manifest may carry sections the toolchain has no use for — what an ecosystem
records about a board, a driver or a deployment. The toolchain reads the sections
it defines and leaves the others alone, so a module can carry them without
being refused. An ecosystem that wants them checked says so in its module
contract, per type: the sections a manifest of that type must carry and the
shape of each, which `rho module validate` then applies.

## The `package` block

A module may say what a published package carries, so a consumer receives the
component rather than the whole working tree:

```yaml
package:
  kind: component
  binary: true
  import_path: rho.log
  interface:
    - src/log.rho
  include:
    - module.yml
    - README.md
    - CHANGELOG.md
    - src
  require:
    - module.yml
    - README.md
    - src
```

- `kind` is `component`, `plugin`, `application` or `library`.
- `binary` says whether the package carries the built command; `false` publishes
  a component's library form only.
- `interface` is the part a dependency actually needs: the Rho library, and
  nothing else.
- `include` is what the package carries.
- `require` is what a consumer must have for it to work.

## Release history

A module's description says what it is and does not change between releases, so
two versions of one module would read alike. Each module carries a `CHANGELOG.md`
beside its manifest, one section per release:

```markdown
## [0.2.0] - 2026-10-12

Added `validate`, which checks a submitted form against its fields.
```

It travels inside the package, and `rho module publish` reads the section for the
version being published and records it on the registry against that release. A
version with no section is published without a note rather than refused. The
note is text a publisher wrote, so it is escaped wherever it is shown and never
treated as markup.
