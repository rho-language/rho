# The two registries

Rho has two registries, and most of the confusion about `rho module` comes from
not knowing which one a command acts on. They are different things with
different authority, and this page is the difference.

| | The local registry | A remote registry |
|---|---|---|
| Where it is | `~/.rho` | A server, named in `~/.rho/config/registry.yaml` |
| Who can change it | you, on this machine | only the module's owner, on that server |
| What it holds | the modules this machine installed | every module anybody published |
| Created by | `rho install` | `rho auth login` points at it |
| Used by the CLI | **only this one** | asked about, never resolved from |

The rule that makes it simple: **the toolchain only ever uses the local
registry.** A component is dispatched to because its directory is under
`~/.rho/modules/installed`, a Rho import resolves because the module is there,
and nothing consults a server to decide what a program may do. A remote
registry is somewhere to *fetch from*; what arrives is installed locally before
it can be used.

(modules:local)=
## The local registry

`rho install` creates it. It is the installation:

```
~/.rho/
  bin/                     rho, rho-deps
  modules/installed/       the modules this machine has, by kind
    component/             rho-fmt@0.1.0, rho-log@0.1.0
    plugin/                rho-doc-toc@0.1.0
    module/                a library
    library/               an interface module
  registry/local/          registry.yaml: which are selected, and where they came from
  auth/                    the credential `rho auth login` stored
  cache/                   fetched packages, before they are installed
  config/                  where a remote registry is
```

Two of those hold the same fact, and the difference matters:

- **`modules/installed/` is the truth about what is available.** A module is
  installed because its directory is there.
- **`registry/local/registry.yaml` is what is *known* about it**: which version a
  project resolves against, and whether the module was published from this
  machine or fetched.

The index is reconciled against the tree whenever it is read, so it heals rather
than drifting: a module removed by hand disappears from both, and a module
installed by something other than `rho module` appears in the listing. A second
source of truth that can disagree with the first is how a toolchain ends up
dispatching to something that is not there.

If the local registry does not exist, every command but `help` says so and
refuses, because a command that reads a registry which is not there would fail
somewhere deeper with a message about a file.

(modules:remote)=
## A remote registry

A remote registry is a service: the `applications/registry` application in this
tree is one. You point the CLI at it and log in:

```sh
rho auth login https://registry.example.com
```

The credential is stored in `~/.rho/auth/`, and every operation that needs to
know who you are presents it. That is what makes the owner of a module
knowable: **the registry records the identity in the token, not the name in the
manifest.**

(modules:ownership)=
## Who owns a module

A module's manifest has to name an author, in the author's own words:

```yaml
module:
  name: rho-router
  author: Aurel Miron
  maintainer: Aurel Miron
  visibility: public
```

That name is what a reader sees and what a maintainer puts in a release note. It
is **not** what the registry checks, because a name in a file is not a
credential. What it records, the first time a name is published, is the identity
in the credential:

```json
{"name": "rho-router", "owner": "miraur", "maintainers": ["miraur"],
 "visibility": "public", "firstPublished": "2026-09-28T10:07:00Z"}
```

The owner is recorded against the **name**, not a version, which is what stops a
re-publish from taking a module over: whoever publishes 0.2.0 cannot become the
owner by publishing it.

- Only the owner (and a maintainer, which is the same identity for now) may
  publish another version.
- Only the owner may withdraw a version or change its visibility. Withdrawing is
  the destructive one — a module that is gone cannot be resolved against, and
  every project depending on it stops building — so it is kept with one identity
  rather than with everyone who can add to the module.

A publish needs a credential on any registry, secured or not. An open registry
serves its modules to anyone; it does not *accept* a module from anyone, because
a module nobody can add a version to or withdraw is stuck the moment it is
published.

(modules:visibility)=
## Public and private

A module is public — anybody may install it — unless its owner says otherwise.
A private module is its owner's alone:

- a stranger cannot install it, and gets the same "not found" a module that does
  not exist gets, so a private name cannot be mapped by watching which names
  answer differently;
- it is not in a search for anybody but its owner;
- its owner can still see it, and install it, and make it public again.

```sh
rho module mine                     # what you published, and its state
rho module visibility orders         # what it is now
rho module visibility orders private # make it yours alone
rho module visibility orders public  # offer it to everybody
```

`mine` exists because a private module is deliberately absent from what
everybody else can see, so it is the only way its owner finds it.

(modules:commands)=
## The commands

Split by which registry they act on, because they are different operations with
different guards:

**The local registry** — about this machine, and needing no authority over
anybody:

| Command | What it does |
|---------|--------------|
| `rho module install <name>` | fetch from a remote registry and install here |
| `rho module uninstall <name>` | remove from here |
| `rho module activate <name>@<version>` | choose the version a project resolves against |
| `rho module deactivate <name>@<version>` | step back from that choice |
| `rho module list` | what is installed here |
| `rho module search` | what is installed here, and what a registry has |

**A remote registry** — about other people, and needing to be its owner:

| Command | What it does |
|---------|--------------|
| `rho module publish <dir>` | pack a module, install it here, and upload it |
| `rho module unpublish <name>@<version>` | withdraw a version |
| `rho module visibility <name> <public\|private>` | change who may install it |
| `rho module mine` | the modules you published |

`publish` does both, because a module published from a machine is on that
machine. `unpublish` does not remove the local copy: withdrawing a module from a
registry is a different act from removing it here, and a user who asked for the
first did not ask to lose the second. It says so when the version is still
installed.

(modules:uninstall-rule)=
## Why uninstall asks you to deactivate first

The selected version is the one a project is compiled and linked against.
Removing it would move the next build to a different version without anyone
asking, so it is refused until it is deactivated:

```sh
$ rho module uninstall rho-log@0.1.0
rho module uninstall: rho-log@0.1.0 is the selected version; deactivate it
first (rho module deactivate rho-log@0.1.0)
```

The one exception is the last version installed: with nothing else there is
nothing for a build to fall back to, so removing it is the point rather than a
change a build would notice.

Deactivating one version of several moves the selection to the version that is
left, so a project never resolves against nothing.

(modules:workflow)=
## The whole thing, end to end

```sh
# You have cloned the repository and built rho. Install the toolchain, which
# creates the local registry.
rho install

# Build and install the components this project needs.
(cd modules/components/rho-fmt && go build -o rho-fmt ./src)
rho module publish modules/components/rho-fmt

# Point at a registry and log in, so publishes are attributed to you.
rho config set remote.url https://registry.example.com
rho auth login https://registry.example.com

# Publish something. The manifest names the author; the token says who you are.
rho module publish modules/components/rho-router

# On another machine, or after clearing your local registry:
rho module install rho-router        # fetches and installs
rho module list rho-router           # and it is usable
```

## Project dependencies

A project resolves its own dependencies separately, and keeps them in
`rho.lock`:

```sh
rho module install      # resolve and install what module.yml declares
rho module update       # move them to what a registry now offers
rho module remove <name> # drop one from the project
```
