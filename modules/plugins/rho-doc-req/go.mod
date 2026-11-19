module github.com/rho/rho-doc-req

go 1.26.11

toolchain go1.26.11

require github.com/rho/rho-req v0.0.0

require rho v0.0.0 // indirect

// The plugin speaks the documentation generator's protocol, which is a pair of
// JSON documents, so it needs nothing from rho-doc. What it needs is the
// reading: requirements, decisions and the trace between them are rho-req's
// model, and reading the YAML a second time here would be a second dialect of it
// that could disagree with `rho req check`. So it is built on rho-req's own
// package, and the pages and the checker read the files the same way.
