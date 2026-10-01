module github.com/rho/rho-cimport

go 1.23

require (
	rho v0.0.0
	github.com/rho/rho-log v0.0.0
)

// The generator reads a project's manifest and writes the Rho, and the C, a
// binding is made of. Both are the toolchain's: the manifest is read the way
// every tool reads it, and the sources it writes are parsed by the same parser
// that will compile them, so a generated file that does not compile is caught
// here rather than in a build.
