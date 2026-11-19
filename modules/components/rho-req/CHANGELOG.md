# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-11-19

Initial release. `rho req check` validates requirements files, ADRs, levels and
`@req`/`@verifies` tags across a workspace of repositories, and with `--base REF`
checks that every approval in a range of commits was made by a maintainer.
`rho req trace` maps each requirement to its children, ADRs, code and tests with
their latest results from `build/test-reports/`, and `--release` is the release
gate. `rho req render` writes the whole as Markdown. `list`, `show`, `new`, `edit`,
`set-status`, `adr new|list|supersede` and `init` read and change the files in
place, keeping their comments, blank lines and key order.

## [0.2.0] - 2026-11-19

`rho req trace` and `show` read `build/test-reports/results.json`, the record every `rho test` (rho-test 0.3.0) writes, before any other report, and it decides for every test it names: a `test fn` is its case, a script's `@verifies` the suite that runs it, a matrix every cell of it (`name!axis=value`). The other reports remain the fallback for a test it does not name, so `--verbose` redirects and the JUnit plugin are no longer needed for evidence. A directory that keeps `docs/architecture/adr/` is a repository of its own, like one that keeps `docs/requirements/`. The package gives what the documentation plugin rho-doc-req needs: `ADRBody` (a decision's text after its front matter) and `RunTime` (when the recorded run happened).

## [0.3.0] - 2026-11-19

`--tsv` on every command that answers with records (`check`, `list`, `show`, `trace`, `adr list`) and on the changes (`new`, `edit`, `set-status`, `adr new`, `adr supersede`, `adr set-status`): the same answers as `--json`, one tab-separated record per line, each starting with its kind (`repo`, `req`, `row`, `gap`, `issue`, `adr`, `created`, `updated`, `error`, ...), with tabs and line breaks in a value escaped. It is for a program that reads lines rather than JSON — the RoboHub keeper's Developer section reads it. A command that fails with `--tsv` also prints its reason as an `error` record. New: `rho req adr set-status ADR STATUS` moves a decision to proposed, accepted or rejected, changing only its status line; superseded remains `adr supersede`'s. Fixed: a repository named by a relative path (`--dir robohub`) was counted twice in its family, once as given and once as found among the workspace's directories.

## [0.3.1] - 2026-11-19

A suite whose `command` is written as a list of arguments (`[python3, tools/smoke.py]`) runs the script it names, so a `@verifies` inside that script counts for the suite, as it did for a command written as one string. The documentation says when a parent is `not run`: whenever one of its children has no evidence of its own.

## [0.4.0] - 2026-11-19

A module nested inside a repository — a directory below it whose module.yml declares a `requirements:` block, such as `robohub/providers/mesh` — is a member of the family of its own: `--workspace` finds it inside every repository of the family (not in `build/`, `vendor/`, `node_modules/`, `testdata/` or hidden directories), it keeps its own `docs/requirements/` and `docs/architecture/adr/` under its own prefix, and its `@req`/`@verifies` tags are counted for it once — the enclosing repository's scan stops at its directory. It is named by its `module.name` (`robohub-provider-mesh`), so `robohub-provider-mesh:ADR-0001` resolves; a repository of the top level keeps its directory's name. Where a module name is taken, `name:` in the nested module's requirements block names it apart, and two members of a family with one name are an error (`repo.duplicate`); a top-level repository is named by its directory whatever its block says, and a `name:` there that differs from it is a warning (`manifest.name`). A nested module finds its `parent` among the repositories that enclose it before the workspace's directories, so it needs no `ROBOHUB_WORKSPACE`. Nothing reached through `build/`, `vendor/`, `node_modules/`, `testdata/` or a hidden directory is nested, so a documentation tree assembled in a repository's `build/` stays a workspace of its own. The `repo` record in `--json` carries `nested`. Fixed: a test manifest's unquoted `- name:` holding an apostrophe ("the toolchain's schema") was read as no name, so the suite's `@verifies` had no result.
