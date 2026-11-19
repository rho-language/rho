======== robohub/requirements/index
# robohub requirements

Generated from `docs/requirements/` when this site was built. Edit the YAML (or use `rho req`), not this page.

Evidence is from the test results other repositories of the family had when this site was built; this one had none, so its own tests show as not run.

## System (SYS)

| ID | Title | Status | Verification | Evidence |
|---|---|---|---|---|
| {ref}`SYS-0001` | Survive a power cut | approved | test | not run |
| {ref}`SYS-0002` | Keep a week of logs | draft | inspection | none |

### SYS-0001: Survive a power cut {#SYS-0001}

After a power cut every node shall return to service without a person.

*Rationale.* Nobody is there at night.

- **Status:** approved
- **Verification:** test
- **Satisfied by:** {ref}`KEEP-0001`, `MESH-0001`
- **Decided by:** {ref}`ADR-0001 <robohub:ADR-0001>`
- **Evidence:** not run
- **Changed:** 2026-11-01
- **Source:** `docs/requirements/system.yml:5`

### SYS-0002: Keep a week of logs {#SYS-0002}

Logs shall be kept for 7 days | no less.

- **Status:** draft
- **Verification:** inspection
- **Evidence:** none
- **Changed:** 2026-11-02
- **Source:** `docs/requirements/system.yml:16`

======== robohub/requirements/trace
# Requirements trace

Every requirement of the 3 repositories of this family ({ref}`robohub <robohub/requirements/index>`, {ref}`robohub-keeper <keeper/requirements/index>`, robohub-mesh), traced to what satisfies it, decides it, implements it and verifies it. Generated when this site was built, as `rho req trace --workspace` sees it.

## Levels

| Level | Title | Requirements | Repositories |
|---|---|---|---|
| SYS | System | 2 | {ref}`robohub <robohub/requirements/index>` |
| NODE | Node | 3 | {ref}`robohub-keeper <keeper/requirements/index>`, robohub-mesh |

## System (SYS)

| ID | Title | Repository | Status | Satisfies |
|---|---|---|---|---|
| {ref}`SYS-0001` | Survive a power cut | {ref}`robohub <robohub/requirements/index>` | approved |  |
| {ref}`SYS-0002` | Keep a week of logs | {ref}`robohub <robohub/requirements/index>` | draft |  |

## Node (NODE)

| ID | Title | Repository | Status | Satisfies |
|---|---|---|---|---|
| {ref}`KEEP-0001` | Sign in survives a restart | {ref}`robohub-keeper <keeper/requirements/index>` | implemented | {ref}`SYS-0001` |
| {ref}`KEEP-0002` | Report the version | {ref}`robohub-keeper <keeper/requirements/index>` | proposed |  |
| `MESH-0001` | Rejoin after a power cut | robohub-mesh | approved | {ref}`SYS-0001` |

## Trace matrix

| ID | Status | Satisfied by | Decisions | Code | Tests | Evidence |
|---|---|---|---|---|---|---|
| {ref}`SYS-0001` | approved | {ref}`KEEP-0001`, `MESH-0001` | {ref}`robohub:ADR-0001 <robohub:ADR-0001>` |  |  | not run |
| {ref}`SYS-0002` | draft |  |  |  |  | none |
| {ref}`KEEP-0001` | implemented |  | {ref}`robohub-keeper:ADR-0001 <robohub-keeper:ADR-0001>`, {ref}`robohub:ADR-0001 <robohub:ADR-0001>`, {ref}`robohub-keeper:ADR-0002 <robohub-keeper:ADR-0002>` | `robohub-keeper/src/session.rho:3` | `session_survives_restart` (pass) | passing |
| {ref}`KEEP-0002` | proposed |  |  |  |  | none |
| `MESH-0001` | approved |  |  |  |  | none |

## Gaps

2 gap(s), 2 of which block a release.

- **not-run** {ref}`SYS-0001`: SYS-0001 is approved and some of its tests have no result (blocks a release)
- **untested** `MESH-0001`: MESH-0001 is approved and has no verifying test and no lower-level requirement (blocks a release)
======== robohub/adr/index
# robohub architecture decisions

Generated from `docs/architecture/adr/` when this site was built. A page of this module refers to a decision as `ADR-NNNN`, any other page as `robohub:ADR-NNNN`, both with the `{ref}` role.

| Decision | Title | Status | Date | Requirements |
|---|---|---|---|---|
| {ref}`ADR-0001 <robohub:ADR-0001>` | Nodes talk over a mesh bus | accepted | 2026-11-01 | {ref}`SYS-0001` |
======== robohub/adr/0001-mesh-bus
# ADR-0001: Nodes talk over a mesh bus {#robohub:ADR-0001}

- **Status:** accepted
- **Date:** 2026-11-01
- **Requirements:** {ref}`SYS-0001`
- **Source:** `docs/architecture/adr/0001-mesh-bus.md`

## Context

Boards are far apart.

## Decision

A mesh bus; see {ref}`SYS-0001`.
======== keeper/requirements/index
# keeper requirements

Generated from `docs/requirements/` when this site was built. Edit the YAML (or use `rho req`), not this page.

Evidence is from the test run of 2026-11-19T18:30:00Z (`build/test-reports/results.json`).

## Node (NODE)

| ID | Title | Status | Verification | Evidence |
|---|---|---|---|---|
| {ref}`KEEP-0001` | Sign in survives a restart | implemented | test | passing |
| {ref}`KEEP-0002` | Report the version | proposed | test | none |

### KEEP-0001: Sign in survives a restart {#KEEP-0001}

The keeper shall keep a signed-in session across a restart.

- **Status:** implemented
- **Verification:** test
- **Satisfies:** {ref}`SYS-0001`
- **Decided by:** {ref}`ADR-0001 <robohub-keeper:ADR-0001>`, {ref}`robohub:ADR-0001 <robohub:ADR-0001>`, {ref}`ADR-0002 <robohub-keeper:ADR-0002>`
- **Implemented in:** `src/session.rho:3`
- **Verified by:** `session_survives_restart` (`tests/session_test.rho:3`): pass
- **Evidence:** passing
- **Owner:** maintainer
- **Changed:** 2026-11-19
- **Tags:** auth
- **Source:** `docs/requirements/node.yml:5`

### KEEP-0002: Report the version {#KEEP-0002}

The keeper shall report its version.

- **Status:** proposed
- **Verification:** test
- **Evidence:** none
- **Changed:** 2026-11-19
- **Source:** `docs/requirements/node.yml:16`

======== keeper/adr/index
# keeper architecture decisions

Generated from `docs/architecture/adr/` when this site was built. A page of this module refers to a decision as `ADR-NNNN`, any other page as `robohub-keeper:ADR-NNNN`, both with the `{ref}` role.

| Decision | Title | Status | Date | Requirements |
|---|---|---|---|---|
| {ref}`ADR-0001 <robohub-keeper:ADR-0001>` | Sessions on disk | superseded | 2026-11-10 | {ref}`KEEP-0001` |
| {ref}`ADR-0002 <robohub-keeper:ADR-0002>` | Sessions in the store | accepted | 2026-11-19 | {ref}`KEEP-0001` |
======== keeper/adr/0001-sessions-on-disk
# ADR-0001: Sessions on disk {#robohub-keeper:ADR-0001}

- **Status:** superseded
- **Date:** 2026-11-10
- **Requirements:** {ref}`KEEP-0001`
- **Superseded by:** {ref}`ADR-0002 <robohub-keeper:ADR-0002>`
- **Source:** `docs/architecture/adr/0001-sessions-on-disk.md`

## Decision

Keep sessions in a file.
======== keeper/adr/0002-sessions-in-the-store
# ADR-0002: Sessions in the store {#robohub-keeper:ADR-0002}

- **Status:** accepted
- **Date:** 2026-11-19
- **Requirements:** {ref}`KEEP-0001`
- **Supersedes:** {ref}`ADR-0001 <robohub-keeper:ADR-0001>`
- **Source:** `docs/architecture/adr/0002-sessions-in-the-store.md`

## Decision

Keep sessions in the store; replaces {ref}`ADR-0001`.
