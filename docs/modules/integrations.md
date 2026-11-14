# Integrations

Rho is a general-purpose language and toolchain: C for a part, Go for an edge or a
server, YAML read natively. It names no ecosystem. An ecosystem — a family of
platforms, runtimes and applications with its own conventions — meets the
toolchain through **an integration plugin**, and through nothing else. No part of
the toolchain looks inside a plugin's folder for a file by convention; it asks
for what an ecosystem provides by kind, for the platform a project is built for.

## The platform a project is built for

A platform is a module that says so in its own manifest:

```yaml
module:
  name: acme-platform
  type: module
  version: 0.3.0
platform: acme
```

A project's platform is found from its module graph: its own manifest first, then
the modules it depends on by `path`, depth first. The first `platform:` found
wins. A project with no such module in its graph has no platform and is built the
standard way, by the standard plugins.

## What an integration plugin is

A plugin (`type: plugin`) that lists its ecosystem under `serves:` and carries an
`integration:` section. It has two halves, and may have either or both:

- **What it answers, as a program.** It fills components' contracts for its
  ecosystem through `extends:`, like any plugin ({ref}`modules/plugins`): the
  target facts of its boards (`rho-target`'s `platform`), its configuration
  (`rho-kconfig`), its C mapping (`rho-cimport`), how its boards are flashed
  (`rho-flash`), and the module types the ecosystem defines (`rho-module`'s
  `module-manifest`, which the module validator consults).
- **What it provides, as data**, in its `integration:` section, read through one
  resolver (`src/integration`).

```yaml
module:
  name: acme-integration
  type: plugin
  version: 0.1.0
main: acme-integration

serves:
  - acme                       # offered only to projects whose platform is acme

integration:
  ecosystem: acme
  workspace_env: ACME_WORKSPACE
  provides:
    bindings:
      protocol-bus-c: bindings/bus.yml
      protocol-transport-rho: bindings/transport.rho
    ports: ports/

package:
  kind: plugin
  include: [module.yml, bindings, ports]
```

| Field | What the toolchain does with it |
|---|---|
| `ecosystem` | The platform identity the plugin bridges. It must also be under `serves:`, so the plugin is offered only to that ecosystem's projects. |
| `workspace_env` | The variable the ecosystem's manifests name the workspace by. It must end in `_WORKSPACE`, which the toolchain resolves to its own workspace when it is unset ({ref}`modules/modules`). |
| `provides.bindings` | Files by binding kind. `protocol-bus-c` is the event bus generated C reaches, read by `rho protocol generate --target c --platform acme`; `protocol-transport-rho` is the framing generated Rho carries, read by `--target rho`. A kind the toolchain does not read is refused. |
| `provides.ports` | A directory that is a Go module of its own, holding Go ports — complete files in `package main` — that a project of the ecosystem may ask for by name ({ref}`building/ports`). Test files there are the ports' own and are not ports. |

A plugin that only provides data needs no `extends:` and no `commands:` — it
plugs into no component, so it has no hooks to answer. `serves:` and
`integration:` alone make it a valid plugin.

## Where the toolchain looks

An integration is one of the plugins the plugin source finds, so it is subject to
the same rules: an installed plugin counts when its version is **enabled**, and a
plugin in the working tree counts only where the caller trusts the tree. Of
those, the ones serving the project's platform are asked.

| Asked for | Installed (enabled) plugins | Plugins in the project's tree |
|---|---|---|
| Ports, during `rho build` | yes | **yes** |
| Protocol bindings, by `rho protocol generate --platform` | yes | no |

Ports trust the tree because a port is Go source compiled into the project, as the
project's own sources are: an application can keep a port beside its code, in a
plugin of its own that serves its platform, and version it with the code rather
than in the platform's plugin. A port a tree plugin provides is merged into the
generated port file like one an installed plugin provides.

## The contract is checked

`rho module validate <plugin>` (which `rho module publish` runs first) holds an
integration plugin to the contract and names every way it falls short:

- no `ecosystem`, or an ecosystem the plugin does not list under `serves:`;
- a `workspace_env` that does not end in `_WORKSPACE`;
- a binding kind the toolchain does not read, a binding that is not a file in the
  plugin, or one the package's `include` leaves out;
- a ports directory without its own `go.mod`, with no ports in it, with a port
  that is not `package main`, or that the package leaves out.

The integration checks run as part of a plugin's relation checks, so they apply
to a plugin that also extends a component; a plugin that only provides data is
accepted without them today.

## An example

`rho-robohub`, in `modules/plugins`, is the integration for the RoboHub
ecosystem. It serves the `robohub` platform, fills the `platform` contract of
`rho-target`, `rho-kconfig`, `rho-cimport` and `rho-flash` and the
`module-manifest` contract of `rho-module`, and provides both protocol bindings
and a ports directory — every half described above, in one plugin.

## What stays out of the toolchain

An ecosystem's names, paths, wire formats, runtime functions and ports live in its
integration plugin and its own repositories. The toolchain carries generic
features an ecosystem uses like any project does: the dependency manager, module
identity, workspace paths, authenticated services, live streams, the test runner,
deployment. A feature added for one ecosystem is added so that any ecosystem can
use it.
