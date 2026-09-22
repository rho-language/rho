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
	"strconv"
	"strings"

	"rho/src/codegen"
	"rho/src/deps"
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
	return CompileFileToWithDependencies(path, outPath, nil)
}

// CompileFileToWithDependencies compiles a source file with additional Rho
// dependency roots available to import resolution.
func CompileFileToWithDependencies(path, outPath string, dependencyRoots []string) error {
	mod, err := loadModuleGraphWithDependencies(path, dependencyRoots)
	if err != nil {
		return err
	}
	return compileModule(*mod, outPath)
}

// CompileProjectTo compiles an entry file and all of its imports/includes.
func CompileProjectTo(path, outPath string) error {
	return CompileFileTo(path, outPath)
}

// GenerateProject emits a small native C project for a Rho application.
// The generated directory contains app.c, app.h, main.c, and CMakeLists.txt.
func GenerateProject(path, outputDir string, includePaths, sourceFiles []string) error {
	return GenerateProjectWithDependencies(path, outputDir, includePaths, sourceFiles, nil)
}

// GenerateProjectWithDependencies emits a native project with dependency
// roots available while loading imported Rho modules.
func GenerateProjectWithDependencies(path, outputDir string, includePaths, sourceFiles, dependencyRoots []string) error {
	mod, err := loadModuleGraphWithDependencies(path, dependencyRoots)
	if err != nil {
		return err
	}
	tc := types.New()
	if err := tc.CheckModule(*mod); err != nil {
		return fmt.Errorf("typechecking failed: %w", err)
	}
	irModule := tc.ToIR(*mod)
	if err := codegen.ValidateModule(irModule); err != nil {
		return fmt.Errorf("codegen validation failed: %w", err)
	}
	hasInit := false
	for _, fn := range irModule.Funcs {
		if fn.Name == "init" && !fn.Extern && len(fn.Params) == 0 {
			hasInit = true
			break
		}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create generated project: %w", err)
	}

	appC := codegen.EmitC(irModule)
	appC = strings.ReplaceAll(appC, "int main(", "int app_main(")
	appC = "/*\n * @file app.c\n * @brief Generated Rho application implementation.\n *\n * @project " + mod.Name + "\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n#include \"app.h\"\n\n" + appC
	appC = "/*\n * @file app.c\n * @brief Generated Rho application implementation.\n *\n * @project " + mod.Name + "\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n#define _POSIX_C_SOURCE 200809L\n#include \"app.h\"\n\n" + appC

	configInclude := ""
	projectRoot := ProjectRoot(path)
	if info, statErr := os.Stat(filepath.Join(projectRoot, "config")); statErr == nil && info.IsDir() {
		config := "/*\n * @file app_config.h\n * @brief Generated Rho application configuration.\n *\n * @project " + mod.Name + "\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n#pragma once\n\n#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n#define RHO_APP_NAME \"" + mod.Name + "\"\n\n#ifdef __cplusplus\n}\n#endif\n"
		config = strings.Replace(config, "app_config.h", "config.h", 1)
		if err := os.WriteFile(filepath.Join(outputDir, "config.h"), []byte(config), 0644); err != nil {
			return fmt.Errorf("write config.h: %w", err)
		}
		configInclude = "#include \"config.h\"\n"
	}

	initCall := ""
	if hasInit {
		initCall = "    init();\n"
	}
	appInit := "void app_init(void)\n{\n" + initCall + "}\n\n"
	appC += "\n" + appInit
	runtimeStart := strings.Index(appC, "static char* rho_concat(const char* a, const char* b) {")
	initStart := strings.Index(appC, "\nvoid app_init(void)")
	if runtimeStart < 0 || initStart < runtimeStart {
		return fmt.Errorf("generated runtime section is missing")
	}
	runtimeBody := appC[runtimeStart:initStart]
	appC = appC[:runtimeStart] + appC[initStart:]
	appC = strings.ReplaceAll(appC, "static char* rho_", "char* rho_")
	runtimeBody = strings.ReplaceAll(runtimeBody, "static char* rho_", "char* rho_")
	runtimeC := "/*\n * @file runtime.c\n * @brief Generated Rho C runtime support.\n *\n * @project " + mod.Name + "\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n#define _POSIX_C_SOURCE 200809L\n#include <stdint.h>\n#include <stdbool.h>\n#include <stdio.h>\n#include <stdlib.h>\n#include <string.h>\n#include <stddef.h>\n\n" + runtimeBody
	if err := os.WriteFile(filepath.Join(outputDir, "runtime.c"), []byte(runtimeC), 0644); err != nil {
		return fmt.Errorf("write runtime.c: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "app.c"), []byte(appC), 0644); err != nil {
		return fmt.Errorf("write app.c: %w", err)
	}
	header := "/*\n * @file app.h\n * @brief Generated Rho application interface.\n *\n * @project " + mod.Name + "\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n#pragma once\n\n#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n#include <stdint.h>\n#include <stdbool.h>\n#include <stddef.h>\n" + configInclude + "\nint app_main(void);\nvoid app_init(void);\n\n#ifdef __cplusplus\n}\n#endif\n"
	if err := os.WriteFile(filepath.Join(outputDir, "app.h"), []byte(header), 0644); err != nil {
		return fmt.Errorf("write app.h: %w", err)
	}

	mainC := "/*\n * @file main.c\n * @brief Generated native system entry point for a Rho application.\n *\n * @project " + mod.Name + "\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n#include \"app.h\"\n\nint main(void)\n{\n    app_init();\n    return app_main();\n}\n"
	if err := os.WriteFile(filepath.Join(outputDir, "main.c"), []byte(mainC), 0644); err != nil {
		return fmt.Errorf("write main.c: %w", err)
	}

	cmake := "cmake_minimum_required(VERSION 3.16)\nproject(" + mod.Name + " C)\n\nset(CMAKE_C_STANDARD 11)\nset(CMAKE_C_STANDARD_REQUIRED ON)\n\nadd_executable(" + mod.Name + " main.c app.c\n"
	cmake += "    runtime.c\n"
	for _, source := range sourceFiles {
		cmake += "    " + filepath.ToSlash(source) + "\n"
	}
	cmake += ")\n\ntarget_include_directories(" + mod.Name + " PRIVATE\n    ${CMAKE_CURRENT_SOURCE_DIR}\n"
	for _, include := range includePaths {
		cmake += "    " + filepath.ToSlash(include) + "\n"
	}
	cmake += ")\n"
	if err := os.WriteFile(filepath.Join(outputDir, "CMakeLists.txt"), []byte(cmake), 0644); err != nil {
		return fmt.Errorf("write CMakeLists.txt: %w", err)
	}
	return nil
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
		if _, err := os.Stat(filepath.Join(current, "module.yml")); err == nil {
			return current
		}
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
	if _, err := os.Stat(filepath.Join(root, "module.yml")); err != nil {
		if _, legacyErr := os.Stat(filepath.Join(root, "rho.mod")); legacyErr != nil {
			return replaceExt(path, ".c")
		}
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
	return loadModuleGraphWithDependencies(entry, nil)
}

func loadModuleGraphWithDependencies(entry string, dependencyRoots []string) (*ast.Module, error) {
	visited := map[string]bool{}
	root, err := loadModuleFile(entry, ProjectRoot(entry), dependencyRoots, visited)
	if err != nil {
		return nil, err
	}
	if manifest, manifestErr := deps.LoadManifest(filepath.Join(ProjectRoot(entry), "rho.mod")); manifestErr == nil {
		root.Features = append(root.Features, manifest.Features...)
	}
	if err := loadFeatureModules(root, entry, ProjectRoot(entry), dependencyRoots, visited); err != nil {
		return nil, err
	}
	return root, nil
}

func loadFeatureModules(module *ast.Module, sourcePath, projectRoot string, dependencyRoots []string, visited map[string]bool) error {
	for _, feature := range module.Features {
		if !strings.HasPrefix(feature, "math.") {
			continue
		}
		name := "std." + feature
		featurePath, err := resolveImport(name, sourcePath, projectRoot, dependencyRoots)
		if err != nil {
			return fmt.Errorf("feature %q: %w", feature, err)
		}
		featureModule, err := loadModuleFile(featurePath, projectRoot, dependencyRoots, visited)
		if err != nil {
			return fmt.Errorf("load feature %q: %w", feature, err)
		}
		module.Decls = append(module.Decls, featureModule.Decls...)
	}
	return nil
}

func loadModuleFile(path, projectRoot string, dependencyRoots []string, visited map[string]bool) (*ast.Module, error) {
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
		return nil, sourceDiagnostic(abs, string(source), "lexing", err)
	}
	mod, err := parser.Parse(toks)
	if err != nil {
		return nil, sourceDiagnostic(abs, string(source), "parsing", err)
	}

	merged := &ast.Module{Name: mod.Name, Imports: nil, Includes: nil, Features: append([]string{}, mod.Features...), Decls: append([]ast.Decl{}, mod.Decls...)}
	for _, include := range mod.Includes {
		includePath := include.Path
		if !filepath.IsAbs(includePath) {
			includePath = filepath.Join(filepath.Dir(abs), includePath)
		}
		included, err := loadModuleFile(includePath, projectRoot, dependencyRoots, visited)
		if err != nil {
			return nil, fmt.Errorf("include %q from %q: %w", include.Path, abs, err)
		}
		merged.Decls = append(merged.Decls, included.Decls...)
		merged.Features = append(merged.Features, included.Features...)
	}
	for _, imp := range mod.Imports {
		importPath, err := resolveImport(imp.Name, abs, projectRoot, dependencyRoots)
		if err != nil {
			return nil, fmt.Errorf("import %q from %q: %w", imp.Name, abs, err)
		}
		imported, err := loadModuleFile(importPath, projectRoot, dependencyRoots, visited)
		if err != nil {
			return nil, err
		}
		markModuleOwnership(imported, imported.Name)
		merged.Decls = append(merged.Decls, imported.Decls...)
		merged.Features = append(merged.Features, imported.Features...)
	}
	if err := loadFeatureModules(merged, abs, projectRoot, dependencyRoots, visited); err != nil {
		return nil, err
	}
	return merged, nil
}

func sourceDiagnostic(path, source, phase string, err error) error {
	message := err.Error()
	byteOffset := -1
	if marker := strings.Index(message, "byte "); marker >= 0 {
		start := marker + len("byte ")
		end := start
		for end < len(message) && message[end] >= '0' && message[end] <= '9' {
			end++
		}
		if end > start {
			if parsed, parseErr := strconv.Atoi(message[start:end]); parseErr == nil {
				byteOffset = parsed
			}
		}
	}
	line, column := 1, 1
	if byteOffset >= 0 {
		if byteOffset > len(source) {
			byteOffset = len(source)
		}
		prefix := source[:byteOffset]
		line = 1 + strings.Count(prefix, "\n")
		if last := strings.LastIndex(prefix, "\n"); last >= 0 {
			column = byteOffset - last
		} else {
			column = byteOffset + 1
		}
	}
	return fmt.Errorf("%s %s:%d:%d: %w", phase, path, line, column, err)
}

func markModuleOwnership(module *ast.Module, owner string) {
	for index, decl := range module.Decls {
		switch declaration := decl.(type) {
		case ast.FuncDecl:
			if declaration.Owner == "" {
				declaration.Owner = owner
				module.Decls[index] = declaration
			}
		case ast.StructDecl:
			if declaration.Owner == "" {
				declaration.Owner = owner
				module.Decls[index] = declaration
			}
		}
	}
}

func resolveImport(name, sourcePath, projectRoot string, dependencyRoots []string) (string, error) {
	stdImport := strings.HasPrefix(name, "std.")
	moduleName := strings.TrimPrefix(name, "std.")
	relative := filepath.FromSlash(strings.ReplaceAll(moduleName, ".", "/"))
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
	if stdImport {
		for current := projectRoot; ; current = filepath.Dir(current) {
			candidate := filepath.Join(current, "std", relative+".rho")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
		if workingDirectory, err := os.Getwd(); err == nil {
			for current := workingDirectory; ; current = filepath.Dir(current) {
				candidate := filepath.Join(current, "std", relative+".rho")
				if _, err := os.Stat(candidate); err == nil {
					return candidate, nil
				}
				parent := filepath.Dir(current)
				if parent == current {
					break
				}
			}
		}
	}
	for _, dependencyRoot := range dependencyRoots {
		candidates := []string{
			filepath.Join(dependencyRoot, "src", relative+".rho"),
			filepath.Join(dependencyRoot, relative+".rho"),
		}
		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
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
