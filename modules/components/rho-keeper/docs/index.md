# The installation dashboard

`rho keeper` shows what is installed, which of it is being used, and what the
components you have need from a plugin.

```sh
rho keeper
rho keeper: http://127.0.0.1:8477/
rho keeper: press ctrl-c to stop
```

## The commands

| Command | What it does |
|---|---|
| `rho keeper` | Serve the dashboard and read until stopped |
| `rho keeper status` | Print what the dashboard shows, without serving it |
| `rho keeper doctor` | What the installed modules need from this machine, and what it has |
| `rho keeper sync` | Bring what the machine has back into line with what is declared |
| `rho keeper fix` | The same, for the one thing at a time a person is looking at |
| `rho keeper install` | Fetch a missing requirement |

## It prefers to fetch

Where a requirement can be satisfied two ways — a package manager or a download
into `~/.rho/toolchains/` — this component fetches. A toolchain under `~/.rho`
is one the installation owns and can remove; one a package manager owns is
shared with the whole machine and out of this toolchain's hands.

It asks for root only for the one thing that needs it, a udev rule, and only
for that one file.

## It reads, mostly

The dashboard shows, and beside everything it shows is the command that changes
it. A page that looks like it can change something is a page a person trusts
with a change they did not mean to make, so the acts a person asks for are
commands with names, and the page is where they read what to type.

## Related

- {ref}`rho-doctor <rho-doctor/index>` — the same report without the dashboard,
  which is what a script or a CI job reads.
- {ref}`handbook/registries` — where installed modules come from.
