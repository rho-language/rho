# Getting started

From an empty machine to a program of your own, in a few minutes.

## 1. Install Rho

Rho is built from source. You need Go 1.26.11 or newer and a C compiler:

```sh
git clone https://github.com/rho-language/rho
cd rho
go build -o ./bin/rho ./cmd/rho
./bin/rho install
export PATH="$HOME/.rho/bin:$PATH"
```

`rho install` creates `~/.rho`, builds the core toolchain from the checkout and
adds it to your shell. {ref}`documentation/installing` says what it needs from the
machine and what each step does.

A first install can build, run and start projects, and it has the language
server. The other tools — the test runner, the formatter, the documentation
generator, the target specs — are **components**, and each one's command exists
once it is installed and enabled. In the checkout, one script builds and installs
all of them:

```sh
./scripts/check.sh
```

`rho help` lists what is reachable right now.

## 2. Run something

Write a file, `hello.rho`:

```rho
module hello

fn main() void {
    print("hello from rho\n");
}
```

```sh
rho run hello.rho
```

`rho run` translates the file to C, compiles it with `gcc` and runs it — the
loop from editing to running is one command.

## 3. Start a project

When a program outgrows one file it becomes a project: a directory with a
`module.yml` that says what it is and which targets it builds. `rho init` writes
one:

```sh
rho init hello-app
cd hello-app
rho run .
```

```
Created a Rho application in hello-app
   module.yml
   src/main.rho
   README.md
   .gitignore
Next:
  cd hello-app && rho run .
```

The new project builds for the `go` target, so `rho run .` generates a Go
program under `build/generated/go/`, builds it and runs it, and needs no
component. `rho init --language c` starts a C project instead, which `rho build`
compiles through the `rho-target` component.

The manifest, the layout and what each target generates are in
{ref}`documentation/generating`. A project is an **application** — the thing you
build and ship — as distinct from the components and plugins a toolchain
installs; {ref}`modules/modules` describes all of them.

## 4. Add a source file and a test

`rho new` starts a source file with its header already written, named after the
module it belongs to:

```sh
rho new src/greeting.rho --description "How the program greets."
rho new tests/greeting_test.rho
```

A file named `*_test.rho` starts as a test file: a `test fn` and a `main` that
calls it. With the `rho-test` component enabled, `rho test` builds and runs it;
{ref}`tools/testing` describes how tests are written and listed.

## Where to go next

- {ref}`language/index` — the language itself.
- {ref}`documentation/generating` — starting a project, and what `rho build` writes.
- {ref}`building/targets` — the backends, services, ports and boards.
- {ref}`modules/modules` — components, plugins, and what an application is.
- {ref}`modules/registries` — where modules come from, and the official registry.
- {ref}`tools/cli` — every command the `rho` CLI answers.
- {ref}`tools/components` — everything the toolchain can be asked to do.
