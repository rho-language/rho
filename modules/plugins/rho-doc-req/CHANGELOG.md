# Changelog

All notable changes to this module are recorded here, one section per release.
A module's description says what it is and does not change between releases; this
says what changed, which is what tells two versions of one module apart. The
section for the version being published travels with it, so a registry shows it
against that version.

## [0.1.0] - 2026-11-19

Initial release. Answers rho-doc's `sources` hook: for every module of a site that keeps requirements (`docs/requirements/`) or architecture decisions (`docs/architecture/adr/`), a requirements page (`<module>/requirements/index`) with every requirement's text, status, verification, upward and downward trace across the site, decisions, code, tests and — when `build/test-reports/` has results — evidence; an index of decisions (`<module>/adr/index`) and a page per decision (`<module>/adr/NNNN-slug`) with its front matter as a header; and, for the root of a family (the repository that keeps `levels.yml`), a trace page (`<module>/requirements/trace`) with every level, every requirement of the family, the matrix and its gaps. Each requirement is a label (`{ref}`GW-0012``), each decision `<repository>:ADR-NNNN`, reachable as `ADR-NNNN` from its own module's pages.

## [0.1.1] - 2026-11-19

Built on rho-req 0.4.0: a module nested inside a repository with a `requirements:` block of its own gets its own requirements and decision pages, its decisions are labelled with its module name (`robohub-provider-mesh:ADR-0001`), and the root's trace page counts its requirements. The documentation says how a nested module is named.
