//
// @file rules_test.go
// @brief Tests for the rules the two carried engines apply.
//
// Every rule here is checked against the thing it is supposed to fire on *and*
// against the thing it must not, because a rule that only has the first test is a
// rule whose false positives nobody would find. The examples in robohub-stm32 supply
// both halves: `examples/rho/hello_world/src/main.rho` is a program that must produce
// nothing, and its `import ffi` is the case that makes the unused-import rule
// interesting — a module whose declarations were generated, imported by name, and
// named by nothing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"strings"
	"testing"
)

// aRho is a Rho source as a rule reads it.
func aRho(path string, text string) Source {
	return Source{Path: path, Root: "/project", Text: text, Generated: map[string]bool{}}
}

// aC is a C source as a rule reads it.
func aC(path string, text string) Source { return aRho(path, text) }

// rulesRun is one rule applied to a source, for the tests that exercise the rules
// directly rather than through the engine.
func rulesRun(t *testing.T, set RuleSet, id string, source Source) []Diagnostic {
	t.Helper()
	rule, ok := set.Find(id)
	if !ok {
		t.Fatalf("the %s engine has no rule called %q", set.Name, id)
	}
	return rule.Apply(source)
}

// idsOf is the rule names a list of findings carries, which is what most assertions
// in this file are about.
func idsOf(found []Diagnostic) []string {
	out := make([]string, 0, len(found))
	for _, one := range found {
		out = append(out, one.Rule)
	}
	return out
}

// assertIDs is that a list of findings carries exactly these rules, in this order.
//
// Exact rather than "contains", because a rule that fires when it should not is the
// failure this file exists to catch, and a test that only looks for what is present
// cannot see it.
func assertIDs(t *testing.T, found []Diagnostic, want ...string) {
	t.Helper()
	got := idsOf(found)
	if len(got) != len(want) {
		t.Fatalf("found %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("found %v, want %v", got, want)
		}
	}
}

func assertNoFindings(t *testing.T, found []Diagnostic, what string) {
	t.Helper()
	if len(found) > 0 {
		t.Errorf("%s: found %v, want nothing", what, idsOf(found))
	}
}

/* -------------------------------------------------------------------------- */
/*  Rho                                                                        */
/* -------------------------------------------------------------------------- */

// helloWorld is `examples/rho/hello_world/src/main.rho`, which must produce nothing.
const helloWorld = `// hello_world — says hello, then says something every second.
//
// It is written against the RoboHub contracts and nothing else.

module main

import ffi

fn main() void {
    // The name the board's own support package gives the user LED.
    let led_name: string = "user_led"

    if robohub_platform_init() != 0 {
        return
    }

    let led: Option[Ptr[RoboHubGPIO]] = robohub_get_gpio(led_name)
    let timer: Option[Ptr[RoboHubTimer]] = robohub_get_timer("system_timer")

    let seconds: u32 = 0
    while true {
        seconds = seconds + 1
        if is_some(led) {
            rho_robohubgpio_write(value(led), 0, seconds % 2)
        }
    }
}
`

func TestRhoEngineIsSilentOnTheRoboHubExample(t *testing.T) {
	// The whole rule set at once, because a test per rule would miss the rule that
	// fires because of what another one does.
	set := RhoRules()
	source := aRho("/project/src/main.rho", helloWorld)
	// As the example actually is: `rho build` has run, so `ffi` exists in
	// `build/generated/modules/`. That is the state the rule has to be silent in.
	source.Generated["ffi"] = true
	for _, rule := range set.Rules {
		found := rule.Apply(source)
		for _, one := range found {
			t.Errorf("%s fired at %d:%d on a program that should be clean: %s",
				rule.ID, one.Line, one.Column, one.Message)
		}
	}
}

func TestRhoUnusedImportSkipsAGeneratedModule(t *testing.T) {
	// The case that decides whether this rule is usable. `ffi`'s declarations were
	// generated from the platform's headers, so nothing in the file names it and
	// every one of its declarations is in scope by virtue of the import. Reporting
	// that would fire on every Rho application with a C interface.
	source := aRho("/project/src/main.rho", helloWorld)
	source.Generated["ffi"] = true
	assertIDs(t, rulesRun(t, RhoRules(), RhoUnusedImport, source))

	// Without the build having generated it, the import is still not obviously wrong
	// — but it is a claim worth making, and that is what the note is for.
	source.Generated = map[string]bool{}
	found := rulesRun(t, RhoRules(), RhoUnusedImport, source)
	if len(found) != 1 || found[0].Rule != RhoUnusedImport {
		t.Fatalf("found %v, want one unused-import finding", idsOf(found))
	}
	if found[0].Severity != SeverityNote {
		t.Errorf("severity %q, want a note: this is a claim, not a defect", found[0].Severity)
	}
	if found[0].Line == 0 {
		t.Error("no line: a person cannot act on an import they are not shown")
	}
}

func TestRhoUnusedImportKeepsAnImportSomethingNames(t *testing.T) {
	source := aRho("/project/src/main.rho", `module main

import os
import platform

fn main() void {
    let x = os.now()
    let y = platform.read()
}
`)
	source.Generated["platform"] = true
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoUnusedImport, source), "both imports in use")
}

func TestRhoModuleNameComparesTheLastSegment(t *testing.T) {
	// `module robohub.hal.gpio` in `gpio.rho` is correct, and comparing the whole
	// string to the file's stem would report every module in a tree of them.
	source := aRho("/project/src/gpio.rho", "module robohub.hal.gpio\n")
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoModuleName, source), "a dotted name matching the file")

	// A name no import can reach is a real defect: `import hello_world` fails with a
	// message about a module that does not exist rather than about this file.
	off := aRho("/project/src/main.rho", "module hello_world\n")
	found := rulesRun(t, RhoRules(), RhoModuleName, off)
	if len(found) != 1 || found[0].Line != 1 {
		t.Errorf("found %v at line %d, want one finding at line 1", idsOf(found), found[0].Line)
	}
}

func TestRhoShadowedBindingIsScopedToOneFunction(t *testing.T) {
	// Two functions each binding `count` is not a shadow, and a rule that said
	// otherwise would fire on every project and be turned off within the week.
	clean := aRho("/project/src/a.rho", `module a

fn first() void {
    let count: u32 = 0
}

fn second() void {
    let count: u32 = 1
}
`)
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoShadowedBinding, clean), "two functions")

	// Within one function the inner binding is the one the code below can see, and
	// the outer one is unreachable from there.
	shadowed := aRho("/project/src/a.rho", `module a

fn first() void {
    let count: u32 = 0
    if true {
        let count: u32 = 1
    }
}
`)
	found := rulesRun(t, RhoRules(), RhoShadowedBinding, shadowed)
	if len(found) != 1 {
		t.Fatalf("found %v, want one shadow", idsOf(found))
	}
	if found[0].Line != 6 {
		t.Errorf("reported at line %d, want 6 — the inner let, not the outer one", found[0].Line)
	}
	if found[0].Severity != SeverityWarning {
		t.Errorf("severity %q, want a warning", found[0].Severity)
	}
}

func TestRhoUnreachableOnlyAfterSomethingThatAlwaysLeaves(t *testing.T) {
	// A `return` inside an `if` does not make the statements after the `if`
	// unreachable, because the `if` may not have been taken.
	reachable := aRho("/project/src/a.rho", `module a

fn first() void {
    if true {
        return
    }
    let after: u32 = 0
}
`)
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoUnreachable, reachable), "after a conditional return")

	// A `while` on a condition does not leave, for exactly that reason.
	conditional := aRho("/project/src/a.rho", `module a

fn first() void {
    while ready() {
        tick()
    }
    let after: u32 = 0
}
`)
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoUnreachable, conditional), "after a conditional loop")

	// The two shapes that reach this rule. Both are accepted by the parser, which is
	// the condition for a rule here being anything but a comment: a `return` followed
	// by a statement is *not* parseable, so the parse rule reports it instead, with a
	// worse message, and this rule has nothing to add.
	afterBreak := aRho("/project/src/a.rho", `module a

fn first() void {
    while true {
        break
        let after: u32 = 0
    }
}
`)
	found := rulesRun(t, RhoRules(), RhoUnreachable, afterBreak)
	if len(found) != 1 || found[0].Line != 6 {
		t.Errorf("found %v, want the statement on line 6 after the break", idsOf(found))
	}

	afterInfiniteLoop := aRho("/project/src/a.rho", `module a

fn first() void {
    while true {
        tick()
    }
    let after: u32 = 0
}
`)
	found = rulesRun(t, RhoRules(), RhoUnreachable, afterInfiniteLoop)
	if len(found) != 1 || found[0].Line != 7 {
		t.Errorf("found %v, want the statement on line 7 after the infinite loop", idsOf(found))
	}
}

func TestRhoUnreachableAfterAnInfiniteLoopWithABreakIsNotReported(t *testing.T) {
	// `while true` with a `break` in it is a loop, not a departure.
	source := aRho("/project/src/a.rho", `module a

fn first() void {
    while true {
        if done() {
            break
        }
    }
    let after: u32 = 0
}
`)
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoUnreachable, source), "after a breakable loop")
}

func TestRhoEmptyBlockAndTheElseThatMeansNothing(t *testing.T) {
	// An `else` with nothing in it says "and do nothing else", which is a statement
	// about the `if` rather than a gap in it.
	deliberate := aRho("/project/src/a.rho", `module a

fn first(flag: bool) void {
    if flag {
        return
    } else {
    }
}
`)
	assertNoFindings(t, rulesRun(t, RhoRules(), RhoEmptyBlock, deliberate), "an empty else")
}

func TestRhoParseReportsWhatTheCompilerCannotRead(t *testing.T) {
	// This is the compiler's own parser, so a file it rejects is a file the build
	// rejects. Reporting it as a clean file because the analyzer could not read it is
	// the one outcome a lint tool must not produce.
	source := aRho("/project/src/broken.rho", "module broken\n\nfn main( void {\n")
	found := rulesRun(t, RhoRules(), RhoParse, source)
	if len(found) != 1 || found[0].Severity != SeverityError {
		t.Fatalf("found %v, want one error", idsOf(found))
	}
	// Every other rule stays silent on a file it cannot understand, rather than
	// reporting on a tree that is not there.
	for _, rule := range RhoRules().Rules {
		if rule.ID == RhoParse {
			continue
		}
		assertNoFindings(t, rule.Apply(source), rule.ID+" on a file that does not parse")
	}
}

func TestRhoTodoIgnoresTheWordInsideAString(t *testing.T) {
	// `print("set the TODO flag")` is not a finding, and a scan that looked at lines
	// without knowing which were inside a string literal would report it.
	source := aRho("/project/src/a.rho", `module a

fn main() void {
    print("the TODO list")
    // FIXME: decide about this
}
`)
	found := rulesRun(t, RhoRules(), RhoTodoComment, source)
	if len(found) != 1 {
		t.Fatalf("found %v, want one marker", idsOf(found))
	}
	if found[0].Line != 5 {
		t.Errorf("reported at line %d, want 5", found[0].Line)
	}
}

/* -------------------------------------------------------------------------- */
/*  C                                                                          */
/* -------------------------------------------------------------------------- */

// helloWorldC is `examples/c/hello_world/src/main.c`, which must also be clean.
const helloWorldC = `#include <stdbool.h>
#include <stdint.h>

#include "platform.h"
#include "registry.h"
#include "robohub/hal/gpio.h"

#define BSP_LED_NAME "user_led"

/* Written by hand where the Rho version calls a helper. */
static void say(rh_u32 seconds)
{
    char line[64];
    const char* prefix = "[";
    rh_u32 at = 0;

    while (*prefix != '\0') {
        line[at++] = *prefix++;
    }
    (void)robohub_console_write(line, at);
}

int main(void)
{
    RoboHubGPIO* led = robohub_get_gpio(BSP_LED_NAME);

    if (robohub_platform_init() != ROBOHUB_OK) {
        return 1;
    }

    say(1);
    if (led != NULL) {
        (void)led->write(led, 0, 1u);
    }
    return 0;
}
`

func TestCEngineIsSilentOnTheCExample(t *testing.T) {
	// The `c` engine covering a hand-written C program is the whole reason one
	// component serves both kinds of project, so this is the test that says it does.
	source := aC("/project/src/main.c", helloWorldC)
	for _, rule := range CRules().Rules {
		for _, one := range rule.Apply(source) {
			t.Errorf("%s fired at %d:%d on a program that should be clean: %s",
				rule.ID, one.Line, one.Column, one.Message)
		}
	}
}

func TestCMissingIncludeGuard(t *testing.T) {
	guarded := aC("/project/src/led.h", `#ifndef LED_H
#define LED_H

#include <stdint.h>

uint32_t led_pin(void);

#endif
`)
	assertNoFindings(t, rulesRun(t, CRules(), CMissingIncludeGuard, guarded), "an #ifndef guard")

	once := aC("/project/src/led.h", "#pragma once\n\n#include <stdint.h>\nuint32_t led_pin(void);\n")
	assertNoFindings(t, rulesRun(t, CRules(), CMissingIncludeGuard, once), "#pragma once")

	// A header with nothing in it does not need a guard. Otherwise every
	// forward-declaration header in a vendor tree is a finding.
	empty := aC("/project/src/forward.h", "#include <stdint.h>\n")
	assertNoFindings(t, rulesRun(t, CRules(), CMissingIncludeGuard, empty), "a header with no declarations")

	unguarded := aC("/project/src/led.h", `#include <stdint.h>

uint32_t led_pin(void);
`)
	found := rulesRun(t, CRules(), CMissingIncludeGuard, unguarded)
	if len(found) != 1 {
		t.Fatalf("found %v, want one finding", idsOf(found))
	}
	if found[0].Line != 3 {
		t.Errorf("reported at line %d, want 3 — the first declaration, not the banner", found[0].Line)
	}

	// A guard in the middle of a header is not a guard: the second inclusion
	// redefines everything above it, which is the whole failure this rule exists for.
	middle := aC("/project/src/led.h", `#include <stdint.h>

uint32_t led_pin(void);

#ifndef LED_H
#define LED_H
#endif
`)
	if found := rulesRun(t, CRules(), CMissingIncludeGuard, middle); len(found) != 1 {
		t.Errorf("found %v, want the header reported: a guard after the declarations is not a guard",
			idsOf(found))
	}
}

func TestCDefinitionInHeaderTellsADefinitionFromADeclaration(t *testing.T) {
	// `void f(void);` in a header is correct and is the ordinary case. A body is what
	// makes it a definition, and the difference is a build failing at link time.
	declared := aC("/project/src/hal.h", `#pragma once
void robohub_platform_init(void);
int robohub_get_gpio(const char* name);
`)
	assertNoFindings(t, rulesRun(t, CRules(), CDefinitionInHeader, declared), "declarations only")

	// The brace on the next line is the ordinary C style, and a scan that looked only
	// at the line carrying the parameter list would miss this entirely — which is the
	// single most common thing the rule is for.
	//
	// The `static inline` definition is the static rule's, not this one's: it links,
	// so "every translation unit defines it again" would be false about it.
	defined := aC("/project/src/hal.h", `#pragma once
static inline uint32_t bump(void)
{
    return 1;
}
int other(void) { return 2; }
`)
	found := rulesRun(t, CRules(), CDefinitionInHeader, defined)
	if len(found) != 1 {
		t.Fatalf("found %v, want the one definition that links", idsOf(found))
	}
	if !strings.Contains(found[0].Message, "other") {
		t.Errorf("found %q, want `other`", found[0].Message)
	}
}

func TestCStaticInHeaderCoversFunctionsAndObjects(t *testing.T) {
	// Two different defects with two different fixes, so two different sentences.
	// The `const` line is the control: it is a declaration like the others and it is
	// not `static`, so it must not be reported — and the name in it is the last word
	// before the semicolon, not the `const` before that.
	source := aC("/project/src/hal.h", `#pragma once
static uint32_t shared_count;
static inline void bump(void)
{
    shared_count++;
}
const char * const name;
`)
	found := rulesRun(t, CRules(), CStaticInHeader, source)
	if len(found) != 2 {
		t.Fatalf("found %v, want the static object and the static function and nothing else",
			idsOf(found))
	}
	if found[0].Line != 2 || found[1].Line != 3 {
		t.Errorf("lines %d and %d, want 2 and 3", found[0].Line, found[1].Line)
	}
}

func TestCUnbalancedBraces(t *testing.T) {
	// The rule that invalidates the others, so it is first in the set and first here.
	open := aC("/project/src/a.c", "int main(void)\n{\n    if (x) {\n        y();\n")
	found := rulesRun(t, CRules(), CUnbalancedBraces, open)
	if len(found) != 1 || found[0].Severity != SeverityError {
		t.Fatalf("found %v, want one error", idsOf(found))
	}
	closed := aC("/project/src/a.c", "int main(void)\n{\n}\n")
	assertNoFindings(t, rulesRun(t, CRules(), CUnbalancedBraces, closed), "balanced")

	// A brace inside a string or a comment is not a brace in the program, and one
	// that was read as one would unbalance a whole file.
	decorated := aC("/project/src/a.c", "int main(void)\n{\n    puts(\"}\");\n    /* { */\n}\n")
	assertNoFindings(t, rulesRun(t, CRules(), CUnbalancedBraces, decorated), "braces in a string and a comment")

	extra := aC("/project/src/a.c", "int main(void)\n{\n}\n}\n")
	found = rulesRun(t, CRules(), CUnbalancedBraces, extra)
	if len(found) != 1 || !strings.Contains(found[0].Message, "never opened") {
		t.Errorf("found %v, want the stray close reported", idsOf(found))
	}
}

func TestCEmptyBodySkipsAWeakHook(t *testing.T) {
	// An empty `weak` definition is the C idiom: it exists so a strong one elsewhere
	// can override it, and it does nothing when nothing does. `rho`'s own generated
	// runtime has four of these, so reporting them would be four findings per Rho
	// project about code nobody wrote.
	hooks := aC("/project/runtime.c", `__attribute__((weak)) void _init(void) {}
__attribute__((weak)) void _fini(void) {}
`)
	assertNoFindings(t, rulesRun(t, CRules(), CEmptyBody, hooks), "weak hooks")

	// A `weak` belonging to whatever came before must not make this one weak too.
	plain := aC("/project/src/a.c", `__attribute__((weak)) void _init(void) {}

void app_init(void)
{
}
`)
	found := rulesRun(t, CRules(), CEmptyBody, plain)
	if len(found) != 1 || found[0].Line != 3 {
		t.Errorf("found %v, want app_init on line 3 and not the weak hook", idsOf(found))
	}

	// A body is not empty because its first line is blank.
	spaced := aC("/project/src/a.c", "void real(void)\n{\n\n    tick();\n}\n")
	assertNoFindings(t, rulesRun(t, CRules(), CEmptyBody, spaced), "a body with a statement")
}

func TestCEmptyBodyDoesNotFireInAHeader(t *testing.T) {
	// A header that declares a function and does not define it is correct, and
	// saying otherwise would be a rule that fires on every interface header in a
	// vendor tree.
	source := aC("/project/src/hal.h", "#pragma once\nvoid robohub_platform_init(void);\n")
	assertNoFindings(t, rulesRun(t, CRules(), CEmptyBody, source), "a declaration in a header")
}

func TestCTodoReadsBlockCommentsAndNotStrings(t *testing.T) {
	source := aC("/project/src/a.c", `/*
 * TODO: the real one
 */
#include <stdint.h>

/* FIXME: decide */
void a(void) {}

const char *text = "TODO";
void b(void) {}
`)
	found := rulesRun(t, CRules(), CTodoComment, source)
	if len(found) != 2 {
		t.Fatalf("found %v, want the two markers and not the word in the string", idsOf(found))
	}
	if found[0].Line != 2 || found[1].Line != 6 {
		t.Errorf("lines %d and %d, want 2 and 6", found[0].Line, found[1].Line)
	}
	if found[0].Column != 4 {
		t.Errorf("first marker at column %d, want 4", found[0].Column)
	}
}

func TestCRulesAndRhoRulesDoNotShareAName(t *testing.T) {
	// The two engines share one namespace with the plugin engines, so a duplicate
	// would mean a report saying `rho-todo-comment` and `c-todo-comment` for one
	// finding, and `--rule` matching the wrong one.
	seen := map[string]string{}
	for _, rule := range append(RhoRules().Rules, CRules().Rules...) {
		if where, taken := seen[rule.ID]; taken {
			t.Errorf("%s is both the %s and the %s rule", rule.ID, where, rule.Description)
		}
		seen[rule.ID] = rule.Description
	}
	if len(seen) == 0 {
		t.Error("no rules at all")
	}
}
