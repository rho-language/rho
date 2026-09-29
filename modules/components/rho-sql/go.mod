module github.com/rho/rho-sql

go 1.23

require rho v0.0.0

// The shape and the client are the component's own; the plugin protocol is the
// toolchain's, so a session with an engine plugin is the same protocol a
// documentation plugin speaks. The replaces make an in-tree build use this
// checkout; a published component pins released versions instead.
replace rho => ../../..
