//
// @file deps_graph.go
// @brief Rho Dependency Graph Exporter - Enterprise.
//
// Implements `rho deps graph --json`, a deterministic JSON graph exporter
// for Rho 0.1. It resolves dependencies and prints a machine-readable graph.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  rho deps graph --json                                                     */
/* -------------------------------------------------------------------------- */

func runDepsGraph(args []string) {
	fs := flag.NewFlagSet("rho deps graph", flag.ExitOnError)
	jsonFlag := fs.Bool("json", false, "output JSON graph")
	fs.Parse(args)

	if !*jsonFlag {
		fmt.Fprintln(os.Stderr, "usage: rho deps graph --json")
		os.Exit(2)
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps graph:", err)
		os.Exit(1)
	}

	manifestPath := filepath.Join(projectRoot, "rho.mod")
	manifest, err := deps.LoadManifest(manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps graph:", err)
		os.Exit(1)
	}

	locator := &deps.DefaultLocator{
		ProjectRoot: projectRoot,
		GlobalCache: filepath.Join(os.Getenv("HOME"), ".rho", "pkg", "mod"),
		LocalCache:  filepath.Join(projectRoot, ".rho-deps"),
	}

	resolver := &deps.Resolver{Locator: locator}
	graph, _, err := resolver.Resolve(manifest, projectRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho deps graph:", err)
		os.Exit(1)
	}

	type Node struct {
		Name    string   `json:"name"`
		Version string   `json:"version"`
		Path    string   `json:"path"`
		Type    string   `json:"type"`
		Deps    []string `json:"deps"`
	}

	var out struct {
		Nodes []Node `json:"nodes"`
	}

	names := make([]string, 0, len(graph.Nodes))
	for name := range graph.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		n := graph.Nodes[name]
		var depsList []string
		for _, d := range n.Deps {
			depsList = append(depsList, d.Name)
		}

		out.Nodes = append(out.Nodes, Node{
			Name:    n.Name,
			Version: n.Version.String(),
			Path:    n.Path,
			Type:    string(n.Type),
			Deps:    depsList,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
