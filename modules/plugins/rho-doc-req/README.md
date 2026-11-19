# rho-doc-req

A plugin for `rho doc` that puts a module's requirements, architecture decisions
and their trace on its documentation site, generated at build time from the files
`rho req` reads — nothing generated is committed.

For every module of a site that keeps `docs/requirements/` or
`docs/architecture/adr/`:

| Page | What it shows |
|---|---|
| `<module>/requirements/index` | Every requirement, by level: text, status, verification, what it satisfies and what satisfies it (linked across the site), decisions, code, tests and, when `build/test-reports/` has results, evidence |
| `<module>/requirements/trace` | Only for the root of a family (it keeps `levels.yml`): the levels, every requirement of the family, the trace matrix and its gaps |
| `<module>/adr/index` | The decisions, by number, with status, date and requirements |
| `<module>/adr/NNNN-slug` | One decision: its front matter as a header, then its text |

Each requirement is a label, so `{ref}`GW-0012`` reaches it from any page; a
decision is the label `<repository>:ADR-NNNN` (`{ref}`robohub-gateway:ADR-0009``),
which a page of its own module may write as `{ref}`ADR-0009``.

It answers rho-doc's `sources` hook, which runs before the site is built, so the
pages are read like the module's own: cross-referenced, checked and searched. The
model is rho-req's Go package, built in: a site needs this plugin, not rho-req.

```sh
cd modules/plugins/rho-doc-req && go build -o rho-doc-req ./src && go test ./...
./bin/rho module publish modules/plugins/rho-doc-req
```

The full description is in [docs/index.md](docs/index.md).
