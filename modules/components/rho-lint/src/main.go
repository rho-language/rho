//
// @file main.go
// @brief `rho lint` — every analyzer over one compilation database, told the same thing.
//
// Four commands, and the run is the only one that is not a report about this
// machine:
//
//   run                the run itself, and the default when no command is named
//   engines            what this machine can lint with, and which of it is installed
//   check              which of the programs a run needs are here, and where the
//                      compilation database is
//   compile-commands   put a compilation database in place on purpose
//
// `engines` and `check` are kept apart from `run` because from inside a run every
// way it can fail looks the same. An analyzer that is not installed, one that
// refused the compilation database, and a clean tree all end as a run that printed
// nothing and exited zero. Separating them is what tells those apart, and it is the
// difference between a pipeline that is green and a pipeline that is green because it
// has been linting nothing since it was set up.
//
// The arguments are read by hand rather than with a flag package, for the reason
// rho-dbg reads its own: both `-flag value` and `--flag=value` arrive, from an
// editor's launch configuration as much as from a person, and a flag package that
// refused the second would fail on configurations that are perfectly ordinary.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	modcore "rho/cmd/rho-module/core"

	"rho/src/deps"
	"rho/src/plugins"
	"rho/src/targetspec"

	rholog "github.com/rho/rho-log/src/logger"

	"github.com/rho/rho-lint/src/rholint"
)

// log is this component's own logger, so an analyzer's output is identifiable in a
// log that several components write to without every line having to say so.
var log = rholog.Default("rho-lint")

func main() {
	os.Exit(run(os.Args[1:]))
}

// The exit codes, which a pipeline reads.
//
// Three rather than two, because "your code has findings" and "this run could not
// happen" are different failures and a pipeline that is told the second one as the
// first will be silenced by whatever a person does about it.
const (
	// exitClean means nothing at or above the threshold.
	exitClean = 0
	// exitFindings means there were, or nothing ran.
	exitFindings = 1
	// exitUsage means the command was asked for something that cannot be done.
	exitUsage = 2
)

func run(args []string) int {
	if len(args) == 0 {
		return analyse(nil)
	}

	// A command may be named or not: `rho lint` and `rho lint run` are the same
	// thing, and a CI workflow that spells it out should not be told it cannot. A
	// first argument that is not one of the commands belongs to the run — it is a
	// project directory, or a flag — rather than being refused as a typo, because
	// `rho lint .` is what a person types.
	rest := args
	switch args[0] {
	case "run", "lint":
		rest = args[1:]
	case "engines":
		return engines(args[1:])
	case "check", "doctor":
		return check(args[1:])
	case "compile-commands", "compdb":
		return compileCommands(args[1:])
	case "-h", "--help", "help":
		usage()
		return exitClean
	case "-list-plugins", "--list-plugins", "-list-engines":
		return listPlugins(args[1:])
	}
	return analyse(rest)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho lint [project] [options]      run every engine over one compilation database")
	fmt.Fprintln(os.Stderr, "       rho lint engines [-json]        what this machine can lint with")
	fmt.Fprintln(os.Stderr, "       rho lint check [-target NAME]   which of the programs a run needs are here")
	fmt.Fprintln(os.Stderr, "       rho lint compile-commands [-out PATH]")
	fmt.Fprintln(os.Stderr, "                                 put a compilation database in place")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "options a run may be given:")
	fmt.Fprintln(os.Stderr, "  --engine NAME         an engine to run; repeatable, and the default is the settings")
	fmt.Fprintln(os.Stderr, "  --rule NAME           one check to run, rather than all of them; repeatable")
	fmt.Fprintln(os.Stderr, "  --format NAME         text (the default) or sarif")
	fmt.Fprintln(os.Stderr, "  --fail-on SEVERITY     error, warning, note or never; the default is the settings")
	fmt.Fprintln(os.Stderr, "  -o, --out PATH        write the report to a file rather than to standard output")
	fmt.Fprintln(os.Stderr, "  --target NAME         the board, which picks the compiler and the database")
	fmt.Fprintln(os.Stderr, "  --db PATH             the compilation database to read")
	fmt.Fprintln(os.Stderr, "  --exclude FRAGMENT    a path fragment never to read; repeatable")
	fmt.Fprintln(os.Stderr, "  --include-outside     read sources outside this project (a platform, not an application)")
	fmt.Fprintln(os.Stderr, "  --write-db PATH       write the derived compilation database here")
	fmt.Fprintln(os.Stderr, "  --force               allow --write-db to replace a database a build already wrote")
	fmt.Fprintln(os.Stderr, "  --quiet               the findings only, with no notes about the machine")
	fmt.Fprintln(os.Stderr, "  --tree-plugins        also run the plugins this project's own tree carries")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "other commands:")
	fmt.Fprintln(os.Stderr, "  -list-plugins         every plugin this machine has, and which component each one extends")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Every analyzer is told the flags the compiler is really invoked with, so a run")
	fmt.Fprintln(os.Stderr, "reads compile_commands.json when a build wrote one and derives one from the")
	fmt.Fprintln(os.Stderr, "project's manifest when nothing has. A module in modules/plugins declaring")
	fmt.Fprintln(os.Stderr, "`extends: rho-lint` with the `engine` contract adds an analyzer: a program,")
	fmt.Fprintln(os.Stderr, "run and read. See modules/components/rho-lint/README.md.")
}

/* -------------------------------------------------------------------------- */
/*  The run                                                                    */
/* -------------------------------------------------------------------------- */

// options are what a run may be given.
type options struct {
	root           string
	target         string
	engines        []string
	rules          []string
	format         string
	failOn         string
	out            string
	database       string
	exclude        []string
	includeOutside bool
	writeDatabase  string
	overwrite      bool
	quiet          bool
	treePlugins    bool
	help           bool
}

// parse reads the arguments of a run.
//
// Hand-written for the reason at the top of this file, and with one extra: a
// repeated flag has to accumulate rather than be replaced. `--engine a --engine b`
// is two engines, and a parser that kept only the last would silently run one.
func parse(args []string) (options, error) {
	chosen := options{format: rholint.KindText}
	takeValue := func(index *int, argument string, inline string, hasInline bool) (string, error) {
		if hasInline {
			return inline, nil
		}
		if *index+1 >= len(args) {
			return "", fmt.Errorf("%s needs a value", argument)
		}
		*index++
		return args[*index], nil
	}

	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "-") {
			// A bare path is the project, because `rho lint <dir>` is what a person
			// types and `rho lint` from inside a directory is the other half of it.
			if chosen.root == "" {
				chosen.root = argument
				continue
			}
			return chosen, fmt.Errorf("unexpected argument %q; a run takes one project directory", argument)
		}
		name := strings.TrimLeft(argument, "-")
		inline := ""
		hasInline := false
		if equals := strings.Index(name, "="); equals >= 0 {
			name, inline, hasInline = name[:equals], name[equals+1:], true
		}

		var err error
		switch name {
		case "engine", "e":
			chosen.engines, err = appendNamed(chosen.engines, takeValue, &index, argument, inline, hasInline)
		case "rule", "rules":
			chosen.rules, err = appendNamed(chosen.rules, takeValue, &index, argument, inline, hasInline)
		case "exclude":
			var one string
			one, err = takeValue(&index, argument, inline, hasInline)
			chosen.exclude = append(chosen.exclude, one)
		case "format", "f":
			chosen.format, err = takeValue(&index, argument, inline, hasInline)
		case "fail-on", "failOn":
			chosen.failOn, err = takeValue(&index, argument, inline, hasInline)
		case "out", "o":
			chosen.out, err = takeValue(&index, argument, inline, hasInline)
		case "target":
			chosen.target, err = takeValue(&index, argument, inline, hasInline)
		case "db", "database", "compdb":
			chosen.database, err = takeValue(&index, argument, inline, hasInline)
		case "write-db", "writeDb":
			chosen.writeDatabase, err = takeValue(&index, argument, inline, hasInline)
		case "include-outside", "includeOutside":
			chosen.includeOutside = true
		case "tree-plugins", "treePlugins":
			chosen.treePlugins = true
		case "force":
			chosen.overwrite = true
		case "quiet", "q":
			chosen.quiet = true
		case "project", "cwd", "C":
			chosen.root, err = takeValue(&index, argument, inline, hasInline)
		case "h", "help":
			chosen.help = true
		default:
			return chosen, fmt.Errorf("unknown option %q", argument)
		}
		if err != nil {
			return chosen, err
		}
	}
	return chosen, nil
}

func appendNamed(into []string, take func(*int, string, string, bool) (string, error),
	index *int, argument, inline string, hasInline bool) ([]string, error) {
	one, err := take(index, argument, inline, hasInline)
	if err != nil {
		return into, err
	}
	return append(into, one), nil
}

func analyse(args []string) int {
	chosen, err := parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint:", err)
		return exitUsage
	}
	if chosen.help {
		usage()
		return exitClean
	}

	settings := rholint.ReadSettings()
	root := chosen.root
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint: the project directory cannot be resolved:", err)
		return exitUsage
	}

	manifest, err := loadManifest(absolute)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint:", err)
		return exitUsage
	}

	// The board, asked of rho-target. A component call rather than a copy of the
	// specs: which compiler a board is compiled with is a fact rho-target holds and
	// its plugins extend, and a derived database naming `gcc` for a board built with
	// `arm-none-eabi-gcc` is a database that makes every analyzer parse out the wrong
	// branches.
	board := rholint.BoardFor(manifest, chosen.target)
	spec := lookupBoard(board, absolute)

	engines := chosen.engines
	if len(engines) == 0 {
		engines = settings.Engines
	}
	// The engines go to `Analyse` unresolved. It is the only place that knows what
	// the enabled plugins actually offer, so a name that is neither carried here nor
	// provided there is refused there with a message naming what the machine does
	// have — which is the answer worth giving. Refusing it here would need this file
	// to ask the plugins too, and would then be answering the same question twice.
	failOn := settings.FailOn
	if chosen.failOn != "" {
		if !rholint.KnownFailOn(chosen.failOn) {
			fmt.Fprintf(os.Stderr, "rho lint: --fail-on %q is not a severity; use error, warning, note or never\n",
				chosen.failOn)
			return exitUsage
		}
		failOn = rholint.Severity(strings.ToLower(chosen.failOn))
	}

	exclude := chosen.exclude
	if len(exclude) == 0 {
		exclude = settings.Exclude
	}
	outside := settings.Outside
	if chosen.includeOutside {
		outside = rholint.OutsideInclude
	}

	result, err := rholint.Analyse(rholint.Options{
		Root:        absolute,
		Target:      chosen.target,
		Manifest:    manifest,
		TargetSpec:  spec,
		Engines:     engines,
		Rules:       chosen.rules,
		Database:    orElse(chosen.database, settings.Database),
		Exclude:     exclude,
		Outside:     outside,
		FailOn:      failOn,
		Write:       chosen.writeDatabase,
		Overwrite:   chosen.overwrite,
		TreePlugins: chosen.treePlugins,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint:", err)
		return exitUsage
	}

	log.Info("lint run finished",
		"root", absolute, "target", board,
		"findings", result.Counts.Total(), "failed", result.Failed)

	if err := writeReport(chosen, result); err != nil {
		fmt.Fprintln(os.Stderr, "rho lint:", err)
		return exitUsage
	}
	if result.Failed {
		return exitFindings
	}
	return exitClean
}

func orElse(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// writeReport is a run's findings in the format that was asked for.
func writeReport(chosen options, result *rholint.Result) error {
	format := chosen.format
	if format == "" {
		format = rholint.KindText
	}

	sink := os.Stdout
	if chosen.out != "" && chosen.out != "-" {
		if directory := filepath.Dir(chosen.out); directory != "" {
			if err := os.MkdirAll(directory, 0o755); err != nil {
				return fmt.Errorf("cannot create %s: %w", directory, err)
			}
		}
		file, err := os.Create(chosen.out)
		if err != nil {
			return fmt.Errorf("cannot write %s: %w", chosen.out, err)
		}
		defer file.Close()
		sink = file
	}

	switch strings.ToLower(format) {
	case rholint.KindSARIF:
		return rholint.WriteSARIF(sink, result.Runs, result.Database.Root)
	case rholint.KindText:
		if !chosen.quiet {
			if err := rholint.WriteSummary(sink, result); err != nil {
				return err
			}
		}
		return rholint.WriteText(sink, result)
	default:
		return fmt.Errorf("--format %q is not a format this component writes; it is text or sarif", format)
	}
}

/* -------------------------------------------------------------------------- */
/*  What this machine has                                                      */
/* -------------------------------------------------------------------------- */

// engines is what this machine can lint with.
func engines(args []string) int {
	// The tree is the project this was run from rather than the process's own
	// directory, so `rho lint engines` from inside a project answers for that
	// project rather than for whichever directory a shell happened to be in.
	source := pluginSource(projectArgument(args))
	source.TrustTree = hasFlag(args, "tree-plugins")
	available := rholint.Discover(source)

	if len(args) > 0 && args[0] == "-json" {
		answer := struct {
			Engines  []rholint.Description `json:"engines"`
			Reports  []string              `json:"reports"`
			Problems []string              `json:"problems,omitempty"`
		}{}
		for _, spec := range available.Builtin {
			answer.Engines = append(answer.Engines, describeSpec(spec, rholint.SourceBuiltin))
		}
		for _, spec := range available.FromPlugins {
			answer.Engines = append(answer.Engines, describeSpec(spec, rholint.SourcePlugin))
		}
		answer.Reports = rholint.FormatsAvailable(source)
		answer.Problems = available.Problems

		encoded, err := json.MarshalIndent(answer, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho lint engines:", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}

	fmt.Println("Analyzers available:")
	for _, spec := range available.Builtin {
		fmt.Printf("  %-16s %s\n", spec.Name, builtinComment(spec))
	}
	for _, spec := range available.FromPlugins {
		fmt.Printf("  %-16s %s\n", spec.Name, pluginComment(spec))
	}
	if len(available.FromPlugins) == 0 {
		fmt.Println()
		fmt.Println("  (no plugin offers another analyzer)")
		fmt.Println("  a module in modules/plugins declaring `extends: rho-lint` with the `engine`")
		fmt.Println("  contract adds one: the analyzer's own program, run and read")
	}
	if len(available.Problems) > 0 {
		fmt.Println()
		fmt.Println("Offered and not usable:")
		for _, problem := range available.Problems {
			fmt.Printf("  %s\n", problem)
		}
	}
	if len(available.Sources) > 0 {
		fmt.Println()
		fmt.Println("Asked:", strings.Join(available.Sources, ", "))
	}

	fmt.Println()
	fmt.Println("Formats:")
	for _, format := range rholint.FormatsAvailable(source) {
		fmt.Printf("  %s\n", format)
	}

	fmt.Println()
	fmt.Println("Checks this component carries:")
	for _, spec := range available.Builtin {
		set, ok := rholint.RulesFor(spec.Name)
		if !ok {
			continue
		}
		fmt.Printf("  %s:\n", spec.Name)
		for _, rule := range set.Rules {
			fmt.Printf("    %-28s %-8s %s\n", rule.ID, rule.Severity, rule.Description)
		}
	}
	return 0
}

func describeSpec(spec rholint.Spec, source string) rholint.Description {
	return rholint.Description{
		Name:      spec.Name,
		Kind:      spec.Kind,
		Languages: spec.ReadLanguages(),
		Program:   spec.Program,
		Format:    readFormatOf(spec, source),
		Comment:   spec.Comment,
		Version:   spec.Version,
		Tool:      spec.Tool,
		Source:    source,
	}
}

// readFormatOf is the format an engine's output is read from, and empty for one that
// is not a program.
//
// A driver's findings come from this component's own rules, which are already in this
// component's shape — there is no format involved, and reporting `gcc` for a driver
// would be a claim about something that does not happen.
func readFormatOf(spec rholint.Spec, source string) string {
	if source == rholint.SourceBuiltin {
		return ""
	}
	return spec.ReadFormat()
}

func builtinComment(spec rholint.Spec) string {
	if spec.Comment == "" {
		return "carried by this component"
	}
	return spec.Comment
}

func pluginComment(spec rholint.Spec) string {
	parts := []string{"provided by a plugin"}
	if spec.Comment != "" {
		parts = append([]string{spec.Comment}, parts...)
	}
	installed := "not installed"
	if resolved, err := rholint.ResolveProgram(spec.Program, spec.Root); err == nil {
		installed = filepath.Base(resolved)
	}
	return fmt.Sprintf("%s; %s; reads %s", strings.Join(parts, " — "), installed,
		strings.Join(spec.ReadLanguages(), " and "))
}

// check is which of the programs a run needs are installed, and where the
// compilation database is.
//
// Because from inside a run the ways it can fail look the same — a clean tree, an
// analyzer that is not installed, and an analyzer that refused the database all end
// as a run that printed nothing. This is where they are told apart, before anything
// has been started.
// projectArgument is the directory a command names, or this one.
func projectArgument(args []string) string {
	for _, flag := range []string{"-project", "--project", "-cwd", "--cwd", "-C", "--C"} {
		if named := namedArg(args, flag); named != "" {
			return named
		}
	}
	return "."
}

func check(args []string) int {
	board := namedArg(args, "-target")
	if board == "" {
		board = namedArg(args, "-preset")
	}
	root := namedArg(args, "-project")
	if root == "" {
		root = namedArg(args, "-cwd")
	}
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint check:", err)
		return 1
	}
	manifest, err := loadManifest(absolute)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint check:", err)
		return 1
	}
	board = rholint.BoardFor(manifest, board)

	settings := rholint.ReadSettings()
	source := pluginSource(absolute)
	source.TrustTree = namedArg(args, "-tree-plugins") != "" || hasFlag(args, "tree-plugins")
	available := rholint.Discover(source)
	missing := 0

	fmt.Printf("%s:\n", whereRun(absolute, board))

	// The compilation database, and which of the two it is.
	path, described := rholint.Where(absolute, settings.Database, board)
	switch {
	case path != "":
		if _, err := rholint.ReadDatabase(path); err != nil {
			fmt.Printf("  %-24s there, and cannot be read: %v\n", "compilation database", err)
			missing++
		} else {
			fmt.Printf("  %-24s %s\n", "compilation database", described)
		}
	default:
		fmt.Printf("  %-24s none, so a run would derive one from module.yml\n", "compilation database")
	}

	// The compiler a derived database would name, because that is a program that
	// has to be there for the derivation to be worth anything.
	compiler := rholint.CompilerFor(lookupBoard(board, absolute))
	if _, err := rholint.ResolveProgram(compiler); err != nil {
		fmt.Printf("  %-24s %s is not installed, and a derived database would name it\n", "compiler", compiler)
		missing++
	} else {
		fmt.Printf("  %-24s %s\n", "compiler", compiler)
	}

	for _, spec := range available.FromPlugins {
		if _, err := rholint.ResolveProgram(spec.Program, spec.Root); err != nil {
			fmt.Printf("  %-24s %s\n", spec.Name, "not installed")
			missing++
			continue
		}
		fmt.Printf("  %-24s installed\n", spec.Name)
	}
	if len(available.Problems) > 0 {
		fmt.Println()
		fmt.Println("Offered and not usable:")
		for _, problem := range available.Problems {
			fmt.Printf("  %s\n", problem)
		}
	}

	fmt.Println()
	if missing == 0 {
		fmt.Println("A run would work here.")
		return 0
	}
	fmt.Printf("A run would be incomplete here: %d of what it needs is not in place.\n", missing)
	fmt.Println("  rho doctor reports on the whole machine, and rho keeper acts on that report")
	return 1
}

func whereRun(root string, board string) string {
	manifest, err := loadManifest(root)
	if err == nil && manifest != nil && strings.TrimSpace(manifest.ModuleName) != "" {
		if board != "" && board != "native" {
			return manifest.ModuleName + " (" + board + ")"
		}
		return manifest.ModuleName
	}
	if board != "" && board != "native" {
		return filepath.Base(root) + " (" + board + ")"
	}
	return filepath.Base(root)
}

// compileCommands puts a compilation database in place on purpose.
//
// A run derives one itself when nothing has it, so this is for the case where that
// should happen now rather than the moment somebody's editor asks — and for the case
// where somebody wants to see what a run would work from, which is a question about
// the flags rather than about the findings.
func compileCommands(args []string) int {
	root := namedArg(args, "-project")
	if root == "" {
		root = namedArg(args, "-cwd")
	}
	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint compile-commands:", err)
		return 1
	}
	manifest, err := loadManifest(absolute)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint compile-commands:", err)
		return 1
	}
	board := rholint.BoardFor(manifest, namedArg(args, "-target"))
	out := namedArg(args, "-out")
	if out == "" {
		out = namedArg(args, "-o")
	}
	if out == "" {
		out = filepath.Join(absolute, "build", "compile_commands.json")
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join(absolute, filepath.FromSlash(out))
	}
	overwrite := false
	for _, argument := range args {
		if argument == "--force" || argument == "-force" {
			overwrite = true
		}
	}
	// Scanned rather than read with `namedArg`, because `-print` takes no value and a
	// value-taking reader looks for the argument after it: `rho lint compile-commands
	// -print` would find nothing, take `printOnly` as false, and write the database
	// the caller had asked to see instead.
	printOnly := false
	for _, argument := range args {
		switch strings.TrimLeft(argument, "-") {
		case "print":
			printOnly = true
		}
	}

	// An existing database is the accurate one and is never derived over, so this
	// reads it and says so rather than replacing it with a worse one.
	if existing, where := rholint.Where(absolute, "", board); existing != "" {
		database, err := rholint.ReadDatabase(existing)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho lint compile-commands:", err)
			return 1
		}
		if printOnly || out == existing {
			fmt.Println(existing)
			fmt.Fprintf(os.Stderr, "rho lint compile-commands: there is one already, %s\n", where)
			fmt.Fprintf(os.Stderr, "  %s\n", describeDatabase(database))
			return 0
		}
	}

	database, err := rholint.Derive(rholint.DeriveRequest{
		Root:     absolute,
		Manifest: manifest,
		Target:   lookupBoard(board, absolute),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho lint compile-commands:", err)
		return 1
	}

	if printOnly {
		encoded, err := json.MarshalIndent(database.Entries, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho lint compile-commands:", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	if err := rholint.WriteDatabase(out, database, overwrite); err != nil {
		fmt.Fprintln(os.Stderr, "rho lint compile-commands:", err)
		return 1
	}
	fmt.Println(out)
	fmt.Fprintf(os.Stderr, "rho lint compile-commands: derived from %s\n", database.From)
	fmt.Fprintf(os.Stderr, "  %s\n", describeDatabase(database))
	fmt.Fprintln(os.Stderr, "  A build that configures CMake writes a better one, and a run reads that first.")
	return 0
}

func describeDatabase(database rholint.Database) string {
	defines := map[string]bool{}
	includes := 0
	for _, entry := range database.Entries {
		for _, define := range entry.Defines() {
			defines[define] = true
		}
		includes += len(entry.Includes())
	}
	return fmt.Sprintf("%s, compiler %s, %s, %s",
		Plural(len(database.Entries), "compilation"), compilerOf(database),
		Plural(len(defines), "define"), Plural(includes, "include path"))
}

func compilerOf(database rholint.Database) string {
	for _, entry := range database.Entries {
		if compiler := entry.Compiler(); compiler != "" {
			return compiler
		}
	}
	return "(none)"
}

func Plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// listPlugins is every plugin this component would load, and what it plugs into.
//
// Here rather than in `engines` because the question is about the machine's setup
// rather than about one run: a person debugging "why is my analyzer not running"
// needs to know whether the plugin was found at all, before the question becomes
// "which engines did it offer".
func listPlugins(args []string) int {
	source := pluginSource(projectArgument(args))
	source.TrustTree = hasFlag(args, "tree-plugins")
	found := source.All()
	if len(found) == 0 {
		fmt.Println("No plugins are installed.")
		fmt.Println("  A module in modules/plugins declaring `extends: rho-lint` is loaded by this component.")
		return 0
	}
	loaded := 0
	for _, plugin := range found {
		marker := " "
		if plugin.Extends == rholint.ComponentName {
			marker = "*"
			loaded++
		}
		fmt.Printf("%s %-24s %-12s extends %s\n", marker, plugin.Name, plugin.Version, plugin.Extends)
		for _, hook := range plugin.Hooks {
			fmt.Printf("    %s\n", hook)
		}
	}
	if loaded > 0 {
		fmt.Println()
		fmt.Printf("* %s loaded by rho lint; the others belong to a component of their own\n",
			Plural(loaded, "plugin"))
	}
	return 0
}

/* -------------------------------------------------------------------------- */
/*  Reading the project                                                        */
/* -------------------------------------------------------------------------- */

// loadManifest is the project's own manifest.
//
// The two failures kept apart, because one of them is not a failure. A directory
// with no manifest is an ordinary case — `rho lint` is asked for a directory and a
// directory may be a set of C sources with no manifest at all, and a run that
// refused it would be a run that could only ever lint Rho projects, which is the one
// thing this component exists not to be.
//
// A manifest that is *there* and does not parse is a different case, and it is
// reported. Swallowing it would make the run say "the project declares no sources",
// which is a statement about the project and a fact about the run: a typo in
// `build.sources:` looks exactly like a project that has none, and the person
// reading it would go looking in the wrong place.
func loadManifest(root string) (*deps.Manifest, error) {
	path := deps.ManifestPath(root)
	if _, err := os.Stat(path); err != nil {
		return nil, nil
	}
	manifest, err := deps.LoadManifest(path)
	if err != nil {
		return nil, fmt.Errorf("%s cannot be read, and a run cannot know what the project declares:\n  %w",
			path, err)
	}
	return manifest, nil
}

// lookupBoard asks rho-target about a board.
//
// A component call rather than a copy of the specs, because which compiler a board
// is compiled with and which defines it always has are facts rho-target holds and its
// plugins extend. A board a project declares is not in the component's own list
// either, which is why the project is named: `rho target show` reads a project's
// platform declarations when it is told which project, and the toolchain that built
// the project is the one that knows.
//
// A board that cannot be resolved is nil rather than an error. The run carries on
// with what it has and says which compiler it fell back to, because a project with a
// preset a platform no longer provides should still get the findings its own sources
// can produce.
func lookupBoard(name string, project string) *targetspec.Target {
	if strings.TrimSpace(name) == "" || name == "native" {
		return nil
	}
	module, err := modcore.LoadInstalledModule("rho-target")
	if err != nil || module == nil || module.Entry() == nil {
		fmt.Fprintf(os.Stderr,
			"rho lint: no target for %s: the rho-target component is not installed.\n"+
				"  A run will use this machine's compiler, which is not the one this board is built with.\n", name)
		return nil
	}
	said := &bytes.Buffer{}
	args := []string{"show", "-json", name}
	if strings.TrimSpace(project) != "" {
		args = []string{"show", "-json", "-project", project, name}
	}
	if err := module.Entry().Run(args, os.Stdin, writeOnly(said)); err != nil {
		message := strings.TrimSpace(said.String())
		if message == "" {
			fmt.Fprintf(os.Stderr, "rho lint: the target %s cannot be read: %v\n", name, err)
		} else {
			fmt.Fprintf(os.Stderr, "rho lint: %s\n", message)
		}
		return nil
	}
	spec, err := targetspec.ParseTarget(said.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho lint: %v\n", err)
		return nil
	}
	return spec
}

// writeOnly is an os.File a component's output can be read through.
//
// The entry point takes files, which is right for a program whose output is a person
// reading it and wrong for this one: a component dispatched by a lint run writes into
// a report, and a line arriving in the middle of it is a line nobody can act on.
func writeOnly(buffer *bytes.Buffer) *os.File {
	read, write, err := os.Pipe()
	if err != nil {
		return nil
	}
	go func() {
		defer read.Close()
		_, _ = io.Copy(buffer, read)
	}()
	return write
}

func pluginSource(root string) plugins.Source { return rholint.PluginSource(root) }

// hasFlag reports whether a flag was given without a value.
//
// Scanned rather than read with `namedArg`, because a boolean that looks for the
// argument after it finds the *next* flag: `rho lint check --tree-plugins --target pico`
// would read `--target` as this flag's value and quietly l信任 the wrong tree.
func hasFlag(args []string, name string) bool {
	for _, argument := range args {
		if strings.TrimLeft(argument, "-") == name {
			return true
		}
	}
	return false
}

// namedArg is the value a flag was given, spelled either way.
//
// Both spellings because a person types both and the help says the double one:
// `--target pico` is the ordinary way to write it, and refusing it would make the
// documented form the one that does not work.
func namedArg(args []string, name string) string {
	bare := strings.TrimLeft(name, "-")
	for index, argument := range args {
		if !strings.HasPrefix(argument, "-") {
			continue
		}
		written := strings.TrimLeft(argument, "-")
		if flag, value, found := strings.Cut(written, "="); found {
			if flag == bare {
				return value
			}
			continue
		}
		if written == bare && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}
