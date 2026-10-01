//
// @file report.go
// @brief What a target spec says, and how a plugin adds one.
//
// A spec is meant to be read: a person checking which pin the LED is on, or why
// a build links a different library than they expected, should not have to open
// JSON that refers to other JSON. So a resolved spec is reported as one thing,
// with the chain it was folded from named.
//
// The same reason makes a plugin worth having here: a board this component has
// never heard of is a fact about that board, and the project that knows it can
// say so without waiting for this component to ship it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhtarget

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"rho/src/plugins"
)

// HookSpec is the hook a plugin answers with a target spec this component does
// not carry.
const HookSpec = "spec"

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it.
const ComponentName = "rho-target"

// SpecRequest is what a plugin is asked: the targets that were found, so it can
// add one or say what it would replace.
type SpecRequest struct {
	Targets []string `json:"targets"`
}

// SpecResponse is what a plugin answers with: the specs it knows.
type SpecResponse struct {
	Specs []Spec `json:"specs"`
}

// LoadWith reads the specs a directory holds and the ones a plugin contributes.
//
// A plugin that fails is reported and its specs are left out: the targets this
// component ships are still there, which is what a project builds with.
// LoadWith reads the specs in every root and then asks any plugin for more.
//
// The roots are read in order and the first to declare a name is the one that
// stands, so a caller puts the roots it wants to win first.
func LoadWith(roots []string, found []plugins.Plugin) *Library {
	library := LoadFrom(roots)
	if len(found) == 0 {
		return library
	}
	answer := plugins.RunHooks(found, ComponentName, HookSpec, SpecRequest{Targets: library.Names()})
	for _, diagnostic := range answer.Diagnostics {
		library.Diagnostics = append(library.Diagnostics, Diagnostic{Message: diagnostic.String()})
	}
	for _, raw := range answer.Payload {
		response := SpecResponse{}
		if err := json.Unmarshal(raw, &response); err != nil {
			library.Diagnostics = append(library.Diagnostics, Diagnostic{
				Message: fmt.Sprintf("a plugin answered the spec hook with something that is not the protocol: %v", err)})
			continue
		}
		for _, spec := range response.Specs {
			if spec.Name == "" {
				library.Diagnostics = append(library.Diagnostics, Diagnostic{Message: "a plugin answered the spec hook with a target that names nothing"})
				continue
			}
			if _, seen := library.specs[spec.Name]; seen {
				library.Diagnostics = append(library.Diagnostics, Diagnostic{
					Message: fmt.Sprintf("a plugin declares %q, which this component also carries; the one this component ships is used", spec.Name)})
				continue
			}
			spec.Source = "a plugin"
			library.specs[spec.Name] = spec
		}
	}
	return library
}

/* -------------------------------------------------------------------------- */
/*  Reporting                                                                  */
/* -------------------------------------------------------------------------- */

// Describe renders a resolved target as a person reads it: every fact, and the
// chain the facts came from.
func Describe(spec Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)\n", spec.Name, spec.Kind)
	if spec.Comment != "" {
		fmt.Fprintf(&b, "\n%s\n", wrap(spec.Comment, 78))
	}

	// Only the specs this was actually resolved from. A part and an architecture
	// are what a spec is *for*, not what it was resolved from, and listing them
	// reads as though a spec were consulted for them.
	// The specs this was actually resolved from, when it has been resolved. A spec
	// handed to a report without being resolved falls back to what it names, since
	// there is nothing else to say.
	chain := spec.ResolvedFrom
	if len(chain) == 0 {
		chain = []string{spec.Name}
		if spec.MCU != "" && spec.MCU != spec.Name {
			chain = append(chain, spec.MCU)
		}
		if spec.Arch != "" && (len(chain) == 0 || chain[len(chain)-1] != spec.Arch) {
			chain = append(chain, spec.Arch)
		}
	}
	fmt.Fprintf(&b, "\nResolved from: %s\n", strings.Join(chain, " -> "))

	if spec.Core != "" {
		fmt.Fprintf(&b, "\ncore:      %s\n", spec.Core)
	}
	if spec.Toolchain.Compiler != "" {
		fmt.Fprintf(&b, "compiler:  %s\n", spec.Toolchain.Compiler)
	}
	if spec.Toolchain.Objcopy != "" {
		fmt.Fprintf(&b, "objcopy:   %s\n", spec.Toolchain.Objcopy)
	}
	if spec.Toolchain.Size != "" {
		fmt.Fprintf(&b, "size:      %s\n", spec.Toolchain.Size)
	}
	if spec.VendorLibrary != "" {
		fmt.Fprintf(&b, "links:     %s\n", spec.VendorLibrary)
	}

	flags := spec.AllFlags()
	if len(flags) > 0 {
		fmt.Fprintf(&b, "\nflags:\n")
		for _, flag := range flags {
			fmt.Fprintf(&b, "  %s\n", flag)
		}
	}
	if len(spec.Defines) > 0 {
		defines := append([]string{}, spec.Defines...)
		sort.Strings(defines)
		fmt.Fprintf(&b, "\ndefines:\n")
		for _, define := range defines {
			fmt.Fprintf(&b, "  %s\n", define)
		}
	}

	regions := spec.Regions()
	if len(regions) > 0 {
		fmt.Fprintf(&b, "\nmemory:\n")
		for _, region := range regions {
			line := fmt.Sprintf("  %-8s %s + %s", region.Name, region.Region.Origin, region.Region.Length)
			if region.Region.Comment != "" {
				line += "  " + region.Region.Comment
			}
			fmt.Fprintf(&b, "%s\n", line)
		}
	}

	if spec.Flash.Tool != "" {
		fmt.Fprintf(&b, "\nflash:\n  %s\n", spec.Flash.Tool)
		if spec.Flash.Command != "" {
			fmt.Fprintf(&b, "  %s\n", spec.Flash.Command)
		}
		if spec.Flash.Comment != "" {
			fmt.Fprintf(&b, "  %s\n", spec.Flash.Comment)
		}
	}

	if len(spec.Pins) > 0 || len(spec.Peripherals) > 0 {
		fmt.Fprintf(&b, "\npin map:\n")
		b.WriteString(indent(Pins(spec), "  "))
	}
	return b.String()
}

// Pins renders a board's pins and the interfaces they are on.
func Pins(spec Spec) string {
	var b strings.Builder
	for _, name := range spec.PinNames() {
		pin := spec.Pins[name]
		label := pin.Name
		if label == "" {
			label = name
		}
		line := fmt.Sprintf("%-8s %s", strings.ToUpper(name), label)
		if pin.Role != "" {
			line += "  " + pin.Role
		}
		if pin.Comment != "" {
			line += "  (" + pin.Comment + ")"
		}
		fmt.Fprintf(&b, "%s\n", line)
	}

	interfaces := make([]string, 0, len(spec.Peripherals))
	for name := range spec.Peripherals {
		interfaces = append(interfaces, name)
	}
	sort.Strings(interfaces)
	if len(interfaces) > 0 {
		b.WriteString("\n")
	}
	for _, name := range interfaces {
		peripheral := spec.Peripherals[name]
		roles := make([]string, 0, len(peripheral.Pins))
		for role := range peripheral.Pins {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		parts := make([]string, 0, len(roles))
		for _, role := range roles {
			parts = append(parts, role+"="+peripheral.Pins[role])
		}
		instance := peripheral.Instance
		if instance == "" {
			instance = name
		}
		fmt.Fprintf(&b, "%-8s %s", instance, strings.Join(parts, " "))
		if peripheral.Comment != "" {
			fmt.Fprintf(&b, "  (%s)", peripheral.Comment)
		}
		fmt.Fprintf(&b, "\n")
	}
	return b.String()
}

// wrap breaks a comment into lines a terminal reads.
func wrap(text string, width int) string {
	words := strings.Fields(text)
	lines := []string{}
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
			continue
		}
		current += " " + word
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

// indent indents every line of a block.
func indent(text, prefix string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for index, line := range lines {
		if line == "" {
			continue
		}
		lines[index] = prefix + line
	}
	return strings.Join(lines, "\n") + "\n"
}
