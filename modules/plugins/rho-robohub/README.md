# rho-robohub

The RoboHub platform, as this toolchain's `platform` contract.

A platform is a mapping. Which family a part belongs to, what macro selects
that family in a vendor's own driver sources, which header the vendor's umbrella
includes by name, how a board is programmed — none of that is Rho's, and all of
it used to be written into the toolchain itself. That is what made Rho a language
for one ecosystem rather than a language with a platform attached.

It is here instead. This plugin fills the `platform` contract of four components
at once, and runs only for a project whose module graph names `platform:
robohub`.

## What it maps

| Component | Hook | What the platform says |
|---|---|---|
| `rho-target` | `spec` | The macro that selects a part's family, and the vendor header the build has to produce a configuration header for |
| `rho-kconfig` | `render` | Nothing: the dialect already writes what RoboHub's C includes |
| `rho-cimport` | `map` | Nothing: RoboHub's interfaces are C the generator already reads |
| `rho-flash` | `flash` | Nothing: every board names its own flash command in its spec |

Three of the four are deliberately empty, and saying so is the contract being
implemented rather than a stub: each is there for the day the platform has
something the component cannot know, and it is where that goes. What is not
repeated is more important — a board reached in two ways, a configuration
written twice, or a type mapped in two places is a description that will
eventually disagree.

The target hook is where the work is. For an STM32-family board it contributes
the two facts a spec cannot derive on its own:

```json
{"target":"nucleo-h743zi","defines":["ROBOHUB_STM32_FAMILY_H7=1"],"hal_header":"stm32h7xx_hal.h"}
```

and it touches nothing that is not an STM32 part, which is what lets one
`rho-target` build for the host, a Pico and an ESP32 with this plugin installed.

## The declaration

A plugin that extends more than one component names each of them, because a
hook name is not unique across the toolchain and `render` is two different
things:

```yaml
serves:
  - robohub

extends:
  - component: rho-target
    implements: [platform]
    hooks: [spec]
  - component: rho-kconfig
    implements: [platform]
    hooks: [render]
  - component: rho-cimport
    implements: [platform]
    hooks: [map]
  - component: rho-flash
    implements: [platform]
    hooks: [flash]

commands:
  - name: target-spec
    run: rho-robohub
    hook: spec
    component: rho-target
  # ... one per component
```

Every contract is named `platform`, because it is the same role each time: one
platform has been mapped, four times, rather than four unrelated things.

`serves` is what makes the mapping happen for a RoboHub project and not for any
other. A build recognizes the platform from the project's module graph — a
platform module declares `platform: robohub` — and runs only the plugins naming
that identity. A project whose graph names none is built the standard way, with
this plugin absent and everything it would have said coming from the target spec.

## Running it by hand

The protocol is the toolchain's: one JSON request in, one JSON response out, so
the program works on its own.

```sh
echo '{"version":1,"hook":"spec","command":"target-spec","component":"rho-target",
       "payload":{"details":[{"name":"nucleo-h743zi","family":"STM32H7"}]}}' | rho-robohub
```

```json
{"version":1,"payload":{"augment":[{"target":"nucleo-h743zi","defines":["ROBOHUB_STM32_FAMILY_H7=1"],"hal_header":"stm32h7xx_hal.h"}]}}
```

## Adding another platform

A platform is supported by writing a plugin of this shape and naming it in
`serves`. Nothing in the toolchain changes: the same four contracts are filled,
a project whose graph names that platform gets that mapping, and every other
project is unaffected.