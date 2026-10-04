module github.com/rho/rho-lint

go 1.23

// A lint run logs through the shared logger, so an analyzer's finding is
// identifiable in a log that several components write to. Which checkout that is,
// the workspace says: a component is versioned independently and its in-tree
// wiring does not travel with it.
require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)