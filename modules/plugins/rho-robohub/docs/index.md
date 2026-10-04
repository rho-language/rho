# The RoboHub platform

RoboHub, expressed as this toolchain's `platform` contract rather than as a
assumption built into it.

## Why it is a plugin

Which family a part belongs to, what macro selects that family in a vendor's own
driver sources, which header the vendor's umbrella includes by name, how a board
is programmed — none of that is a fact about a language. Written into the
toolchain, it makes the toolchain a language for one ecosystem: another platform
could not be supported without editing the toolchain, and a build for the host
carried the machinery of a platform it was not using.

As a contract, each of those is a question a component asks a plugin, and a
platform is whatever answers them. Four components ask, through four hooks, and
one plugin answers all four.

## What it declares

The plugin fills the `platform` contract of `rho-target`, `rho-kconfig`,
`rho-cimport` and `rho-flash`. The contract is named `platform` in each because
it is the same role each time: one platform has been mapped, four times, rather
than four unrelated things.

| Component | Hook | What the platform contributes |
|---|---|---|
| `rho-target` | `spec` | The family macro and the vendor HAL header for an STM32 board |
| `rho-kconfig` | `render` | Nothing — the dialect writes what RoboHub's C includes |
| `rho-cimport` | `map` | Nothing — RoboHub's interfaces are C the generator already reads |
| `rho-flash` | `flash` | Nothing — every board names its own flash command |

## The macro and the header

For a board of an STM32 family, `rho-target` receives two facts it cannot derive
without guessing:

- the macro the vendor's driver sources test for to select the family —
  `ROBOHUB_STM32_FAMILY_H7=1`;
- the umbrella header the vendor includes by its bare name — `stm32h7xx_hal.h`,
  from which the build produces the `stm32h7xx_hal_conf.h` the vendor does not
  ship.

Both come from the family the target declares (`STM32H7`), with the vendor's
`STM32` stem dropped. A target that already declares either is left alone: the
platform module and this plugin are two descriptions of one platform, and a fact
stated in both is a fact that will eventually disagree.

## Recognition

The mapping is chosen by what the project is, not by what is installed. A
platform module declares its identity:

```yaml
module:
  name: robohub-stm32
  type: platform
platform: robohub
```

A build walks the project's module graph, and a module that declares
`platform: robohub` identifies the platform. The plugins that serve that
identity are the ones the run chooses between.

A project whose graph names no platform is built the standard way: only
platform-agnostic plugins run, and a target spec carries everything the build
needs. That is the case that matters — Rho is a language first, and a platform
is something a project may have.