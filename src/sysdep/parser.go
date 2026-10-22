//
// @file parser.go
// @brief Reading what a module declared, from the manifest's own loader.
//
// The toolchain's manifests are read with its own YAML reader, and this reads them
// the same way. A second reader — a library that only this block uses — would accept
// a manifest the rest of the toolchain rejects, and the symptom of that would be a
// system requirement that is silently ignored on the one machine where it mattered.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package sysdep

import (
	"regexp"
	"strings"

	"rho/src/yaml"
)

// Parse reads the `system_deps` block of a manifest.
//
// Empty for a block that is not there, which is most modules: saying nothing about
// the machine is the normal case and is not a failure.
func Parse(node yaml.Value) *SystemDeps {
	if node.Kind != yaml.KindMap {
		return nil
	}
	block, isMap := node.Map["system_deps"]
	if !isMap || block.Kind != yaml.KindMap || len(block.Map) == 0 {
		return nil
	}

	out := &SystemDeps{}
	if entries := block.Map["executables"]; entries.Kind == yaml.KindList {
		for _, entry := range entries.List {
			if parsed, ok := parseExecutable(entry); ok {
				out.Executables = append(out.Executables, parsed)
			}
		}
	}
	if entries := block.Map["env_vars"]; entries.Kind == yaml.KindList {
		for _, entry := range entries.List {
			if entry.Kind != yaml.KindMap {
				continue
			}
			name := text(entry.Map["name"])
			if name == "" {
				continue
			}
			out.EnvVars = append(out.EnvVars, EnvVarDep{
				Name:        name,
				Required:    truthy(entry.Map["required"]),
				Description: text(entry.Map["description"]),
			})
		}
	}
	if entries := block.Map["udev_rules"]; entries.Kind == yaml.KindList {
		for _, entry := range entries.List {
			if entry.Kind != yaml.KindMap {
				continue
			}
			name := text(entry.Map["name"])
			if name == "" {
				continue
			}
			out.UdevRules = append(out.UdevRules, UdevRuleDep{
				Name:        name,
				TargetPath:  text(entry.Map["target_path"]),
				Description: text(entry.Map["description"]),
				Source:      text(entry.Map["source"]),
				Optional:    truthy(entry.Map["optional"]),
			})
		}
	}
	if !out.Declared() {
		return nil
	}
	return out
}

func parseExecutable(node yaml.Value) (ExecutableDep, bool) {
	if node.Kind != yaml.KindMap {
		return ExecutableDep{}, false
	}
	name := text(node.Map["name"])
	if name == "" {
		return ExecutableDep{}, false
	}
	dep := ExecutableDep{
		Name:           name,
		Version:        text(node.Map["version"]),
		VersionFlag:    text(node.Map["version_flag"]),
		VersionPattern: text(node.Map["version_pattern"]),
		Description:    text(node.Map["description"]),
		// Required defaults to true. A system dependency that a module does not need
		// is not one, and a block full of optional programs nobody checks for is a
		// block that reports nothing.
		Required: true,
	}
	if declared, isSet := node.Map["required"]; isSet && declared.Kind == yaml.KindString {
		dep.Required = truthy(declared)
	}
	if fetch, ok := parseFetch(node.Map["fetch"]); ok {
		dep.Fetch = fetch
	}
	install := node.Map["install"]
	if install.Kind == yaml.KindMap {
		dep.Install = map[string]string{}
		for manager, value := range install.Map {
			dep.Install[strings.ToLower(strings.TrimSpace(manager))] = text(value)
		}
		if len(dep.Install) == 0 {
			dep.Install = nil
		}
	}
	return dep, true
}

// text is a scalar's value, unquoted.
//
// Unquoted here because a manifest writer will write `">=10.0"` — a string that
// starts with a comparison, which YAML would otherwise read as something else
// entirely — and reading it back with the quotes still on it would make every
// constraint unsatisfiable in a way that looks like a version mismatch.
// parseFetch is the user-space recipe, when the module gave one.
func parseFetch(node yaml.Value) (*FetchRecipe, bool) {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil, false
	}
	recipe := &FetchRecipe{
		URL:     text(node.Map["url"]),
		Archive: text(node.Map["archive"]),
		Dir:     text(node.Map["dir"]),
		Binary:  text(node.Map["binary"]),
		Version: text(node.Map["version"]),
		// Lower-cased so the comparison with a computed digest is not a question
		// of which case a vendor's page happened to print.
		SHA256: strings.ToLower(text(node.Map["sha256"])),
	}
	if recipe.URL == "" {
		return nil, false
	}
	return recipe, true
}

func text(node yaml.Value) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(node.Str), `"'`))
}

func truthy(node yaml.Value) bool {
	switch strings.ToLower(text(node)) {
	case "false", "no", "0", "off":
		return false
	default:
		return true
	}
}

/* -------------------------------------------------------------------------- */
/*  Version patterns                                                           */
/* -------------------------------------------------------------------------- */

// ExtractVersion is the version in what a program printed, as the module said to
// read it.
//
// A pattern whose first group is the version is honoured; a pattern with no group
// is not, because a pattern without a group says where the version is and not which
// version-shaped run in it is — and a program that prints four of them, as `gdb
// (Ubuntu 12.1-3ubuntu1~22.04) 12.1` does, means the last one.
func ExtractVersion(output string, pattern string) (Version, bool) {
	if strings.TrimSpace(pattern) == "" {
		return ParseVersion(output)
	}
	expression, err := regexp.Compile(pattern)
	if err != nil {
		// A pattern that does not compile is a module's mistake, and the best thing
		// this can do about it is read the version the way it would have without one:
		// the requirement is then checked rather than skipped.
		return ParseVersion(output)
	}
	match := expression.FindStringSubmatch(output)
	if match == nil {
		return Version{}, false
	}
	if len(match) > 1 {
		for _, group := range match[1:] {
			if version, ok := ParseVersion(group); ok {
				version.Text = group
				return version, true
			}
		}
	}
	// No group captured, or none of the groups held a version: fall back to the
	// whole match, which is the version when the pattern is anchored on one.
	return ParseVersion(match[0])
}

// PatternCompiles is whether a module's version pattern is one this can use.
//
// Reported rather than corrected, because a pattern that does not compile is
// something a person wrote and only they can decide what it should have been —
// and a report that says so is a report they can act on.
func PatternCompiles(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return nil
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return err
	}
	return nil
}
