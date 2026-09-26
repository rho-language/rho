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
