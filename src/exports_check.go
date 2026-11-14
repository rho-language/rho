//
// @file exports_check.go
// @brief A call into an included module that the module does not export is said.
//
// @project Rho Programming Language
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"rho/src/deps"
	"rho/src/frontend/ast"
)

// An included module is pasted into the program that includes it, so every
// function it has is reachable by name from everywhere — the ones it means for
// others and the ones it keeps for itself. A module that lists its exports
// (`provides.exports`) says which are which; a call from outside it to one it
// does not list is reported, once per build and name, so the boundary it
// declared is seen even where the language cannot enforce it.

var (
	exportsMu     sync.Mutex
	exportsWarned = map[string]bool{}
	exportsCache  = map[string]*moduleBoundary{}
	exportsDefRE  = regexp.MustCompile(`(?m)^fn ([a-z_][a-z0-9_]*)\(`)
)

// moduleBoundary is a module's own functions and the ones it exports.
type moduleBoundary struct {
	name    string
	root    string
	defined map[string]bool
	exports map[string]bool
}

// moduleRootOf is the folder of the module.yml a source file belongs to.
func moduleRootOf(file string) string {
	for dir := filepath.Dir(file); ; {
		if _, err := os.Stat(filepath.Join(dir, "module.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// boundaryOf reads a module's own functions (from its src/*.rho) and the ones
// it exports, which are none when it lists no `provides.exports`.
func boundaryOf(root string) *moduleBoundary {
	exportsMu.Lock()
	defer exportsMu.Unlock()
	if b, ok := exportsCache[root]; ok {
		return b
	}
	b := &moduleBoundary{root: root, defined: map[string]bool{}, exports: map[string]bool{}}
	exportsCache[root] = b
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err == nil && manifest != nil {
		b.name = manifest.ModuleName
		if manifest.Provides != nil {
			for _, name := range manifest.Provides.Exports {
				b.exports[name] = true
			}
		}
	}
	files, _ := filepath.Glob(filepath.Join(root, "src", "*.rho"))
	for _, file := range files {
		data, readErr := os.ReadFile(file)
		if readErr != nil {
			continue
		}
		for _, m := range exportsDefRE.FindAllStringSubmatch(string(data), -1) {
			b.defined[m[1]] = true
		}
	}
	return b
}

// calledNames are the plain function names a file's own declarations call.
func calledNames(decls []ast.Decl) []string {
	seen := map[string]bool{}
	var walk func(v reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Struct:
			if call, ok := v.Interface().(ast.CallExpr); ok {
				if id, isIdent := call.Callee.(*ast.IdentifierExpr); isIdent {
					seen[id.Name] = true
				} else if id, isIdent := call.Callee.(ast.IdentifierExpr); isIdent {
					seen[id.Name] = true
				}
			}
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					walk(v.Field(i))
				}
			}
		}
	}
	for _, decl := range decls {
		walk(reflect.ValueOf(decl))
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// fileCalls is one loaded file: the module it belongs to and what it calls.
type fileCalls struct {
	file  string
	own   string
	calls []string
}

// exportsLoads are the files of each program being loaded, keyed by the load's
// visited set, so two programs loaded at once keep apart.
var exportsLoads = map[uintptr][]fileCalls{}

// recordFileCalls notes what a loaded file calls, for checkProgramExports.
func recordFileCalls(visited map[string]bool, file string, decls []ast.Decl) {
	key := reflect.ValueOf(visited).Pointer()
	entry := fileCalls{file: file, own: moduleRootOf(file), calls: calledNames(decls)}
	exportsMu.Lock()
	exportsLoads[key] = append(exportsLoads[key], entry)
	exportsMu.Unlock()
}

// dropProgramExports forgets a load that ended before it was checked.
func dropProgramExports(visited map[string]bool) {
	exportsMu.Lock()
	delete(exportsLoads, reflect.ValueOf(visited).Pointer())
	exportsMu.Unlock()
}

// checkProgramExports reports, once the whole program is loaded, the calls its
// files make into the other modules in it that those modules do not export.
// Every included module shares one namespace, so the call may be in any file,
// not only the one that includes the module.
func checkProgramExports(visited map[string]bool) {
	key := reflect.ValueOf(visited).Pointer()
	exportsMu.Lock()
	files := exportsLoads[key]
	delete(exportsLoads, key)
	exportsMu.Unlock()
	roots := map[string]bool{}
	for _, f := range files {
		if f.own != "" {
			roots[f.own] = true
		}
	}
	for _, f := range files {
		for root := range roots {
			if root == f.own {
				continue
			}
			b := boundaryOf(root)
			if len(b.exports) == 0 {
				continue
			}
			for _, name := range f.calls {
				// A name the calling module defines itself is its own.
				if !b.defined[name] || b.exports[name] || (f.own != "" && boundaryOf(f.own).defined[name]) {
					continue
				}
				warnKey := b.name + "." + name + "<" + f.own
				exportsMu.Lock()
				warned := exportsWarned[warnKey]
				exportsWarned[warnKey] = true
				exportsMu.Unlock()
				if !warned {
					fmt.Fprintf(os.Stderr, "warning: %s calls %s, which the %s module does not export (provides.exports)\n",
						strings.TrimPrefix(f.file, filepath.Dir(f.own)+string(filepath.Separator)), name, b.name)
				}
			}
		}
	}
}
