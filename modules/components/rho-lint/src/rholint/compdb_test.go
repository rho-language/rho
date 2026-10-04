//
// @file compdb_test.go
// @brief Tests for the compilation database: finding, reading and deriving one.
//
// The examples in board-lib are the material, and the two of them are the two
// cases this file has to get right. `examples/c/hello_world` declares its source and
// names six boards; `examples/rho/hello_world` declares neither and has a generated C
// project beside it. A run over one is a run over a declared source, and a run over
// the other is a run over a derived one — and both have to end with an analyzer being
// told the right compiler and the right defines.
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
	"os"
	"path/filepath"
	"testing"

	"rho/src/deps"
	"rho/src/targetspec"
)

// writeFile is a file in a temporary tree, for a test that needs a project.
func writeFile(t *testing.T, path string, text string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// aManifest is a manifest written for a test that needs one.
func aManifest(t *testing.T, root string, text string) *deps.Manifest {
	t.Helper()
	writeFile(t, filepath.Join(root, "module.yml"), text)
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil {
		t.Fatalf("the manifest does not load: %v", err)
	}
	return manifest
}

// aTarget is a resolved target spec, which is where the compiler and the board's own
// defines come from.
func aTarget() *targetspec.Target {
	return &targetspec.Target{
		Name:      "nucleo-h753zi",
		Toolchain: targetspec.Toolchain{Compiler: "arm-none-eabi-gcc"},
		Defines:   []string{"STM32H753xx", "USE_HAL_DRIVER"},
	}
}

/* -------------------------------------------------------------------------- */

func TestReadDatabaseReadsBothSpellingsOfACommand(t *testing.T) {
	// `arguments` is a list and `command` is one string, and a database written by
	// CMake has the first while one written by a person's script has the second. The
	// analyzer must be handed the same thing either way.
	arguments := filepath.Join(t.TempDir(), "args.json")
	writeFile(t, arguments, `[
	  {"directory": "/project", "file": "/project/src/main.c",
	   "arguments": ["arm-none-eabi-gcc", "-DSTM32H753xx", "-c", "/project/src/main.c"]}
	]`)
	command := filepath.Join(t.TempDir(), "cmd.json")
	writeFile(t, command, `[
	  {"directory": "/project", "file": "/project/src/main.c",
	   "command": "arm-none-eabi-gcc -DSTM32H753xx -c /project/src/main.c"}
	]`)

	for _, path := range []string{arguments, command} {
		database, err := ReadDatabase(path)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
		if len(database.Entries) != 1 {
			t.Fatalf("%s: %d entries, want 1", filepath.Base(path), len(database.Entries))
		}
		if got := database.Entries[0].Compiler(); got != "arm-none-eabi-gcc" {
			t.Errorf("%s: compiler %q, want arm-none-eabi-gcc", filepath.Base(path), got)
		}
		if got := database.Entries[0].Source(); got != "/project/src/main.c" {
			t.Errorf("%s: source %q, want /project/src/main.c", filepath.Base(path), got)
		}
	}
}

func TestReadDatabaseRefusesAFileThatIsNotOne(t *testing.T) {
	// The failure that matters most: a database that cannot be read must not become
	// a run that analyzed nothing and reported success.
	for name, text := range map[string]string{
		"object":        `{"version": 3}`,
		"empty":         `[]`,
		"nameless":      `[{"directory": "/project", "arguments": ["gcc", "-c"]}]`,
		"not even json": `compile_commands.json is not what it says`,
	} {
		path := writeFile(t, filepath.Join(t.TempDir(), "compile_commands.json"), text)
		if _, err := ReadDatabase(path); err == nil {
			t.Errorf("%s: a database that is not one was accepted", name)
		}
	}
}

func TestEntryLanguagePrefersTheFlagOverTheExtension(t *testing.T) {
	// A compiler told `-x c++` about a `.c` file compiles it as C++, and an analyzer
	// that believed the extension would be told the wrong language.
	entry := Entry{
		File:      "src/main.c",
		Arguments: []string{"clang++", "-x", "c++", "-c", "src/main.c"},
	}
	if got := entry.Language(); got != LanguageCPP {
		t.Errorf("language %q, want %q", got, LanguageCPP)
	}
	plain := Entry{File: "src/main.c", Arguments: []string{"gcc", "-c", "src/main.c"}}
	if got := plain.Language(); got != LanguageC {
		t.Errorf("language %q, want %q", got, LanguageC)
	}
	joined := Entry{File: "a.c", Arguments: []string{"gcc", "-xc++", "-c", "a.c"}}
	if got := joined.Language(); got != LanguageCPP {
		t.Errorf("joined -x form: language %q, want %q", got, LanguageCPP)
	}
}

func TestEntryReadsDefinesAndIncludes(t *testing.T) {
	entry := Entry{
		Directory: "/project",
		File:      "src/main.c",
		Arguments: []string{
			"gcc", "-DFIRST", "-D", "-DSECOND=2",
			"-Iinclude", "-I", "src", "-isystem", "generated",
			// A bare `-I` at the end is a compiler error, and the argument after it is
			// not a directory: consuming it would describe a directory called `-DSIZE`.
			"-I", "-DSIZE=1",
			"-c", "src/main.c",
		},
	}
	defines := entry.Defines()
	if len(defines) != 3 || defines[0] != "FIRST" || defines[2] != "SIZE=1" {
		t.Errorf("defines %v, want FIRST, SECOND=2 and SIZE=1", defines)
	}
	includes := entry.Includes()
	want := []string{"/project/include", "/project/src", "/project/generated"}
	if len(includes) != len(want) {
		t.Fatalf("includes %v, want %v", includes, want)
	}
	for index, one := range want {
		if includes[index] != one {
			t.Errorf("includes %v, want %v", includes, want)
			break
		}
	}
}

func TestSplitCommandHonoursQuotes(t *testing.T) {
	// A path with a space in it is one argument, and splitting it wrong produces a
	// database pointing at a file that is not there.
	got := splitCommand(`gcc -I"/opt/my tools" -o out.o main.c`)
	if len(got) != 5 {
		t.Fatalf("got %v, want five arguments", got)
	}
	if got[1] != "-I/opt/my tools" {
		t.Errorf("second argument %q, want the quoted path whole", got[1])
	}
}

/* -------------------------------------------------------------------------- */

func TestWherePrefersTheBoardsOwnBuildDirectory(t *testing.T) {
	// A project written for six boards has six databases and they disagree about
	// which branches of a header exist. Reading the newest of them would be an
	// analysis of whichever board was built last.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "generated", "c", "build",
		"nucleo-f753zi", "compile_commands.json"), databaseFor("/project/f753/app.c"))
	writeFile(t, filepath.Join(root, "build", "generated", "c", "build",
		"nucleo-h753zi", "compile_commands.json"), databaseFor("/project/h753/app.c"))

	path, where := Where(root, "", "nucleo-h753zi")
	if filepath.Base(filepath.Dir(path)) != "nucleo-h753zi" {
		t.Errorf("read %s, want the h753zi one", path)
	}
	if where == "" {
		t.Error("no description of where it was found")
	}
}

func TestWhereFallsBackToTheBoardNeutralPlace(t *testing.T) {
	// Each of the places a project produces one in, in the order they are looked for.
	// `out/` is here because `cmake -B out` is what this toolchain's own handbook
	// tells a person to write, so a project that followed it has its database there
	// and nowhere else.
	for _, place := range []string{
		"build/compile_commands.json",
		"compile_commands.json",
		"out/compile_commands.json",
		"build/generated/c/compile_commands.json",
		"cmake-build-debug/compile_commands.json",
	} {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, filepath.FromSlash(place)), databaseFor("/project/app.c"))
		path, _ := Where(root, "", "nucleo-h753zi")
		if want := filepath.Join(root, filepath.FromSlash(place)); path != want {
			t.Errorf("read %s, want %s", path, want)
		}
	}
}

func TestWhereNamesAMissingDeclaredDatabase(t *testing.T) {
	// A database somebody pointed at is used or reported missing. Never replaced by
	// one of the other places: they would be for a different board.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "compile_commands.json"), databaseFor("/project/app.c"))

	path, where := Where(root, "build/other.json", "nucleo-h753zi")
	if path == "" {
		t.Fatal("no path reported for a declared database that is not there")
	}
	if where == "" || !contains(where, "not there") {
		t.Errorf("described as %q, want it reported missing", where)
	}
}

func databaseFor(source string) string {
	encoded, _ := json.Marshal([]Entry{{Directory: "/", File: source,
		Arguments: []string{"gcc", "-c", source}}})
	return string(encoded)
}

func contains(haystack string, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for index := 0; index+len(needle) <= len(haystack); index++ {
			if haystack[index:index+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

/* -------------------------------------------------------------------------- */

func TestDeriveUsesTheBoardsCompilerAndDefines(t *testing.T) {
	// The board's compiler and the board's defines, because both are facts
	// rho-target holds and its plugins extend. A database naming `gcc` for a board
	// built with `arm-none-eabi-gcc` makes every analyzer parse out the wrong
	// branches.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.c"), "int main(void) { return 0; }\n")
	manifest := aManifest(t, root, `module:
  name: hello-world
  type: module
  version: 0.1.0
main: src/main.c
build:
  language: c
  sources:
    - src/main.c
`)

	database, err := Derive(DeriveRequest{Root: root, Manifest: manifest, Target: aTarget()})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(database.Entries) != 1 {
		t.Fatalf("%d entries, want 1", len(database.Entries))
	}
	arguments := database.Entries[0].Arguments
	if arguments[0] != "arm-none-eabi-gcc" {
		t.Errorf("compiler %q, want arm-none-eabi-gcc", arguments[0])
	}
	if !contains(joined(arguments), "-DSTM32H753xx") {
		t.Errorf("the board's define is missing from %v", arguments)
	}
	// An empty argument is a filename to a compiler, not a no-op.
	for _, argument := range arguments {
		if argument == "" {
			t.Errorf("an empty argument in %v", arguments)
		}
	}
	if !database.Derived {
		t.Error("a derived database is not labelled as derived")
	}
}

// joined is a command as one line, which is what the assertions read against.
func joined(arguments []string) string {
	out := ""
	for _, argument := range arguments {
		out += argument + " "
	}
	return out
}

func TestDeriveFallsBackToTheGeneratedProject(t *testing.T) {
	// A Rho project declares no sources: `rho build` works them out from the
	// manifest it generates. So a run over one is a run over the generated project's
	// own sources — and only its own, because a walk of the whole build directory
	// would find the platform's HAL as well and every finding would be about
	// somebody else's code.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "module.yml"), `module:
  name: hello-world
  type: module
  version: 0.1.0
main: src/main.rho
`)
	writeFile(t, filepath.Join(root, "src", "main.rho"), "module main\n")
	writeFile(t, filepath.Join(root, "build", "generated", "c", "main.c"), "int main(void){}\n")
	writeFile(t, filepath.Join(root, "build", "generated", "c", "runtime.c"), "/* runtime */\n")
	writeFile(t, filepath.Join(root, "build", "generated", "c", "vendor", "hal.c"), "/* hal */\n")

	database, err := Derive(DeriveRequest{Root: root})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(database.Entries) != 2 {
		t.Fatalf("%d entries, want main.c and runtime.c: %v", len(database.Entries), database.Entries)
	}
	for _, entry := range database.Entries {
		if contains(entry.File, "hal.c") {
			t.Errorf("a nested dependency's source got into the database: %s", entry.File)
		}
	}
}

func TestDeriveOfAProjectWithNoCIsEmptyRatherThanAnError(t *testing.T) {
	// A Rho project that has not been generated has no C at all, and the `rho` engine
	// still has its sources to read. Refusing to hand back an empty database would
	// cost a run the one analysis it can do, to protect a database nobody needs.
	// Where the absence *is* an error is `compile-commands`, which exists to write one.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.rho"), "module main\n")

	database, err := Derive(DeriveRequest{Root: root})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(database.Entries) != 0 {
		t.Errorf("derived %d entries for a project with no C", len(database.Entries))
	}
	if !database.Derived {
		t.Error("an empty database is not labelled as derived, and a run would say it read one")
	}
}

func TestBoardForPrefersTheBoardsOwnName(t *testing.T) {
	// Six presets and no toolchain, which is the shape both board-lib examples
	// have: the first preset is what a build uses when it is given no choice.
	root := t.TempDir()
	manifest := aManifest(t, root, `module:
  name: hello-world
  type: module
  version: 0.1.0
main: src/main.c
build:
  language: c
  presets:
    - nucleo-f446ze
    - nucleo-h753zi
`)
	if got := BoardFor(manifest, ""); got != "nucleo-f446ze" {
		t.Errorf("board %q, want the first preset", got)
	}
	if got := BoardFor(manifest, "nucleo-h753zi"); got != "nucleo-h753zi" {
		t.Errorf("board %q, want the named one to win", got)
	}
}

func TestCompilerForFallsBackToThisMachine(t *testing.T) {
	// A project with no board has no cross compiler, and a database whose compiler is
	// the empty string cannot be used by anything.
	if got := CompilerFor(nil); got != HostCompiler {
		t.Errorf("compiler %q, want %q", got, HostCompiler)
	}
}

func TestWriteDatabaseRefusesToReplaceOne(t *testing.T) {
	// A database a build wrote is the accurate one, and replacing it with a derived
	// one would make every later run worse and say nothing about having done it.
	path := filepath.Join(t.TempDir(), "compile_commands.json")
	if err := WriteDatabase(path, Database{Entries: []Entry{{File: "/a.c"}}}, false); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := WriteDatabase(path, Database{Entries: []Entry{{File: "/b.c"}}}, false); err == nil {
		t.Error("an existing database was replaced without being asked to")
	}
	if err := WriteDatabase(path, Database{Entries: []Entry{{File: "/b.c"}}}, true); err != nil {
		t.Errorf("forced write: %v", err)
	}
	database, err := ReadDatabase(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if database.Entries[0].File != "/b.c" {
		t.Errorf("the forced write did not take: %s", database.Entries[0].File)
	}
}

/* -------------------------------------------------------------------------- */

func TestUnitsOfAndFilesOf(t *testing.T) {
	database := Database{Entries: []Entry{
		{Directory: "/project", File: "src/main.c", Arguments: []string{"gcc", "-c", "src/main.c"}},
		{Directory: "/project", File: "src/main.cpp", Arguments: []string{"g++", "-xc++", "-c", "src/main.cpp"}},
		{Directory: "/project", File: "src/main.c", Arguments: []string{"gcc", "-c", "src/main.c"}},
	}}
	units := database.UnitsOf()
	if len(units) != 2 {
		t.Fatalf("%d units, want 2 — the same source twice is one unit", len(units))
	}

	// An engine reading C gets C++ with it, because that is what a superset means.
	files := FilesOf(units, []string{LanguageC})
	if len(files) != 2 {
		t.Errorf("C engine got %v, want both the C and the C++", files)
	}
	if got := FilesOf(units, []string{LanguageRho}); len(got) != 0 {
		t.Errorf("Rho engine got %v, want nothing", got)
	}
}

func TestGeneratedModulesAreTheNamesTheBuildWrote(t *testing.T) {
	// The `ffi` module in a Rho application: its declarations were generated from a
	// platform's headers, so a source importing it names none of them and uses every
	// one. The unused-import rule needs to know this to avoid firing on every
	// application with a C interface.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "build", "generated", "modules", "ffi.rho"), "module ffi\n")
	writeFile(t, filepath.Join(root, "build", "generated", "modules", "notes.txt"), "ignored")

	names := GeneratedModules(root)
	if !names["ffi"] {
		t.Errorf("generated modules %v, want ffi", names)
	}
	if names["notes"] {
		t.Errorf("a non-source file was taken for a module: %v", names)
	}
	if len(GeneratedModules(t.TempDir())) != 0 {
		t.Error("a project with nothing generated reported generated modules")
	}
}

func TestRhoSourcesSkipTheDirectoriesThatAreNotTheProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.rho"), "module main\n")
	writeFile(t, filepath.Join(root, "std", "os.rho"), "module os\n")
	writeFile(t, filepath.Join(root, "vendor", "x.rho"), "module x\n")
	writeFile(t, filepath.Join(root, "build", "generated", "c", "src", "main.rho"), "module main\n")

	found := RhoSources(root)
	if len(found) != 1 || filepath.Base(found[0]) != "main.rho" {
		t.Errorf("found %v, want only the project's own main.rho", found)
	}
	if !contains(found[0], "src") {
		t.Errorf("found %v, want the one under src", found)
	}
}

/* -------------------------------------------------------------------------- */

func TestExclusionsMatchWholePathComponents(t *testing.T) {
	// `vendor` matching `/home/me/vendorstuff/` would exclude code because of what a
	// directory is called near it, which is not what anybody means by `vendor`.
	cases := []struct {
		path     string
		fragment string
		want     bool
	}{
		{"/project/vendor/stm32/x.c", "vendor", true},
		{"/project/x/vendor/y.c", "vendor", true},
		{"/project/vendorstuff/x.c", "vendor", false},
		{"/project/src/main.c", "vendor", false},
		{"/project/build/generated/c/main.c", "build/generated", true},
		{"/project/buildout/main.c", "build", false},
		{"/project/buildout/main.c", "*out", true},
	}
	for _, one := range cases {
		if got := containsSequence(one.path, one.fragment); got != one.want {
			t.Errorf("containsSequence(%q, %q) = %v, want %v", one.path, one.fragment, got, one.want)
		}
	}
}

func TestExclusionsKeepTheProjectAndDropAnotherCheckout(t *testing.T) {
	// An application's generated project compiles its platform's sources beside its
	// own, and none of them are the application's to fix. A platform analyzed as a
	// project in its own right has the opposite answer, and the setting is what
	// decides which.
	root := t.TempDir()
	units := []Unit{
		{File: filepath.Join(root, "src", "main.c")},
		{File: "/elsewhere/platform/src/hal.c"},
	}
	excluding := Exclusions{Root: root, Outside: OutsideExclude}
	if len(excluding.Filter(units)) != 1 {
		t.Errorf("excluding kept %v, want only the project's own", excluding.Filter(units))
	}
	including := Exclusions{Root: root, Outside: OutsideInclude}
	if len(including.Filter(units)) != 2 {
		t.Errorf("including kept %v, want both", including.Filter(units))
	}
}

func TestRelativeKeepsPathsInsideTheRootAndAdmitsTheRest(t *testing.T) {
	// A report that says `/home/me/...` says nothing about the code; a SARIF `uri`
	// that differs per machine cannot be grouped. But a `..` chain resolves against
	// the wrong base to a file that is not there, so those stay absolute.
	root := filepath.Join("/project", "")
	if got := Relative(filepath.Join(root, "src", "main.c"), root); got != "src/main.c" {
		t.Errorf("Relative = %q, want src/main.c", got)
	}
	if got := Relative("/elsewhere/x.c", root); got != "/elsewhere/x.c" {
		t.Errorf("Relative = %q, want the absolute path", got)
	}
	if got := Relative("", root); got != "" {
		t.Errorf("Relative = %q, want empty", got)
	}
}
