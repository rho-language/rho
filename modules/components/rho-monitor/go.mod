module github.com/rho/rho-monitor

go 1.26.10

toolchain go1.26.10

// A run logs through the shared logger, so a monitor's own output — which
// transport it chose, what it could not open, when it stopped — is identifiable in
// a log several components write to. `rho` is the manifest model, which is how a
// project's `monitor:` block is read.
//
// Which checkout those are is the workspace's business: a component is versioned
// independently and its in-tree wiring does not travel with it.
require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)
