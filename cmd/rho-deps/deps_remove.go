//
// @file deps_remove.go
// @brief Rho Dependency Remove - Enterprise.
//
// Implements `rho deps remove`, a deterministic manifest editor for Rho 0.1.
// It removes a dependency block from rho.mod.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  rho deps remove                                                           */
/* -------------------------------------------------------------------------- */

func runDepsRemove(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho deps remove <name>")
		os.Exit(2)
	}

	name := args[0]

	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps remove:", err)
		os.Exit(1)
	}

	manifestPath := deps.ManifestPath(projectRoot)
	contentBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps remove:", err)
		os.Exit(1)
	}

	content := string(contentBytes)
	if filepath.Base(manifestPath) == "module.yml" {
		lines := strings.Split(content, "\n")
		var out []string
		removing := false
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "- name:") && strings.TrimSpace(strings.TrimPrefix(trim, "- name:")) == name {
				removing = true
				continue
			}
			if removing && (strings.HasPrefix(line, "  - ") || (len(line) > 0 && line[0] != ' ' && trim != "")) {
				removing = false
			}
			if !removing {
				out = append(out, line)
			}
		}
		if len(out) == len(lines) {
			fmt.Fprintf(os.Stderr, "dependency %s not found\n", name)
			os.Exit(1)
		}
		if err := os.WriteFile(manifestPath, []byte(strings.Join(out, "\n")), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "rho deps remove:", err)
			os.Exit(1)
		}
		fmt.Printf("Removed dependency:\n  - %s\n", name)
		return
	}

	// Check if dependency exists
	header := "dep " + name
	found := false
	for _, ln := range strings.Split(content, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, header) && (len(trim) == len(header) || trim[len(header)] == ' ' || trim[len(header)] == '{') {
			found = true
			break
		}
	}
	if !found {
		fmt.Fprintf(os.Stderr, "dependency %s not found\n", name)
		os.Exit(1)
	}

	// Remove block: dep <name> { ... }
	lines := strings.Split(content, "\n")
	var out []string
	depth := 0

	for _, ln := range lines {
		trim := strings.TrimSpace(ln)

		if depth == 0 && strings.HasPrefix(trim, header) && (len(trim) == len(header) || trim[len(header)] == ' ' || trim[len(header)] == '{') {
			depth = strings.Count(trim, "{") - strings.Count(trim, "}")
			continue
		}

		if depth > 0 {
			depth += strings.Count(trim, "{") - strings.Count(trim, "}")
			continue
		}

		out = append(out, ln)
	}

	newContent := strings.Join(out, "\n")

	if err := os.WriteFile(manifestPath, []byte(newContent), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps remove:", err)
		os.Exit(1)
	}

	fmt.Printf("Removed dependency:\n  - %s\n", name)
}
