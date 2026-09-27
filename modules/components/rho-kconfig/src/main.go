//
// @file main.go
// @brief The rho kconfig command.
//
// The configuration bridge, as a command a person or a build runs. A generated
// project's CMake calls `merge`; a person calls `list`, `set` and `menuconfig` to
// see and change what a build was configured with.
//
// Usage:
//
//	rho kconfig merge   [flags] <fragment>...   resolve a configuration and write it
//	rho kconfig list    [flags] <fragment>...   print every symbol and where its value came from
//	rho kconfig set     NAME VALUE               set one value in the project's defconfig
//	rho kconfig values                          print the configuration a project declared
//	rho kconfig --help
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

	"github.com/rho/rho-kconfig/src/rhokconfig"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "merge":
		return merge(args[1:])
	case "list":
		return list(args[1:])
	case "set":
		return set(args[1:])
	case "values":
		return values(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "rho kconfig: unknown command %q\n", args[0])
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
	fmt.Fprintln(os.Stderr, `usage: rho kconfig <command> [flags] [fragment...]

  merge    resolve a project's configuration and write the files its dialect produces
  list     print every symbol, its value, and the layer the value came from
  set      set one value in the project's defconfig
  values   print the Kconfig bridge a project declared in its manifest`)
}

/* -------------------------------------------------------------------------- */
/*  merge                                                                     */
/* -------------------------------------------------------------------------- */

func merge(args []string) int {
	fs := flag.NewFlagSet("rho kconfig merge", flag.ExitOnError)
	root := fs.String("root", "", "the project the configuration is for (default: the path, or the working directory)")
	out := fs.String("out", "", "where the files are written (default: the project)")
	dialect := fs.String("dialect", "", "linux, espidf or freertos (default: what the project declared)")
	defconfig := fs.String("defconfig", "", "a file of values to start from")
	module := fs.String("module", "", "a Rho module of constants to apply")
	force := fs.Bool("force", false, "write over a file that is already there")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	// Flags are read wherever they appear, because `rho kconfig merge <path>
	// --force` is what a person types first and a parser that stops at the path
	// would read the flag as a fragment to open.
	flagArgs, rest := splitArgs(args, map[string]bool{"root": true, "out": true, "dialect": true, "library": true, "section": true, "title": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	// A path is the first thing that is not a flag, and only when the caller did
	// not name one: a run that passes --root and then a fragment names a fragment,
	// and reading it as the path would resolve the wrong project and lose it.
	fragments := rest
	if *root == "" {
		if len(fragments) > 0 {
			*root = fragments[0]
			fragments = fragments[1:]
		} else {
			*root = "."
		}
	}

	options := rhokconfig.Options{
		Root:      *root,
		Fragments: fragments,
		Dialect:   *dialect,
		Defconfig: *defconfig,
		Module:    *module,
		Overwrite: *force,
	}

	// A run that names a path and no fragment reads what the project declared: a
	// project's configuration is what its manifest says it is, and making the
	// caller repeat it would let the two disagree.
	if len(options.Fragments) == 0 {
		absolute, absErr := filepath.Abs(*root)
		if absErr != nil {
			fmt.Fprintln(os.Stderr, "rho kconfig:", absErr)
			return 1
		}
		declared, err := rhokconfig.ReadProjectConfig(absolute)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho kconfig:", err)
			return 1
		}
		if !declared.Configured {
			fmt.Fprintf(os.Stderr, "rho kconfig: %s declares no configuration; add a config section to module.yml\n", *root)
			return 1
		}
		options.Fragments = declared.Fragments
		if options.Dialect == "" {
			options.Dialect = declared.Dialect
		}
		if options.Defconfig == "" {
			options.Defconfig = declared.Defconfig
		}
		if options.Module == "" {
			options.Module = declared.Module
		}
		if len(options.Values) == 0 {
			options.Values = declared.Values
		}
	}
	if *out != "" {
		options.Root = *out
	}
	// A person asks a question about a project, and the project knows what it
	// declared: the fragments, the dialect, the values and the module. So a run
	// with no arguments reads the project's own declaration rather than asking
	// for it again, and a build that passes them is not made to repeat itself.
	if len(options.Fragments) == 0 {
		declared, err := rhokconfig.ReadProjectConfig(options.Root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho kconfig:", err)
			return 1
		}
		if !declared.Configured {
			fmt.Fprintln(os.Stderr, "rho kconfig: this project declares no configuration bridge, so it is built with none")
			fmt.Fprintln(os.Stderr, "rho kconfig: add a build.config section to module.yml to declare one")
			return 1
		}
		options.Fragments = declared.Fragments
		if options.Dialect == "" {
			options.Dialect = declared.Dialect
		}
		if options.Defconfig == "" {
			options.Defconfig = declared.Defconfig
		}
		if options.Module == "" {
			options.Module = declared.Module
		}
		if len(options.Values) == 0 {
			options.Values = declared.Values
		}
	}

	result, err := rhokconfig.Merge(options)
	report(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho kconfig:", err)
		return 1
	}
	if !*quiet {
		for _, path := range result.Written {
			fmt.Println("wrote", path)
		}
	}
	// A configuration with a problem in it is a problem, even when the files
	// were written: a build that resolved a symbol to nothing has still lost a
	// setting, and whoever ran it needs to know. A note is not a problem, so it
	// is printed and does not fail the build.
	if len(result.Problems()) > 0 {
		return 1
	}
	return 0
}

// report prints what the merge found, a problem on standard error and a note
// where it belongs: a note about a file that was kept is not a defect.
func report(result rhokconfig.Result) {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Problem() {
			fmt.Fprintln(os.Stderr, "rho kconfig:", diagnostic)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if !diagnostic.Problem() {
			fmt.Fprintln(os.Stderr, "rho kconfig:", diagnostic)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  list                                                                      */
/* -------------------------------------------------------------------------- */

func list(args []string) int {
	fs := flag.NewFlagSet("rho kconfig list", flag.ExitOnError)
	root := fs.String("root", "", "the project the configuration is for (default: the path, or the working directory)")
	dialect := fs.String("dialect", "", "which dialect the project declared")
	defconfig := fs.String("defconfig", "", "a file of values to start from")
	module := fs.String("module", "", "a Rho module of constants to apply")
	flagArgs, rest := splitArgs(args, map[string]bool{"root": true, "dialect": true, "defconfig": true, "module": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	fragments := rest
	if *root == "" {
		if len(fragments) > 0 {
			*root = fragments[0]
			fragments = fragments[1:]
		} else {
			*root = "."
		}
	}
	options := rhokconfig.Options{
		Root:      *root,
		Fragments: fragments,
		Dialect:   *dialect,
		Defconfig: *defconfig,
		Module:    *module,
	}
	if len(options.Fragments) == 0 {
		declared, err := rhokconfig.ReadProjectConfig(options.Root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho kconfig:", err)
			return 1
		}
		if !declared.Configured {
			fmt.Fprintln(os.Stderr, "rho kconfig: this project declares no configuration bridge")
			return 1
		}
		options.Fragments = declared.Fragments
		if options.Dialect == "" {
			options.Dialect = declared.Dialect
		}
		if options.Defconfig == "" {
			options.Defconfig = declared.Defconfig
		}
		if options.Module == "" {
			options.Module = declared.Module
		}
		if len(options.Values) == 0 {
			options.Values = declared.Values
		}
	}

	result, err := rhokconfig.Merge(options)
	report(result)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho kconfig:", err)
		return 1
	}
	fmt.Print(rhokconfig.List(result.Configuration))
	return 0
}

/* -------------------------------------------------------------------------- */
/*  set                                                                       */
/* -------------------------------------------------------------------------- */

func set(args []string) int {
	fs := flag.NewFlagSet("rho kconfig set", flag.ExitOnError)
	root := fs.String("root", ".", "the project")
	fragment := fs.String("fragment", "", "a fragment that declares the symbol")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: rho kconfig set NAME VALUE")
		return 2
	}
	name, value := fs.Arg(0), fs.Arg(1)

	// Setting a value needs to know the symbol exists, so it is checked against
	// the fragments: a value for a symbol nothing declares would be written and
	// never read, which is worse than being refused.
	// A fragment is required to check the symbol against, and a project that set
	// none is told so rather than writing a value nothing reads.
	if *fragment == "" {
		fmt.Fprintln(os.Stderr, "rho kconfig: set needs --fragment, the Kconfig file that declares the symbol")
		return 2
	}
	declared, diagnostics := rhokconfig.ReadFragments(*root, []string{*fragment})
	for _, diagnostic := range diagnostics {
		fmt.Fprintln(os.Stderr, "rho kconfig:", diagnostic)
	}
	configuration, _ := rhokconfig.Resolve(declared, nil)

	defconfig := "kconfig/rho-defconfig"
	rendered, setDiagnostics := rhokconfig.Set(*root, defconfig, name, value, configuration)
	for _, diagnostic := range setDiagnostics {
		fmt.Fprintln(os.Stderr, "rho kconfig:", diagnostic)
	}
	if len(setDiagnostics) > 0 {
		return 1
	}
	path := *root + "/" + defconfig
	if err := os.MkdirAll(*root+"/kconfig", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "rho kconfig:", err)
		return 1
	}
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "rho kconfig:", err)
		return 1
	}
	fmt.Println("wrote", path, name+"="+value)
	return 0
}

/* -------------------------------------------------------------------------- */
/*  values                                                                    */
/* -------------------------------------------------------------------------- */

func values(args []string) int {
	fs := flag.NewFlagSet("rho kconfig values", flag.ExitOnError)
	root := fs.String("root", ".", "the project")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	config, err := rhokconfig.ReadProjectConfig(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho kconfig:", err)
		return 1
	}
	if !config.Configured {
		fmt.Println("This project declares no configuration bridge, so it is built with none.")
		fmt.Println("Add a build.config section to module.yml to declare one.")
		return 0
	}
	fmt.Println("dialect:   ", config.Dialect)
	fmt.Println("fragments: ", strings.Join(config.Fragments, " "))
	if config.Defconfig != "" {
		fmt.Println("defconfig: ", config.Defconfig)
	}
	if config.Module != "" {
		fmt.Println("module:    ", config.Module)
	}
	if len(config.Values) > 0 {
		fmt.Println("values:    ", len(config.Values), "set in module.yml")
	}
	return 0
}
