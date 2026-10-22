module github.com/rho/rho-test

go 1.26.10

toolchain go1.26.10

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

require github.com/rho/rho-fmt v0.0.0 // indirect

// In-tree builds use this checkout. A published component pins released
// versions of its dependencies instead, so these lines do not travel.
