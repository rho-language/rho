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
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rho/rho-target/src/rhtarget"
)

// build compiles a generated project for a target.
//
// The steps are the ones a board build always has, in the order they have to
// happen: resolve what the target is, write the files that describe it into the
// project, then configure and build it. Writing them before configuring is what
// makes the toolchain file take effect — CMake reads it to learn which compiler
// it is configuring for, so a file written after `project()` is too late.
func build(args []string) int {
	fs := flag.NewFlagSet("rho target build", flag.ExitOnError)
	target := fs.String("target", "", "the target to build for")
	project := fs.String("project", "", "the generated project to build")
	buildDir := fs.String("build-dir", "", "where the build tree goes (default: <project>/build/<target>)")
	kind := fs.String("type", "Release", "the CMake build type")
	quiet := fs.Bool("quiet", false, "print nothing; the exit code is the answer")
	treePlugins := fs.Bool("tree-plugins", false, "also read the plugins the tree carries")
	// Flags are read wherever they appear, because `rho target build --target pico`
	// is what a person types first.
	flagArgs, rest := splitArgs(args, map[string]bool{"target": true, "project": true, "build-dir": true})
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

	library := library(*treePlugins)
	spec, diagnostics := library.Resolve(*target)
	report(append(library.Diagnostics, diagnostics...))
	if spec.Name == "" {
		fmt.Fprintf(os.Stderr, "rho target build: no target named %q. `rho target list` names the ones there are.\n", *target)
		return 1
	}

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
	if err := writeTargetFiles(absolute, spec); err != nil {
		fmt.Fprintln(os.Stderr, "rho target build:", err)
		return 1
	}

	tree := *buildDir
	if tree == "" {
		tree = filepath.Join(absolute, "build", *target)
	}

	if err := configureAndBuild(absolute, tree, *kind, *quiet); err != nil {
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
func configureAndBuild(project, tree, buildType string, quiet bool) error {
	if _, err := exec.LookPath("cmake"); err != nil {
		return fmt.Errorf("cmake is required to build a target: %w", err)
	}

	configure := exec.Command("cmake", "-S", project, "-B", tree,
		"-DCMAKE_BUILD_TYPE="+buildType)
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
