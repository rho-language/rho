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
GET /            the page
GET /api/state   the whole reading: modules, contracts, toolchain, errors
GET /api/modules just the modules
GET /healthz     liveness
```

## What it is not

It does not install, enable, disable, or configure anything. Those are the
commands, and they are printed next to the thing they change. It does not start
a background process: it runs while you run it, and stopping it is stopping the
terminal.
