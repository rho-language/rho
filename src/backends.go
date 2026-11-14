//
// @file backends.go
// @brief Refusing, by name, a program that uses what its target cannot supply.
//
// A standard-library module is written for every backend unless it says
// otherwise, and one that says otherwise says it in its header:
//
//	// @backends go-server
//	// @port sql
//
// `@backends` lists where the module works — `c`, `go` (any Go target) or
// `go-server` (a Go target of kind server) — and `@port` names the port that
// supplies its foreign functions on a Go target. A built-in function that only
// some backends supply is listed in the type checker's table
// (types.BuiltinBackends) in the same terms.
//
// A program that imports such a module, or calls such a function, for a target
// that cannot supply it is refused before anything is generated, with the module
// or function and the backend named. Before this, the same program compiled and
// then failed in a tool the compiler does not speak for: a C link error about
// `gpio_hw_init`, a Go "undefined: stdSqlSqlOpen".
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/ir"
	"rho/src/projectgen"
	"rho/src/types"
)

// stdBackendTag is what a standard-library module's header says about where it
// works.
type stdBackendTag struct {
	// Module is the module's name, `std.web.http`.
	Module string
	// File is the source it was read from.
	File string
	// Backends are where it works; none means every backend.
	Backends []string
	// Port is the port that supplies its foreign functions on a Go target, or "".
	Port string
}

// readStdBackendTag reads the header of one source file: its `module` line and
// any `@backends` and `@port` lines in the comments before it. A file that is not
// a std module answers false.
func readStdBackendTag(path string) (stdBackendTag, bool) {
	file, err := os.Open(path)
	if err != nil {
		return stdBackendTag{}, false
	}
	defer file.Close()
	tag := stdBackendTag{File: path}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			tag.Module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
			break
		}
		comment := strings.TrimSpace(strings.TrimPrefix(line, "//"))
		if !strings.HasPrefix(line, "//") {
			continue
		}
		if value, ok := headerValue(comment, "@backends"); ok {
			for _, backend := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' }) {
				tag.Backends = append(tag.Backends, backend)
			}
		}
		if value, ok := headerValue(comment, "@port"); ok {
			tag.Port = value
		}
	}
	if tag.Module != "std" && !strings.HasPrefix(tag.Module, "std.") {
		return stdBackendTag{}, false
	}
	return tag, true
}

// headerValue is the text after a header key, `@backends go-server` read as
// "go-server". A colon after the key is allowed.
func headerValue(comment, key string) (string, bool) {
	if !strings.HasPrefix(comment, key) {
		return "", false
	}
	rest := comment[len(key):]
	if rest != "" && rest[0] != ' ' && rest[0] != ':' && rest[0] != '\t' {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ":")), true
}

// backendTarget is what a refusal is decided against: the target's emitter and
// kind, and the ports its generated project gets.
type backendTarget struct {
	ID      string
	Emitter string
	Kind    string
	Ports   []string
}

func backendTargetOf(target projectgen.Target, ports []string) backendTarget {
	return backendTarget{ID: target.ID, Emitter: target.Emitter, Kind: target.TargetKind(), Ports: ports}
}

// supports reports whether one backend name covers the target.
func (t backendTarget) supports(backend string) bool {
	switch backend {
	case "c":
		return t.Emitter == "c"
	case "go":
		return t.Emitter == "go"
	case "go-server":
		return t.Emitter == "go" && t.Kind == "server"
	}
	return false
}

// allows reports whether a rule's backends and port are met by the target, and
// says what is missing when they are not.
func (t backendTarget) allows(backends []string, port string) (bool, string) {
	if len(backends) > 0 {
		covered := false
		for _, backend := range backends {
			if t.supports(backend) {
				covered = true
				break
			}
		}
		if !covered {
			return false, "is for the " + strings.Join(backends, ", ") + " backend"
		}
	}
	if port != "" && t.Emitter == "go" {
		for _, rendered := range t.Ports {
			if rendered == port {
				return true, ""
			}
		}
		return false, "needs the " + port + " port, which this target does not render"
	}
	return true, ""
}

// describe names a target for a message: its id, and the backend behind it when
// the id does not say.
func (t backendTarget) describe() string {
	backend := t.Emitter
	if t.Emitter == "go" && t.Kind == "server" {
		backend = "go-server"
	}
	if t.ID == backend {
		return "the " + t.ID + " target"
	}
	return "the " + t.ID + " target (" + backend + ")"
}

// checkBackendSupport refuses a program that imports a std module, or calls a
// built-in, that the target cannot supply. Every refusal is listed, so one build
// says everything that has to change.
func checkBackendSupport(compiled *ast.Module, module ir.ModuleIR, target backendTarget) error {
	if compiled == nil {
		return nil
	}
	refusals := []string{}
	tagged := map[string]bool{}
	for _, tag := range stdBackendTags(compiled) {
		tagged[tag.Module] = true
		ok, why := target.allows(tag.Backends, tag.Port)
		if ok {
			continue
		}
		line := tag.Module + " " + why
		if importer := importerOf(compiled, tag.Module); importer != "" {
			line += " (imported by " + importer + ")"
		}
		refusals = append(refusals, line)
	}
	for _, call := range module.BackendCalls {
		// A call inside a module that says where it works is that module's
		// business: the module is refused, by name, where the call could not be
		// made.
		if tagged[call.Module] {
			continue
		}
		rule, restricted := types.BuiltinBackends(call.Name)
		if !restricted {
			continue
		}
		ok, why := target.allows(rule.Backends, rule.Port)
		if ok {
			continue
		}
		refusals = append(refusals, call.Name+" "+why+" ("+callPlace(call)+")")
	}
	if len(refusals) == 0 {
		return nil
	}
	return fmt.Errorf("this program cannot be built for %s:\n  %s", target.describe(), strings.Join(refusals, "\n  "))
}

// callPlace says where a recorded call is: the module, when it is not the
// program's own, and the line and column.
func callPlace(call ir.BackendCall) string {
	place := "called"
	if call.Module != "" {
		place += " in " + call.Module
	}
	if call.Line > 0 {
		place += fmt.Sprintf(" at %d:%d", call.Line, call.Column)
	}
	return place
}

// stdBackendTags are the tagged std modules among the files a program was
// loaded from, in the order they were loaded.
func stdBackendTags(compiled *ast.Module) []stdBackendTag {
	out := []stdBackendTag{}
	for _, file := range compiled.LoadedFiles {
		if !strings.Contains(filepath.ToSlash(file), "/std/") {
			continue
		}
		tag, ok := readStdBackendTag(file)
		if !ok || (len(tag.Backends) == 0 && tag.Port == "") {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// importerOf is the first loaded file that imports a module, as a path relative
// to the working directory when it can be, or "" when nothing names it (a module
// a feature or a declared database brought in).
func importerOf(compiled *ast.Module, module string) string {
	for _, file := range compiled.LoadedFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(content), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "import" && fields[1] == module {
				if wd, err := os.Getwd(); err == nil {
					if relative, err := filepath.Rel(wd, file); err == nil && !strings.HasPrefix(relative, "..") {
						return relative
					}
				}
				return file
			}
		}
	}
	return ""
}
