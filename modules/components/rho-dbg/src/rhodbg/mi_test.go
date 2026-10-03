//
// @file mi_test.go
// @brief Tests for reading the Machine Interface.
//
// MI is a grammar, and the failures a parser of it has are all cases that look
// like ordinary input: a path with a comma in it, a program's own output on the
// console stream, a field the debugger sent for a different record. So these are
// the real records, quoted where they were taken from, and the awkward ones.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"strings"
	"testing"
)

func TestAResultRecordIsRead(t *testing.T) {
	record, err := Parse(`15^done,threads=[{id="1",name="main"},{id="2",target-id="Thread 0x1"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if record.Token != "15" || record.Kind != KindResult || record.Class != "done" {
		t.Fatalf("expected token 15 and ^done, got %+v", record)
	}
	threads, found := record.Results.List("threads")
	if !found || len(threads) != 2 {
		t.Fatalf("expected two threads, got %v", threads)
	}
	first, isTuple := threads[0].(Tuple)
	if !isTuple || first.Text("id") != "1" || first.Text("name") != "main" {
		t.Errorf("expected the first thread, got %v", threads[0])
	}
}

// A stack is what a session reads most, and it is nested three deep: a list of
// frames, each a tuple, each with a tuple of its own for the file and the line.
func TestAStackIsReadAllTheWayDown(t *testing.T) {
	record, err := Parse(`3^done,stack=[frame={level="0",addr="0x0000000000400abc",func="main",file="main.c",fullname="/src/main.c",line="42",arch="armv7"},frame={level="1",addr="0x0000000000400100",func="__libc_init_array",line="14"}]`)
	if err != nil {
		t.Fatal(err)
	}
	frames, _ := record.Results.List("stack")
	if len(frames) != 2 {
		t.Fatalf("expected two frames, got %v", frames)
	}
	first := frames[0].(Tuple)
	// A frame list holds *named* entries: `frame={...}`. The name is what says which
	// kind of entry it is, and a reader that ignores it loses every frame.
	inner, isTuple := first["frame"].(Tuple)
	if !isTuple {
		t.Fatalf("expected a frame inside the entry, got %v", first)
	}
	if inner.Text("func") != "main" || inner.Text("file") != "main.c" {
		t.Errorf("expected the innermost frame, got %v", inner)
	}
	if line, found := inner.Int("line"); !found || line != 42 {
		t.Errorf("expected line 42, got %v", first["frame"])
	}
	if full := inner.Text("fullname"); full != "/src/main.c" {
		t.Errorf("expected the file's full name, got %q", full)
	}
}

// Escapes are where a parser quietly corrupts a value. A file name with a
// backslash in it — every Windows machine, every MinGW toolchain — comes through
// here with the backslashes intact, and a value that lost them is a path that does
// not exist.
func TestEscapesInValuesSurvive(t *testing.T) {
	record, err := Parse(`1^done,bkpt={number="1",file="C:\\Users\\a\\build\\main.c",line="7",cond="x == 1\n"}`)
	if err != nil {
		t.Fatal(err)
	}
	bkpt, found := record.Results.Tuple("bkpt")
	if !found {
		t.Fatalf("expected a breakpoint, got %v", record.Results)
	}
	if got := bkpt.Text("file"); got != `C:\Users\a\build\main.c` {
		t.Errorf("expected the path with its backslashes, got %q", got)
	}
	if got := bkpt.Text("cond"); got != "x == 1\n" {
		t.Errorf("expected the newline, got %q", got)
	}
}

// A comma inside a value is the case a split-on-comma parser gets wrong, and it is
// ordinary: a command line in a breakpoint condition, a file name a person made.
func TestACommaInsideAValueIsNotASeparator(t *testing.T) {
	record, err := Parse(`2^done,cond="x == 1, y == 2",number="4"`)
	if err != nil {
		t.Fatal(err)
	}
	if got := record.Results.String("cond"); got != "x == 1, y == 2" {
		t.Errorf("expected both conditions, got %q", got)
	}
	if number, found := record.Results.Int("number"); !found || number != 4 {
		t.Errorf("expected the number beside it, got %v", record.Results["number"])
	}
}

func TestNotificationsAreToldApartFromAnswers(t *testing.T) {
	for _, line := range []string{
		`*stopped,reason="breakpoint-hit",disp="keep",bkptno="1",frame={addr="0x1",func="main"},thread-id="1",stopped-threads="all"`,
		`=thread-group-started,id="i1",pid="42"`,
		`+download,section=".text",bytes-read="0x400"`,
		`^connected`,
		`^running`,
	} {
		record, err := Parse(line)
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if record.Results == nil {
			t.Errorf("%s: expected results, got none", line)
		}
	}
	if record, _ := Parse(`^connected`); record.Class != "connected" {
		t.Errorf("expected ^connected, got %q", record.Class)
	}
	stopped, _ := Parse(`*stopped,reason="breakpoint-hit",thread-id="1"`)
	if stopped.Kind != KindAsyncStop {
		t.Errorf("expected an async stop, got %q", stopped.Kind)
	}
	if reason := stopped.Results.String("reason"); reason != "breakpoint-hit" {
		t.Errorf("expected the reason, got %q", reason)
	}
}

// The console stream carries what the program under test prints, and a program
// that prints a blank line does not break the session.
func TestTheProgramsOwnOutputIsRead(t *testing.T) {
	record, err := Parse(`~"LED on\r\n"`)
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != KindTargetOut {
		t.Fatalf("expected the target's stream, got %q", record.Kind)
	}
	if got := record.Results.String("output"); got != "LED on\r\n" {
		t.Errorf("expected the output with its newline, got %q", got)
	}

	// And a blank line is not a record at all, which is how a reader knows to show
	// it rather than to complain about it.
	if _, err := Parse("\n"); err == nil {
		t.Error("expected a blank line not to be a record")
	}
	// A prompt is not a record either: reading one as an answer would consume the
	// answer to the command before it.
	if _, err := Parse("(gdb) "); err == nil {
		t.Error("expected a prompt not to be a record")
	}
}

func TestAnErrorRecordSaysWhatWentWrong(t *testing.T) {
	record, err := Parse(`9^error,msg="No symbol table info available."`)
	if err != nil {
		t.Fatal(err)
	}
	if record.Class != "error" {
		t.Fatalf("expected an error, got %q", record.Class)
	}
	if got := record.Results.String("msg"); got != "No symbol table info available." {
		t.Errorf("expected the message, got %q", got)
	}
}

// A field a debugger did not send is absent, not an error. Every command is asked
// for fields that only some answers carry, and a reader that failed on every absent
// field would fail on every command.
func TestAFieldThatIsNotThereIsEmptyRatherThanAnError(t *testing.T) {
	record, _ := Parse(`7^done`)
	if got := record.Results.String("msg"); got != "" {
		t.Errorf("expected an absent field to read as empty, got %q", got)
	}
	if _, found := record.Results.Int("number"); found {
		t.Error("expected an absent field not to be found")
	}
	if _, found := record.Results.Tuple("bkpt"); found {
		t.Error("expected an absent tuple not to be found")
	}
	if _, found := record.Results.List("threads"); found {
		t.Error("expected an absent list not to be found")
	}
}

func TestNumbersAreReadInBothForms(t *testing.T) {
	record, err := Parse(`4^done,level="0",addr="0x400abc",frame={level="0x0"},mask="0x1F",count="-3"`)
	if err != nil {
		t.Fatal(err)
	}
	if value, found := record.Results.Int("addr"); !found || value != 0x400abc {
		t.Errorf("expected a hexadecimal number, got %d (%v)", value, found)
	}
	if value, _ := record.Results.Int("mask"); value != 0x1F {
		t.Errorf("expected the mask, got %d", value)
	}
	if value, _ := record.Results.Int("count"); value != -3 {
		t.Errorf("expected a negative number, got %d", value)
	}
	inner := record.Results["frame"].(Tuple)
	if value, found := inner.Int("level"); !found || value != 0 {
		t.Errorf("expected 0x0 to read as zero, got %d (%v)", value, found)
	}
}

// Every value a command carries is quoted, whatever it looks like. A path with a
// space in it is the difference between a session that starts and one that fails
// with a syntax error the editor never shows.
func TestEveryValueIsQuoted(t *testing.T) {
	command := With("-file-exec-and-symbols", "/home/a b/with \"quotes\".elf")
	line := command.String("7")
	if !strings.HasPrefix(line, `7-file-exec-and-symbols "/home/a b/`) {
		t.Fatalf("expected the token, the command and a quoted path, got %q", line)
	}
	if !strings.Contains(line, `\"quotes\"`) {
		t.Errorf("expected the inner quotes to be escaped, got %q", line)
	}
	if !strings.Contains(line, "/home/a b/") {
		t.Errorf("expected the space to be inside the quotes, got %q", line)
	}
}

// MI's own shape: a token, a command name, and arguments separated by spaces. Not
// a comma after the name and `name=value` pairs, which is what the *result* records
// use — and which gdb answers with a usage message when it is sent as a command.
func TestCommandsRenderTheWayMiReadsThem(t *testing.T) {
	if got := Of("-exec-continue").String("4"); got != "4-exec-continue" {
		t.Errorf("expected the token and the command, got %q", got)
	}
	if got := Of("-stack-list-frames").Num(0).Num(20).String("7"); got != "7-stack-list-frames 0 20" {
		t.Errorf("expected two positional numbers, got %q", got)
	}
	// An option is the flag and then its value, and a number is never quoted where
	// a number is expected.
	insert := Of("-break-insert").Opt("-c", "x == 1").Opt("-i", "2").Word(Location("main.c", 42))
	if got := insert.String(""); got != `-break-insert -c "x == 1" -i 2 main.c:42` {
		t.Errorf("expected two options and then a location, got %q", got)
	}
	if got := Of("-data-read-memory-bytes").Arg("0x40011000").Num(4).String(""); got != "-data-read-memory-bytes 0x40011000 4" {
		t.Errorf("expected an address and a length, got %q", got)
	}
	// And a location is a file and a line as gdb writes one, because gdb's
	// `-break-insert` has no `-source`/`-line` options to write.
	if got := Location("src/main.c", 21); got != "src/main.c:21" {
		t.Errorf("expected a location, got %q", got)
	}
}

func TestAMalformedRecordIsRefusedByName(t *testing.T) {
	for _, line := range []string{
		`5^done,{`,
		`5^done,key=`,
		`5^done,key="unterminated`,
		`5^done,[1,2`,
	} {
		if _, err := Parse(line); err == nil {
			t.Errorf("expected %q to be refused", line)
		}
	}
}

// A stop reason is what an editor puts in its status bar, so the mapping is the
// thing worth testing: a Cortex-M fault arrives as a signal, and reporting that as
// a plain stop sends a person to the wrong pane.
func TestAStopReasonBecomesSomethingAnEditorCanSay(t *testing.T) {
	for reason, want := range map[string]StopKind{
		"breakpoint-hit":       StopBreakpoint,
		"watchpoint-trigger":   StopBreakpoint,
		"end-stepping-range":   StopStep,
		"entry-point-hit":      StopEntry,
		"signal-received":      StopException,
		"":                     StopPause,
		"something-unexpected": StopPause,
	} {
		if got := stopKindOf(reason); got != want {
			t.Errorf("%q: expected %q, got %q", reason, want, got)
		}
	}
}
