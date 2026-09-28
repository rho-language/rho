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
// SPDX-License-Identifier: MIT
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
  headers    list the declarations that were read from the C headers`)
}

/* -------------------------------------------------------------------------- */
/*  generate                                                                   */
/* -------------------------------------------------------------------------- */

func generate(args []string, check bool) int {
	fs := flag.NewFlagSet("rho cimport", flag.ExitOnError)
	module := fs.String("module", cimport.DefaultModule, "the Rho module to generate the interface as")
	sourceDir := fs.String("src", cimport.DefaultSourceDir, "where the generated files are written, relative to the project")
	force := fs.Bool("force", false, "write over a generated file that is already there")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	usePlugins := fs.Bool("plugins", true, "run the plugins that extend this run")
	noPlugins := fs.Bool("no-plugins", false, "do not run the plugins that extend this run")
	library := fs.String("library", "", "the C library the project links, when the manifest does not say")
	// Flags are read wherever they appear, because `rho cimport generate <path>
	// --force` is what a person types first and a parser that stops at the path
	// would silently ignore the rest.
	flagArgs, rest := splitArgs(args, map[string]bool{"module": true, "src": true, "library": true})
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
	options := cimport.Options{
		Root:        absolute,
		Headers:     connection.Headers,
		IncludeDirs: connection.IncludeDirs,
		Library:     connection.Library,
		SourceDir:   *sourceDir,
		Module:      *module,
		Force:       *force,
		Check:       check,
	}
	if *usePlugins {
		// Only the installed plugins: a generation reads the tree it was pointed
		// at, and a plugin from that tree is code from whoever wrote it.
		options.Plugins = plugins.Source{RhoHome: deps.RhoHome()}.Discover(cimport.ComponentName)
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
	return connection, nil
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
