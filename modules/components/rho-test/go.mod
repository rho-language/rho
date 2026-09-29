module github.com/rho/rho-test

go 1.23

require (
	github.com/rho/rho-fmt v0.0.0
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

// In-tree builds use this checkout. A published component pins released
// versions of its dependencies instead, so these lines do not travel.
replace (
	github.com/rho/rho-fmt => ../rho-fmt
	github.com/rho/rho-log => ../rho-log
	rho => ../../..
)
