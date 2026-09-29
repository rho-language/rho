module github.com/rho/rho-doc

go 1.23

require (
	github.com/rho/rho-fmt v0.0.0
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

// A component is versioned independently, so it depends on the compiler and on
// the other components it uses rather than being compiled into them. The
// replaces make an in-tree build use this checkout; a published component pins
// released versions instead.
replace (
	github.com/rho/rho-fmt => ../rho-fmt
	github.com/rho/rho-log => ../rho-log
	rho => ../../..
)
