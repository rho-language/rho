module github.com/rho/rho-router

go 1.23

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

// A component is versioned independently, so it depends on the compiler and on
// the components it uses rather than being compiled into them. The front end it
// reads with is the compiler's own, which is the point: a route is read here
// exactly as the compiler read it. Which checkout that is, the workspace says.
