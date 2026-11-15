//
// @file comments_test.go
// @brief The formatter keeps every comment, where it was written.
//
// Each place a comment can be written has a test of its own here, and every
// Rho source in the repository is formatted and compared, comment for comment,
// with what it was. A corpus outside the repository can be checked the same
// way, read-only, by naming its directories in RHO_FMT_CORPUS.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

// TestMain lets a test run the command itself, in a child process, so its exit
// status can be asserted: RHO_FMT_MAIN set runs main with RHO_FMT_ARGS.
func TestMain(m *testing.M) {
	if os.Getenv("RHO_FMT_MAIN") == "1" {
		os.Args = append([]string{"rho-fmt"}, strings.Fields(os.Getenv("RHO_FMT_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// formatted formats a source the test expects to format.
func formatted(t *testing.T, src string) string {
	t.Helper()
	out, err := FormatSource(src)
	if err != nil {
		t.Fatalf("FormatSource: %v\n%s", err, src)
	}
	again, err := FormatSource(out)
	if err != nil {
		t.Fatalf("re-formatting: %v\n%s", err, out)
	}
	if again != out {
		t.Fatalf("formatting is not stable\n--- once ---\n%s\n--- twice ---\n%s", out, again)
	}
	return out
}

// expectFormat checks a source formats to exactly what is wanted.
func expectFormat(t *testing.T, src, want string) {
	t.Helper()
	if got := formatted(t, src); got != want {
		t.Fatalf("unexpected formatting:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: each place a comment is written                                    */
/* -------------------------------------------------------------------------- */

func TestCommentsFileHeaderKept(t *testing.T) {
	src := `//
// @file a.rho
// @brief A module.
//
// @stability stable
//
// SPDX-License-Identifier: MIT
//

module  a

fn f() void {}
`
	want := `//
// @file a.rho
// @brief A module.
//
// @stability stable
//
// SPDX-License-Identifier: MIT
//

module a

fn f() void {
}
`
	expectFormat(t, src, want)
}

func TestCommentsHeaderDirectlyAboveModule(t *testing.T) {
	src := "// @backends c\nmodule a\n"
	expectFormat(t, src, "// @backends c\nmodule a\n")
}

func TestCommentsDocCommentStaysOnItsDeclaration(t *testing.T) {
	src := `module a
import std.core.option
// Point is a point.
struct Point { x: i32, y: i32 }
// read_all answers every byte.
// @deprecated use read_bytes instead
fn read_all() void {}
// LIMIT is how many.
const LIMIT: u32 = 4
// Id names a thing.
type Id = u32
// Mode is a mode.
enum Mode { Off, On }
// ext is from C.
extern fn ext(x: i32) -> i32
`
	want := `module a

import std.core.option

// Point is a point.
struct Point {
    x: i32
    y: i32
}

// read_all answers every byte.
// @deprecated use read_bytes instead
fn read_all() void {
}

// LIMIT is how many.
const LIMIT: u32 = 4

// Id names a thing.
type Id = u32

// Mode is a mode.
enum Mode {
    Off
    On
}

// ext is from C.
extern fn ext(x: i32) -> i32
`
	expectFormat(t, src, want)
	// The tag is still read where the compiler reads it.
	out := formatted(t, src)
	toks, err := lexer.Lex(out)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := parser.Parse(toks)
	if err != nil {
		t.Fatal(err)
	}
	parser.AttachDocTags(mod, out)
	fn := mod.Decls[1].(ast.FuncDecl)
	if fn.Deprecated == nil || fn.Deprecated.Note != "use read_bytes instead" {
		t.Fatalf("the @deprecated tag did not survive: %+v", fn.Deprecated)
	}
}

func TestCommentsBetweenDeclarationsKeepTheirBlankLines(t *testing.T) {
	src := `module a

fn f() void {}


// ---- section ----

// g does g.
fn g() void {}
// detached

fn h() void {}
// end of file
`
	want := `module a

fn f() void {
}

// ---- section ----

// g does g.
fn g() void {
}

// detached

fn h() void {
}
// end of file
`
	expectFormat(t, src, want)
}

func TestCommentsInsideStructAndEnum(t *testing.T) {
	src := `module a

struct S {
    // x is first.
    x: u8 // trailing x
    y: u8, /* trailing y */
    // at the end of the struct
}

enum E {
    // A is first.
    A = 1 // trailing A
    B(u32) // trailing B
    C { w: i32 } // trailing C
    // after the variants
}

struct Empty {
    // nothing yet
}
`
	want := `module a

struct S {
    // x is first.
    x: u8 // trailing x
    y: u8 /* trailing y */
    // at the end of the struct
}

enum E {
    // A is first.
    A = 1 // trailing A
    B(u32) // trailing B
    C { w: i32 } // trailing C
    // after the variants
}

struct Empty {
    // nothing yet
}
`
	expectFormat(t, src, want)
}

func TestCommentsInsideFunctionBodies(t *testing.T) {
	src := `module a

fn f(x: i32, xs: Slice[i32]) i32 { // trailing the header
    // above the let
    let y = x + 1 // trailing the let

    // after a blank line
    if y > 2 { // trailing the if
        return y // trailing the return
        // end of the then block
    } else { // trailing the else
        // inside the else
        y = 0
    } // trailing the close
    while y < 10 {
        y += 1
        // end of the loop
    }
    for i in xs {
        // in the for
        y += i
    }
    // end of the body
    return y
    // after the return
}
`
	want := `module a

fn f(x: i32, xs: Slice[i32]) i32 { // trailing the header
    // above the let
    let y = x + 1 // trailing the let

    // after a blank line
    if y > 2 { // trailing the if
        return y // trailing the return
        // end of the then block
    } else { // trailing the else
        // inside the else
        y = 0
    } // trailing the close
    while y < 10 {
        y += 1
        // end of the loop
    }
    for i in xs {
        // in the for
        y += i
    }
    // end of the body
    return y
    // after the return
}
`
	expectFormat(t, src, want)
}

func TestCommentsInsideMatchArms(t *testing.T) {
	src := `module a

enum Shape { Circle(i32), Empty }

fn area(s: Shape) i32 {
    match s { // trailing the match
        // above the circle
        Shape.Circle(r) => { // trailing the arm
            // inside the arm
            return r * r
        } // trailing the arm close
        // above the wildcard
        _ => 0, // trailing an expression arm
        // end of the match
    }
    if let Shape.Circle(r) = s { // trailing the if let
        // inside the if let
        return r
    }
    return 0
}
`
	want := `module a

enum Shape {
    Circle(i32)
    Empty
}

fn area(s: Shape) i32 {
    match s { // trailing the match
        // above the circle
        Shape.Circle(r) => { // trailing the arm
            // inside the arm
            return r * r
        } // trailing the arm close
        // above the wildcard
        _ => {
            0 // trailing an expression arm
        }
        // end of the match
    }
    if let Shape.Circle(r) = s { // trailing the if let
        // inside the if let
        return r
    }
    return 0
}
`
	expectFormat(t, src, want)
}

func TestCommentsInsideClosures(t *testing.T) {
	src := `module a

fn f() void {
    let g = fn(x: i32) -> i32 { // trailing the closure header
        // inside the closure
        return x // trailing its return
        // end of the closure
    } // trailing the statement
    g(1)
}
`
	want := `module a

fn f() void {
    let g = fn(x: i32) -> i32 { // trailing the closure header
        // inside the closure
        return x // trailing its return
        // end of the closure
    } // trailing the statement
    g(1)
}
`
	expectFormat(t, src, want)
}

func TestCommentsInsideMultiLineArgumentsGoAboveTheLine(t *testing.T) {
	// The printer puts a call on one line, so a comment between its arguments
	// has no line of its own there: it goes above the statement, in order, and
	// the comment after the last argument stays at the end of the line.
	src := `module a

fn f() void {
    let xs = [
        1, // one
        2, /* two */
        3
    ] // the list
    g(
        // the first
        xs,
        4 /* inline */ )
}
`
	want := `module a

fn f() void {
    // one
    /* two */
    let xs = [ 1, 2, 3 ] // the list
    // the first
    /* inline */
    g(xs, 4)
}
`
	expectFormat(t, src, want)
}

func TestCommentsBlockCommentsKeptAsWritten(t *testing.T) {
	src := `/*
 * A block header.
 */
module a

/* -------------------------------------------------------------------------- */
/*  Section                                                                   */
/* -------------------------------------------------------------------------- */

/**
 * f does f.
 */
fn f() void {
    /* a block
       comment in a body */
    g(/* an argument */ 1)
}
`
	want := `/*
 * A block header.
 */
module a

/* -------------------------------------------------------------------------- */
/*  Section                                                                   */
/* -------------------------------------------------------------------------- */

/**
 * f does f.
 */
fn f() void {
    /* a block
       comment in a body */
    /* an argument */
    g(1)
}
`
	expectFormat(t, src, want)
}

func TestCommentsInHeaderItems(t *testing.T) {
	src := `module a // the module

// The namespace.
namespace robo.v1
include "common.rho" // the shared part
// what is used
import std.core.option
feature "x" // a feature

// A message.
message Hello {
    // who
    device: u32 // trailing device
}

rpc Ping {
    // asks
    request { seq: u32 }
    response {
        // answers
        seq: u32
    }
}
`
	want := `module a // the module

// The namespace.
namespace robo.v1
include "common.rho" // the shared part
// what is used
import std.core.option
feature "x" // a feature

// A message.
message Hello {
    // who
    device: u32 // trailing device
}

rpc Ping {
    // asks
    request {
        seq: u32
    }
    response {
        // answers
        seq: u32
    }
}
`
	expectFormat(t, src, want)
}

func TestCommentsElseIfAndTrailingAfterBraces(t *testing.T) {
	src := `module a

fn f(x: i32) i32 {
    if x == 1 {
        return 1
    } else if x == 2 { // second
        return 2 // two
    } else {
        // otherwise
        return 3
    } // done
}
`
	out := formatted(t, src)
	for _, c := range []string{"// second", "// two", "// otherwise", "// done"} {
		if !strings.Contains(out, c) {
			t.Fatalf("%s was lost:\n%s", c, out)
		}
	}
	if !strings.Contains(out, "return 2 // two\n") {
		t.Fatalf("the trailing comment moved:\n%s", out)
	}
}

// The formatter refuses a source whose comments it would not give back. No
// source the printer knows does that, so the check itself is what is tested.
func TestCommentsMismatchIsRefused(t *testing.T) {
	_, list, err := lexer.LexComments("module a\n// kept\n/* also */\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := sameComments(list, "module a\n// kept\n"); err == nil || !strings.Contains(err.Error(), "lose the comment \"/* also */\"") {
		t.Fatalf("a lost comment was not reported: %v", err)
	}
	if err := sameComments(list, "module a\n// kept\n/* also */\n// new\n"); err == nil || !strings.Contains(err.Error(), "add the comment") {
		t.Fatalf("an added comment was not reported: %v", err)
	}
	if err := sameComments(list, "module a\n/* also */\n// kept   \n"); err != nil {
		t.Fatalf("the same comments were reported different: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: -check                                                             */
/* -------------------------------------------------------------------------- */

// -check names the files that need formatting, prints no source and exits 1;
// with every file formatted it prints nothing and exits 0.
func TestCLICheckNamesFilesAndExitsOne(t *testing.T) {
	dir := t.TempDir()
	messy := filepath.Join(dir, "messy.rho")
	clean := filepath.Join(dir, "clean.rho")
	messySrc := "// header\nmodule  m\nfn f() void {} // trailing\n"
	cleanSrc := "// header\nmodule m\n\nfn f() void {\n} // trailing\n"
	if err := os.WriteFile(messy, []byte(messySrc), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clean, []byte(cleanSrc), 0644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, int) {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), "RHO_FMT_MAIN=1", "RHO_FMT_ARGS="+strings.Join(args, " "))
		out, err := cmd.Output()
		if exit, ok := err.(*exec.ExitError); ok {
			return string(out), exit.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(out), 0
	}

	out, code := run("-check", messy, clean)
	if code != 1 {
		t.Fatalf("-check exited %d, want 1", code)
	}
	if out != messy+"\n" {
		t.Fatalf("-check printed %q, want only the file that needs formatting", out)
	}
	if got, _ := os.ReadFile(messy); string(got) != messySrc {
		t.Fatal("-check changed the file")
	}

	out, code = run("-check", clean)
	if code != 0 || out != "" {
		t.Fatalf("-check on a formatted file: exit %d, output %q", code, out)
	}
}

/* -------------------------------------------------------------------------- */
/*  Tests: every source keeps its comments                                    */
/* -------------------------------------------------------------------------- */

// checkKeepsComments formats a source and reports how its comments, its tags
// and its stability fared. It never writes the file.
func checkKeepsComments(t *testing.T, path string) {
	t.Helper()
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(original)
	toks, before, err := lexer.LexComments(src)
	if err != nil {
		t.Skipf("source does not lex: %v", err)
	}
	toks, _ = stripTestMarkers(toks)
	mod, err := parser.Parse(toks)
	if err != nil {
		t.Skipf("source does not parse: %v", err)
	}
	once, err := FormatSource(src)
	if err != nil {
		t.Fatalf("formatting failed: %v", err)
	}
	_, after, err := lexer.LexComments(once)
	if err != nil {
		t.Fatalf("the formatted source does not lex: %v", err)
	}
	if want, got := commentTexts(before), commentTexts(after); !reflect.DeepEqual(want, got) {
		t.Errorf("comments changed: %d then %d\n%s", len(want), len(got), once)
	}
	twice, err := FormatSource(once)
	if err != nil {
		t.Fatalf("re-formatting failed: %v", err)
	}
	if twice != once {
		t.Errorf("formatting is not stable\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}

	// The tags the toolchain reads from comments read the same afterwards.
	for _, tag := range []string{parser.StabilityTag, parser.DeprecatedTag, "@backends", "@port", "@file"} {
		wantValue, wantFound := parser.HeaderTag(src, tag)
		gotValue, gotFound := parser.HeaderTag(once, tag)
		if wantValue != gotValue || wantFound != gotFound {
			t.Errorf("header tag %s: %q %v then %q %v", tag, wantValue, wantFound, gotValue, gotFound)
		}
	}
	afterMod, err := ParseRho(once)
	if err != nil {
		t.Fatalf("the formatted source does not parse: %v", err)
	}
	parser.AttachDocTags(mod, src)
	parser.AttachDocTags(afterMod, once)
	if want, got := deprecations(mod), deprecations(afterMod); !reflect.DeepEqual(want, got) {
		t.Errorf("@deprecated tags changed: %v then %v", want, got)
	}
}

// deprecations lists what a module's doc comments mark deprecated.
func deprecations(m *ast.Module) []string {
	var out []string
	note := func(name string, d *ast.Deprecation) {
		if d != nil {
			out = append(out, name+": "+d.Note)
		}
	}
	for _, decl := range m.Decls {
		switch d := decl.(type) {
		case ast.FuncDecl:
			note(d.Name, d.Deprecated)
		case ast.StructDecl:
			note(d.Name, d.Deprecated)
		case ast.ConstDecl:
			note(d.Name, d.Deprecated)
		case ast.TypeDecl:
			note(d.Name, d.Deprecated)
		case ast.EnumDecl:
			note(d.Name, d.Deprecated)
			for _, v := range d.Variants {
				note(d.Name+"."+v.Name, v.Deprecated)
			}
		}
	}
	return out
}

func TestFormatterKeepsEveryCommentInRepoSources(t *testing.T) {
	for _, path := range repoSources(t) {
		t.Run(relativeName(t, path), func(t *testing.T) {
			checkKeepsComments(t, path)
		})
	}
}

// RHO_FMT_CORPUS names further directories, separated by the list separator,
// whose sources are checked the same way and never written.
func TestFormatterKeepsEveryCommentInCorpus(t *testing.T) {
	corpus := os.Getenv("RHO_FMT_CORPUS")
	if corpus == "" {
		t.Skip("RHO_FMT_CORPUS names no directories")
	}
	for _, dir := range filepath.SplitList(corpus) {
		files, err := rhoFilesUnder(dir)
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
		for _, path := range files {
			t.Run(path, func(t *testing.T) {
				checkKeepsComments(t, path)
			})
		}
	}
}

// relativeName is a source's path from the repository root, which names a
// subtest unambiguously where a base name would not.
func relativeName(t *testing.T, path string) string {
	rel, err := filepath.Rel(repoRoot(t), path)
	if err != nil {
		return path
	}
	return rel
}

/* -------------------------------------------------------------------------- */
/*  Tests: test files and struct literals                                     */
/* -------------------------------------------------------------------------- */

// A test file's `test fn` declarations are read and printed with their
// marker, their doc comments and the comments in them.
func TestFormatKeepsTestFunctions(t *testing.T) {
	src := `module greeting_test

import std.assert.assert

// prefix is Hello.
test   fn prefix_is_hello() void {
    // the prefix
    assert_eq(prefix(), "Hello ") // compared
}
test fn second() void {}

fn main() void {
    prefix_is_hello()
}
`
	want := `module greeting_test

import std.assert.assert

// prefix is Hello.
test fn prefix_is_hello() void {
    // the prefix
    assert_eq(prefix(), "Hello ") // compared
}

test fn second() void {
}

fn main() void {
    prefix_is_hello()
}
`
	expectFormat(t, src, want)
}

// A module may be called test without its first function becoming a test.
func TestAModuleNamedTestIsNotATestMarker(t *testing.T) {
	expectFormat(t, "module test\nfn f() void {}\n", "module test\n\nfn f() void {\n}\n")
}

// A struct literal is written as the language reference writes it, with the
// brace spaced from its type, in a value and in a pattern alike.
func TestFormatSpacesAStructLiteralsBrace(t *testing.T) {
	src := `module a

struct Point { x: i32, y: i32 }
enum Shape { Rect { w: i32, h: i32 }, Empty }

fn f(s: Shape) i32 {
    let p = Point{0, 0}
    let q: Point = Point  {  1, 2 }
    match s {
        Shape.Rect{ w, h } => { return w * h }
        _ => { return p.x + q.y }
    }
}
`
	out := formatted(t, src)
	for _, want := range []string{"let p = Point { 0, 0 }\n", "let q: Point = Point { 1, 2 }\n", "Shape.Rect { w, h } => {\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
