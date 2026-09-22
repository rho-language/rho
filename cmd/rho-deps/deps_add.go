//
// @file deps_add.go
// @brief Rho Dependency Add - Enterprise.
//
// Implements `rho deps add`, a deterministic manifest editor for Rho 0.1.
// It inserts a new dependency block into rho.mod.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  rho deps add                                                              */
/* -------------------------------------------------------------------------- */

func runDepsAdd(args []string) {
	fs := flag.NewFlagSet("rho deps add", flag.ExitOnError)

	git := fs.String("git", "", "git URL")
	ver := fs.String("version", "", "version range")
	typ := fs.String("type", "c", "dependency type: c|rho")
	path := fs.String("path", "", "local path override")
	env := fs.String("env", "", "environment variable override")

	// The public syntax places the name before flags, while flag.FlagSet
	// stops parsing at the first positional argument.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		args = append(args[1:], args[0])
	}
	fs.Parse(args)

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho deps add <name> --git <url> --version \"<range>\" --type <c|rho>")
		os.Exit(2)
	}

	name := fs.Arg(0)
	if name == "" || strings.ContainsAny(name, " \t{}\n\r") {
		fmt.Fprintln(os.Stderr, "invalid dependency name")
		os.Exit(2)
	}

	if *git == "" {
		fmt.Fprintln(os.Stderr, "missing --git")
		os.Exit(2)
	}
	if *ver == "" {
		fmt.Fprintln(os.Stderr, "missing --version")
		os.Exit(2)
	}

	// Validate type
	if *typ != "c" && *typ != "rho" {
		fmt.Fprintln(os.Stderr, "invalid --type, must be c or rho")
		os.Exit(2)
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps add:", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	contentBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps add:", err)
		os.Exit(1)
	}

	content := string(contentBytes)

	// Check if dependency already exists
	if strings.Contains(content, "dep "+name+" {") || strings.Contains(content, "dep "+name+"\n") {
		fmt.Fprintf(os.Stderr, "dependency %s already exists\n", name)
		os.Exit(1)
	}

	// Build dependency block
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("dep ")
	b.WriteString(name)
	b.WriteString(" {\n")
	b.WriteString(fmt.Sprintf("    type = \"%s\"\n", *typ))
	b.WriteString(fmt.Sprintf("    git = %s\n", strconv.Quote(*git)))
	b.WriteString(fmt.Sprintf("    version = %s\n", strconv.Quote(*ver)))

	if *path != "" {
		b.WriteString(fmt.Sprintf("    path = %s\n", strconv.Quote(*path)))
	}
	if *env != "" {
		b.WriteString(fmt.Sprintf("    env = %s\n", strconv.Quote(*env)))
	}

	b.WriteString("}\n")

	// Append to manifest
	newContent := content + b.String()

	if err := os.WriteFile(manifestPath, []byte(newContent), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "rho deps add:", err)
		os.Exit(1)
	}

	fmt.Printf("Added dependency:\n  - %s (%s)\n", name, *ver)
}
