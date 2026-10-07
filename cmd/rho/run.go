package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"rho/src/deps"

	src "rho/src"
)

var commandRunner = exec.Command

// RunFile compiles a Rho source file to C, builds a host executable, and runs it.
func RunFile(path string) error {
	return RunFileWithDependencies(path, nil)
}

func RunFileWithDependencies(path string, dependencyRoots []string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve source path: %w", err)
	}

	cOut := src.ProjectOutputPath(abs)
	if err := src.CompileFileToWithDependencies(abs, cOut, dependencyRoots); err != nil {
		return fmt.Errorf("rho compile failed: %w", err)
	}

	workDir := src.ProjectRoot(abs)
	exe := filepath.Join(workDir, "build", "run", filepath.Base(replaceExt(abs, "")))
	os.MkdirAll(filepath.Dir(exe), 0755)
	defer os.Remove(exe)
	// A connected project compiles C of its own — the shim a generated binding
	// needs, and the host C a Rho program writes — so those files are part of the
	// program and are compiled with it. Without them a binding that declares
	// foreign functions cannot link, which is the whole point of one.
	sources := []string{cOut}
	// A connected project compiles against its library's headers, so the paths
	// the manifest declares are the compiler's own: without them a foreign
	// declaration names a type the C compiler has never heard of.
	headers := []string{}
	libraries := []string{}
	if manifest, err := deps.LoadManifest(deps.ManifestPath(workDir)); err == nil && manifest != nil {
		sources = append(sources, manifest.BuildSources...)
		if connection := manifest.Connection; connection != nil {
			// A manifest names directories; a compiler is told where to look
			// with a flag, so each one becomes its own -I.
			headers = append(headers, "-I"+workDir)
			for _, dir := range connection.IncludeDirs {
				headers = append(headers, "-I"+dir)
			}
			// The library a project links, as a path to the archive or shared
			// object, when the manifest names one: a host run has no CMake to do
			// it, so the library is given the way every other C build would.
			if path := connection.Path; path != "" {
				libraries = append(libraries, path)
			}
		}
	}
	// The argument list is built rather than passed to Command, because the
	// include paths and the extra sources are the project's own and the rest —
	// the compiler, the standard, the output — is not. The library comes last,
	// because a linker resolves an archive in the order it is given it and a
	// library before the program that needs it contributes nothing.
	arguments := []string{"-O2", "-std=c11", "-o", exe}
	arguments = append(arguments, headers...)
	arguments = append(arguments, sources...)
	arguments = append(arguments, libraries...)
	compile := commandRunner("gcc", arguments...)
	compile.Dir = workDir
	compile.Stdout = os.Stdout
	compile.Stderr = os.Stderr
	if err := compile.Run(); err != nil {
		return fmt.Errorf("C compilation failed: %w", err)
	}

	run := commandRunner(exe)
	run.Dir = workDir
	run.Stdin = os.Stdin
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	if err := run.Run(); err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	return nil
}

func replaceExt(path, newExt string) string {
	base := filepath.Base(path)
	for index := len(base) - 1; index >= 0; index-- {
		if base[index] == '.' {
			return path[:len(path)-(len(base)-index)] + newExt
		}
	}
	return path + newExt
}
