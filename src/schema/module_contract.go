//
// @file module_contract.go
// @brief What an ecosystem says a module of one of its types is.
//
// The toolchain's own schema knows the types the toolchain defines. Everything
// else — a driver, a board, a device, a platform — belongs to an ecosystem, and
// the toolchain has no business carrying that list. An ecosystem publishes a
// module contract instead: the types it allows, which of them are native, and
// the sections a manifest of each type has to carry.
//
// The contract arrives through the ecosystem's platform plugin. The toolchain
// parses it and applies it without knowing what the words mean, which is what
// keeps Rho a language for more than one ecosystem.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"rho/src/yaml"
)

// ModuleContract is an ecosystem's declaration of the module types it allows
// and the structure each one is written to.
type ModuleContract struct {
	// Version is the contract format, so a toolchain and an ecosystem from
	// different releases can say so rather than misread each other.
	Version int `json:"version"`
	// Types is every type the ecosystem adds, keyed by the word a manifest
	// writes in `module.type`.
	Types map[string]TypeContract `json:"types"`
}

// TypeContract is what one ecosystem type is.
type TypeContract struct {
	// Native means the type is compiled and linked rather than run. It declares
	// no `main`, and a manifest that gives it one is saying two things at once.
	Native bool `json:"native,omitempty"`
	// ModuleID requires `module.id`: the stable identity a consumer keys on
	// when a name alone can be shared by two modules in different repositories.
	ModuleID bool `json:"module_id,omitempty"`
	// Main says whether an entry point is required. Empty follows Native: a
	// native type needs none, a non-native one needs one.
	Main string `json:"main,omitempty"`
	// Required lists top-level sections the manifest must carry, each with a
	// value. A key written with nothing under it is not a declaration.
	Required []string `json:"required,omitempty"`
	// AnyOf lists groups of sections of which at least one has to be present.
	// It is how a type says "one of these lists" without saying which.
	AnyOf [][]string `json:"any_of,omitempty"`
	// Fields are the sections this type validates, in the same shape the
	// toolchain's own schema uses.
	Fields []ContractField `json:"fields,omitempty"`
}

// ContractField is one field of a contract's type schema.
type ContractField struct {
	Name       string          `json:"name"`
	Required   bool            `json:"required,omitempty"`
	RejectNull bool            `json:"reject_null,omitempty"`
	Kind       string          `json:"kind,omitempty"`
	Allowed    []string        `json:"allowed,omitempty"`
	Fields     []ContractField `json:"fields,omitempty"`
	Item       *ContractField  `json:"item,omitempty"`
}

// ParseModuleContract reads a contract an ecosystem published.
func ParseModuleContract(raw []byte) (*ModuleContract, error) {
	var contract ModuleContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return nil, fmt.Errorf("invalid module contract: %w", err)
	}
	if contract.Version != 1 {
		return nil, fmt.Errorf("unsupported module contract version %d", contract.Version)
	}
	if len(contract.Types) == 0 {
		return nil, fmt.Errorf("module contract declares no types")
	}
	return &contract, nil
}

// Merge adds the types of another contract. A type declared twice is the
// second one, because the contracts are read in the order a plugin answered
// them and a later answer is a more specific one.
func (c *ModuleContract) Merge(other *ModuleContract) {
	if other == nil {
		return
	}
	if c.Types == nil {
		c.Types = map[string]TypeContract{}
	}
	for name, entry := range other.Types {
		c.Types[name] = entry
	}
}

// TypeNames returns the contract's type words, in a stable order.
func (c *ModuleContract) TypeNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Types))
	for name := range c.Types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Allows reports whether the contract adds a type.
func (c *ModuleContract) Allows(moduleType string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Types[moduleType]
	return ok
}

// ValidateModuleManifestWithContract validates a manifest against the
// toolchain's base schema and the ecosystem's contract.
//
// A contract is not a second schema language: it says which types are allowed
// and which sections a type must carry, and the toolchain's own validator does
// the reading. A type the contract does not name is validated as the toolchain
// would validate it alone, so a module whose ecosystem has not published a
// contract is not made unreadable by that.
func ValidateModuleManifestWithContract(data yaml.Value, moduleType string, contract *ModuleContract) ValidationErrors {
	if contract == nil || !contract.Allows(moduleType) {
		return ValidateModuleManifest(data, moduleType)
	}
	typeContract := contract.Types[moduleType]

	types := append([]string{}, ValidModuleTypes...)
	types = append(types, contract.TypeNames()...)

	module := moduleSchema(types)
	module.Fields = mergeContractFields(module.Fields, typeContract.Fields)

	result := Validate(data, module)
	errs := ValidationErrors(result.Errors)
	errs = append(errs, ValidateModuleRules(data, moduleType)...)

	// A type whose consumers key on it needs a stable identity, not only a name.
	if typeContract.ModuleID {
		if err := requireModuleID(data, moduleType); err != nil {
			errs = append(errs, *err)
		}
	}

	// The sections the type says make it what it is.
	for _, section := range typeContract.Required {
		if !hasSection(data, section) {
			errs = append(errs, ValidationError{
				Path:     section,
				Message:  fmt.Sprintf("module type '%s' must declare a '%s' section", moduleType, section),
				Severity: SeverityError,
			})
		}
	}

	// A list type may be one of several lists, and it has to be one of them.
	if len(typeContract.AnyOf) > 0 {
		found := false
		choices := []string{}
		for _, group := range typeContract.AnyOf {
			choices = append(choices, group...)
			for _, section := range group {
				if hasSection(data, section) {
					found = true
				}
			}
		}
		if !found {
			errs = append(errs, ValidationError{
				Path:     typeContract.AnyOf[0][0],
				Message:  fmt.Sprintf("module type '%s' must declare one of: %s", moduleType, joinWords(choices)),
				Severity: SeverityError,
			})
		}
	}

	// An entry point is a fact about the type, not about the ecosystem that
	// happens to define it.
	mainRule := typeContract.Main
	if mainRule == "" {
		if typeContract.Native {
			mainRule = "forbidden"
		} else {
			mainRule = "required"
		}
	}
	switch mainRule {
	case "required":
		if !hasSection(data, "main") {
			errs = append(errs, ValidationError{
				Path:     "main",
				Message:  fmt.Sprintf("module type '%s' must declare a main entry point", moduleType),
				Severity: SeverityError,
			})
		}
	case "forbidden":
		if hasSection(data, "main") {
			errs = append(errs, ValidationError{
				Path:     "main",
				Message:  fmt.Sprintf("module type '%s' is native and declares no main", moduleType),
				Severity: SeverityError,
			})
		}
	}

	return errs
}

// requireModuleID reports a type that declares module.id without a qualified
// value. A dotted identity is the minimum that says which repository and path
// the module belongs to; a bare name is what it was supposed to disambiguate.
func requireModuleID(data yaml.Value, moduleType string) *ValidationError {
	module, ok := data.Map["module"]
	id := ""
	if ok && module.Kind == yaml.KindMap {
		id = module.Map["id"].Str
	}
	if id == "" {
		return &ValidationError{
			Path:     "module.id",
			Message:  fmt.Sprintf("module type '%s' must declare a stable module.id", moduleType),
			Severity: SeverityError,
		}
	}
	if strings.ContainsAny(id, " \t\n") || strings.Contains(id, "/") {
		return &ValidationError{
			Path:     "module.id",
			Message:  fmt.Sprintf("module.id %q must be a single identity with no whitespace; prefix a nested module with its repository and path, such as 'repository.path.name'", id),
			Severity: SeverityError,
		}
	}
	return nil
}

// ValidateModuleManifestPackage validates the identity of a manifest whose
// ecosystem contract is not available, which is the case for a package read
// from an archive. The type is not restricted: the package was validated when
// it was published, and a toolchain installing it is not the authority on
// another ecosystem's words.
func ValidateModuleManifestPackage(data yaml.Value) ValidationErrors {
	result := Validate(data, moduleSchema(nil))
	return ValidationErrors(result.Errors)
}

// mergeContractFields adds a contract's fields to the base schema, replacing a
// field the base already declares. A contract may tighten what a base field
// allows, and it must not produce two validators for one key.
func mergeContractFields(base []FieldSchema, extra []ContractField) []FieldSchema {
	for _, field := range extra {
		converted := contractField(field)
		replaced := false
		for i := range base {
			if base[i].Name == converted.Name {
				base[i] = converted
				replaced = true
				break
			}
		}
		if !replaced {
			base = append(base, converted)
		}
	}
	return base
}

// contractField converts a contract field to the toolchain's own field schema.
func contractField(field ContractField) FieldSchema {
	out := FieldSchema{
		Name:          field.Name,
		Required:      field.Required,
		RejectNull:    field.RejectNull,
		Kind:          field.Kind,
		AllowedValues: field.Allowed,
	}
	for _, nested := range field.Fields {
		out.Fields = append(out.Fields, contractField(nested))
	}
	if field.Item != nil {
		item := contractField(*field.Item)
		out.ItemSchema = &ListItemSchema{Kind: item.Kind, Fields: item.Fields}
	}
	return out
}

// hasSection reports whether a manifest carries a section with a value. A key
// written with nothing under it parses as null and is not a declaration.
func hasSection(data yaml.Value, name string) bool {
	value, ok := data.Map[name]
	return ok && value.Kind != yaml.KindNull
}

// joinWords writes a list the way an error message reads.
func joinWords(words []string) string {
	if len(words) == 0 {
		return ""
	}
	out := words[0]
	for _, word := range words[1:] {
		out += ", " + word
	}
	return out
}
