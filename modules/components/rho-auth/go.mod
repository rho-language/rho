module github.com/rho/rho-auth

go 1.23

require github.com/rho/rho-log v0.0.0

require github.com/rho/rho-fmt v0.0.0 // indirect

// In-tree builds use this checkout. A published component pins released
// versions of its dependencies instead, so these lines do not travel.
replace (
	github.com/rho/rho-fmt => ../rho-fmt
	github.com/rho/rho-log => ../rho-log
)
