//
// @file go_backend.go
// @brief Go artifacts for the generated-project descriptor.
//
// Renders the Go roles from Rho IR using the codegen Go backend. The target
// kind decides the shape of the generated project: `cli` produces a runnable
// command, `server` produces an HTTP service for web APIs and REST backends.
//
// The output is a self-contained Go module: `go build` and `go run .` work in
// the generated directory with no further wiring.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package projectgen

import (
	"rho/src/codegen"
)

// EmitGoProject renders the Go artifacts for a generated project.
func EmitGoProject(ctx Context) (Artifacts, error) {
	artifacts := Artifacts{}

	kind := ctx.Target.TargetKind()
	implementation := codegen.EmitGoForTarget(ctx.IR, kind)
	if implementation == "" {
		return artifacts, errGoUnsupportedMain
	}
	artifacts.Implementation = implementation

	// runtime.go only carries the helpers the implementation references, so
	// it never gains an unused import when a server is generated too.
	artifacts.Runtime = codegen.EmitGoRuntime(implementation)

	requires := ctx.Target.Requires
	modulePath := ctx.Target.ModulePath

	switch kind {
	case "server":
		artifacts.Server = codegen.EmitGoServer(ctx.IR)
		artifacts.Entrypoint = codegen.EmitGoServerEntrypoint(ctx.IR)
		// A server target still exposes the application entry point, so
		// runtime helpers referenced only from main keep their imports.
		artifacts.Runtime = codegen.EmitGoRuntime(implementation + artifacts.Server + artifacts.Entrypoint)
	default:
		artifacts.Entrypoint = codegen.EmitGoEntrypoint(ctx.IR)
	}

	artifacts.Build = codegen.EmitGoModWithRequirements(ctx.IR, modulePath, requires)

	return artifacts, nil
}
