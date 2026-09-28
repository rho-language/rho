//
// @file main.go
// @brief The rho target command.
//
// Board and architecture specs, and the files a build needs from them:
//
//	rho target list                        what this component knows
//	rho target show <name>                 one resolved spec, and where every
//	                                       fact in it came from
//	rho target pins <name>                 the pin map of a board
//	rho target generate <project> --target <name>
//	                                       write the CMake and the linker script
//
// A spec is resolved before it is shown or written, so what a caller reads is the
// whole target: a board with the toolchain of its chip and the flags of its
// architecture.
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

	rholog "github.com/rho/rho-log/src/logger"
	"github.com/rho/rho-target/src/rhtarget"
)

// log is the component's own logger, so a generation is identifiable in a shared
// log.
var log = rholog.Default("rho-target")

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it.
const ComponentName = "rho-target"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "list":
		return list(args[1:])
	case "show":
		return show(args[1:])
	case "pins":
		return pins(args[1:])
	case "generate":
		return generate(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "rho target: unknown command %q\n", args[0])
	usage()
	return 2
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: rho target <command> [flags]

  list       print every target this component knows
  show       print one resolved target spec
  pins       print the pin map of a board
  generate   write the CMake and the linker script of a target into a project`)
}

/* -------------------------------------------------------------------------- */
/*  The specs this component carries                                           */
/* -------------------------------------------------------------------------- */

// library reads the specs a run can see: the ones this component carries, and the
// ones a plugin supplies.
//
// The specs travel with the component, so they are read from beside the binary
// when it has been installed and from the working tree when it is being built:
// a component that could only find its own data once published would be one that
// cannot be run from a checkout.
func library(trust bool) *rhtarget.Library {
	root := specRoot()
	found := plugins.Source{Tree: root, RhoHome: deps.RhoHome(), TrustTree: trust}.Discover(ComponentName)
	return rhtarget.LoadWith(root, found)
}

// specRoot is the directory the specs are read from.
func specRoot() string {
	if executable, err := os.Executable(); err == nil {
		if dir := filepath.Join(filepath.Dir(executable), "targets"); isDir(dir) {
			return dir
		}
	}
	return "targets"
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

/* -------------------------------------------------------------------------- */
/*  list                                                                       */
/* -------------------------------------------------------------------------- */

func list(args []string) int {
	fs := flag.NewFlagSet("rho target list", flag.ExitOnError)
	boardsOnly := fs.Bool("boards", false, "list only the boards")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	library := library(false)
	report(library.Diagnostics)
	if len(library.Names()) == 0 {
		fmt.Fprintln(os.Stderr, "rho target: no spec was found; this component carries targets/*.json")
		return 1
	}

	specs := []rhtarget.Spec{}
	for _, spec := range library.Effective() {
		if *boardsOnly && spec.Kind != rhtarget.KindBoard {
			continue
		}
		specs = append(specs, spec)
	}
	fmt.Printf("%d target(s)\n", len(specs))
	for _, spec := range specs {
		detail := string(spec.Kind)
		parts := []string{}
		if spec.Core != "" {
			parts = append(parts, spec.Core)
		}
		if spec.Toolchain.Compiler != "" {
			parts = append(parts, spec.Toolchain.Compiler)
		}
		if spec.Memory.Flash.Origin != "" {
			parts = append(parts, "flash "+spec.Memory.Flash.Length)
		}
		if spec.VendorLibrary != "" {
			parts = append(parts, spec.VendorLibrary)
		}
		if len(parts) > 0 {
			detail += ": " + strings.Join(parts, ", ")
		}
		fmt.Printf("  %-14s %s\n", spec.Name, detail)
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  show                                                                       */
/* -------------------------------------------------------------------------- */

func show(args []string) int {
	fs := flag.NewFlagSet("rho target show", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho target show <name>")
		return 2
	}
	library := library(false)
	spec, diagnostics := library.Resolve(fs.Arg(0))
	report(append(library.Diagnostics, diagnostics...))
	if spec.Name == "" {
		return 1
	}
	fmt.Print(rhtarget.Describe(spec))
	return 0
}

/* -------------------------------------------------------------------------- */
/*  pins                                                                       */
/* -------------------------------------------------------------------------- */

func pins(args []string) int {
	fs := flag.NewFlagSet("rho target pins", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho target pins <name>")
		return 2
	}
	library := library(false)
	spec, diagnostics := library.Resolve(fs.Arg(0))
	report(append(library.Diagnostics, diagnostics...))
	if spec.Name == "" {
		return 1
	}
	fmt.Print(rhtarget.Pins(spec))
	return 0
}

/* -------------------------------------------------------------------------- */
/*  generate                                                                   */
/* -------------------------------------------------------------------------- */

func generate(args []string) int {
	fs := flag.NewFlagSet("rho target generate", flag.ExitOnError)
	target := fs.String("target", "", "the target to generate for")
	directory := fs.String("dir", "cmake", "where the files are written, relative to the project")
	force := fs.Bool("force", false, "write over a file that is already there")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	treePlugins := fs.Bool("tree-plugins", false, "also read the plugins the tree carries")
	// Flags are read wherever they appear, because `rho target generate <project>
	// --target pico` is what a person types first.
	flagArgs, rest := splitArgs(args, map[string]bool{"target": true, "dir": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "rho target generate: --target names the board to generate for")
		return 2
	}
	root := "."
	if len(rest) > 0 {
		root = rest[0]
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho target:", err)
		return 1
	}

	library := library(*treePlugins)
	spec, diagnostics := library.Resolve(*target)
	report(append(library.Diagnostics, diagnostics...))
	if spec.Name == "" {
		return 1
	}

	written := []string{}
	for _, file := range rhtarget.Files(spec, *directory) {
		destination := filepath.Join(absolute, filepath.FromSlash(file.Path))
		if !inside(absolute, destination) {
			fmt.Fprintf(os.Stderr, "rho target: %s is outside the project\n", file.Path)
			return 1
		}
		if _, statErr := os.Stat(destination); statErr == nil && !*force {
			// A file a person edited is theirs; one that was generated is
			// replaced, because that is what regenerating means.
			fmt.Fprintf(os.Stderr, "rho target: %s is already there and was kept; --force writes over it\n", file.Path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "rho target:", err)
			return 1
		}
		if err := os.WriteFile(destination, []byte(file.Content), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "rho target:", err)
			return 1
		}
		written = append(written, file.Path)
	}

	if !*quiet {
		for _, name := range written {
			fmt.Println("wrote", name)
		}
		if spec.VendorLibrary != "" {
			fmt.Printf("\n%s links %s. Add to the project's module.yml:\n\n", spec.Name, spec.VendorLibrary)
			fmt.Printf("    c_connection:\n      library: %s\n", spec.VendorLibrary)
		}
		if len(written) > 0 {
			fmt.Printf("\nThe generated CMakeLists includes %s/toolchain.cmake before its project()\n", *directory)
			fmt.Printf("and %s/target.cmake after it.\n", *directory)
		}
		log.Info("target written", "target", spec.Name, "files", fmt.Sprint(len(written)))
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

// report prints what the specs could not say, which is what a person needs to
// fix them.
func report(diagnostics []rhtarget.Diagnostic) {
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(os.Stderr, "rho target:", diagnostic)
	}
}

// splitArgs separates the flags from the path a command was given.
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

// inside reports whether a path is inside a directory.
func inside(dir, path string) bool {
	relative, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
