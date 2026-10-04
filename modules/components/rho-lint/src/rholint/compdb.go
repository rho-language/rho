//
// @file compdb.go
// @brief The compilation database: what every analyzer is told the code is.
//
// An embedded C static analysis is only as good as the flags it was given. Without
// `-DSTM32H743xx` the analyzer parses out the branches a build excludes — which
// are usually the ones that pick the board — and reports nothing about the ones it
// exists for. Without the vendor's include paths it cannot see a peripheral's
// header at all. Without `-std=c11` it reads `_Bool` as a typo.
//
// So this component insists on one thing before it runs anything: a compilation
// database, which is the list of arguments the compiler is really invoked with.
// Two ways to have one, and the order matters:
//
//  1. one a build already wrote. A CMake project configured with
//     `CMAKE_EXPORT_COMPILE_COMMANDS` produces one containing the flags that came
//     out of the toolchain file, the Kconfig merge and the dependency's own
//     exported include paths. Nothing derived from a manifest knows any of that.
//  2. one derived from the project's `module.yml`, which is a guess and is
//     labelled as one wherever it is reported.
//
// The distinction is carried all the way into the output rather than smoothed
// over, because a finding from an analyzer that was given a guessed command line
// is a weaker claim than one from an analyzer that was given the real one, and a
// report that cannot tell them apart is a report that overstates half its
// contents.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/targetspec"
)

// Entry is one compilation, as a compilation database states it.
//
// `arguments` and `command` are the same command in the two spellings the format
// allows, and this reads both. `arguments` is preferred where both are there,
// because a command is a string that has to be split again and a path with a space
// in it is a path that will be split wrong.
type Entry struct {
	// Directory is the working directory the compilation ran in. Every relative
	// path in the entry — the file, the output, the include paths — is relative to
	// this and not to the database.
	Directory string `json:"directory"`
	// File is the source, absolute or relative to Directory.
	File string `json:"file"`
	// Arguments is the compiler's argument list, including argv[0].
	Arguments []string `json:"arguments,omitempty"`
	// Command is the same command as one line. Read when `arguments` is absent.
	Command string `json:"command,omitempty"`
	// Output is where the object file went, which no analyzer needs and which is
	// kept so a database written here and given back to CMake is the same file.
	Output string `json:"output,omitempty"`
}

// Database is a compilation database: a list of compilations.
type Database struct {
	// Entries are the compilations, in the order the file listed them.
	Entries []Entry
	// Path is where the database was read from, or written to. Empty for one that
	// was only ever derived.
	Path string
	// Derived says this database was worked out from the project's manifest rather
	// than read from a build, and every finding a run produces carries that fact.
	Derived bool
	// Root is the project directory this database is for, which is what a report's
	// paths are made relative to.
	//
	// Kept on the database rather than passed to the writer separately because every
	// path in it — the files, the include directories — is relative to that one
	// directory, and a writer asked to relativize without being told it produces
	// absolute paths.
	Root string
	// From is where it came from, said in a report: the CMake build directory, or
	// the manifest and the target spec.
	From string
}

// Compiler is the compiler an entry is compiled with, as an argument list states
// it: argv[0] is the program, and a wrapped compiler is still its own name.
//
// Returns "" for an entry that names no command at all, which a database written
// by something other than CMake does happen to contain.
func (e Entry) Compiler() string {
	if len(e.Arguments) > 0 {
		return e.Arguments[0]
	}
	if command := splitCommand(e.Command); len(command) > 0 {
		return command[0]
	}
	return ""
}

// Command2 is the entry's command as an argument list, whatever spelling it came
// in. Empty when it named none.
func (e Entry) Command2() []string {
	if len(e.Arguments) > 0 {
		return e.Arguments
	}
	return splitCommand(e.Command)
}

// Source is the entry's source as an absolute path, which is what every analyzer
// wants and what `directory` is for.
//
// An entry whose directory is missing has its file taken as given: a database
// written by hand with absolute paths needs no directory, and refusing to read one
// would be a rule about the file's shape rather than about the project.
func (e Entry) Source() string {
	if filepath.IsAbs(e.File) {
		return filepath.Clean(e.File)
	}
	if strings.TrimSpace(e.Directory) == "" {
		return filepath.Clean(e.File)
	}
	return filepath.Clean(filepath.Join(e.Directory, e.File))
}

// Language is the language of this entry's source, from the compiler's own
// arguments.
//
// Read from `-x` when the command sets it, and from the extension otherwise.
// `-x` first because a compiler told `-x c++` about a file named `.c` is
// compiling it as C++ and the extension would say the opposite — which is exactly
// the sort of disagreement an analyzer must not be left to resolve itself.
func (e Entry) Language() string {
	for index, argument := range e.Command2() {
		if argument == "-x" && index+1 < len(e.Command2()) {
			return normalizeLanguage(e.Command2()[index+1])
		}
		if value, ok := strings.CutPrefix(argument, "-x"); ok && value != "" {
			return normalizeLanguage(value)
		}
	}
	return LanguageOf(e.File)
}

// LanguageOf is the language a source is, from its extension.
func LanguageOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".c":
		return LanguageC
	case ".cc", ".cpp", ".cxx", ".c++":
		return LanguageCPP
	case ".h", ".hh", ".hpp", ".hxx":
		// A header has no language of its own; it is whatever the source including
		// it is, and the engine that reads headers is the one reading sources.
		return LanguageHeader
	case ".rho":
		return LanguageRho
	default:
		return LanguageUnknown
	}
}

// The languages an entry can be in.
const (
	LanguageC       = "c"
	LanguageCPP     = "cpp"
	LanguageHeader  = "header"
	LanguageRho     = "rho"
	LanguageUnknown = ""
)

// normalizeLanguage is a `-x` value as one of the languages here.
func normalizeLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "c", "c-header":
		return LanguageC
	case "c++", "cxx", "cpp":
		return LanguageCPP
	case "rho":
		return LanguageRho
	default:
		return LanguageUnknown
	}
}

// Includes are the include directories of a compilation, as absolute paths.
//
// Both spellings, because a compiler accepts both and a build writes whichever the
// project's style uses: `-Iinclude` and `-I include` are the same flag. The second
// form needs the *next* argument, which is why this walks the list by index rather
// than over it — a `for _, argument := range` cannot see the argument after the one
// it is looking at, and would silently drop every separately-spelled include path a
// hand-written CMake file produces.
//
// The system paths are not filtered out here. They are the compiler's own and the
// same on every machine, but a `-isystem` entry is a project's own choice about
// where a header is looked for, and a report that lists it is being asked what the
// build was told.
func (e Entry) Includes() []string {
	found := []string{}
	arguments := e.Command2()
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch {
		case argument == "-I" || argument == "-isystem":
			// The path is the next argument — but only when the next argument is not
			// itself a flag. A trailing `-I` is a compiler error, and consuming
			// whatever follows it would turn a malformed command line into a database
			// describing a directory called `-Dsomething`.
			if index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "-") {
				index++
				found = append(found, resolveArgumentPath(e, arguments[index]))
			}
		case strings.HasPrefix(argument, "-I") && len(argument) > 2:
			found = append(found, resolveArgumentPath(e, argument[2:]))
		case strings.HasPrefix(argument, "-isystem") && len(argument) > 8:
			found = append(found, resolveArgumentPath(e, argument[8:]))
		}
	}
	return found
}

// Defines are the macros a compilation has, as `-D` without the `-D`.
func (e Entry) Defines() []string {
	found := []string{}
	for _, argument := range e.Command2() {
		if value, ok := strings.CutPrefix(argument, "-D"); ok && value != "" {
			found = append(found, value)
		}
	}
	return found
}

func resolveArgumentPath(entry Entry, path string) string {
	if filepath.IsAbs(path) || strings.TrimSpace(entry.Directory) == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(entry.Directory, path))
}

/* -------------------------------------------------------------------------- */
/*  Finding one                                                                */
/* -------------------------------------------------------------------------- */

// Where is the compilation database a run should read.
//
// A *list* of places rather than a walk, and the distinction is the whole design:
// a search that finds somebody else's database in a build directory two levels
// down shows a project an analysis of code that is not its own. Each place is
// named in the manifest's settings and each is one a project or a build produces.
//
// `board` decides which of several databases is read, and it is not a detail. A
// project written for six Nucleo boards has six databases — one per CMake build
// directory, one per set of defines — and they disagree about which branches of a
// header exist. Reading the newest of them would be an analysis of whichever board
// happened to be built last, reported as though it were about this project.
//
// `declared` is what the settings or the command line named, and it is not a
// candidate among the others: a database somebody pointed at is used or reported
// missing, never replaced.
func Where(root string, declared string, board string) (string, string) {
	if strings.TrimSpace(declared) != "" {
		path := declared
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
		if existing(path) {
			return path, "named by the settings"
		}
		return path, "named by the settings, and not there"
	}

	for _, place := range placesFor(board) {
		path := filepath.Join(root, filepath.FromSlash(place))
		if existing(path) {
			return path, describePlace(place, board)
		}
	}
	return "", ""
}

// placesFor is the order the database is looked for in, for a board.
//
// The board's own build directory first when a board was named, because that is
// the one whose flags are the flags being asked about. The rest are the places a
// project that is not board-specific produces, and they are the same list in the
// same order whichever board was named — a project that has one database has one
// because it has one board.
func placesFor(board string) []string {
	places := []string{}
	if name := strings.TrimSpace(board); name != "" {
		places = append(places,
			"build/generated/c/build/"+name+"/compile_commands.json",
			"build/build/"+name+"/compile_commands.json",
		)
	}
	return append(places, defaultPlaces()...)
}

// describePlace says where a database was found, in words a report can use.
//
// The board's directory is named by the board and the rest by the path, because
// "there is a database" is not an answer to "is this the one for this board".
func describePlace(place string, board string) string {
	for _, prefix := range []string{
		"build/generated/c/build/", "build/build/",
	} {
		if after, ok := strings.CutPrefix(place, prefix); ok {
			if name, ok := strings.CutSuffix(after, "/compile_commands.json"); ok {
				if board != "" && name == board {
					return place + ", which is " + name + "'s own build directory"
				}
				return place + ", a build for " + name
			}
		}
	}
	return place + ", where a CMake build puts it"
}

// defaultPlaces is where a CMake project writes its compilation database, in the
// order it is looked for.
//
// `build/` first because that is what a generated Rho project uses, since
// `build_dir` defaults to it. `compile_commands.json` at the root next, because a
// project that configures CMake in place has it there. The IDE directories last,
// because a project that has one has usually also got a CMake tree somewhere and
// the generated one is the one the build actually uses.
func defaultPlaces() []string {
	return []string{
		"build/compile_commands.json",
		"compile_commands.json",
		"build/out/compile_commands.json",
		// A bare `out/` is where this toolchain's own documented workflow puts one —
		// `cmake -B out && cmake --build out` — so a project following the handbook
		// has its database here and nowhere else.
		"out/compile_commands.json",
		"build/generated/c/compile_commands.json",
		"build/generated/c/out/compile_commands.json",
		"cmake-build-debug/compile_commands.json",
		"cmake-build-release/compile_commands.json",
	}
}

// rootOf is the project directory a database belongs to, worked out from where it
// was found.
//
// The two directories a build writes one into are named here rather than searched
// for: `build/compile_commands.json` and `compile_commands.json` are where a Rho
// project's and a plain CMake project's are, and a path three levels down from a
// database is the project rather than a directory in it.
func rootOf(path string) string {
	directory := filepath.Dir(path)
	switch filepath.Base(directory) {
	case "build", "out", "cmake-build-debug", "cmake-build-release":
		return filepath.Dir(directory)
	}
	return directory
}

// existing is a path that is a file, or nothing.
//
// A file and not merely a path that exists, because every caller of this asks
// whether a *file* is there to be read: a directory of that name is not a
// compilation database, and answering that it is would send a reader into a
// directory it cannot parse.
func existing(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// present is a path that exists at all, a directory included.
func present(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

/* -------------------------------------------------------------------------- */
/*  Reading one                                                                */
/* -------------------------------------------------------------------------- */

// ReadDatabase reads a compilation database.
//
// Refused rather than repaired when the file is not a database. An analyzer given
// a database it cannot parse produces findings about nothing, and the run that
// reported them as a clean tree is worse than one that said the file is not what
// it claims to be.
func ReadDatabase(path string) (Database, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Database{}, fmt.Errorf("the compilation database cannot be read: %w", err)
	}
	entries := []Entry{}
	if err := json.Unmarshal(data, &entries); err != nil {
		return Database{}, fmt.Errorf(
			"%s is not a compilation database; it is JSON that is not a list of compilations: %w", path, err)
	}
	usable := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if strings.TrimSpace(entry.File) == "" {
			continue
		}
		usable = append(usable, entry)
	}
	if len(usable) == 0 {
		return Database{}, fmt.Errorf(
			"%s is a list of compilations and not one of them names a source file", path)
	}
	return Database{Entries: usable, Path: path, From: path, Root: rootOf(path)}, nil
}

/* -------------------------------------------------------------------------- */
/*  Deriving one                                                               */
/* -------------------------------------------------------------------------- */

// DeriveRequest is what a compilation database is worked out from.
//
// Everything is a fact a project already declared rather than a guess about how a
// project is probably laid out, which is why this takes the manifest and the
// resolved target rather than a directory to search.
type DeriveRequest struct {
	// Root is the project directory every path is relative to.
	Root string
	// Manifest is the project's own, which is where the sources, the include
	// directories and the defines come from.
	Manifest *deps.Manifest
	// Target is the board's resolved spec, which is where the compiler and the
	// board's own defines come from. Nil for a project that targets this machine,
	// which is a real case and not a missing one.
	Target *targetspec.Target
	// Sources are the files to make entries for, when the caller already knows
	// them. Empty means the manifest's own list, and then the generated project's
	// sources under the build directory.
	Sources []string
}

// Derive works a compilation database out of what a project declared.
//
// The compiler comes from the target spec, because which compiler a board is
// compiled with is a fact rho-target holds and its plugins extend, and a
// compilation database naming `gcc` for a board built with `arm-none-eabi-gcc` is
// a database that will make every analyzer parse out the wrong branches.
//
// The defines are the board's and the project's, in that order, because the
// board's are the ones that pick the chip and the project's are the ones that pick
// a feature, and a project that redefines one of the board's means to.
//
// Labelled derived everywhere it is reported, because this is the second-best
// answer and the label is what lets a report be honest about which findings rest
// on it.
func Derive(request DeriveRequest) (Database, error) {
	root := request.Root
	if root == "" {
		root = "."
	}
	sources, err := declaredSources(request)
	if err != nil {
		return Database{}, err
	}
	if len(sources) == 0 {
		// Not an error. A Rho project that has not been generated has no C at all,
		// and the `rho` engine still has its sources to read — refusing to hand back
		// an empty database would cost a run the one analysis it *can* do, in exchange
		// for protecting a database nobody needs. The caller that actually requires
		// entries — `compile-commands` — is where the absence is reported.
		from := "the project's module.yml"
		if request.Target != nil {
			from += fmt.Sprintf(" and the %s target spec", request.Target.Name)
		}
		return Database{Derived: true, From: from, Root: absolute(root)}, nil
	}

	compiler := CompilerFor(request.Target)
	defines := definesFor(request)
	includes := includesFor(request, root)
	directory := absolute(root)
	standard := standardFor(request)

	entries := make([]Entry, 0, len(sources))
	for _, source := range sources {
		arguments := make([]string, 0, len(defines)+len(includes)+6)
		arguments = append(arguments, compiler)
		// Only when the extension does not already say it. An empty argument is not
		// a no-op to a compiler or to an analyzer reading the database: it is a
		// filename, and the database would be describing a source called "".
		if flag := languageFlag(source); flag != "" {
			arguments = append(arguments, flag)
		}
		// The standard, when the manifest states one. `-std=c11` is the difference
		// between reading `_Bool` as a type and reading it as a typo, and it is one
		// of the flags an analyzer is most often left guessing.
		if standard != "" {
			arguments = append(arguments, standard)
		}
		for _, define := range defines {
			arguments = append(arguments, "-D"+define)
		}
		for _, include := range includes {
			arguments = append(arguments, "-I"+include)
		}
		arguments = append(arguments, "-c", source)
		entries = append(entries, Entry{
			Directory: directory,
			File:      source,
			Arguments: arguments,
		})
	}

	from := "the project's module.yml"
	if request.Target != nil {
		from += fmt.Sprintf(" and the %s target spec", request.Target.Name)
	}
	return Database{Entries: entries, Derived: true, From: from, Root: directory}, nil
}

// HostCompiler is the compiler a project that names no target is compiled with.
//
// Said rather than left empty: an analyzer given a database whose compiler is the
// empty string has no way to run at all, and a database that works on this machine
// is a real answer for the common case of a project that targets the host.
const HostCompiler = "cc"

// CompilerFor is the compiler a compilation for a target uses.
func CompilerFor(target *targetspec.Target) string {
	if target != nil {
		if compiler := strings.TrimSpace(target.Toolchain.Compiler); compiler != "" {
			return compiler
		}
	}
	return HostCompiler
}

// languageFlag is the `-x` that says what a source is, where its extension does
// not.
//
// Every source here is C, C++ or Rho and the compiler's extension rules know two
// of those, so the flag is only for Rho — and Rho's generated translation is `.c`
// anyway, which is the case where the flag would be wrong.
func languageFlag(source string) string {
	if LanguageOf(source) == LanguageCPP {
		return "-xc++"
	}
	return ""
}

// sourcesFor is what entries are made for, in the order they are reported.
func declaredSources(request DeriveRequest) ([]string, error) {
	root := request.Root
	if root == "" {
		root = "."
	}
	if len(request.Sources) > 0 {
		return absoluteAll(request.Sources, root), nil
	}

	manifest := request.Manifest
	if manifest != nil {
		declared := append([]string{}, manifest.BuildSources...)
		for _, directory := range manifest.BuildSourceDirs {
			found, err := sourcesInDirectory(filepath.Join(root, filepath.FromSlash(directory)))
			if err != nil {
				return nil, err
			}
			declared = append(declared, found...)
		}
		if len(declared) > 0 {
			return absoluteAll(declared, root), nil
		}
	}

	// Nothing declared, which for a Rho project is the ordinary case: `rho build`
	// works out the sources from the manifest it generates. They are the generated
	// project's own, so that is where they are read from.
	return absoluteAll(generatedSources(root), root), nil
}

// generatedSources are the sources a generated C project has, when it has been
// generated.
//
// A list rather than a walk of the whole build directory, because a walk finds the
// dependency trees as well — and a database covering a vendor's sources is a
// database whose findings are all about somebody else's code.
func generatedSources(root string) []string {
	directory := filepath.Join(root, "build", "generated", "c")
	if !present(directory) {
		return nil
	}
	found, err := sourcesInDirectory(directory)
	if err != nil {
		return nil
	}
	return found
}

// sourcesInDirectory is the C and Rho sources in a directory, one level deep and
// in name order.
//
// One level, which is where generated sources actually are, and a statement about
// where files live rather than a guess about where somebody put one.
func sourcesInDirectory(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read %s: %w", directory, err)
	}
	found := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".c", ".cpp", ".cc", ".rho":
			found = append(found, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(found)
	return found, nil
}

// definesFor are the macros every compilation for this project has.
func definesFor(request DeriveRequest) []string {
	defines := []string{}
	seen := map[string]bool{}
	add := func(values []string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			// Already there: the first one wins, so the board's define is not
			// shadowed by the project's statement of the same thing.
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			defines = append(defines, value)
		}
	}
	if request.Target != nil {
		add(request.Target.Defines)
	}
	if request.Manifest != nil {
		add(request.Manifest.BuildDefines)
	}
	return defines
}

// includesFor are the include directories every compilation for this project has.
func includesFor(request DeriveRequest, root string) []string {
	includes := []string{}
	if request.Manifest == nil {
		return includes
	}
	for _, directory := range request.Manifest.BuildIncludeDirs {
		includes = append(includes, resolveArgumentPath(Entry{Directory: absolute(root)}, directory))
	}
	return includes
}

// standardFor is the `-std` a compilation carries, or "" for none.
//
// Only what a manifest states. This toolchain generates C11 projects and the
// generated project says so in its own CMakeLists, so a derived database saying
// nothing is better than one asserting a standard the build may not use.
func standardFor(request DeriveRequest) string {
	if request.Target == nil || request.Manifest == nil {
		return ""
	}
	if len(request.Manifest.BuildPresets) == 0 && request.Manifest.BuildToolchain == "" {
		return ""
	}
	return DefaultStandard
}

// DefaultStandard is the C standard a generated project is compiled at, which is
// the one a derived database states when the project is built for a board.
//
// The same value the generated CMakeLists sets, read in one place rather than
// restated: a derived database claiming a standard the generated build does not
// use would be a database that differs from the real one for no stated reason.
const DefaultStandard = "-std=c11"

// BoardFor is the board a project's own manifest says a build is for, when the
// caller named none.
//
// `build.toolchain` first because it names one board; then the first entry of
// `build.presets`, which is a list because a program is not for one board and whose
// first entry is the one a build uses when it is given no choice. The examples in
// robohub-stm32 are this case: six presets, no toolchain, and every build of them
// is for one of the six.
func BoardFor(manifest *deps.Manifest, named string) string {
	if board := strings.TrimSpace(named); board != "" {
		return board
	}
	if manifest == nil {
		return ""
	}
	if toolchain := strings.TrimSpace(manifest.BuildToolchain); toolchain != "" {
		return toolchain
	}
	for _, preset := range manifest.BuildPresets {
		if trimmed := strings.TrimSpace(preset); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  Writing one                                                                */
/* -------------------------------------------------------------------------- */

// WriteDatabase writes a compilation database out.
//
// Written where the settings say rather than over anything already there. A
// database a CMake build wrote is the accurate one and this would be replacing it
// with a worse one, so writing over an existing database is refused unless the
// caller has said it means it.
func WriteDatabase(path string, database Database, overwrite bool) error {
	if existing(path) && !overwrite {
		return fmt.Errorf(
			"%s already exists; it is the accurate one, so it is not replaced.\n"+
				"  Pass --force to replace it anyway", path)
	}
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("cannot create %s: %w", directory, err)
		}
	}
	encoded, err := json.MarshalIndent(database.Entries, "", "  ")
	if err != nil {
		return fmt.Errorf("the compilation database could not be encoded: %w", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("the compilation database could not be written: %w", err)
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  What the database is about                                                 */
/* -------------------------------------------------------------------------- */

// Units are the sources a run analyses, each with the language it is in.
//
// One type for both kinds of project, which is what lets a Rho application and a
// hand-written C one go through the same run: the generated C is a unit like any
// other, and the `.rho` it came from is a unit of the other language. An engine
// declares the languages it reads and is given the units that are its own.
type Unit struct {
	// File is the source, absolute.
	File string
	// Language is one of the languages here.
	Language string
	// From is what asked for this source: the compilation database, the project's
	// own list, or the command line.
	From string
}

// UnitsOf is what a database describes, one unit per compilation.
func (d Database) UnitsOf() []Unit {
	units := make([]Unit, 0, len(d.Entries))
	seen := map[string]bool{}
	for _, entry := range d.Entries {
		source := entry.Source()
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		units = append(units, Unit{
			File:     source,
			Language: entry.Language(),
			From:     "the compilation database",
		})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].File < units[j].File })
	return units
}

// FilesOf is the sources in a list of units, of the languages named.
//
// A language named as `c` takes C++ with it, because an analyzer that reads C
// reads C++ — that is what a superset means — and an engine that wanted to be
// fussy about it would be the only thing in the run that is.
func FilesOf(units []Unit, languages []string) []string {
	wanted := map[string]bool{}
	header := false
	for _, language := range languages {
		switch strings.ToLower(strings.TrimSpace(language)) {
		case LanguageC:
			wanted[LanguageC] = true
			wanted[LanguageCPP] = true
			wanted[LanguageHeader] = true
			header = true
		case LanguageCPP:
			wanted[LanguageCPP] = true
			wanted[LanguageHeader] = true
			header = true
		case LanguageHeader:
			wanted[LanguageHeader] = true
			header = true
		default:
			wanted[strings.ToLower(strings.TrimSpace(language))] = true
		}
	}

	found := []string{}
	for _, unit := range units {
		if !wanted[unit.Language] {
			continue
		}
		found = append(found, unit.File)
	}
	if header {
		// A header is not compiled, so it is never in a database. It is read
		// because the C rules are about what a header declares, and the only way to
		// know which headers a project has is to look beside its sources.
		for _, unit := range units {
			if wanted[unit.Language] && unit.Language != LanguageHeader {
				found = append(found, siblingHeaders(unit.File)...)
			}
		}
	}
	return dedupe(found)
}

// siblingHeaders are the headers beside a source: the one with its own name and
// the one every C project's sources include.
func siblingHeaders(source string) []string {
	directory, name := filepath.Split(source)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	found := []string{}
	for _, candidate := range []string{base + ".h", base + ".hpp"} {
		path := filepath.Join(directory, candidate)
		if existing(path) {
			found = append(found, path)
		}
	}
	return found
}

func dedupe(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

/* -------------------------------------------------------------------------- */
/*  What is left out                                                           */
/* -------------------------------------------------------------------------- */

// OutsideExclude and OutsideInclude are what to do about a source outside the
// project's own directory.
//
// A setting rather than a fragment because the two cases are opposites and a
// fragment can only ever mean one of them. An application's generated project
// compiles its platform's sources beside its own and none of them are the
// application's to fix; a platform linted as a project in its own right has
// sources that, from a consumer's point of view, are exactly a dependency's path.
// The same code is linted once by whoever owns it.
const (
	OutsideExclude = "exclude"
	OutsideInclude = "include"
)

// Exclusions decides which of a project's sources a run reads.
//
// The decision is made once here and shown in the run's output rather than applied
// silently per engine, because "no findings" has two very different causes — the
// code is clean, and half the code was never read — and a report that cannot tell
// them apart is a report that will be believed the first time it is wrong.
type Exclusions struct {
	// Root is the project directory. Empty means every path is outside it, which
	// with `exclude` would leave nothing to read; a caller with no root is a
	// caller linting explicit paths, so outside-ness is not decided here.
	Root string
	// Fragments are path sequences that exclude a source wherever they appear.
	Fragments []string
	// Outside is `exclude` or `include`. Anything else is read as `exclude`, which
	// is the answer that cannot overstate what was analysed.
	Outside string
}

// Keeps reports whether a source is read.
//
// A fragment matches when it appears as a path sequence in the source's path or in
// its path relative to the project — either, because the checkout a dependency
// came from is usually somewhere else on the disk entirely and only the absolute
// path says so.
func (e Exclusions) Keeps(unit Unit) bool {
	for _, fragment := range e.Fragments {
		fragment = strings.Trim(strings.TrimSpace(fragment), "/")
		if fragment == "" {
			continue
		}
		if containsSequence(unit.File, fragment) {
			return false
		}
		if e.Root != "" {
			if relative, err := filepath.Rel(e.Root, unit.File); err == nil {
				if containsSequence(filepath.ToSlash(relative), fragment) {
					return false
				}
			}
		}
	}
	if e.Outside == OutsideInclude || e.Root == "" {
		return true
	}
	relative, err := filepath.Rel(e.Root, unit.File)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// Filter is the units a run reads.
func (e Exclusions) Filter(units []Unit) []Unit {
	kept := make([]Unit, 0, len(units))
	for _, unit := range units {
		if e.Keeps(unit) {
			kept = append(kept, unit)
		}
	}
	return kept
}

// LeftOut is how many units a set of exclusions dropped, for the run's own output.
//
// Counted and reported rather than not, for the reason at the top of this section.
func (e Exclusions) LeftOut(all []Unit) int { return len(all) - len(e.Filter(all)) }

// containsSequence reports whether a path has a fragment as a path sequence.
//
// Whole components, not a substring: `vendor` matching `/home/me/vendorstuff/` is
// a rule that excludes code because of what a directory is called near it, and a
// person who wrote `vendor/` would not expect that. A fragment may still name a
// partial component — `build/generated` — because a `/` inside it is what says
// there is more than one component.
func containsSequence(path string, fragment string) bool {
	normalised := strings.Trim(filepath.ToSlash(path), "/")
	parts := strings.Split(normalised, "/")
	wanted := strings.Split(fragment, "/")
	if len(wanted) > len(parts) {
		return false
	}
	for start := 0; start+len(wanted) <= len(parts); start++ {
		if matchesParts(parts[start:start+len(wanted)], wanted) {
			return true
		}
	}
	return false
}

func matchesParts(parts []string, wanted []string) bool {
	for index, one := range wanted {
		// A leading `*` makes this component a suffix match, which is the one
		// wildcard worth having: `*_generated` says "any directory that ends in
		// _generated" without also saying anything about a file.
		if strings.HasPrefix(one, "*") {
			if !strings.HasSuffix(parts[index], strings.TrimPrefix(one, "*")) {
				return false
			}
			continue
		}
		if parts[index] != one {
			return false
		}
	}
	return true
}

func absolute(path string) string {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return resolved
}

func absoluteAll(paths []string, root string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if filepath.IsAbs(path) {
			out = append(out, filepath.Clean(path))
			continue
		}
		out = append(out, filepath.Join(absolute(root), filepath.FromSlash(path)))
	}
	return dedupe(out)
}

// splitCommand reads a command string into its arguments, with quotes honoured.
//
// The same spelling the shared plugin runner uses, for the same reason: a path
// with a space in it is one argument, and splitting it wrong produces a database
// that points at a file that is not there.
func splitCommand(value string) []string {
	out := []string{}
	current := strings.Builder{}
	quote := rune(0)
	started := false
	for _, c := range value {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			current.WriteRune(c)
		case c == '\'' || c == '"':
			quote = c
			started = true
		case c == ' ' || c == '\t':
			if started {
				out = append(out, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(c)
			started = true
		}
	}
	if started {
		out = append(out, current.String())
	}
	return out
}
