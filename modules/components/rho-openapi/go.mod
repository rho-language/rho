module github.com/rho/rho-openapi

go 1.26.10

toolchain go1.26.10

require (
	github.com/rho/rho-doc v0.0.0
	github.com/rho/rho-log v0.0.0
	github.com/rho/rho-router v0.0.0
	rho v0.0.0
)

// An API description is written from what rho-router read, so this component
// depends on it for the reading rather than reading Rho sources a second time:
// two readings are two dialects, and they drift. rho-doc is what the pages are
// rendered through, so an API reference is a page of the same documentation as
// everything else rather than a site of its own.
//
// A component is versioned independently, so it depends on the toolchain and on
// the components it uses rather than being compiled into them. Which checkout
// those are is the workspace's business, not this module's.
