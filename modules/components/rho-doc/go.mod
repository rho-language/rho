module github.com/rho/rho-doc

go 1.26.10

toolchain go1.26.10

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

require github.com/rho/rho-fmt v0.0.0 // indirect

// A component is versioned independently, so it depends on the compiler and on
// the other components it uses rather than being compiled into them. Which
// checkout those are is the workspace's business, not this module's, so there is
// no path here that only means something on one machine.
