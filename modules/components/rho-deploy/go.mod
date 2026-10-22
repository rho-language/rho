module github.com/rho/rho-deploy

go 1.26.10

toolchain go1.26.10

// The deployer reads what rho-target resolved and carries it out. It asks the
// toolchain for that answer by invoking it rather than importing it, so a spec
// is read the same way whether a build or a deploy asked for it — and so this
// component builds with no dependency on the toolchain at all.
