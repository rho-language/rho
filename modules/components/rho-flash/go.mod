module github.com/rho/rho-flash

go 1.23

// The flasher reads what rho-target resolved and runs the command that resolved
// to. It asks the toolchain for that answer by invoking it rather than importing
// it, so a spec is read the same way whether a build or a flash asked for it —
// and so this component builds with no dependency on the toolchain at all.
