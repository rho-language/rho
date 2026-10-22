module github.com/rho/rho-fmt

go 1.26.10

toolchain go1.26.10

// The formatter needs the Rho front end. A component is versioned
// independently, so it depends on the compiler as a module rather than being
// compiled into it. Which checkout that is, the workspace says.
require rho v0.0.0
