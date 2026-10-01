module github.com/rho/rho-kconfig

go 1.23

require rho v0.0.0

// The bridge reads a project's manifest and, when a project names a module of
// constants, parses it with the compiler's own front end. Both live in the
// toolchain, so the component depends on the toolchain rather than a copy of it.
