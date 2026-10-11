//
// @file init.go
// @brief `rho init` — start a project.
//
// By default this writes a Rho application: a manifest, one source file, and
// nothing it depends on. When the project in front of you declares `generate:`
// in its module.yml — a drivers repository that knows how to start a driver and
// an example that uses it — the same command writes what the project says it
// can, from the variables a person supplies.
//
// Which one happens is not a flag: it is what the manifest declares. The
// decision, and the writing, are `src/scaffold`, so the keeper does exactly the
// same thing through the same function.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"rho/src/scaffold"
)

// setFlags collects repeated `--set key=value` options.
type setFlags map[string]string

func (s setFlags) String() string { return "" }

func (s setFlags) Set(pair string) error {
	key, value, ok := strings.Cut(pair, "=")
	if !ok {
		return fmt.Errorf("expected key=value, got %q", pair)
	}
	s[strings.TrimSpace(key)] = value
	return nil
}

func initCLI(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project", ".", "the project whose module.yml declares the generators; default is here")
	generator := fs.String("generator", "", "which generator to use when the project declares several")
	name := fs.String("name", "", "the name of what is generated")
	category := fs.String("category", "", "the category, such as temperature")
	description := fs.String("description", "", "one line about what is generated")
	language := fs.String("language", "go", "the build language for a Rho application: go or c")
	targets := fs.String("target", "", "comma-separated build targets for a Rho application")
	set := setFlags{}
	fs.Var(set, "set", "a generator variable, as key=value; may be repeated")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()

	// Whether generation is active here is read from the manifest, so the
	// listing and the writing agree about what this directory is.
	generators, err := scaffold.Generators(*project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho init:", err)
		return 1
	}

	// A project that generates, with no directory named: say what it can make.
	if len(generators) > 0 && len(rest) == 0 {
		listGenerators(*project, generators)
		fmt.Fprintln(os.Stderr, "rho init: name a directory, for example")
		fmt.Fprintln(os.Stderr, "  rho init --category temperature --name ds18b20 driver drivers/temperature/ds18b20")
		return 0
	}

	// Position: `<directory>`, or `<generator> <directory>` where a project
	// declares several and a person names one on the command line.
	generatorName := *generator
	directory := ""
	if len(generators) == 0 {
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "usage: rho init [--name NAME] [--language go|c] [--target a,b] <directory>")
			return 2
		}
		directory = rest[0]
	} else {
		switch len(rest) {
		case 1:
			directory = rest[0]
		case 2:
			generatorName, directory = rest[0], rest[1]
		default:
			fmt.Fprintln(os.Stderr, "usage: rho init [--project DIR] [--category C] [--name N] [<generator>] <directory>")
			return 2
		}
	}

	var targetList []string
	for _, target := range strings.Split(*targets, ",") {
		if trimmed := strings.TrimSpace(target); trimmed != "" {
			targetList = append(targetList, trimmed)
		}
	}

	results, chosen, err := scaffold.Init(scaffold.Request{
		Project:     *project,
		Dir:         directory,
		Name:        *name,
		Generator:   generatorName,
		Category:    *category,
		Description: *description,
		Set:         set,
		Language:    *language,
		Targets:     targetList,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho init:", err)
		if len(generators) > 0 {
			listGenerators(*project, generators)
		}
		return 1
	}
	report(results, chosen)
	return 0
}

// report says what was written, under the generator's name when one was used.
func report(results []scaffold.Result, generator string) {
	if generator == "" {
		for _, result := range results {
			fmt.Printf("Created a Rho application in %s\n", result.Dir)
			for _, file := range result.Files {
				fmt.Println("  ", file)
			}
			fmt.Printf("Next:\n  cd %s && rho run .\n", result.Dir)
		}
		return
	}
	fmt.Printf("Generated %q: %d director%s\n", generator, len(results), plural(len(results)))
	for _, result := range results {
		fmt.Printf("  %s\n", result.Dir)
		for _, file := range result.Files {
			fmt.Println("     ", file)
		}
	}
}

func listGenerators(project string, generators []scaffold.Generator) {
	fmt.Printf("%s can generate:\n", project)
	names := make([]string, 0, len(generators))
	width := 0
	for _, generator := range generators {
		names = append(names, generator.Name)
		if len(generator.Name) > width {
			width = len(generator.Name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		for _, generator := range generators {
			if generator.Name != name {
				continue
			}
			fmt.Printf("  %-*s  %s\n", width, generator.Name, generator.Description)
			for _, variable := range generator.Variables {
				fmt.Printf("  %-*s    needs %s — %s\n", width, "", variable.Name, variable.Description)
			}
		}
	}
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
