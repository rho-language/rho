# rho-kconfig

The configuration bridge. It maps what a Rho project declares into the Kconfig,
`sdkconfig` and `FreeRTOSConfig.h` that the toolchains around it read.

```sh
rho kconfig values            # what this project declared
rho kconfig list              # every symbol, its value, and where it came from
rho kconfig set CONFIG_SPI_MASTER n
rho kconfig merge --dialect espidf <fragment>...
```

## What it is for

Embedded projects are configured by the tools that build them: ESP-IDF reads
`sdkconfig`, a kernel build reads `.config` and `include/generated/autoconf.h`,
and a FreeRTOS project reads `FreeRTOSConfig.h`. The same settings, spelled three
ways, in directories that must be exactly right.

So a project declares its configuration **once**, in its own terms, and this
component writes it in the form the toolchain it targets reads. The project does
not carry a `sdkconfig` it has to keep in step with its own code.

## A project declares it

```yaml
# module.yml
build:
  language: c

config:
  dialect: espidf
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_SPI_MASTER: "y"
    CONFIG_UART_BAUD: "115200"
  module: config.rho
```

The fragments **define** the symbols, `values` **set** them, and `module` is a
Rho module of constants for a project that would rather write its configuration
as the code it is.

## The build resolves it

`rho build` does not resolve the configuration. It generates a project that
*does*, in its own CMake:

```cmake
include(rho-config.cmake)
add_executable(app main.c app.c runtime.c)
# every source depends on the resolved .config
```

CMake runs `rho kconfig merge` before compiling, and the generated sources
include the header it wrote. That is what keeps a value change from needing a
regeneration, and it is written in the build file rather than the generator so a
person can read what a build does before it does it.

## Precedence

Later wins: defaults, then `kconfig/rho-defconfig`, then `module.yml` values,
then `config.rho`. A value for a symbol no fragment declares is refused rather
than written, because a value nothing reads is a setting that silently did
nothing.

## Dialects

| Dialect | Writes | Read by |
|---------|--------|----------|
| `linux` | `.config`, `include/generated/autoconf.h` | A kernel build and out-of-tree drivers |
| `espidf` | `sdkconfig`, `build/config/sdkconfig.h` | The ESP-IDF build |
| `freertos` | `FreeRTOSConfig.h`, `.config` | A FreeRTOS project |

Each writes where its reader looks, which is part of the contract: a header in
the wrong directory is a header nothing includes.

## Reading it from Rho

A program asks its own configuration through the `rho.kconfig` library, supplied
by the `kconfig` port the way logging is supplied by `logemit`:

```rho
import rho.kconfig

fn init() {
    if !kconfig_enabled("CONFIG_SPI_MASTER") {
        info("board", "no SPI bus");
    }
}
```

## Extending it

| Hook | When it runs | What a plugin answers with |
|------|--------------|---------------------------|
| `render` | After the merge, before anything is written | Files in one more dialect |
| `write` | After every dialect has been written | Nothing yet; for a step that observes |

A plugin that fails is reported and skipped: one broken extension must not cost a
project its configuration.
