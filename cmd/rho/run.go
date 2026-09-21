package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	src "rho/src"
)

var commandRunner = exec.Command

// RunFile compiles a Rho source file to C, builds a host executable, and runs it.
func RunFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve source path: %w", err)
	}

	cOut := src.ProjectOutputPath(abs)
	if err := src.CompileFileTo(abs, cOut); err != nil {
		return fmt.Errorf("rho compile failed: %w", err)
	}

	exe := replaceExt(abs, "") + "_bin"
	defer os.Remove(exe)

	workDir := src.ProjectRoot(abs)
	compile := commandRunner("gcc", "-O2", "-std=c11", "-o", exe, cOut)
	compile.Dir = workDir
	compile.Stdout = os.Stdout
	compile.Stderr = os.Stderr
	if err := compile.Run(); err != nil {
		return fmt.Errorf("C compilation failed: %w", err)
	}

	run := commandRunner(exe)
	run.Dir = workDir
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
