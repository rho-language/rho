//
// @file test_report.go
// @brief A test build: every `test fn` says it passed when it returns.
//
// `rho test` runs a Rho test file as a program, and the program's output is what
// says which tests passed. A test asserts by stopping the program, so a test that
// returned has passed — but only the program knows that it returned, and a test
// that `main` never calls says nothing at all. Left to the program, a test that
// never ran and a test that passed silently looked the same.
//
// So in a test build the compiler reports each test itself. The function keeps
// its name and its signature; its body moves to a function of its own, and the
// name becomes a wrapper that calls the body and then writes `[PASS] <name>` on
// standard output. A test that stops the program never gets back to the wrapper,
// so it never reports a pass; a test nobody calls reports nothing, and the runner
// fails it for that.
//
// @project Rho Programming Language
// @author Aurel
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"fmt"
	"strings"

	"rho/src/ir"
)

// TestBodySuffix is what a reported test's own body is renamed to end with.
const TestBodySuffix = "__rho_test_body"

// WithTestReports wraps each named function of a module so that it writes
// `[PASS] <name>\n` to standard output each time it returns.
//
// The wrapper keeps the function's Rho name and C symbol, so every call, every
// function value and every prototype still reaches it, and returns what the body
// returned. The line is written with `rho_print`, which is unbuffered: a later
// test that stops the program cannot take an earlier pass with it.
//
// A name that is not a function the module defines is an error, because a test
// the runner expects to hear from and the compiler cannot find would otherwise be
// reported as never having run.
func WithTestReports(m ir.ModuleIR, names []string) (ir.ModuleIR, error) {
	if len(names) == 0 {
		return m, nil
	}
	wanted := map[string]bool{}
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			wanted[name] = true
		}
	}
	found := map[string]bool{}
	funcs := make([]ir.FuncIR, 0, len(m.Funcs)+len(wanted))
	for _, fn := range m.Funcs {
		if !wanted[fn.Name] || fn.Extern || fn.Name == "main" || found[fn.Name] {
			funcs = append(funcs, fn)
			continue
		}
		found[fn.Name] = true
		symbol := cSymbolFor(fn)

		body := fn
		body.Name = fn.Name + TestBodySuffix
		body.CName = symbol + TestBodySuffix

		arguments := make([]string, 0, len(fn.Params))
		for _, parameter := range fn.Params {
			arguments = append(arguments, parameter.Name)
		}
		call := body.CName + "(" + strings.Join(arguments, ", ") + ")"
		report := ir.Instr{Op: ir.OpRaw, Dest: fmt.Sprintf("rho_print(\"[PASS] %s\\n\");", fn.Name)}

		wrapper := ir.FuncIR{Name: fn.Name, CName: fn.CName, Params: fn.Params, ReturnType: fn.ReturnType}
		if fn.ReturnType.Kind == ir.TypeVoid {
			wrapper.Body = []ir.Instr{
				{Op: ir.OpExpr, Args: []string{call}},
				report,
			}
		} else {
			result := "rho_test_result"
			wrapper.Body = []ir.Instr{
				{Op: ir.OpLet, Type: cTypeFor(fn.ReturnType), Dest: result, Args: []string{call}},
				report,
				{Op: ir.OpReturn, Args: []string{result}},
			}
		}
		funcs = append(funcs, body, wrapper)
	}
	var missing []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name != "" && !found[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return m, fmt.Errorf("--test-report names no function this program defines: %s", strings.Join(missing, ", "))
	}
	m.Funcs = funcs
	return m, nil
}
