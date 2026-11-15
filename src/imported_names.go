//
// @file imported_names.go
// @brief What each import of a file brings into scope, as the compiler loads it.
//
// A Rho import brings its module's declarations into scope by their own names:
// `import std.assert.assert` and then `assert_eq(a, b)`, with nothing in the
// call that names the module. A linter asking whether an import is used has to
// know those names, and only the loader knows them — it is the loader that finds
// the module's file, follows a dependency's `provides`, and records which module
// each declaration came from.
//
// @project Rho Programming Language
// @date 2026-11-15
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"path/filepath"

	"rho/src/frontend/ast"
)

// ImportedNames loads a file as the entry of a program, with text standing in
// for the file on disk, and answers the names each module it imports declares,
// keyed by the module's import name: what a bare call to one of them is a use
// of. The second result is false when the program could not be loaded, and a
// caller then knows nothing about what the imports provide.
func ImportedNames(path, text string) (map[string][]string, bool) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	roots := projectDependencyRoots(ProjectRoot(abs))
	setSourceOverlay(abs, text)
	module, loadErr := loadModuleGraphWithDependencies(abs, roots)
	clearSourceOverlay(abs)
	if loadErr != nil || module == nil {
		return nil, false
	}
	names := map[string][]string{}
	for _, declaration := range module.Decls {
		owner, name := declarationOwner(declaration)
		if owner == "" || name == "" {
			continue
		}
		names[owner] = append(names[owner], name)
	}
	return names, true
}

// declarationOwner is the module a declaration was imported from and its name;
// an empty owner is the file's own.
func declarationOwner(declaration ast.Decl) (owner string, name string) {
	switch one := declaration.(type) {
	case ast.FuncDecl:
		return one.Owner, one.Name
	case ast.StructDecl:
		return one.Owner, one.Name
	case ast.EnumDecl:
		return one.Owner, one.Name
	case ast.TypeDecl:
		return one.Owner, one.Name
	case ast.ConstDecl:
		return one.Owner, one.Name
	}
	return "", ""
}
