# Starting a project

`rho init` starts a project. What it writes depends on where it is run, and it
depends on the manifest, not a flag.

## A Rho application

In a directory with no generation manifest, `rho init` writes a Rho
application: a `module.yml`, one `src/main.rho`, a `README.md` and a
`.gitignore`. It depends on nothing, so it builds and runs as it stands:

```sh
rho init my-app
cd my-app && rho run .
```

| Flag | Default | What it does |
|---|---|---|
| `--name NAME` | the directory's name | The module name, made into one (see below) |
| `--description TEXT` | `A Rho application.` | The manifest's one-line description, and the `@brief` of `module.yml` and `src/main.rho` |
| `--target a,b` | the language | The build targets, comma-separated (`c`, `go`, `go-server`) |
| `--language go\|c` | the first target's language, else `go` | `build.language` |

Flags may come before or after the directory. A module name holds lower-case
letters, digits and `_`, so the directory's name is made into one — `my-app`
becomes `my_app` — and `rho init` says so when the two differ. The name is what
`src/main.rho` declares, what the binaries are called and what an import of the
module is written as; `--name` chooses another.

What is built is `build.targets`: a project per target under
`build/generated/<target>/`. `build.language` is the language the module is
built as (`c` or `go`): a project that depends on a module whose language is `c`
treats it as a C library, and for an application it is informational. `rho init`
takes it from the first target, so `--target c,go` writes `language: c`; the
manifest check warns when it is not the language of any target.

A directory that already holds a `module.yml` is refused rather than written
over. The manifest it writes, without its header comment:

```yaml
# my-app/module.yml
module:
  name: my_app
  type: rho
  version: 0.1.0
  description: A Rho application.

main: src/main.rho

build:
  language: go
  targets:
    - go
```

`src/main.rho` starts with the same header `rho new` writes — the file, the
description, the module, the author and the date — and declares `module my_app`:
`main.rho` is the module itself.

## Where a build puts the program

`rho build` writes each target's generated project under
`build/generated/<target>/` and then builds it:

| Target | The program | Named after |
|---|---|---|
| `go`, `go-server` | `build/<name>` | the manifest's `module.name` |
| `c` (and a board) | `build/output/<name>` | the module `src/main.rho` declares |

The build's last line names the program (`Built binary: build/output/my_app`).
These are the defaults: the manifest's `build.dir` and `build.output_dir`, and a
project's own descriptor (`project.yml`), can move them.
`rho test` never writes there: a test program is built under `build/test/`.

## A project a module knows how to generate

A repository that is a *collection* — not one module but many of a kind, such
as a drivers repository — declares in its `module.yml` what a person may start
from it. `rho init` in that directory reads that declaration and writes what it
says, instead of a Rho application, because that is what the directory is for.

```yaml
# module.yml at the repository root
module:
  name: acme-drivers
  type: collection
  version: 0.1.0

generate:
  - name: driver
    description: A new device driver, and the example that uses it
    variables:
      - name: category
        description: The category the driver belongs to, such as temperature
      - name: name
        description: The device, such as ds18b20
    outputs:
      - path: drivers/{{category}}/{{name}}
        template: templates/driver
      - path: examples/{{category}}/{{name}}/c
        template: templates/example
```

- `name` is what a person types to choose the generator.
- `variables` are what it asks for; `rho init` fills them from the options a
  person passes (`--category`, `--name`, or `--set key=value` for anything
  else) and refuses to write until each has a value.
- `outputs` are the directories to write. One generator can write several — a
  driver and the example that uses it — because the example is not useful
  without the driver it names.

Each output has a `path`, where it goes relative to the destination, and a
`template`, a directory of files relative to the repository root. `{{name}}`
and the other variables are substituted in the path, in every file and
directory name, and in every file's contents, so a template writes `{{name}}`
once and the file named after the device and the code inside it agree.

```sh
rho init --project . --category temperature --name ds18b20 driver drivers/temperature/ds18b20
```

Run with no arguments in a repository that declares generators, `rho init`
lists them:

```sh
rho init --project .
```

## Generators in a file

When the definitions are long, `generate:` names a file that holds them instead
of holding them itself. The file's schema is the same; its list is under
`generators:`.

```yaml
# module.yml
generate: generate.yml
```

```yaml
# generate.yml
generators:
  - name: driver
    # ...
```

The value may also be written inline, as above, for a repository with one or
two. Both forms are read the same way, so moving between them changes nothing
a person types.
