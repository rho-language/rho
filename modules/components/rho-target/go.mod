module github.com/rho/rho-target

go 1.26.10

toolchain go1.26.10

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

// A target spec is data this component carries, and the compiler it describes is
// the toolchain's. Which checkout that is, the workspace says.
