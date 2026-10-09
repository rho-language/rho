# The handbook

How the Rho toolchain is put together, and how a project uses it.

## Pages

- {ref}`getting-started <documentation/getting-started>` — install, write a
  program, build it and run it.
- {ref}`handbook/architecture` — the whole RoboHub stack: the modules, and how a
  reading travels from a sensor to a gateway.
- {ref}`handbook/installing` — what is needed, and how the toolchain is installed.
- {ref}`handbook/modules` — the three kinds of module, and what each is for.
- {ref}`handbook/registries` — the local and remote registries, and who may
  change what is in each.
- {ref}`handbook/ports` — how a project asks for what only a host can do.
- {ref}`handbook/networking` — the network contract, and how each platform
  supplies it.
- {ref}`handbook/documentation` — how a module documents itself, and how this
  site is built.
- {ref}`handbook/testing` — how a Rho project is tested.

## The shape of a project

A project is a directory with a manifest:

```
my-service/
  module.yml        what this is, what it needs, what it generates and builds
  src/main.rho      the program
  tests/            Rho tests
  docs/             its documentation, if it has any
```

`module.yml` is the manifest the whole toolchain reads. Its `module:` block
says what the module is:

| Type | What it is | Example |
|------|------------|---------|
| `rho` | An application: it has a `main` and generates a program | the registry |
| `component` | A tool: it publishes a command and is dispatched to | `rho-fmt`, `rho-log` |
| `plugin` | An extension: it adds to a tool rather than adding a command | `rho-doc-toc` |
| `module` | A library: it exports declarations and nothing else | a shared package |

## Installing

`rho install` builds the toolchain and creates the local registry, which is
where installed modules live. What it needs, and what a fresh installation can
and cannot do, is {ref}`handbook/installing`.

## Building

```sh
rho build <project-dir>
```

Every declared target is generated under `build/generated/<target>/`. A project
that declares `c`, `go` and `go-server` produces all three, because the choice
of backend is a property of the project rather than of the machine:

| Target | What it produces |
|--------|------------------|
| `c` | A native executable, through cmake |
| `go` | A Go command |
| `go-server` | A Go web service: routes, templates and a static directory |

## Running

```sh
rho run <project-dir>
```

A component is reached by the command its manifest publishes:

```sh
rho fmt src/main.rho
rho log -level info -subject registry -message started
rho test <project-dir>
rho doc . -out build/docs
```

`rho <command>` looks the command up among the installed components and runs the
one that published it. The CLI does not know these commands: they arrive with
the component, which is why adding a tool does not mean changing the CLI.
