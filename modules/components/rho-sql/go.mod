module github.com/rho/rho-sql

go 1.26.10

toolchain go1.26.10

require rho v0.0.0

// The shape and the client are the component's own; the plugin protocol is the
// toolchain's, so a session with an engine plugin is the same protocol a
// documentation plugin speaks. Which checkout that is, the workspace says.
