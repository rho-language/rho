//
// @file descriptor_default.go
// @brief Built-in project descriptor for the C and Go backends.
//
// The default descriptor ships with the compiler so `rho build` works with no
// project files. A project may override it by adding project.yml at its root;
// the two descriptors use the same schema and the same roles.
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
