# Integrations

Rho is a general-purpose language and toolchain: C for a part, Go for an edge or a
server, YAML read natively. It names no ecosystem. An ecosystem (a family of
platforms, runtimes and applications with its own conventions) meets the toolchain
through **one integration plugin**, and through nothing else.

`rho-robohub` is the integration for the RoboHub ecosystem, and the reference for
writing another.

## What an integration plugin is

A plugin (`type: plugin`) with two halves:

- **What it answers, as a program**: it fills the components' plugin contracts
  for its ecosystem through `extends:`, like any plugin. A platform's target facts
  (`rho-target`'s `platform`), its configuration (`rho-kconfig`), its C mapping
  (`rho-cimport`), how its boards are flashed (`rho-flash`), and the module types
  the ecosystem defines (`rho-module`'s `module-manifest`).
- **What it provides, as data**: declared in its `integration:` section and read
  by the toolchain through one resolver (`src/integration`), never by looking in
  the plugin's folders.

```yaml
serves:
  - acme                       # runs only for projects whose platform is acme

integration:
  ecosystem: acme
  workspace_env: ACME_WORKSPACE
  provides:
    bindings:
      protocol-bus-c: bindings/bus.yml
      protocol-transport-rho: bindings/transport.rho
    ports: ports/
```

| Field | What the toolchain does with it |
|---|---|
| `ecosystem` | The platform identity. It must also be under `serves:`, so the plugin is offered only to that ecosystem's projects. |
| `workspace_env` | The variable the ecosystem's manifests name the workspace by. It ends in `_WORKSPACE`, which the toolchain resolves to its own workspace when unset. |
| `provides.bindings` | Files by binding kind. `protocol-bus-c` is the event bus generated C reaches, read by `rho protocol generate --target c --platform acme`; `protocol-transport-rho` is the framing generated Rho carries, read by `--target rho`. A kind the toolchain does not read is refused. |
| `provides.ports` | A directory that is a Go module of its own, holding Go ports (complete files in `package main`) a project of the ecosystem may ask for by name, merged into its generated port file. Test files there are the ports' own. |

## The contract is checked

`rho module validate <plugin>` holds an integration plugin to the contract and
names every way it falls short: the ecosystem it does not serve, a binding kind
the toolchain does not read, a binding that is not a file, ports that are not a Go
module or not `package main`, anything it provides that its package would leave
out. A plugin that passes is one the toolchain can rely on.

## What stays out of the toolchain

An ecosystem's names, paths, wire formats, runtime functions and ports live in
its integration plugin and its own repositories. The toolchain carries generic
features an ecosystem uses like any project does: the dependency manager, module
identity, workspace paths, authenticated services, live streams, the test runner,
deployment. A feature added for one ecosystem is added so that any ecosystem can
use it.
