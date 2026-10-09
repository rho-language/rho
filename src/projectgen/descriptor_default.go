//
// @file descriptor_default.go
// @brief Built-in project descriptor for the C and Go backends.
//
// The default descriptor ships with the compiler so `rho build` works with no
// project files. A project writes its own targets out in module.yml under
// build.targets, in the same schema and with the same roles, and a target of
// the same id replaces the default's; a project that keeps its descriptor apart
// can add a project.yml at its root instead.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import (
	_ "embed"
)

// DefaultDescriptorYAML is the descriptor used when a project does not
// provide its own project.yml.
//
//go:embed descriptor.default.yml
var DefaultDescriptorYAML []byte

// DefaultDescriptor returns the built-in descriptor, parsed and validated.
func DefaultDescriptor() (*Descriptor, error) {
	return ParseDescriptor(DefaultDescriptorYAML)
}

// DefaultDescriptorFor returns the built-in descriptor parsed once per call.
// It exists as a named helper so callers do not embed the parse themselves.
func LoadDefaultDescriptor() (*Descriptor, error) {
	return DefaultDescriptor()
}
