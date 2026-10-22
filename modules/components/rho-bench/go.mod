module github.com/rho/rho-bench

go 1.26.10

toolchain go1.26.10

// A benchmark run orchestrates other components rather than reimplementing them:
// the static footprint comes from rho-mem, a hardware run comes from rho-test, and
// a capture comes from rho-monitor. What lives here is the part none of them do —
// the harness a benchmark is written against, the statistics a set of runs is
// summarised by, and the comparison against a baseline that turns a number into a
// verdict.
//
// The one dependency is the shared logger and the manifest model. Everything else is
// reached as a component call, the way the CLI reaches them, because a component
// that imported another's library would be a copy of it that nobody extends.
require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)