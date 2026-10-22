module github.com/rho/rho-doctor

go 1.26.10

toolchain go1.26.10

require (
	github.com/rho/rho-log v0.0.0
	rho v0.0.0
)

// The health check reads what modules declared, so it reads the manifest loader
// and the shared system-dependency domain. Both are the toolchain, and both are
// found in the workspace rather than in this module's manifest: a component names
// what it depends on, and go.work says which checkout that is.
