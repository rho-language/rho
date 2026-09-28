# rho-kconfig

The configuration bridge. It maps what a Rho project says into the Kconfig,
sdkconfig and `FreeRTOSConfig.h` that the toolchains around it read.

```sh
rho kconfig values            # what this project declared
rho kconfig list <fragment>   # every symbol, its value, and where it came from
rho kconfig set CONFIG_SPI_MASTER n
rho kconfig merge --dialect espidf <fragment>...
```

## What it is for

Embedded projects are configured by the tools that build them: ESP-IDF reads
`sdkconfig`, the Linux kernel and its out-of-tree drivers read `.config` and
`include/generated/autoconf.h`, and a FreeRTOS project reads `FreeRTOSConfig.h`.
The same settings, spelled three ways, in three directories that must be exactly
right.

So a Rho project declares its configuration **once**, in its own terms, and this
component writes it in the form the toolchain it targets reads. The project does
not carry a `sdkconfig` it has to keep in step with its own code; it declares what
it wants and the build derives what the tool needs.

## How a project declares it

```yaml
# module.yml
build:
  language: c
  targets:
    - c

config:
  dialect: espidf
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_SPI_MASTER: "y"
    CONFIG_UART_BAUD: "115200"
  module: config.rho
```

Three places, each for its own kind of setting:

| Place | What belongs there |
|-------|-------------------|
| `kconfig_fragments` | The Kconfig files that **define** the symbols. A target ships its own; a project adds to them. |
| `values` | The symbols this project **sets**, where a project has an opinion of its own. |
| `module` | A Rho module of constants, for a project that would rather write its configuration as the code it is. |

`dialect` says which headers to write: `linux`, `espidf` or `freertos`.

## The dialect

`rho build` does not resolve the configuration. It generates a project that
*does*, in its own CMake, which is what keeps a build reproducible and lets a
value change without regenerating anything:

```cmake
include(rho-config.cmake)
add_executable(app main.c app.c runtime.c)
# ... and every source depends on the resolved .config
```

CMake runs `rho kconfig merge` before compiling, and the generated sources
include what it wrote. The same thing a person does by hand is what the build
does, which is why it is written in the build file rather than in the generator:
you can read it, and you can change it.

## Values from Rho source

A project may write its configuration as the code it is:

```rho
module config

fn init() {
    let CONFIG_UART_BAUD: u32 = 115200
    let CONFIG_SPI_MASTER: bool = true
    let CONFIG_LOG_LEVEL: string = "info"
}
```

The file is read with the compiler's own parser, so it is held to the same syntax
the compiler holds it to, and a value is a literal: a number, a string, or a flag.
A binding that does not name a symbol is the program's own state and is left
alone, so one module may hold both.

## Where values come from, and which wins

Later wins. This is the whole precedence rule:

1. **defaults** — from the fragments themselves
2. **defconfig** — `kconfig/rho-defconfig`, the baseline a project commits
3. **module.yml** — `build.config.values`
4. **config.rho** — the module of constants, which is a project's own opinion

A value for a symbol no fragment declares is refused rather than written, because
a value nothing reads is a setting that silently did nothing. A value of the wrong
type is refused for the same reason, earlier: a header that does not compile four
files later is a worse place to find out.

A symbol nothing sets keeps its default, and a flag with no default is off.

## As a library

A program reads its own configuration through the `rho.kconfig` library, which is
supplied by the `kconfig` port the same way logging is supplied by `logemit`:

```rho
import rho.kconfig

fn init() {
    if !kconfig_enabled("CONFIG_SPI_MASTER") {
        info("board", "no SPI bus; falling back to the status line");
    }
}
```

The generated code includes the header the bridge wrote, so a value is resolved
before the program starts rather than read at run time.

## Dialects

| Dialect | Writes | Read by |
|---------|--------|----------|
| `linux` | `.config`, `include/generated/autoconf.h` | The kernel and out-of-tree drivers |
| `espidf` | `sdkconfig`, `build/config/sdkconfig.h` | The ESP-IDF build |
| `freertos` | `FreeRTOSConfig.h`, `.config` | A FreeRTOS project |

Each writes where its reader looks, which is part of the contract: a header in the
wrong directory is a header nothing includes. FreeRTOS symbols are written
without the `CONFIG_` prefix, because that is how `FreeRTOSConfig.h` is written
by hand, and an unset flag is left out rather than defined to zero, because there
those are different statements.

## Extending it

The component takes plugins, over the same mechanism every component does. It
offers one hook: `render`, which runs once after the fragments are merged and
before anything is written, and answers with files to write beside the dialect's
own. That is how a project targets a tool this component has never heard of.

```yaml
# the plugin's module.yml
extends: rho-kconfig
hooks: [render]
commands:
  - name: my-dialect
    run: my-kconfig-dialect
    hook: render
```

The request is a JSON envelope with the configuration in its `payload`:

```json
{
  "version": 1, "hook": "render", "command": "my-dialect", "component": "rho-kconfig",
  "payload": {
    "dialect": "espidf",
    "values": [{ "name": "CONFIG_SPI_MASTER", "type": "bool", "value": "y", "from": "defconfig" }]
  }
}
```

and the answer is the same envelope, with the files in its payload:

```json
{ "version": 1, "payload": { "files": [{ "path": "prj.conf", "content": "…" }] } }
```

:::{note}
The payload is *inside* the envelope, not the whole document: a plugin that
decodes the whole request as its own payload finds every field empty and no
error to tell it so.
:::

A plugin that fails is reported and its files are left out: the configuration the
project declared is still written.

## Building it

```sh
go build -o rho-kconfig ./src
go test ./...
```

To have a build call it, publish it:

```sh
rho module publish modules/components/rho-kconfig
```
