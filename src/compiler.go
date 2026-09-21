//
// @file compiler.go
// @brief Rho Compiler - Orchestrator v3.
//
// Enterprise v3: top-level Rho 0.1 compiler driver.
// Pipeline: lexer → parser → typechecker → IR → C codegen.
// Exposes a clean API for CLI tools: rho build, rho run.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rho

/* -------------------------------------------------------------------------- */
/*  Imports                                                                   */
/* -------------------------------------------------------------------------- */

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/codegen"
	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
	types "rho/src/types"
)

/* -------------------------------------------------------------------------- */
/*  Public API                                                                */
/* -------------------------------------------------------------------------- */

// CompileFile runs the full Rho pipeline on a single source file
// and emits a corresponding .c file next to it.
func CompileFile(path string) error {
	return CompileFileTo(path, replaceExt(path, ".c"))
}

// CompileFileTo compiles one Rho source file into the requested C output path.
func CompileFileTo(path, outPath string) error {
	mod, err := loadModuleGraph(path)
	if err != nil {
		return err
	}
	return compileModule(*mod, outPath)
}

// CompileProjectTo compiles an entry file and all of its imports/includes.
func CompileProjectTo(path, outPath string) error {
	return CompileFileTo(path, outPath)
}

// ProjectRoot returns the directory containing rho.mod, or the source file's
// directory when the file is not part of a project.
func ProjectRoot(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Dir(path)
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for current := abs; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "rho.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
	}
}

// ProjectOutputPath returns build/<entry>.c for a project and a sibling .c
// path for a standalone source file.
func ProjectOutputPath(path string) string {
	root := ProjectRoot(path)
	if _, err := os.Stat(filepath.Join(root, "rho.mod")); err != nil {
		return replaceExt(path, ".c")
	}
	return filepath.Join(root, "build", filepath.Base(replaceExt(path, ".c")))
}

func compileModule(mod ast.Module, outPath string) error {
	tc := types.New()
	if err := tc.CheckModule(mod); err != nil {
		return fmt.Errorf("typechecking failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := codegen.EmitCToFile(outPath, tc.ToIR(mod)); err != nil {
		return fmt.Errorf("codegen failed: %w", err)
	}
	return nil
}

func loadModuleGraph(entry string) (*ast.Module, error) {
	visited := map[string]bool{}
	root, err := loadModuleFile(entry, ProjectRoot(entry), visited)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func loadModuleFile(path, projectRoot string, visited map[string]bool) (*ast.Module, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	if visited[abs] {
		return &ast.Module{}, nil
	}
	visited[abs] = true

	source, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", abs, err)
	}
	toks, err := lexer.Lex(string(source))
	if err != nil {
		return nil, fmt.Errorf("lexing %q failed: %w", abs, err)
	}
	mod, err := parser.Parse(toks)
	if err != nil {
		return nil, fmt.Errorf("parsing %q failed: %w", abs, err)
	}

	merged := &ast.Module{Name: mod.Name, Imports: nil, Includes: nil, Decls: append([]ast.Decl{}, mod.Decls...)}
	for _, include := range mod.Includes {
		includePath := include.Path
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(abs), includePath)
		}
		included, err := loadModuleFile(includePath, projectRoot, visited)
		if err != nil {
			return nil, fmt.Errorf("include %q from %q: %w", include.Path, abs, err)
		}
		merged.Decls = append(merged.Decls, included.Decls...)
	}
	for _, imp := range mod.Imports {
		importPath, err := resolveImport(imp.Name, abs, projectRoot)
		if err != nil {
			return nil, fmt.Errorf("import %q from %q: %w", imp.Name, abs, err)
		}
		imported, err := loadModuleFile(importPath, projectRoot, visited)
		if err != nil {
			return nil, err
		}
		merged.Decls = append(merged.Decls, imported.Decls...)
	}
	return merged, nil
}

func resolveImport(name, sourcePath, projectRoot string) (string, error) {
	relative := filepath.FromSlash(strings.ReplaceAll(name, ".", "/"))
	candidates := []string{
		filepath.Join(projectRoot, "src", relative+".rho"),
		filepath.Join(projectRoot, relative+".rho"),
		filepath.Join(filepath.Dir(sourcePath), relative+".rho"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("file not found for module %q", name)
}

// CompileSource compiles a Rho source string into a C output file.
func CompileSource(source string, outPath string) error {
	// 1. Lexing
	toks, err := lexer.Lex(source)
	if err != nil {
		return fmt.Errorf("lexing failed: %w", err)
	}

	// 2. Parsing
	modAST, err := parser.Parse(toks)
	if err != nil {
		return fmt.Errorf("parsing failed: %w", err)
	}

	// 3. Typechecking
	tc := types.New()
	if err := tc.CheckModule(*modAST); err != nil {
		return fmt.Errorf("typechecking failed: %w", err)
	}

	// 4. AST → IR
	modIR := tc.ToIR(*modAST)

	// 5. IR → C
	if err := codegen.EmitCToFile(outPath, modIR); err != nil {
		return fmt.Errorf("codegen failed: %w", err)
	}

	return nil
}

/* -------------------------------------------------------------------------- */
/*  CLI Entry Point                                                           */
/* -------------------------------------------------------------------------- */

// RunCLI is a thin, testable wrapper around flag parsing and CompileFile.
// It is intended to be called from cmd/rho/main.go.
func RunCLI(args []string) int {
	fs := flag.NewFlagSet("rho", flag.ContinueOnError)
	out := fs.String("o", "", "output C file (optional, default: <input>.c)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "rho: flag parse error:", err)
		return 2
	}

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: rho [options] <file.rho>")
		return 2
	}

	in := fs.Arg(0)
	inAbs, _ := filepath.Abs(in)

	if *out != "" {
		outAbs, _ := filepath.Abs(*out)
		if err := CompileFileTo(inAbs, outAbs); err != nil {
			fmt.Fprintln(os.Stderr, "rho:", err)
			return 1
		}
		return 0
	}

	if err := CompileFile(inAbs); err != nil {
		fmt.Fprintln(os.Stderr, "rho:", err)
		return 1
	}

	return 0
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

func replaceExt(path, newExt string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[:i] + newExt
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	return path + newExt
}
