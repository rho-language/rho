# rho-req

Requirements, architecture decisions, and the trace from them to code and tests.

Every feature goes requirement -> ADR (when it changes the architecture) ->
implementation tagged `@req ID` -> tests tagged `@verifies ID`. `rho req` checks
that the chain holds, across a workspace of repositories, and makes the edits
without disturbing the rest of the files.

```sh
rho req init --root                    # scaffold the layout in a root repository
rho req new --level SYS --title "Survive a power cut" --statement "..."
rho req check --workspace              # schema, ids, links, levels, tags, cycles
rho req check --base origin/main       # ... and only maintainers approved
rho req show SYS-0001                  # up, down, ADRs, code, tests
rho req trace --release                # the release gate
rho req render                         # build/requirements/requirements.md
rho req edit GW-0012 --add satisfies=SYS-0003
rho req set-status GW-0012 approved
rho req adr new --title "Reset sessions after a restart" --requirements GW-0012
rho req adr set-status ADR-0009 accepted
rho req list --workspace --tsv         # records for a program: --json or --tsv
```

A repository carries `docs/requirements/**/*.yml`, `docs/architecture/adr/NNNN-slug.md`,
and a `requirements: { prefix: GW, parent: robohub }` block in its module.yml; the
root of a workspace also carries `docs/requirements/levels.yml` and
`MAINTAINERS.yml`. Ordinary documentation pages (`docs/doc/`) are never read.

`rho req trace` takes each test's latest result from `build/test-reports/results.json`,
the record every `rho test` writes, and falls back to the other reports there
(JUnit, `[PASS]`/`[FAIL]` text, TAP, `go test -v`). On a documentation site the
same requirements, decisions and trace are pages, written at build time by the
`rho-doc-req` plugin, which is built on this component's package.

The maintainer check is a guard, not an authority: commit authors can be faked,
so branch protection and CODEOWNERS are what enforce who approves.

The full description of the files, the levels, the statuses, the tags and every
command is in [docs/index.md](docs/index.md).

```sh
# Build, and try it without installing
cd modules/components/rho-req && go build -o rho-req ./src && go test ./...
```
