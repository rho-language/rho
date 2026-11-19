# rho-req

Requirements, architecture decisions, and the trace from them to the code that
implements them and the tests that verify them. Reached as `rho req`.

Every feature follows one path:

1. one or more **requirements**, in `docs/requirements/`;
2. an **ADR** when it changes the architecture, in `docs/architecture/adr/`;
3. the **implementation**, with `@req ID` above what implements it;
4. the **tests**, with `@verifies ID` above what verifies it.

`rho req check` says whether the four agree, `rho req trace` says how far each
requirement has got, and the editing commands change the files without
disturbing anything else in them.

```sh
rho req check                     # this repository
rho req check --workspace         # every repository of the family
rho req check --base origin/main  # ... and only maintainers approved
rho req trace --release           # the release gate
rho req show GW-0012              # one requirement, up and down
rho req new --level NODE --title "..." --statement "..." --satisfies SYS-0007
rho req set-status GW-0012 approved
```

## The files a repository carries

```
docs/requirements/**/*.yml          requirements, any number of files, in folders if wanted
docs/requirements/levels.yml        the levels; only in the root of a workspace or a standalone repository
docs/architecture/adr/NNNN-slug.md  architecture decision records
MAINTAINERS.yml                     who may approve; in the root or a standalone repository
module.yml                          requirements: { prefix: GW, parent: robohub }
```

Only those two directories are read.
repository of its own). Ordinary documentation pages live
elsewhere (in RoboHub, `docs/doc/`) and are never read as requirements or ADRs,
whatever they contain. `docs/architecture/` holds architecture artefacts such as
ADRs, not prose.

### module.yml

```yaml
requirements:
  prefix: GW        # the prefix of the ids this repository allocates
  parent: robohub   # the repository that holds levels.yml and the system level
```

Both keys are optional; any other key in the block is an error, except `name`
in a nested module (below). A repository with no `parent` is a root (if others
name it as theirs) or standalone.

A module **nested** inside a repository — a directory below it whose
module.yml declares a `requirements:` block, such as `robohub/providers/mesh` —
is a member of the family of its own: it keeps its own `docs/requirements/` and
`docs/architecture/adr/`, allocates ids under its own prefix, and its tags are
its own. It is named by its manifest's `module.name` (`robohub-provider-mesh`),
because its directory's name (`mesh`) says nothing outside the repository; where
that name is taken by another member of the family, the block names it apart:

```yaml
requirements:
  name: robohub-gateway-contract   # a nested module only; the default is module.name
  prefix: GWC
  parent: robohub
```

A repository of the workspace's top level is named by its directory, which is
what a `parent:` finds, whatever its block says; a `name:` there that is not the
directory's name is warned about (`manifest.name`). A nested module copied out to
the top level under its name — as a documentation tree does — is not.

### Levels

Levels are N and ordered, highest first. A level can be a whole subsystem.

```yaml
levels:
  - { name: SYS,  title: System }
  - { name: NODE, title: Node }
  - { name: SUB,  title: Subsystem }
  - { name: UNIT, title: Unit }
```

A repository takes its levels from its own `docs/requirements/levels.yml`, else
from its parent's, up the chain; with none anywhere, the four above are the
default. A level name is one upper-case word.

### A requirements file

```yaml
level: NODE
prefix: GW            # sub-prefixes are allowed for a subsystem: GW-AUTH
title: Gateway
requirements:
  - id: GW-0012
    title: Sessions survive a gateway restart
    statement: After a restart the gateway shall re-establish a session with every enrolled board within 30 s, without re-enrollment.
    rationale: A power cut must not need a person.
    satisfies: [SYS-0007]
    status: approved
    verification: test
    decided_by: [ADR-0009]
    owner: maintainer
    changed: 2026-11-19
    tags: [security]
```

| Key | Required | Meaning |
|---|---|---|
| `id` | yes | `<PREFIX>-NNNN`, four digits, the file's prefix; unique across the workspace |
| `title` | yes | a short name |
| `statement` | yes | what shall hold; plain, quoted, or a `>` / `\|` block |
| `rationale` | no | why |
| `satisfies` | no | requirements of a **higher** level this one serves; may cross repositories |
| `status` | yes | see below |
| `verification` | yes | `test`, `analysis`, `inspection` or `demonstration` |
| `decided_by` | no | ADRs: `ADR-0009` (this repository's) or `robohub:ADR-0003` |
| `owner` | no | who answers for it |
| `changed` | no (warned) | `YYYY-MM-DD`; every `rho req` edit sets it to today |
| `tags` | no | free words |

File keys are `level`, `prefix`, `title` (optional) and `requirements`. An
unknown key anywhere is an error. A file at the top level uses the top level's
name as its prefix (`SYS`); any other file uses the repository's prefix or a
sub-prefix of it (`GW`, `GW-AUTH`).

Comments, blank lines, quoting and key order are yours: the tool's edits keep
them.

### Statuses

```
draft -> proposed -> approved -> implemented -> verified
```

plus `rejected` and `retired`. `rho req set-status` allows: draft -> proposed;
proposed -> draft or approved; approved -> proposed or implemented;
implemented -> proposed, approved or verified; verified -> proposed or
implemented; rejected -> draft; any status -> rejected or retired. `--force`
allows anything else.

`approved`, `implemented` and `verified` are the **committed** statuses: only a
maintainer moves a requirement into one (see Governance). Changing the
`statement` of a committed requirement with `rho req edit` sends it back to
`proposed`, because what was approved is no longer what is written;
`--keep-status` keeps it, for a typo.

### An ADR

```markdown
---
id: ADR-0009
title: Reset sessions after a gateway restart
status: accepted          # proposed | accepted | superseded | rejected
date: 2026-11-19
requirements: [GW-0012, SYS-0007]
supersedes: []
superseded_by: []
---

## Context
## Decision
## Consequences
```

The file is `NNNN-slug.md` and its number is its id. `0000-*.md` is the
template and is never read as a decision. ADR ids are per repository: `ADR-0009`
means this repository's, and another repository's is written
`<repository>:ADR-0009`, where the repository is the directory name — for a
nested module, its name (`robohub-provider-mesh:ADR-0001`). A
requirement and an ADR are linked when either side names the other
(`decided_by` or `requirements`). A superseded ADR names its successor in
`superseded_by`.

### MAINTAINERS.yml

```yaml
maintainers:
  - name: Ada Maintainer
    github: ada
    email: ada@example.org
```

### Tags in code and tests

```rho
// @req GW-0012
fn restore_sessions() void { ... }

// @verifies GW-0012
test fn sessions_survive_restart() void { ... }
```

```python
# @verifies GW-0012, GW-0013
```

A tag is a comment line (`//`, `#`, `/*`, `*`) that begins with `@req` or
`@verifies`, followed by ids separated by commas or spaces; reading stops at the
first word that is not an id, so a reason may follow. Files read: `.rho`, `.c`,
`.h`, `.cpp`, `.cc`, `.hpp`, `.go`, `.py`, `.sh`, and YAML test manifests
(`tests.yml`, or YAML under `tests/`). `build/`, `vendor/`, `node_modules/`,
`testdata/` and hidden directories are skipped, and so is a nested module's
directory: its tags are counted for it, once, not for the repository around it.
A directory below a repository whose module.yml has no `requirements:` block
(a test project, a module that keeps no requirements) is part of the repository
around it.

What a tag sits above is the next line that is not blank or a comment, and its
symbol is read from it: a Rho `fn` / `test fn`, a Go `func`, a C function or a
GoogleTest `TEST(Suite, Name)`, a Python `def`, a shell function, a manifest's
`- name:`. That symbol is the test's name for its result. A `@verifies` in a
Python or shell script verifies through the suite in `tests/tests.yml` (or
`tests.yml`) whose `command` runs that script, and the suite's name is the
test's name.

## How a workspace resolves

A repository is the nearest directory, at or above where the command runs, that
has `docs/requirements/`, `docs/architecture/adr/`, a `.git`, or a module.yml with a `requirements:`
block. Its name is its directory name; a nested module's is its module name (see
module.yml above). A directory is nested when a directory above it keeps
`docs/requirements/` or `docs/architecture/adr/` or declares a `requirements:`
block; the directories `$ROBOHUB_WORKSPACE` and `$RHO_WORKSPACE` name, and what
is above them, never enclose anything, and neither does a directory reached
through `build/`, `vendor/`, `node_modules/`, `testdata/` or a hidden directory —
a documentation tree assembled in `build/` is a workspace of its own, whose
repositories are named by their directories.

A `parent` is first looked for among the repositories that enclose the module,
by name, so `robohub/providers/mesh` with `parent: robohub` needs no variable.
Otherwise it is found as a sibling of the outermost repository:
`$ROBOHUB_WORKSPACE/<parent>`, then `$RHO_WORKSPACE/<parent>`, then
`../<parent>`. The repositories that reach the same root through their parents
are a **family**, found among the subdirectories of those same workspace
directories and, inside each of them, among the nested modules (`build/`,
`vendor/`, `node_modules/`, `testdata/` and hidden directories are not looked
in), and the family is the workspace: ids are unique in it, `satisfies` and
qualified ADR references resolve in it, and `new` numbers across it. Names are
unique in it too: two members with one name are an error (`repo.duplicate`). Without `--workspace` a command
reports on the repository it runs in; references still resolve against the
whole family. A standalone repository is a family of one.

## The commands

All commands take `--dir DIR` (or a directory as the first argument where shown)
and read flags before or after it. Exit status: 0 clean, 1 problems or a failed
change, 2 a wrong command line.

### rho req check [dir] [--workspace] [--base REF] [--strict] [--json]

Errors: YAML that does not parse (with its line), unknown and missing keys,
values outside their set, dates not `YYYY-MM-DD`, id format and prefix, a file
prefix that is not the repository's, an unknown level, duplicate ids in the
family, `satisfies` that resolves to nothing, to itself, or to a level that is
not higher, `decided_by` to an ADR that does not exist, ADR front matter, an ADR
numbered differently from its file, ADR `requirements`, `supersedes` and
`superseded_by` that resolve to nothing, a superseded ADR with no successor,
`@req`/`@verifies` tags naming nothing, cycles, an unresolved `parent`.

Warnings: a committed `test`-verified requirement with no `@verifies` and no
open child, an ADR about nothing, a requirement with no `changed`, a tag or a
`satisfies` naming a rejected or retired requirement, a script no suite runs.
Warnings exit 0 unless `--strict`.

`--base REF` is described under Governance.

### rho req list [dir] [--level L] [--status S] [--repo R] [--tag T] [--workspace] [--json]

### rho req show ID [--dir DIR] [--json]

The requirement, its upward trace (what it satisfies, transitively), its
downward trace (what satisfies it, as a tree), its ADRs, its `@req` code and its
`@verifies` tests with their latest results.

### rho req trace [dir] [--workspace] [--json] [--release]

For each requirement: its children, ADRs, code, tests and their results, and its
**evidence**: `failing` when a verifying test or a child's evidence failed;
`not run` when nothing failed but some of its evidence is missing — one of its
tests has no result, or one of its children has no evidence of its own, so a
parent is `passing` only when every child is; `passing` when there is evidence
and all of it passed; `none` when it has no verifying test and no open child.

Test results come from each repository's `build/test-reports/`. First from
`results.json`, the record every `rho test` run writes (rho-test 0.3.0 and
later): it names every suite and case with its status, and it decides for every
test it names. A `test fn` is found as its case by name (preferring the case
whose file is the tag's file); a script's `@verifies` is the suite that runs it,
whose one case carries the suite's name; a suite expanded from a matrix is every
cell of it (`name!axis=value`), failing when any cell failed.

A test the record does not name is looked up in the other files there,
recognised by content: JUnit XML (what the `rho-test-junit` plugin writes as
`report-N.txt`), `rho test`'s own `[PASS]`/`[FAIL]`/`[SKIP]` lines (as saved by
`rho test --verbose > build/test-reports/rho-test.txt`), TAP, and `go test -v`.
When two of those name a case, the newer file wins.

Gaps: `untested` (a committed `test` requirement with no evidence),
`unverified` (a committed `analysis`/`inspection`/`demonstration` requirement
not yet marked verified), `failing`, `not-run`, `verified-unproven` (status
verified, evidence not passing), `dangling-tag`, and `orphan-adr`. All but
`orphan-adr` block a release: `--release` exits 1 when any of them is found —
the release gate. A requirement verified by analysis, inspection or
demonstration counts as verified when a maintainer sets its status to
`verified`.

### rho req render [dir] [--workspace] [--out FILE|DIR|-] [--level L]

Markdown: a section per level with a table (id, title, status, verification,
satisfies) and every requirement's full text, an ADR index, and the trace
matrix with its gaps. It is generated, not committed: the default is
`build/requirements/requirements.md` in the repository, `--out -` writes to
standard output. `--level` limits the level sections.

### On a documentation site

`rho doc` shows the same, generated when the site is built, through the plugin
{ref}`rho-doc-req <rho-doc-req/index>`: per repository a requirements page
(`<module>/requirements/index`, a label per requirement, so `{ref}`GW-0012``
reaches it), an index of decisions (`<module>/adr/index`) and a page per decision
(`<module>/adr/NNNN-slug`, labelled `<repository>:ADR-NNNN`), and on the root of a
family the trace page (`<module>/requirements/trace`). The plugin is built on this
component's package, so the site and `rho req check` read the files the same way.

### rho req new

```sh
rho req new --level NODE --title "..." --statement "..." \
  [--prefix GW] [--file gateway/sessions.yml] [--satisfies SYS-0007] \
  [--rationale ...] [--decided-by ADR-0009] [--owner ...] [--tags a,b] \
  [--status draft] [--verification test]
```

Allocates the next free id for the prefix across the workspace, appends the
requirement to the file of that level and prefix (the first such file, or
`--file` relative to `docs/requirements/`; a new `<level>-<prefix>.yml` with a
header when there is none), in the style of the items already there, and prints
the id. The prefix defaults to the top level's name at the top level and to the
repository's prefix below it.

### rho req edit ID

```sh
rho req edit GW-0012 --set title="..." --add satisfies=SYS-0003 --remove tags=old --remove owner
```

`--set key=value` sets a scalar, or a whole list for `satisfies`, `decided_by`
and `tags`; `--add` and `--remove key=value` change one list entry;
`--remove key` removes an optional key. Ids do not change. `changed:` becomes
today. Everything else in the file stays byte for byte: a scalar is replaced on
its line and keeps its trailing comment, a block scalar keeps its `>` or `|` and
indent, a flow list is rewritten in place, a block list gains or loses one line,
and a missing key is inserted after the key before it in the order above. A
refused edit writes nothing.

### rho req set-status ID STATUS [--force]

### rho req adr new --title T [--requirements A,B] [--status proposed]

Writes `docs/architecture/adr/NNNN-slug.md` with the next number, the front
matter filled in, and the body of `0000-template.md` (or a built-in Context /
Decision / Consequences template).

### rho req adr list [--workspace] [--json|--tsv]

### rho req adr supersede OLD NEW

OLD becomes `superseded` with NEW in `superseded_by`; NEW gets OLD in
`supersedes`. Either may be another repository's, written `<repo>:ADR-NNNN`.

### rho req adr set-status ADR STATUS [--dir DIR]

Moves a decision to `proposed`, `accepted` or `rejected`; only its `status:`
line changes. `superseded` is refused here, because `adr supersede` records the
successor in both front matters, and a superseded decision stays superseded.
Accepting a decision is an approval: `check --base` holds it to the same rule as
a requirement becoming approved.

### rho req init [dir] [--root] [--level L --prefix P] [--parent REPO]

Creates `docs/requirements/` and `docs/architecture/adr/0000-template.md`; with
`--root`, also `docs/requirements/levels.yml` and `MAINTAINERS.yml`; with
`--level` and `--prefix`, the first requirements file; with `--prefix` or
`--parent`, appends a `requirements:` block to an existing module.yml. Nothing
that exists is overwritten, and nothing outside those paths is touched.

## Governance

`rho req check --base REF` reads every commit in `REF..HEAD` (first parents)
that touched a requirements file or an ADR, and finds each requirement whose
status became `approved`, `implemented` or `verified` and each ADR that became
`accepted`. The author of that commit must be in the `MAINTAINERS.yml` that
governs the repository — its own, else its parent's — matched by email, by
name, or by the GitHub login (including a `NNN+login@users.noreply.github.com`
address). Otherwise: `only a maintainer approves`. When the maintainers file is
in the same git repository it is read as it was at `REF`, so a change cannot
approve itself by adding its author.

**This is a guard, not an authority.** A commit's author is whatever the
committer configured, and anybody can write a maintainer's name into a commit.
What makes the rule hold is the forge: branch protection that requires a
review, and a CODEOWNERS file that makes the maintainers the required
reviewers of `docs/requirements/` and `docs/architecture/`. The check catches
the honest mistake and makes the rule visible in CI; it does not replace them.

## JSON

`check --json` prints `{ok, repos, errors, warnings, issues: [{severity, code,
repo, file, line, id, message}]}`; `trace --json` prints `{requirements: [...],
gaps: [...], blocking, release_ready}`; `list`, `show` and `adr list` print the
records. The codes are stable and meant for a dashboard to read.

## TSV

`--tsv` gives the same answers as tab-separated records, for a program that
reads lines rather than JSON (the RoboHub keeper's Developer section). Every
line starts with the record's kind, so anything else in the same stream — a log
line, an error printed to standard error — is told apart by its first field. In
a field a backslash, a tab, a line feed and a carriage return are written `\\`,
`\t`, `\n` and `\r`; a list is its items joined by commas; a yes/no is `yes`
or `no`.

| Record | Fields | From |
|---|---|---|
| `repo` | name, dir, prefix, parent, maintainers file, levels | `list`, `trace`, `check`, `adr list` |
| `req` | id, repo, level, status, verification, title, satisfies, tags, file, line, owner, changed | `list`, `show` |
| `statement`, `rationale`, `decided_by`, `evidence` | the value | `show` |
| `up`, `down` | depth, id, title, level, status | `show` |
| `adr` | repo, id, status, date, title, requirements, supersedes, superseded_by, file | `show`, `adr list` |
| `code` | repo, file, line, symbol | `show` |
| `test` | requirement, result, repo, file, line, test, kind, report | `show`, `trace` |
| `row` | id, repo, level, status, verification, evidence, title, satisfies, children, adrs, code count, test count | `trace` |
| `gap` | code, blocking, id, repo, file, line, message | `trace` |
| `issue` | severity, code, repo, file, line, id, message | `check` |
| `summary` | blocking, release ready (`trace`); ok, errors, warnings, requirements (`check`) | `trace`, `check` |
| `created` | id, repo, file | `new`, `adr new` |
| `updated` | id, file, status, reopened | `edit`, `set-status`, `adr set-status` |
| `superseded` | old, new | `adr supersede` |
| `error` | message | any command that failed |
