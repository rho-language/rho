module github.com/rho/rho-dbg

go 1.23

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

require github.com/rho/rho-fmt v0.0.0 // indirect

// A debugging session logs through the shared logger, so a session is
// identifiable in a log that several components write to. Which checkout that
// is, the workspace says: a component is versioned independently and its
// in-tree wiring does not travel with it.