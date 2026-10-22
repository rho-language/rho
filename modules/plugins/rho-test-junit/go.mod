module github.com/rho/rho-test-junit

go 1.26.10

toolchain go1.26.10

// The plugin speaks the documentation generator's protocol, which is a pair of
// JSON documents rather than a library, so it depends on nothing at all. That
// is deliberate: a plugin that is a program someone else wrote has to be able to
// answer the protocol without agreeing on a version of anything.
