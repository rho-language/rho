# Interfaces, providers and bindings

An **interface** says what a family of providers can do. A **provider** is one
implementation of it. The **application** decides which providers it compiles in
and how each is configured, in its own manifest, and the interface's code decides
how to use them. Nothing in between chooses for the application: a library that
needs to send an alert depends on the interface, not on a mail or chat service.

## The interface

An interface is a module with an `interface` section, each function written with
its signature as Rho writes it:

```yaml
module:
  name: acme-alert
  type: module
  version: 1.0.0
main: src/alert.rho
provides:
  import: acme.alert
  exports: [raise]
interface:
  functions:
    send: "(level: string, message: string) u32"
```

The interface's own code is ordinary Rho. What it gets from the binding is
generated into it when an application is built (below), so `raise` can be written
against providers it has never heard of.

## A provider

A provider says which interface it implements, exports each of its functions with
exactly the declared signature, and lists the settings it reads:

```yaml
module:
  name: acme-alert-mail
  type: provider
  version: 0.1.0
main: src/provider.rho
implements: [acme.alert]
provides:
  import: acme.alert_mail
  exports: [send]
  settings: [smtp_host, recipient]
```

```rho
module acme.alert_mail

extern fn rho_bind_config(provider: string, key: string) string

fn send(level: string, message: string) u32 {
    let host: string = rho_bind_config("acme-alert-mail", "smtp_host")
    // ... connect to host and send the message
    return 0
}
```

`implements` names the interface by its import name. `provides.import` is
required: the binding imports each provider through it. `provides.settings` is
the complete list of keys an application may configure the provider with.

## The application binds

The application declares the interface and its providers as dependencies, and
binds them under `bind:`, keyed by the interface's import name:

```yaml
deps:
  acme-alert: ">=1.0.0"
  acme-alert-log: ">=0.1.0"
  acme-alert-mail: ">=0.1.0"

bind:
  acme.alert:
    providers: [acme-alert-log, acme-alert-mail]
    config:
      acme-alert-mail:
        smtp_host: mail.example.com
        recipient: ops@example.com
        enabled: "false"
```

Only the bound providers are compiled in. The build is refused, naming the
problem, when a bound provider:

- is not a declared dependency of the application;
- does not list the interface under `implements`;
- publishes no `provides.import`;
- is configured with a key that is not in its `provides.settings`;
- does not export one of the interface's functions, or exports it with another
  signature.

Every provider accepts `enabled` without declaring it, so an interface can let an
installation switch a provider off without removing it from the binding. The key
means nothing to the toolchain; the interface's code reads it and decides.

## What the binding generates

For each interface it imports, the build writes a module into
`build/bindings/<interface>.rho` and makes it part of the interface:

| Generated | What it does |
|---|---|
| `<function>_to(provider, ...)` | For each interface function, e.g. `send_to(provider, level, message)`: calls that provider's function, or answers the zero value when the name is not bound |
| `bound_providers()` | The names of the bound providers, one per line, in the order `bind:` lists them |
| `bind_config(provider, key)` | A provider's setting, through `rho_bind_config` |
| `bind_config_set(provider, key, value)` | Records a setting, through `rho_bind_config_set` |

So the interface's code decides the policy — every provider, as an alert does,
or the first one that answers:

```rho
fn raise(level: string, message: string) u32 {
    var failed: u32 = 0
    var index: u32 = 0
    while true {
        let name: string = str_field(bound_providers(), "\n", index)
        if name == "" {
            break
        }
        if bind_config(name, "enabled") != "false" {
            if send_to(name, level, message) != 0 {
                failed = failed + 1
            }
        }
        index = index + 1
    }
    return failed
}
```

## Settings at run time: `bind.conf`

A provider reads its settings with `rho_bind_config(provider, key)`. The answer is:

1. the value in the application's `bind.conf`, one `provider.key=value` per line,
   when the file has that key;
2. otherwise the value the application's `bind:` gave it;
3. otherwise `""`.

The file is in the workspace a supervisor gives the application
(`RHO_APP_WORKSPACE`), or in `~/.rho/apps/<application>/` for a program started
by hand. Because the file is read on each call, a running installation changes a
setting without a rebuild. `bind_config_set` writes it — replacing the line the
key had — which is how a settings page changes a provider's configuration in
place. The file is written readable by its owner only, and its directory likewise,
because a setting may be a secret.

The configuration is carried by the Go backends, which generate it into
`rho_bindings.go`.

## An interface nobody bound

A module the application depends on may import an interface the application does
not bind. That is allowed, because a library can be useful with or without the
capability: a store can send an alert when the application binds a channel, and
is still a store when it binds none. The build says so once:

```text
warning: my-app does not bind acme.alert, which a module it depends on imports; those calls answer nothing
```

Every `<function>_to` then answers its function's zero value — `""`, `0`, `0.0`
or `false` — and `bound_providers()` is empty, so the module can tell and do
without.

An unbound import in the application's **own** sources is refused instead, with
the binding to add: there it is a binding someone forgot.

```text
acme.alert is an interface; bind it to providers in module.yml (bind: acme.alert: providers: [...])
```

## Versions

An interface's functions are part of what its version promises: changing a
signature breaks every provider, so `rho module check` and `rho module publish`
treat it as a breaking change, and list the providers in the workspace that
implement it ({ref}`modules/modules`).
