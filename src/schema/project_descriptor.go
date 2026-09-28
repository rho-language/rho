//
// @file project_descriptor.go
// @brief Schema definition for the generated-project descriptor.
//
// A project descriptor (`project.yml`) declares the layout of the projects
// that `rho build` generates from Rho sources. Each target language owns a
// list of output files tagged with a role, so adding a new backend means
// adding a descriptor entry rather than changing the generator.
//
// Structure:
//
//   version: 1
//   targets:
//     - id: c
//       emitter: c
//       description: Native C project built with CMake
//       files:
//         - path: app.c
//           role: implementation
//         - path: CMakeLists.txt
//           role: build
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package schema

import (
	"fmt"

	"rho/src/yaml"
)

// Project descriptor roles. A role names the part an emitted file plays in
// the generated project; the generator supplies the contents for each role.
const (
	RoleImplementation = "implementation"
	RoleInterface      = "interface"
	RoleRuntime        = "runtime"
	RoleEntrypoint     = "entrypoint"
	RoleServer         = "server"
	RoleBuild          = "build"
)

// ValidProjectRoles lists every role a descriptor file may use.
var ValidProjectRoles = []string{
	RoleImplementation,
	RoleInterface,
	RoleRuntime,
	RoleEntrypoint,
	RoleServer,
	RoleBuild,
}

// Target kinds. The kind selects the shape of the generated entry point: a
// command line program, or a long-running HTTP server for web APIs.
const (
	TargetKindCLI    = "cli"
	TargetKindServer = "server"
)

// ValidTargetKinds lists every supported target kind.
var ValidTargetKinds = []string{TargetKindCLI, TargetKindServer}

// Descriptor entry kinds. A `dir` entry creates a folder in the generated
// project without writing any file into it, which generated projects need for
// asset and template directories.
const (
	EntryKindFile = "file"
	EntryKindDir  = "dir"
)

// ValidEntryKinds lists every supported descriptor entry kind.
var ValidEntryKinds = []string{EntryKindFile, EntryKindDir}

// PortFSStore is the built-in filesystem module store. It supplies the
// storage externs a registry service declares, so the project needs no
// hand-written host code.
const PortFSStore = "fsstore"

// PortHTTPRoute is the built-in HTTP routing port. It supplies typed parameter
// binding, the router a server target dispatches through, and the middleware
// pipeline wrapped around it, so a web project stays pure Rho.
const PortHTTPRoute = "httproute"

// Routers a server target may dispatch through. The standard library comes
// first and is the default: http.ServeMux has carried method-and-wildcard
// patterns since Go 1.22, so a generated service needs no module download to
// route a request. The rest are adapters, and each costs a dependency the
// project asked for rather than one it inherited.
const (
	// RouterServeMux is the Go 1.22+ http.ServeMux.
	RouterServeMux = "serveMux"
	// RouterChi is the go-chi/chi router.
	RouterChi = "chi"
	// RouterGin is the gin-gonic/gin router.
	RouterGin = "gin"
	// RouterFiber is the gofiber/fiber router.
	RouterFiber = "fiber"
)

// ValidRouters lists every router a target may name.
var ValidRouters = []string{RouterServeMux, RouterChi, RouterGin, RouterFiber}

// Middleware names. Each one is a func(http.Handler) http.Handler the generated
// server composes around the router, in the order the target lists them.
const (
	// MiddlewareRequestID stamps every request with an identifier and echoes
	// it back, so a log line and a client report name the same request.
	MiddlewareRequestID = "request_id"
	// MiddlewareCORS answers preflight and sets the cross-origin headers.
	MiddlewareCORS = "cors"
	// MiddlewareRateLimit bounds how often one client may call the service.
	MiddlewareRateLimit = "rate_limit"
	// MiddlewareJWT verifies a bearer token and refuses the request without
	// one that is valid.
	MiddlewareJWT = "jwt"
)

// Security schemes a service may declare. The list is deliberately short: a
// scheme this toolchain cannot describe is one a document would state wrongly,
// and saying nothing is better than stating the wrong thing.
var ValidSecuritySchemes = []string{SecurityNone, SecurityBearer}

// The ways a caller proves who it is.
const (
	// SecurityNone is a service that asks a caller for nothing.
	SecurityNone = "none"
	// SecurityBearer is an HTTP bearer token in the Authorization header.
	SecurityBearer = "bearer"
)

// ValidMiddlewares lists every middleware a target may compose.
var ValidMiddlewares = []string{
	MiddlewareRequestID,
	MiddlewareCORS,
	MiddlewareRateLimit,
	MiddlewareJWT,
}

// ProjectDescriptorSchema returns the schema for project.yml.
func ProjectDescriptorSchema() Schema {
	return Schema{
		Name: "project.yml",
		Fields: []FieldSchema{
			{Name: "version", Required: true, RejectNull: true, Kind: KindString},
			// output_dir is the project-relative root that generated projects
			// are written to. A target may override it.
			{Name: "output_dir", Required: false, Kind: KindString},
			// build_dir is the project-relative root that compiled artefacts
			// (the generated C translation unit and the native binary) go to.
			{Name: "build_dir", Required: false, Kind: KindString},
			{
				Name:       "targets",
				Required:   true,
				RejectNull: true,
				Kind:       KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindMap,
					Fields: []FieldSchema{
						{Name: "id", Required: true, RejectNull: true, Kind: KindString},
						{Name: "emitter", Required: true, RejectNull: true, Kind: KindString},
						{Name: "description", Required: false, Kind: KindString},
						// kind selects the entry point shape: cli or server.
						{Name: "kind", Required: false, Kind: KindString, AllowedValues: ValidTargetKinds},
						// module_path overrides the derived Go module path.
						{Name: "module_path", Required: false, Kind: KindString},
						// output_dir overrides the descriptor-wide output root
						// for this target.
						{Name: "output_dir", Required: false, Kind: KindString},
						// requires lists extra Go module requirements.
						{Name: "requires", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
						{Name: "build_flags", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
						// ports names built-in port implementations the backend
						// generates, so a Rho project stays pure Rho while its
						// extern declarations are supplied by the compiler.
						{Name: "ports", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
						// router names the HTTP router a server target
						// dispatches through. It is what a target declares to
						// ask for the routing port.
						{Name: "router", Required: false, Kind: KindString, AllowedValues: ValidRouters},
						// api says what the service says about itself, over
						// and above what its routes say. It is read by the
						// component that writes an API description, which is
						// why it is a section rather than a port: what a
						// description needs is facts, not generated code.
						{Name: "api", Required: false, Kind: KindMap, Fields: []FieldSchema{
							// security names how a caller proves who it is.
							{Name: "security", Required: false, Kind: KindString, AllowedValues: ValidSecuritySchemes},
							// security_description says where a credential comes
							// from, which is the one thing a document cannot
							// derive from anything.
							{Name: "security_description", Required: false, Kind: KindString},
							// public_paths lists what is served without one.
							{Name: "public_paths", Required: false, Kind: KindString},
						}},
						// middleware lists the func(http.Handler) http.Handler
						// wrappers composed around the router, outermost first.
						// Every option a middleware understands has a default,
						// so a project names one and overrides what it cares
						// about.
						{Name: "middleware", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
							Kind: KindMap,
							Fields: []FieldSchema{
								{Name: "name", Required: true, RejectNull: true, Kind: KindString, AllowedValues: ValidMiddlewares},
								{Name: "options", Required: false, Kind: KindMap},
							},
						}},
						// templates names template files carried into the
						// generated project, each resolved from
						// <project root>/templates/<name>.<ext>.
						{Name: "templates", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
							Kind: KindMap,
							Fields: []FieldSchema{
								{Name: "name", Required: true, RejectNull: true, Kind: KindString},
								{Name: "file", Required: true, RejectNull: true, Kind: KindString},
							},
						}},
						// copy lists hand-written files placed in the generated
						// project, such as the Go implementations of extern ports.
						{Name: "copy", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
							Kind: KindMap,
							Fields: []FieldSchema{
								{Name: "from", Required: true, RejectNull: true, Kind: KindString},
								{Name: "to", Required: false, Kind: KindString},
							},
						}},
						{
							Name:       "files",
							Required:   true,
							RejectNull: true,
							Kind:       KindList,
							ItemSchema: &ListItemSchema{
								Kind: KindMap,
								Fields: []FieldSchema{
									{Name: "path", Required: true, RejectNull: true, Kind: KindString},
									// kind selects file (default) or dir. A dir entry
									// creates a folder in the generated project.
									{Name: "kind", Required: false, Kind: KindString, AllowedValues: ValidEntryKinds},
									{Name: "role", Required: false, Kind: KindString, AllowedValues: ValidProjectRoles},
									{Name: "optional", Required: false, Kind: KindString},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ValidateProjectDescriptor applies the base schema plus the rules a plain
// field list cannot express:
//
//   - a `file` entry must name a role, since a role selects its contents
//   - a `dir` entry must not name a role, since a folder has no contents
func ValidateProjectDescriptor(data yaml.Value) ValidationErrors {
	result := Validate(data, ProjectDescriptorSchema())
	errs := ValidationErrors(result.Errors)

	targets, ok := data.Map["targets"]
	if !ok || targets.Kind != yaml.KindList {
		return errs
	}

	for t, targetNode := range targets.List {
		files, ok := targetNode.Map["files"]
		if !ok || files.Kind != yaml.KindList {
			continue
		}
		for i, fileNode := range files.List {
			kind := fileNode.Map["kind"].Str
			if kind == "" {
				kind = EntryKindFile
			}
			role := fileNode.Map["role"].Str
			path := fmt.Sprintf("targets[%d].files[%d]", t, i)

			switch kind {
			case EntryKindDir:
				if role != "" {
					errs = append(errs, ValidationError{
						Path:     path + ".role",
						Message:  "a dir entry creates a folder and must not name a role",
						Severity: SeverityError,
					})
				}
			default:
				if role == "" {
					errs = append(errs, ValidationError{
						Path:     path + ".role",
						Message:  fmt.Sprintf("a %s entry must name a role", kind),
						Severity: SeverityError,
					})
				}
			}
		}
	}

	return errs
}
