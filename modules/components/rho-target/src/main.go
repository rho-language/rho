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
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
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
	case "build":
		return build(args[1:])
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
  generate   write the CMake and the linker script of a target into a project
  build      build a generated project for a target: write its CMake, then compile it`)
}

/* -------------------------------------------------------------------------- */
/*  The specs this component carries                                           */
/* -------------------------------------------------------------------------- */

// extraSpecDirs are directories of specs a run was handed or found, most
// significant first.
//
// A platform carries the target specs for the hardware it supports, because the
// memory map and the flash command of a part belong to whoever ships that part.
// This component therefore does not have them, and reading only its own
// directory would leave a target a platform provides unreachable.
//
// They are found rather than told, wherever that is possible. `--specs` still
// says where they are, for a directory that is neither beside a project nor this
// component's own; but a build of a project that depends on a platform already
// knows where that platform's targets are, and being made to repeat the path is
// how the same board ends up described in two places and one of them drifts.
var extraSpecDirs []string

// declaredSpecFiles are the spec files a dependency named one by one, read as
// themselves rather than searched for.
//
// A directory of specs and a named file are different contracts: a platform keeps
// every part's spec in one place and names the directory, while a board carries
// its own and names the file. Reading a named file by looking around its directory
// reads whatever else is in there as well.
var declaredSpecFiles []string

// declaredSpecDirs returns the target spec directories a project's dependencies
// declare they keep their specs in.
//
// The path comes from the module's own manifest, never from its shape on disk. A
// directory called `targets` beside a dependency is a guess that reads as a
// contract and is not one: it stops being right the moment a platform keeps its
// specs somewhere else, and it was never a promise that it would go on being
// there. A module that has hardware says so in a `hardware:` block, and this
// reads what it said.
//
// A dependency reached by git rather than by path has not been fetched yet, and
// what it declares is read by whoever fetches it.
// declaredSpecDirs returns the directories of specs a project's dependencies
// declare they keep their specs in.
//
// A platform keeps every *part's* spec in one place and names that directory. A
// board carries its own and names the file; those come from `boardSpecFiles`,
// which is a different contract and is not searched for here.
func declaredSpecDirs(projectRoot string) []string {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil {
		return nil
	}
	dirs := []string{}
	for _, dep := range manifest.Deps {
		if dep.Type != deps.DepTypeC || dep.Path == "" {
			continue
		}
		root := dep.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(projectRoot, root)
		}
		platform, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || !platform.Hardware.Declared() || platform.Hardware.Targets == "" {
			continue
		}
		targets := platform.Hardware.Targets
		if !filepath.IsAbs(targets) {
			targets = filepath.Join(root, targets)
		}
		if isDir(targets) {
			dirs = append(dirs, targets)
		}
	}
	return dirs
}

// boardSpecFiles returns the spec file each board a project's platform declares
// names, read as that file.
//
// The chain is the contract and nothing is searched for: the platform names its
// boards folder, each board names its own spec file, and that file is the spec.
// The board's folder also holds its manifest, its presets and its support
// package, so searching it reads a manifest as if it described hardware.
func boardSpecFiles(projectRoot string) []string {
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil {
		return nil
	}
	files := []string{}
	seen := map[string]bool{}
	for _, dep := range manifest.Deps {
		if dep.Type != deps.DepTypeC || dep.Path == "" {
			continue
		}
		root := dep.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(projectRoot, root)
		}
		platform, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || !platform.Hardware.Declared() || platform.Hardware.Boards == "" {
			continue
		}
		boards := platform.Hardware.Boards
		if !filepath.IsAbs(boards) {
			boards = filepath.Join(root, boards)
		}
		entries, err := os.ReadDir(boards)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			boardRoot := filepath.Join(boards, entry.Name())
			board, err := deps.LoadManifest(deps.ManifestPath(boardRoot))
			if err != nil || board == nil || board.Hardware == nil || board.Hardware.Target == "" {
				continue
			}
			file := board.Hardware.Target
			if !filepath.IsAbs(file) {
				file = filepath.Join(boardRoot, file)
			}
			if info, err := os.Stat(file); err != nil || info.IsDir() {
				continue
			}
			if seen[file] {
				continue
			}
			seen[file] = true
			files = append(files, file)
		}
	}
	return files
}

// withDeclaredSpecs adds the spec directories a project's dependencies declare
// they keep their specs in, ahead of anything else this run was handed.
//
// Every command that reads a target does this, because a target that is declared
// and cannot be found is not a target, and it is not right for `show`, `build`
// and `list` to disagree about which targets exist.
func withDeclaredSpecs(projectRoot string) {
	for _, dir := range declaredSpecDirs(projectRoot) {
		extraSpecDirs = append([]string{dir}, extraSpecDirs...)
	}
	for _, file := range boardSpecFiles(projectRoot) {
		declaredSpecFiles = append([]string{file}, declaredSpecFiles...)
	}
}

func library(trust bool) *rhtarget.Library {
	root := specRoot()
	found := plugins.Source{Tree: root, RhoHome: deps.RhoHome(), TrustTree: trust, Platform: buildPlatform()}.Discover(ComponentName)

	// Specs a build was handed come first, so that a platform's spec for a board
	// shadows a shipped one of the same name rather than the other way round: a
	// build told where the hardware is should build for the hardware it was told
	// about. `native` and the Cortex-M architecture are still read alongside them,
	// so a build that needs one is not left without it.
	roots := append([]string{}, extraSpecDirs...)

	// Specs a dependency named as a file are read as those files.
	//
	// A board names the spec file it carries, because its memory map and how it is
	// reached are that board's. Handing its *directory* to a reader that searches
	// directories would instead read its manifest, its presets and its support
	// package as if they were hardware.
	library := rhtarget.LoadFrom(append(roots, specRoots()...))
	for _, path := range declaredSpecFiles {
		library.AddFile(path)
	}
	// A platform plugin answers about the targets this project carries, so it is
	// asked only once they are all loaded. Asked earlier it would answer about a
	// library that had not read the project's boards yet, which is every board a
	// platform supplies.
	return rhtarget.Apply(library, found)
}

// buildPlatform is the platform identity the build being served is for.
//
// Read from the application, not from the directory this component's own specs
// live in: the specs are the component's, and the platform is the project's.
//
// Which flag carries the application depends on the command, and getting that
// wrong reads a generated tree for a fact it cannot have. `build` is handed the
// generated tree in `-project` and the application in `-for`, so `--for` is
// asked first; `show`, `list` and the rest name the application in `-project`.
// A build that names neither is run from the project it is building, so the
// working directory is the answer in that case.
//
// Nothing recognized is a real answer and not a failure: the platform-agnostic
// plugins run and the specs carry what the build needs, which is what a host
// build, or a build for a platform nobody has written a plugin for, is.
func buildPlatform() string {
	if root := pathFlag(os.Args[1:], "for"); root != "" {
		return deps.Platform(root)
	}
	if root := pathFlag(os.Args[1:], "project"); root != "" {
		return deps.Platform(root)
	}
	if cwd, err := os.Getwd(); err == nil {
		return deps.Platform(cwd)
	}
	return ""
}

// pathFlag is the value of a `-name value` or `-name=value` flag, or "" when the
// caller did not give it.
func pathFlag(args []string, name string) string {
	for i, arg := range args {
		switch {
		case arg == "-"+name || arg == "--"+name:
			if i+1 < len(args) {
				return args[i+1]
			}
		case strings.HasPrefix(arg, "-"+name+"="):
			return strings.TrimPrefix(arg, "-"+name+"=")
		case strings.HasPrefix(arg, "--"+name+"="):
			return strings.TrimPrefix(arg, "--"+name+"=")
		}
	}
	return ""
}

// specRoot is the directory the specs this component ships are read from.
func specRoot() string {
	if executable, err := os.Executable(); err == nil {
		if dir := filepath.Join(filepath.Dir(executable), "targets"); isDir(dir) {
			return dir
		}
	}
	return "targets"
}

// specRoots are the directories target specs are read from, most significant
// first.
//
// The Rho home comes first, so a target declared in it wins over the one this
// component ships. That is what makes ~/.rho/targets an override rather than a
// second opinion: a machine whose host compiler is not the one the shipped
// `native` names, or whose board needs a memory map this component does not
// have, says so in its own directory and is not argued with.
//
// The first tree to declare a name is the one that stands, so the order here is
// the whole of what decides a target — a plugin cannot override a shipped spec
// either, and for the same reason.
func specRoots() []string {
	roots := []string{}
	if home := deps.RhoHome(); home != "" {
		roots = append(roots, filepath.Join(home, "targets"))
	}
	return append(roots, specRoot())
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

/* -------------------------------------------------------------------------- */
/*  list                                                                       */
/* -------------------------------------------------------------------------- */

func list(args []string) int {
	if root, project := projectFlag(args); project {
		withDeclaredSpecs(root)
	}
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
	// JSON is how a program reads a target rather than a person. It carries the
	// resolved spec and nothing else, so a consumer — a flasher, most of all —
	// gets the board's own words for how it is reached rather than a second
	// opinion compiled into it.
	asJSON := fs.Bool("json", false, "print the resolved spec as JSON")
	project := fs.String("project", ".", "the project whose platform declares the target")
	// Flags are read wherever they appear, because `rho target show pico -json`
	// is what a person types first.
	flagArgs, rest := splitArgs(args, map[string]bool{"project": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho target show [-json] [-project dir] <name>")
		return 2
	}
	withDeclaredSpecs(*project)
	lib := library(false)
	spec, diagnostics := lib.Resolve(rest[0])
	report(append(lib.Diagnostics, diagnostics...))
	if spec.Name == "" {
		return 1
	}
	if *asJSON {
		encoded, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho target show:", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	fmt.Print(rhtarget.Describe(spec))
	return 0
}

/* -------------------------------------------------------------------------- */
/*  pins                                                                       */
/* -------------------------------------------------------------------------- */

func pins(args []string) int {
	if root, project := projectFlag(args); project {
		withDeclaredSpecs(root)
	}
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
	specs := fs.String("specs", "", "colon-separated directories of target specs to read, such as a platform's")
	// Whether the link writes its map, as in `rho target build`. The same choice,
	// spelled the same way, because it is the same setting.
	linkMap := linkMapFlag(fs)
	// Flags are read wherever they appear, because `rho target generate <project>
	// --target pico` is what a person types first.
	flagArgs, rest := splitArgs(args, map[string]bool{"target": true, "dir": true, "specs": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	_ = specs
	if err := addSpecDirs(fs, rest); err != nil {
		fmt.Fprintln(os.Stderr, "rho target generate:", err)
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

	// The project is where the platform's specs are declared, so it is read for
	// them before the library is built. A target a dependency declares is a
	// target, and a build that could not see it would report the board as one
	// that does not exist.
	withDeclaredSpecs(absolute)
	library := library(*treePlugins)
	spec, diagnostics := library.Resolve(*target)
	report(append(library.Diagnostics, diagnostics...))
	if spec.Name == "" {
		return 1
	}

	written := []string{}
	spec.LinkMap = *linkMap
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

/* -------------------------------------------------------------------------- */
/*  Handed-in specs                                                            */
/* -------------------------------------------------------------------------- */

// addSpecDirs records the directories a build was given.
//
// It is a separate pass because the flags are read by each command and the
// directories are wanted by all of them, and a value a reader has to set before
// parsing is a value a reader can forget.
func addSpecDirs(fs *flag.FlagSet, args []string) error {
	values := specDirValues(fs, args)
	for _, dir := range values {
		if dir == "" {
			continue
		}
		if !isDir(dir) {
			return fmt.Errorf("no target specs in %s", dir)
		}
		extraSpecDirs = append(extraSpecDirs, dir)
	}
	return nil
}

// specDirValues finds --specs whether it is declared on the set or only present
// in the arguments, so that a command which does not declare the flag still
// honours one that was typed.
func specDirValues(fs *flag.FlagSet, args []string) []string {
	if declared := fs.Lookup("specs"); declared != nil {
		if value, ok := declared.Value.(flag.Getter); ok {
			if text, ok := value.Get().(string); ok && text != "" {
				return filepath.SplitList(text)
			}
		}
	}
	values := []string{}
	for index := 0; index+1 < len(args); index++ {
		switch args[index] {
		case "--specs", "-specs":
			values = append(values, filepath.SplitList(args[index+1])...)
		}
	}
	return values
}

// projectFlag reads a project out of an argument list, and whether one was named
// at all.
//
// A command that takes a project declares it as a flag like any other, so this
// only has to answer whether the caller said one: an absent project means the
// current directory is the project, which is what running a command inside a
// checkout means.
func projectFlag(args []string) (string, bool) {
	for i, arg := range args {
		switch {
		case arg == "-project" || arg == "--project":
			if i+1 < len(args) {
				return args[i+1], true
			}
		case strings.HasPrefix(arg, "-project="):
			return strings.TrimPrefix(arg, "-project="), true
		case strings.HasPrefix(arg, "--project="):
			return strings.TrimPrefix(arg, "--project="), true
		}
	}
	return "", false
}
