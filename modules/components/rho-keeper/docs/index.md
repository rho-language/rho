# The installation dashboard

`rho keeper` is a local dashboard for a Rho installation: what is installed,
which version of it is in use, what the components need a plugin to be, what the
remote registry offers, and what this machine is missing. Beside everything it
shows is the command that changes it, and its own subcommands act on what
{ref}`rho doctor <rho-doctor/index>` reports.

It is a component like any other, installed and not enabled until you say so:

```sh
rho module enable rho-keeper
rho keeper
```

```
rho keeper: http://127.0.0.1:8477/
rho keeper: press ctrl-c to stop
```

## The commands

| Command | What it does |
|---|---|
| `rho keeper` / `rho keeper start` | Serve the dashboard until stopped |
| `rho keeper status` | What is installed and enabled, one line per module, without serving anything |
| `rho keeper doctor` | What this machine has against what the installed modules need: the same report as `rho doctor` |
| `rho keeper sync` | Install everything the report says is missing, after showing the plan and asking |
| `rho keeper fix` | The same, one requirement at a time, asking about each |
| `rho keeper install <module> <dependency>` | Install one requirement of one module |

| Flag | For | What it does |
|---|---|---|
| `-addr ADDRESS` | `start` | The address to listen on; a bare number is a port on loopback |
| `--dry-run` (`-n`) | `sync`, `fix`, `install` | Print what would be run, and run none of it |
| `--yes` (`-y`) | `sync`, `fix`, `install` | Answer every question in advance, for a script |
| `--module NAME` | `sync`, `fix` | Only that module's requirements; a trailing name does the same |
| `--path DIR` | `sync`, `fix`, `install` | A module in a folder rather than an installed one: an application in its checkout asking about itself |

```sh
rho keeper sync --dry-run              # the plan, and nothing done
rho keeper sync --module rho-dbg       # one module's requirements
rho keeper fix --path ./my-app         # an application that is not installed
rho keeper install rho-dbg arm-none-eabi-gdb --yes
```

## Where it listens

The address is the `addr` setting, which the component's manifest declares with
the default `127.0.0.1:8477` and which is kept in the `config/keeper.yml` that
travels with the module. `-addr` overrides it for a run and writes the new value
back to that file, so a port chosen once is the port later runs use.

Loopback is the default because a page that lists what a machine has and can
install is not something to put on a network by accident. An address beyond
loopback is a decision made on purpose, and it requires a token: the keeper
refuses to start without `RHO_KEEPER_TOKEN`, and every request must then carry it
in the `Authorization` header (or as `?token=` for a page, since a browser's
`EventSource` cannot set a header). A write that a browser says came from another
site is refused with `403` either way.

## It prefers to fetch

Where a requirement can be satisfied two ways — a package manager, or a prebuilt
archive fetched into `~/.rho/toolchains/` — this component fetches. A toolchain
under the Rho home needs no privileges, belongs to the person rather than the
machine, and comes away with the directory it was put in.

Root is asked for only to install a udev rule, and only for that one file: the
file is compared before it is copied, the source is named before the prompt, and
the copy is the one elevated command.

## It shows commands

The dashboard reads everything and changes little: which version of a module is
enabled, a module's declared settings, installing a module from the registry,
uninstalling one that is not in use. Each is the same function or the same `rho module` command a terminal runs,
and the page prints it, so anything done here can be repeated or undone in a
terminal. Installing a system requirement is shown as the `rho keeper` command
that does it, because a page is a good place to read a command and a poor place to
be the only way to run one.

The same state is served as JSON for a script, under `/api/v1/` (`state`,
`modules/{name}`, `registry`), with `/healthz` for liveness.

## Related

- {ref}`rho-doctor <rho-doctor/index>` — the report without the dashboard, which
  is what a script or a CI job reads.
- {ref}`modules/registries` — where installed modules come from.
