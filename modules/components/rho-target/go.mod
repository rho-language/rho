module github.com/rho/rho-target

go 1.23

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

// A target spec is data this component carries, and the compiler it describes is
// the toolchain's. The replaces make an in-tree build use this checkout.
replace (
	github.com/rho/rho-log => ../rho-log
	rho => ../../..
)
