module github.com/rho/rho-robohub

go 1.23

// The plugin speaks four components' protocols, which are pairs of JSON
// documents rather than libraries, so it depends on nothing at all. That is
// deliberate: a platform plugin is a program that maps one platform onto the
// toolchain's contracts, and it has to answer each contract without agreeing
// with the others on a version of anything.