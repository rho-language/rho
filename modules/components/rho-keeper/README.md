# rho-keeper

A local dashboard for a Rho installation: what is installed, which of it is
being used, and what the components you have installed need a plugin to be.

```
rho keeper
```

```
rho keeper: http://127.0.0.1:8477/
rho keeper: press ctrl-c to stop
```

It reads. That is the whole of the first version, and it is deliberate: a page
that looks like it can change something is a page a person trusts with a change
they did not mean to make. So the dashboard shows, and next to every thing it
shows is the command that changes it. Nothing here takes a credential or runs
anything a request asked for.

## What it shows

- **Every module** — component, plugin and library — with its version, what its
  own manifest says about it, and whether it is the version a run would use.
- **Every version installed**, not only the one in use. A machine may hold
  several, and showing one would imply there was only one.
- **What each component needs a plugin to be**, and which plugins fill it. That
  is the question a hook list cannot answer: two plugins may answer the same
  hook and be nothing alike, so a plugin says what it *is* and this shows that.
- **What the remote registry offers**, and which of it is not here yet. The
  registry section lists every module the configured registry holds, marks the
  ones this machine already has, and installs one with the same
  `rho module install` a terminal would run.
- **What can be removed.** A module that is not in use offers an **Uninstall**
  that removes this machine's copy — the same `rho module uninstall` a terminal
  would run. It is refused while the module is in use, so the row offers Disable
  first; once it is gone the registry offers it again, and installing brings it
  back.
- **What could not be read.** A manifest that will not parse, a record that will
  not load, a module whose contract nobody can name — each is reported with its
  reason rather than left off the page, because a list that quietly leaves things
  out is a list nobody can trust to be complete.

## What it is

A component, so `rho keeper` dispatches to it the way `rho doc` dispatches to
the generator. It is installed but not enabled, like every other module: having
it on the machine is not a decision to run it.

```sh
rho module enable rho-keeper
rho keeper
```

## The port

Loopback, on `8477`, because a dashboard that lists what a machine has and what
it can install is not something to expose to a network by default. Asking for a
different address is a decision a person makes on purpose:

```sh
rho keeper -addr 127.0.0.1:9000
```

It says so on standard error when it is reachable beyond the machine it is on.
An address beyond loopback **requires a token**: `rho keeper` refuses to start
without `RHO_KEEPER_TOKEN`, and then every request must present it — in the
`Authorization` header, or as `?token=` for a page (a browser's `EventSource`
cannot set a header). Open the page as
`http://<address>/#/…?token=<token>`, or keep the address on loopback and need
no token at all.

A write from another page is refused whether or not a token is set: the
browser's `Origin` and `Sec-Fetch-Site` labels are read, and a request that says
it came from elsewhere, and that would change something, is answered `403`.
Command-line callers send neither label and are unaffected.

## The remote registry

The registry section answers the other half of "what have I got": what the
registry this machine is pointed at holds, and which of it is not here. The
address comes from `~/.rho/config/registry.yaml`, the same file `rho module
search` and `rho module install` read:

```sh
rho config set remote.url https://registry.rho-lang.org
```

Listing reads the registry's public search, so no credential is needed to see
what it offers. Installing runs the same command a terminal would, and the page
shows it:

```
rho module install rho-monitor@0.1.0
```

A module that is already held is marked installed, with the version held; one
that is not carries an **Install** button and the exact command behind it. A
registry the machine is not pointed at, or one that cannot be reached, is said
in a sentence rather than shown as an empty list.

## The commands

```
rho keeper serve     serve the dashboard on a port (default 127.0.0.1:8477)
rho keeper status    what is installed and enabled, on one line per module
```

`status` is the same reading without the page, so a person in a terminal and a
person with a browser are told the same thing — which is the reason the reading
is a library and neither command reimplements it.

## The API

The same state, as data, so a script is not left parsing a page:

```
GET  /                        the page
GET  /api/v1/state            the whole reading: modules, contracts, toolchain, errors
GET  /api/v1/modules/{name}   one module in full
GET  /api/v1/registry         what the remote registry offers, and what is held
POST /api/v1/registry/install install one module from the remote registry
POST /api/v1/modules/{name}/uninstall  remove this machine's copy, once it is off
GET  /healthz                 liveness
```

## What it is not

It is not a second implementation of anything. Enabling, disabling, configuring,
installing a module and installing a system requirement each run the same
function or the same command the CLI does, and the page prints the command it
ran — so a change made here is one a person can repeat, or undo, in a terminal.
It does not start a background process: it runs while you run it, and stopping it
is stopping the terminal.

## The system half

`rho doctor` says what this machine has against what the installed modules need, and
these commands act on that report. The report is *read* from the doctor rather than
worked out here, so a terminal and this dashboard cannot tell one person two things
about one machine.

```sh
rho keeper doctor              # the report, in the doctor's words
rho keeper sync --dry-run      # what would be done, and none of it
rho keeper sync                # do it, after showing it and asking
rho keeper fix                 # one thing at a time, asking about each
rho keeper install rho-dbg arm-none-eabi-gdb
```

Three things about how it acts, and they are the whole design:

**A fetch comes first.** A prebuilt archive, fetched into `~/.rho/toolchains/` and run
from there, needs no privileges at all — and it is preferred for reasons that have
nothing to do with sudo: it belongs to the person rather than to the machine, it does
not change when the machine's package manager does, and it comes away with the
directory it was put in.

**Root is asked for twice, for two files.** A udev rule is a file in a system
directory and nothing else can put it there. So: the file is compared before it is
copied, the source is named before the prompt appears, the copy is the one elevated
command, and the reload only happens if the copy worked. Both the words and the
commands are printed first.

**The page shows commands; it does not run them.** Every fix appears as the command a
person would type, in a form they can copy. A dashboard is a good place to *read* a
command and a poor place to be the only way to run one, because installing a package
asks for privileges.