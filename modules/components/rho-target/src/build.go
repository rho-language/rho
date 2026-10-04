//
// @file build.go
// @brief `rho target build` — build a generated project for a named target.
//
// This is the embedded half of `rho build`. The toolchain transpiles Rho to C
// and writes a project that is a real CMake project; this command is what turns
// that project into a binary for a particular piece of hardware. It writes the
// files the target needs — the toolchain file, the flags and memory map, the
// linker script — and then hands the project to CMake.
//
// The compiler, the memory map and the flags are read from a target spec rather
// than decided here, so that the same build runs for a Raspberry Pi Pico, an
// ESP32-C3 and the machine it is sitting on. There is no separate path for the
// host: the host is the target named `native`, whose compiler is the one already
// installed.
//
// @project Rho
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
	"os/exec"
	"path/filepath"
	"strings"

	"rho/src/deps"

	"github.com/rho/rho-target/src/rhtarget"
)

// build compiles a generated project for a target.
//
// The steps are the ones a board build always has, in the order they have to
// happen: resolve what the target is, write the files that describe it into the
// project, then configure and build it. Writing them before configuring is what
// makes the toolchain file take effect — CMake reads it to learn which compiler
// it is configuring for, so a file written after `project()` is too late.
// linkMapFlag declares whether a link writes its map, and how it is asked for.
//
// `on`/`off` rather than a boolean flag because a project declares the same
// choice and the two have to be spellable the same way: `rho build --no-link-map`
// and `build.link_map: false` are one setting, and a setting that could be
// written two ways and read as two different things is a setting nobody trusts.
//
// It is read as a word rather than declared as a boolean, because a boolean flag
// that took `on` would refuse it — and `on` is the spelling the manifest uses for
// the same fact, so the two have to accept the same words.
//
// The default is on. The map is a file the linker writes as a byproduct of work
// it is already doing, and a project that wants the memory a build used should
// not have to know that a flag exists to ask.
type linkMapValue struct{ given *bool }

func (v linkMapValue) String() string {
	if v.given == nil {
		return "on"
	}
	if *v.given {
		return "on"
	}
	return "off"
}

func (v linkMapValue) Set(raw string) error {
	switch strings.TrimSpace(raw) {
	case "on", "true", "yes", "1":
		*v.given = true
	case "off", "false", "no", "0":
		*v.given = false
	default:
		return fmt.Errorf("expected on or off, got %q", raw)
	}
	return nil
}

func linkMapFlag(fs *flag.FlagSet) *bool {
	given := new(bool)
	*given = true
	fs.Var(linkMapValue{given}, "link-map", "write the linker map beside the program (on or off)")
	return given
}

func build(args []string) int {
	fs := flag.NewFlagSet("rho target build", flag.ExitOnError)
	target := fs.String("target", "", "the target to build for")
	project := fs.String("project", "", "the generated project to build")
	// `for` is the project whose dependencies are read for the targets they
	// declare. It is a different directory from the generated project: a board is
	// declared by the platform the application depends on, and that dependency is
	// named in the application's manifest, which is not in the generated tree.
	//
	// Defaulting it to the generated project's parent would be a guess about the
	// directory layout, and a build that found a target by walking upwards would
	// be reading a relationship it had not been told.
	forProject := fs.String("for", "", "the project whose dependencies declare the target (default: the generated project)")
	buildDir := fs.String("build-dir", "", "where the build tree goes (default: <project>/build/<target>)")
	kind := fs.String("type", "Release", "the CMake build type")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	treePlugins := fs.Bool("tree-plugins", false, "also read the plugins the tree carries")
	// Whether the link writes its map. On by default, because the map is how a
	// build is explained: which region a byte landed in and which object put it
	// there. A build that turned it off would still be correct and would still be
	// the build somebody has to rebuild with a flag to understand.
	linkMap := linkMapFlag(fs)
	// Flags are read wherever they appear, because `rho target build --target pico`
	// is what a person types first.
	flagArgs, rest := splitArgs(args, map[string]bool{"target": true, "project": true, "for": true, "build-dir": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "rho target build: --target names the target to build for")
		return 2
	}

	root := "."
	if *project != "" {
		root = *project
	} else if len(rest) > 0 {
		root = rest[0]
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho target:", err)
		return 1
	}

	// The project's own dependencies are where the platform's specs are declared,
	// so that project is read for them before the library is built. A target a
	// dependency declares is a target, and a build that could not see it would
	// report the board as one that does not exist.
	specSource := absolute
	if *forProject != "" {
		if declared, absErr := filepath.Abs(*forProject); absErr == nil {
			specSource = declared
		}
	}
	withDeclaredSpecs(specSource)
	library := library(*treePlugins)
	spec, diagnostics := library.Resolve(*target)
	report(append(library.Diagnostics, diagnostics...))
	if spec.Name == "" {
		fmt.Fprintf(os.Stderr, "rho target build: no target named %q. `rho target list` names the ones there are.\n", *target)
		return 1
	}
	spec.LinkMap = *linkMap

	// Checked before the project's files are written: a target whose compiler is
	// not installed cannot be built by anything that follows, and finding out
	// after the project's cmake/ directory has been overwritten with another
	// target's flags leaves a tree that is wrong in a way nobody asked for.
	if err := toolchainIsAvailable(spec); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}

	// The files a target needs go into the project's own cmake/ directory,
	// because that is where the generated CMakeLists includes them from. They are
	// written every time rather than only when missing: a spec can change under a
	// checkout, and a build that used the previous flags would produce a binary
	// that does not match the spec it was built from.
	// Where the finished binary goes, taken from the project's own manifest.
	//
	// It is a contract rather than a fixed place because a project keeps its
	// binaries somewhere it chose, and a build that decided would either ignore
	// that or overwrite one target's binary with another's. Inside the build
	// directory either way: a binary is a build output.
	output := ""
	if project, manifestErr := deps.LoadManifest(deps.ManifestPath(specSource)); manifestErr == nil && project != nil {
		output = project.OutputDirectory()
		if !filepath.IsAbs(output) {
			output = filepath.Join(specSource, output)
		}
	}

	// Where the finished binary is written, told to CMake rather than moved
	// afterwards.
	//
	// CMake's own output directory is the right mechanism because it is the one
	// that survives: a file copied after the build is a second copy of something
	// that already exists somewhere else, and the first one is the one a
	// subsequent incremental build would still consider current.
	if output != "" {
		if err := os.MkdirAll(output, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "rho target build:", err)
			return 1
		}
	}

	// The dependency declarations go in before the target's own files, because
	// both have to be on disk before CMake configures and the order they are
	// written in has no other meaning.
	//
	// It is written from the application's manifest, which is where its
	// dependencies are named — not from the generated directory, which is a
	// build output and says nothing about what this build depends on.
	if err := writeDepsFiles(specSource, filepath.Join(absolute, depsFileName)); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}

	// The HAL's configuration header, written into the platform's vendor tree if
	// it is not already there.
	//
	// Here rather than in `writeTargetFiles`, because that one writes what a target
	// spec *declares* into the generated project, and this is what a vendor's headers
	// *require* and ST does not ship: the umbrella header includes a configuration
	// header by a bare name, and the only version of that file anywhere is the
	// vendor's template and whatever somebody derived from it by hand. It goes with
	// the vendor tree rather than into the generated project because it is a
	// property of the family, and two projects of different families should not each
	// end up with their own copy of the same file.
	//
	// Both halves of this are part of a build for the same reason: a project should
	// be able to be built from a checkout that contains nothing but sources, with
	// every vendor repository fetched and every file a vendor needs but does not
	// ship produced along the way.
	if err := writeHalConf(specSource, spec); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}

	if err := writeTargetFiles(absolute, spec); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}

	// The flags are also written into the project, beside the target's own
	// files, for two reasons.
	//
	// A definition passed only on the command line is invisible to anything that
	// reads the project rather than building it, and CMake reports one that no
	// file mentions as unused — which for a flag that decides whether a test is
	// registered is a confusing way to say that nobody looked at it.
	//
	// And they belong with the target's own declarations. The compiler, the flags
	// and the memory map are what this target is; so is the fact that this build
	// is for a microcontroller, and where it says that it should say in the same
	// place and the same way.
	if err := writeBuildFlags(absolute, specSource, *target, output); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}

	tree := *buildDir
	if tree == "" {
		tree = filepath.Join(absolute, "build", *target)
	}

	// What this build may register is decided before CMake is asked anything,
	// from what the project's dependencies declared about their own tests. The
	// answer is passed as definitions rather than inferred by a build file, so a
	// build file that registers a test for a target that cannot run it is a
	// build file that was not told.
	if err := configureAndBuild(absolute, tree, output, specSource, *target, *kind, *quiet); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}
	return 0
}

// writeTargetFiles writes the CMake and linker files a target needs into a
// project, and returns the first thing that went wrong.
func writeTargetFiles(project string, spec rhtarget.Spec) error {
	for _, file := range rhtarget.Files(spec, "cmake") {
		destination := filepath.Join(project, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(destination, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// configureAndBuild runs CMake over a generated project.
//
// Two commands rather than one because they fail for different reasons and a
// reader needs to know which: a configure failure is a project or a spec that
// does not make sense, and a build failure is a compile error.
//
// Both are run with their output captured rather than connected straight to this
// process's, because a failure needs the output in the error — the line the
// compiler complained about is the useful part. Captured output is echoed as it
// arrives rather than held until the end, so a long build still shows progress.
func configureAndBuild(project, tree, output, specSource, targetName, buildType string, quiet bool) error {
	if _, err := exec.LookPath("cmake"); err != nil {
		return fmt.Errorf("cmake is required to build a target: %w", err)
	}

	arguments := []string{"-S", project, "-B", tree, "-DCMAKE_BUILD_TYPE=" + buildType}
	arguments = append(arguments, testFlagsFor(specSource, targetName)...)

	configure := exec.Command("cmake", arguments...)
	if out, err := configure.CombinedOutput(); err != nil {
		if !quiet {
			os.Stderr.Write(out)
		}
		return fmt.Errorf("cmake could not configure the project for this target: %w", err)
	} else if !quiet {
		os.Stdout.Write(out)
	}

	compile := exec.Command("cmake", "--build", tree, "--parallel")
	if out, err := compile.CombinedOutput(); err != nil {
		if !quiet {
			os.Stderr.Write(out)
		}
		return fmt.Errorf("the project did not build: %w", err)
	} else if !quiet {
		os.Stdout.Write(out)
	}

	if !quiet {
		fmt.Println("Built:", tree)
	}
	return nil
}

// toolchainIsAvailable reports whether the compiler a spec names can be run.
//
// It is checked before anything is written or configured, because "arm-none-eabi-gcc
// is not installed" and "the linker script has a syntax error" are both reported
// by CMake as a configure failure, and only one of them is fixed by installing a
// package. Naming the compiler that is missing is the difference between a
// person knowing what to do and a person reading a CMake traceback.
func toolchainIsAvailable(spec rhtarget.Spec) error {
	compiler := spec.Toolchain.Compiler
	if compiler == "" {
		return nil
	}
	if _, err := exec.LookPath(compiler); err != nil {
		return fmt.Errorf("this target needs %s, which is not on the path. Install the toolchain for this target, or build a target whose compiler is installed: %w",
			compiler, err)
	}
	return nil
}

// writeBuildFlags records what this build may register, in the project's own
// CMake.
//
// The command line says it to CMake; this says it to the project. A build file
// reads a variable rather than an argument list, so a project that has never
// seen the command line — opened in an editor, configured by hand — can still
// tell what kind of build it is.
func writeBuildFlags(project, specSource, targetName, output string) error {
	path := filepath.Join(project, "cmake", "rho-build-flags.cmake")

	var b strings.Builder
	b.WriteString("# @file cmake/rho-build-flags.cmake\n")
	b.WriteString("# @brief What this build may register.\n")
	b.WriteString("#\n")
	b.WriteString("# @project rho-target\n")
	b.WriteString("# @version 0.1\n")
	b.WriteString("# @date 2026\n")
	b.WriteString("#\n")
	b.WriteString("# SPDX-License-Identifier: Apache-2.0\n\n")
	b.WriteString("# Written by rho-target from the target spec and from what this project's\n")
	b.WriteString("# dependencies declared about their tests. Included by the generated\n")
	b.WriteString("# CMakeLists, and set on the command line as well so that a value given\n")
	b.WriteString("# explicitly still wins.\n\n")
	for _, flag := range testFlagsFor(specSource, targetName) {
		name, value, found := strings.Cut(strings.TrimPrefix(flag, "-D"), "=")
		if !found {
			continue
		}
		fmt.Fprintf(&b, "set(%s %s CACHE BOOL %q)\n", name, value,
			"declared by rho-target for this target")
	}

	if output != "" {
		b.WriteString("\n# Where this project's built binary is written, from the project's own\n")
		b.WriteString("# manifest. Said here as well as on the command line so that anything\n")
		b.WriteString("# reading the project sees it too, and so that an incremental build is\n")
		b.WriteString("# written where the last one was rather than beside it.\n")
		fmt.Fprintf(&b, "set(CMAKE_RUNTIME_OUTPUT_DIRECTORY %s CACHE PATH \"the project's output directory\")\n",
			cmakeString(output))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
