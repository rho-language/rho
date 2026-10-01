module github.com/rho/rho-lsp

go 1.23

// The language server reads and type-checks Rho sources, so it depends on the
// compiler rather than being part of it. The require says so, and the workspace
// says which checkout that is — without the require, nothing names the
// dependency at all.
require rho v0.0.0
