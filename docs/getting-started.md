# Getting started

From an empty machine to a program of your own, in a few minutes.

## 1. Install Rho

Rho is a compiler and a toolchain, built from source today. Two commands put
`rho` on your `PATH`:

```sh
git clone https://github.com/rho-language/rho
cd rho && go build -o ./bin/rho ./cmd/rho && ./bin/rho install
export PATH="$HOME/.rho/bin:$PATH"
```

`rho install` creates `~/.rho`, builds the toolchain from the checkout, and adds
it to your shell. What it needs from the machine, and what to do when something
is missing, is {ref}`handbook/installing`.

A fresh install can build and run. The tools that add a command — the formatter,
the documentation generator, the test runner — are **components**, and each is
reachable once it is installed and enabled. {ref}`handbook/modules` says what
that means; a first install has none of them, and `rho help` lists what is
reachable now.

## 2. Run something

Write a file:

```rho
module hello

fn main() void {
    print("hello from rho\n");
}
```

```sh
rho run hello.rho
```

`rho run` compiles the file for the host, builds it, and runs it — the whole
loop from editing to running is one command.

## 3. Start a project

When a program outgrows one file it becomes a project: a directory with a
`module.yml` that says what it is and which targets it builds. `rho init` writes
one:

```sh
rho init hello
cd hello
rho run .
```

```
Created a Rho application in hello
   module.yml
   src/main.rho
   README.md
   .gitignore
```

The manifest, the layout, and what each target generates are in
{ref}`handbook/generating`. A project is an **application** — the thing you build
and ship — and is described apart from the components and plugins a toolchain
installs, in {ref}`handbook/modules`.

## Where to go next

- {ref}`language/index` — the language itself.
- {ref}`handbook/installing` — installing in depth, and what a fresh install can do.
- {ref}`handbook/generating` — starting a project, and what `rho build` writes.
- {ref}`handbook/modules` — components and plugins, and what an application is.
- {ref}`handbook/registries` — where modules come from, and the official registry.
- {ref}`components <documentation/components>` — everything the toolchain can be asked to do.
