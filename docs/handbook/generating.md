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

```yaml
# my-app/module.yml
module:
  name: my-app
  type: rho
  version: 0.1.0
  description: A Rho application.

main: src/main.rho

build:
  language: go
  targets:
    - go
```

## A project a module knows how to generate

A repository that is a *collection* — not one module but many of a kind, such
as a drivers repository — declares in its `module.yml` what a person may start
from it. `rho init` in that directory reads that declaration and writes what it
says, instead of a Rho application, because that is what the directory is for.

```yaml
# module.yml at the repository root
module:
  name: robohub-drivers
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
