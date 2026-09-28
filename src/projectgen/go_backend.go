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

// ServerConfigFor reads how a target routes and what it wraps the router in,
// from the descriptor entry the backend is rendering.
func ServerConfigFor(target Target) codegen.ServerConfig {
	config := codegen.ServerConfig{Router: target.Router}
	for _, item := range target.Middleware {
		config.Middleware = append(config.Middleware, codegen.MiddlewareSpec{
			Name:    item.Name,
			Options: item.Options,
		})
	}
	return config
}

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

	// An adapter is a dependency the project asked for by naming a router, so
	// its requirement is written for it rather than left to be remembered.
	if requirement := ctx.Target.RouterRequirement(); requirement != "" {
		requires = append(append([]string{}, requires...), requirement)
	}

	switch kind {
	case "server":
		// How a service routes and what it wraps the router in is the
		// descriptor's decision, so the backend reads it rather than choosing.
		server := ServerConfigFor(ctx.Target)
		artifacts.Server = codegen.EmitGoServer(ctx.IR, server)
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
