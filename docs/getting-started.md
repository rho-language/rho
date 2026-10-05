# Getting started

From nothing to a program running on the machine, in three commands.

## 1. Install the toolchain

```sh
git clone https://github.com/rho-language/rho
cd rho
./check.sh
```

`check.sh` builds the core tools, installs them under `~/.rho`, publishes every
module in the tree and installs the ones the tree carries. What it needs from
the machine, and what a fresh installation can and cannot do, is
{ref}`handbook/installing`.

After it finishes, `rho` is on your `PATH` at `~/.rho/bin/rho`:

```sh
rho install --update    # rebuild and refresh from the tree, at any time
```

## 2. Write a program

```sh
mkdir hello && cd hello
```

`module.yml` says what the directory is. A program is a `rho` module: it has a
`main`, and it is generated into a program for each target it declares.

```yaml
module:
  name: hello
  type: rho
  version: 0.1.0
  description: says hello

main: src/main.rho

build:
  language: rho
  targets:
    - id: c
```

```rho
module hello

fn main() void {
    print("hello from rho\n");
}
```

## 3. Build and run

```sh
rho build .     # generate every declared target under build/generated/
rho run .       # build it if needed, then run it
```

`rho run` on a host target runs the generated program directly. For a program
built for a board, `rho run` has nothing to run it on — that is what
{ref}`rho-flash <rho-flash/index>` is for.

## Talking to C header files

A program that calls into C declares the headers it may use, and
{ref}`rho-cimport <rho-cimport/index>` generates the Rho side of them — one
`extern fn` per declaration, and a shim per call:

```yaml
c_connection:
  library: sensor-lib
  headers:
    - registry.h
    - sensor/hal/spi.h
```

```rho
module app

import ffi

fn main() void {
    let handle: Option[Ptr[Sensor]] = sensor_open("bus0")
    if is_some(handle) {
        sensor_read(value(handle))
    }
}
```

Nothing about the header is written out by hand: the declarations come from the
header the project named, so a header that changes changes them with it.

## Measuring what you built

The toolchain carries tools for the questions a firmware raises, each published
as a component:

```sh
rho mem                 # where the program's memory went, and whether it fits
rho bench               # how long it takes, and how that compares with a baseline
rho monitor             # the board's console, while a debugger holds the probe
rho target show pico    # what a board's build is described by
```

`rho mem` reads the linker map a build writes by default, so it needs no
configuration to answer the first question. {ref}`rho-mem <rho-mem/index>` says what
it does with it.

## Where to go next

- {ref}`language/index` — the language itself.
- {ref}`handbook/modules` — the three kinds of module, and what each is for.
- {ref}`handbook/registries` — where modules are installed from, and how a
  remote registry is used.
- {ref}`components <documentation/components>` — everything the toolchain can be asked to do.
