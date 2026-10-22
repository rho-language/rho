module github.com/rho/rho-test-gtest

go 1.26.10

toolchain go1.26.10

// The plugin speaks the test runner's protocol, which is a pair of JSON documents
// rather than a library, so it depends on nothing at all. That is deliberate: a
// plugin is a program somebody else wrote, and it has to be able to answer the
// protocol without agreeing on a version of anything — including of the runner it
// extends, which is a component that moves.