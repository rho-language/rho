//
// @file main.go
// @brief The rho cimport command.
//
// Generates a project's foreign interface from the C headers it links, and the C
// that makes the calls Rho cannot. It reads what a project declared, so it is
// run once per project rather than written by hand.
//
// Usage:
//
//	rho cimport generate [flags] [path]   read the headers and write the binding
//	rho cimport check    [flags] [path]   report what would be generated
//	rho cimport headers  [flags] [path]   list the declarations that were read
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
	"path/filepath"
	"strings"

	"rho/src/deps"

	"rho/src/plugins"

	"github.com/rho/rho-cimport/src/cimport"
	rholog "github.com/rho/rho-log/src/logger"
)

// log is the generator's own logger, so a generation is identifiable in a
// shared log.
var log = rholog.Default("rho-cimport")

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "generate":
		return generate(args[1:], false)
	case "check":
		return generate(args[1:], true)
	case "headers":
		return headers(args[1:])
	case "resolve":
		return resolveCmd(args[1:])
	case "deps":
		return depsCmd(args[1:])
	case "preset":
		return presetCmd(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "rho cimport: unknown command %q\n", args[0])
	usage()
	return 2
}

// splitArgs separates the flags from the path a command was given.
//
// A flag that takes a value may be written `-name value` or `-name=value`, so the
// value is consumed with it: without that, `--force` after a path would be read
// as a second path.
func splitArgs(args []string, takesValue map[string]bool) (flags []string, rest []string) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			rest = append(rest, argument)
			continue
		}
		flags = append(flags, argument)
		name := strings.TrimLeft(argument, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if takesValue[name] && index+1 < len(args) {
			index++
			flags = append(flags, args[index])
		}
	}
	return flags, rest
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: rho cimport <command> [flags] [path]

  generate   read the C headers a project links and write its foreign interface
  check      report what would be generated, writing nothing
  headers    list the declarations that were read from the C headers
  preset     print the preset a project builds against
  resolve    find the preset a build names, and report what it needs
  deps       fetch and report the C dependencies a project builds against`)
}

/* -------------------------------------------------------------------------- */
/*  generate                                                                   */
/* -------------------------------------------------------------------------- */

func generate(args []string, check bool) int {
	fs := flag.NewFlagSet("rho cimport", flag.ExitOnError)
	module := fs.String("module", cimport.DefaultModule, "the Rho module to generate the interface as")
	sourceDir := fs.String("src", "", "where the generated Rho interface is written (default: <build dir>/generated/modules)")
	shimDir := fs.String("shim-dir", "", "where the generated C shim is written (default: <build dir>/generated/c)")
	// Where the sources the selected preset declares are recorded, for the build
	// to compile. It is a file rather than a flag because the build reads it after
	// this command has finished, and a build that had to ask would be asking a
	// different question than the one this answered.
	record := fs.String("record-sources", "", "write the preset's declared sources here, one per line")
	presetName := fs.String("preset", "", "the preset this build selected; its declared sources are what gets compiled")
	force := fs.Bool("force", false, "write over a generated file that is already there")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	usePlugins := fs.Bool("plugins", true, "run the plugins that extend this run")
	noPlugins := fs.Bool("no-plugins", false, "do not run the plugins that extend this run")
	library := fs.String("library", "", "the C library the project links, when the manifest does not say")
	// Flags are read wherever they appear, because `rho cimport generate <path>
	// --force` is what a person types first and a parser that stops at the path
	// would silently ignore the rest.
	flagArgs, rest := splitArgs(args, map[string]bool{"module": true, "src": true, "shim-dir": true, "record-sources": true, "preset": true, "library": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	root := "."
	if len(rest) > 0 {
		root = rest[0]
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport:", err)
		return 1
	}

	// What a project links is in its manifest, so the generator is told what it
	// needs rather than asked for it: a binding generated from different headers
	// than the build links is a binding that calls nothing.
	connection, err := readConnection(absolute, *library)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport:", err)
		return 1
	}
	if len(connection.Headers) == 0 {
		fmt.Fprintf(os.Stderr, "rho cimport: %s declares no C headers to read.\n", root)
		fmt.Fprintln(os.Stderr, "rho cimport: add a c_connection section to module.yml naming them.")
		return 1
	}

	if *noPlugins {
		*usePlugins = false
	}
	// Where generated files go is the project's decision, taken from the build
	// directory its manifest declares. A flag overrides it, because a person
	// asking for somewhere else is entitled to be there for this one run.
	buildDir := "build"
	if manifest, manifestErr := deps.LoadManifest(deps.ManifestPath(absolute)); manifestErr == nil && manifest != nil {
		buildDir = manifest.BuildDirectory()
	}
	if *sourceDir == "" {
		*sourceDir = filepath.Join(buildDir, "generated", "modules")
	}
	if *shimDir == "" {
		*shimDir = filepath.Join(buildDir, "generated", "c")
	}

	options := cimport.Options{
		Root:        absolute,
		Headers:     connection.Headers,
		IncludeDirs: connection.IncludeDirs,
		Library:     connection.Library,
		SourceDir:   *sourceDir,
		ShimDir:     *shimDir,
		Module:      *module,
		Force:       *force,
		Check:       check,
	}
	if *usePlugins {
		// Only the installed plugins: a generation reads the tree it was pointed
		// at, and a plugin from that tree is code from whoever wrote it.
		options.Plugins = plugins.Source{RhoHome: deps.RhoHome(), Platform: deps.Platform(absolute)}.Discover(cimport.ComponentName)
	}
	// The preset declares which vendor sources belong to this build — the startup
	// assembly and the system file, for a board preset, and the family's own
	// sources for an architecture one. A binding generator is the only thing in a
	// build that has read the preset, so it is also the only thing that can say
	// what to compile: a build that worked it out separately would be reading the
	// preset twice and could easily disagree with itself about which board it is.
	if *record != "" {
		if err := cimport.RecordPresetSources(absolute, *record, *presetName); err != nil {
			fmt.Fprintln(os.Stderr, "rho cimport:", err)
			return 1
		}
	}

	result, err := cimport.Generate(options)
	report(result, check)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport:", err)
		return 1
	}
	if problems(result) {
		return 1
	}
	if !*quiet && !check {
		for _, file := range result.Files {
			fmt.Println("wrote", filepath.Join(root, file))
		}
		log.Info("foreign interface generated",
			"declarations", fmt.Sprint(result.Declarations),
			"shims", fmt.Sprint(result.Shims))
	}
	return 0
}

// readConnection is the native library a project links, as its manifest says.
func readConnection(root, library string) (deps.CConnection, error) {
	manifest, err := deps.LoadManifest(deps.ManifestPath(root))
	if err != nil {
		return deps.CConnection{}, err
	}
	if manifest == nil || manifest.Connection == nil {
		if library == "" {
			return deps.CConnection{}, fmt.Errorf("this project declares no c_connection section")
		}
		return deps.CConnection{Library: library}, nil
	}
	connection := *manifest.Connection
	if library != "" {
		connection.Library = library
	}
	// Where the headers are is the platform's to say. A project names the
	// headers it may call and the library it links; the directories holding them
	// belong to whoever owns the headers, and a project that restated them would
	// be keeping a second copy of a list that changes when the platform does.
	//
	// A project that declares its own include directories keeps them, and they
	// are searched first: a project is allowed to add a directory of its own,
	// and to place one ahead of the platform's.
	if len(connection.IncludeDirs) == 0 {
		connection.IncludeDirs = declaredIncludeDirs(root, manifest)
	}
	return connection, nil
}

// declaredIncludeDirs returns the include directories the project's C
// dependencies declare for their consumers, and those of the dependencies those
// dependencies have.
//
// It walks the chain rather than only the project's own, because a platform is
// usually not where its headers are: it declares the board it supports and
// depends on the library that implements it, and the headers a program calls
// belong to that library. A project depending on the platform does not name the
// library, and making it name the library is what this replaces.
//
// Each directory is read from the manifest that declares it, resolved against
// that manifest's own module. A path a dependency publishes is the promise; a
// directory beside it is a guess.
func declaredIncludeDirs(root string, manifest *deps.Manifest) []string {
	dirs := []string{}
	seen := map[string]bool{}
	visited := map[string]bool{}

	var walk func(base string, m *deps.Manifest, depth int)
	walk = func(base string, m *deps.Manifest, depth int) {
		// A cycle in a dependency graph would otherwise be a loop, and a cycle
		// is a broken graph rather than a reason to read fewer of it.
		if depth > 16 || visited[base] {
			return
		}
		visited[base] = true

		for _, dir := range m.BuildIncludeDirs {
			resolved := dir
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(base, resolved)
			}
			if seen[resolved] {
				continue
			}
			seen[resolved] = true
			dirs = append(dirs, resolved)
		}

		for _, dep := range m.Deps {
			if dep.Type != deps.DepTypeC || dep.Path == "" {
				continue
			}
			dependencyRoot := dep.Path
			if !filepath.IsAbs(dependencyRoot) {
				dependencyRoot = filepath.Join(base, dependencyRoot)
			}
			dependency, err := deps.LoadManifest(deps.ManifestPath(dependencyRoot))
			if err != nil {
				continue
			}
			walk(dependencyRoot, dependency, depth+1)
		}
	}

	walk(root, manifest, 0)
	return dirs
}

// report prints what the generator read and what it could not.
func report(result cimport.Result, check bool) {
	if check {
		for _, file := range result.Files {
			fmt.Println("would write", file)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintln(os.Stderr, "rho cimport:", diagnostic)
	}
}

// problems reports whether anything was read that a binding must not claim to
// expose. A binding with a gap in it is worse than one that was not generated,
// so a run with a gap fails.
func problems(result cimport.Result) bool {
	hard := 0
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Message, "already there") {
			// A kept file is something to say, not something to refuse over.
			continue
		}
		hard++
	}
	return hard > 0
}

/* -------------------------------------------------------------------------- */
/*  headers                                                                    */
/* -------------------------------------------------------------------------- */

func headers(args []string) int {
	fs := flag.NewFlagSet("rho cimport headers", flag.ExitOnError)
	library := fs.String("library", "", "the C library, when the manifest does not say")
	flagArgs, rest := splitArgs(args, map[string]bool{"library": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	root := "."
	if len(rest) > 0 {
		root = rest[0]
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport:", err)
		return 1
	}
	connection, err := readConnection(absolute, *library)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho cimport:", err)
		return 1
	}

	header, diagnostics := cimport.ReadDeclared(absolute, connection)
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(os.Stderr, "rho cimport:", diagnostic)
	}
	if len(header.Decls) == 0 {
		fmt.Fprintln(os.Stderr, "rho cimport: no declaration was read")
		return 1
	}

	fmt.Printf("%d declaration(s) from %d header(s)\n", len(header.Decls), len(header.Includes))
	for _, decl := range header.Decls {
		detail := decl.Type
		if decl.Kind == cimport.KindFunc {
			detail = fmt.Sprintf("(%d parameters) %s", len(decl.Params), decl.Result)
		}
		if decl.Kind == cimport.KindStruct || decl.Kind == cimport.KindUnion {
			detail = fmt.Sprintf("%d members", len(decl.Fields))
		}
		if decl.Kind == cimport.KindEnum {
			detail = fmt.Sprintf("%d members", len(decl.Fields))
		}
		if decl.Kind == cimport.KindConst {
			detail = "= " + decl.Value
		}
		fmt.Printf("  %-8s %-28s %s\n", decl.Kind, decl.Name, detail)
	}
	return 0
}
