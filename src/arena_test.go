//
// @file arena_test.go
// @brief A long-running loop that builds strings runs in a fixed string arena.
//
// The point of the arena is a firmware that builds strings every iteration and
// runs for months. So the test is that program: a loop shaped like a sensor
// loop, run a hundred thousand times against an arena of 512 bytes. Without the
// releases the compiler emits it would run out within a few dozen iterations; with
// them it runs to the end. The same program with one string kept outside the loop
// runs out, which is what shows the test can tell the two apart — and the build
// says so first: a warning on a host, a refusal for a board.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const arenaFirmware = `module app

fn describe(t: u32) string {
    let unit = "C"
    return "T=" + int_to_str(t) + unit
}

fn main() void {
    var i: u32 = 0
    var state: string = "idle"
    var warm: u32 = 0
    while i < 100000 {
        let line = describe(i % 50) + " " + state
        if str_contains(line + "|", "T=7C") {
            state = "warm"
            warm = warm + 1
        }
        if i % 7 == 0 {
            i = i + 1
            continue
        }
        if i % 25000 == 1 {
            print(line + "\n")
        }
        i = i + 1
    }
    print("done " + int_to_str(warm) + "\n")
}
`

// writeArenaProject writes a project that builds for C.
func writeArenaProject(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(parityManifest, "    - go\n", "", 1)
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return entry
}

// captureStderr runs f and returns what it wrote to standard error.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = write
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(read)
		done <- string(data)
	}()
	f()
	os.Stderr = saved
	write.Close()
	return <-done
}

// buildFirmwareOnHost generates a project and compiles its C with a board-sized
// string arena, as a board build would have one.
func buildFirmwareOnHost(t *testing.T, entry string, arenaBytes string) string {
	t.Helper()
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}
	targets, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", "")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	dir := targets[0].Dir
	binary := filepath.Join(t.TempDir(), "firmware")
	args := []string{"-std=c11", "-g", "-O0"}
	if sanitizersWork(t) {
		args = append(args, "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined")
	}
	args = append(args, "-DRHO_STRING_ARENA_BYTES="+arenaBytes, "-o", binary,
		filepath.Join(dir, "main.c"), filepath.Join(dir, "app.c"), filepath.Join(dir, "runtime.c"), "-I", dir, "-lm")
	if out, err := exec.Command("gcc", args...).CombinedOutput(); err != nil {
		t.Fatalf("gcc: %v\n%s", err, out)
	}
	return binary
}

func TestAFirmwareLoopRunsInAFixedStringArena(t *testing.T) {
	entry := writeArenaProject(t, arenaFirmware)
	var binary string
	warnings := captureStderr(t, func() { binary = buildFirmwareOnHost(t, entry, "512") })
	if strings.Contains(warnings, "warning:") {
		t.Errorf("a loop that keeps no string outside it should build without a warning:\n%s", warnings)
	}
	result := runBinary(binary)
	if result.failed != "" {
		t.Fatalf("the loop ran out of a 512-byte arena or misbehaved: %s\nstdout: %q", result.failed, result.stdout)
	}
	want := "T=1C idle\nT=1C warm\nT=1C warm\ndone 2000\n"
	if result.stdout != want {
		t.Fatalf("got %q, want %q", result.stdout, want)
	}
}

// The same loop keeping each line in an element of a Slice over storage
// declared outside it cannot be released: no name reaches the string as a
// whole. On a host the
// build warns and the program runs out of a fixed arena; for a board the build
// is refused before anything is flashed.
func TestALoopThatKeepsItsStringsWarnsOnAHostAndIsRefusedForABoard(t *testing.T) {
	escaping := strings.Replace(arenaFirmware, `            state = "warm"`, `            seen[warm % 2] = line`, 1)
	escaping = strings.Replace(escaping, `    var warm: u32 = 0`, `    var warm: u32 = 0
    var seen = store[..]`, 1)
	escaping = strings.Replace(escaping, `    var i: u32 = 0`, `    var store: string[2]
    var i: u32 = 0`, 1)
	entry := writeArenaProject(t, escaping)

	var binary string
	warnings := captureStderr(t, func() { binary = buildFirmwareOnHost(t, entry, "512") })
	if !strings.Contains(warnings, "warning:") || !strings.Contains(warnings, `"seen"`) || !strings.Contains(warnings, "line 14") {
		t.Errorf("expected a warning naming the loop and the variable, got:\n%s", warnings)
	}
	result := runBinary(binary)
	// A hosted panic goes to standard error, as it does on Go (a board prints it
	// on its console); runBinary keeps standard error in the failure.
	if result.failed == "" || !strings.Contains(result.failed, "string arena exhausted") {
		t.Fatalf("expected the loop to run out of the arena, got failed=%q stdout=%q", result.failed, result.stdout)
	}

	_, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", "pico")
	if err == nil {
		t.Fatal("expected a board build to be refused")
	}
	for _, want := range []string{"pico is a board", `"seen"`, "line 14"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should say %q: %v", want, err)
		}
	}

	// A host — what the caller passes when the spec has no fixed arena — warns.
	if _, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", ""); err != nil {
		t.Errorf("a host build should only warn: %v", err)
	}
}

// The loop that keeps its latest line in a variable declared outside it — the
// last reading, the status text — is released every iteration except for that
// line. It builds for a board without a word, runs a hundred thousand times in
// 512 bytes, and the line it kept is there after the loop.
const lastValueFirmware = `module app

extern fn rho_arena_used() u64

fn describe(t: u32, label: string) string {
    return label + "=" + int_to_str(t) + "dC"
}

fn main() void {
    var last: string = ""
    var status: string = "idle"
    var n: u32 = 0
    var peak: u64 = 0
    let start = rho_arena_used()
    while n < ITERATIONS {
        let line = describe(n % 600, "node" + int_to_str(n % 5))
        last = line
        if n % 1000 == 0 {
            status = "ok " + int_to_str(n / 1000)
        }
        if rho_arena_used() - start > peak {
            peak = rho_arena_used() - start
        }
        n = n + 1
    }
    print(last + " " + status + " peak " + i64_to_str(peak as i64) + "\n")
}
`

func TestALoopKeepingItsLatestStringRunsInBoundedMemory(t *testing.T) {
	source := strings.Replace(lastValueFirmware, "ITERATIONS", "100000", 1)
	entry := writeArenaProject(t, source)
	var binary string
	warnings := captureStderr(t, func() { binary = buildFirmwareOnHost(t, entry, "512") })
	if strings.Contains(warnings, "warning:") {
		t.Errorf("a loop that replaces a string outside it should build without a warning:\n%s", warnings)
	}
	result := runBinary(binary)
	if result.failed != "" {
		t.Fatalf("the loop ran out of a 512-byte arena or misbehaved: %s\nstdout: %q", result.failed, result.stdout)
	}
	if !strings.HasPrefix(result.stdout, "node4=399dC ok 99 peak ") {
		t.Fatalf("got %q", result.stdout)
	}
	if _, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", "pico"); err != nil {
		t.Fatalf("a board build of the loop should not be refused: %v", err)
	}
}

// Run long: a host's growing arena stays at the size of what is kept plus one
// iteration's strings over two million iterations, and a board's 4096 bytes
// last a million.
func TestALoopKeepingItsLatestStringStaysFlatOverMillionsOfIterations(t *testing.T) {
	if testing.Short() {
		t.Skip("long run")
	}
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}
	for _, run := range []struct {
		name, iterations string
		defines          []string
	}{
		{"host", "2000000", []string{"-DRHO_STRING_ARENA_GROWS"}},
		{"board", "1000000", []string{"-DRHO_STRING_ARENA_BYTES=4096"}},
	} {
		t.Run(run.name, func(t *testing.T) {
			entry := writeArenaProject(t, strings.Replace(lastValueFirmware, "ITERATIONS", run.iterations, 1))
			targets, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", "")
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			dir := targets[0].Dir
			binary := filepath.Join(t.TempDir(), "firmware")
			args := append([]string{"-std=c11", "-O1"}, run.defines...)
			args = append(args, "-o", binary, filepath.Join(dir, "main.c"), filepath.Join(dir, "app.c"),
				filepath.Join(dir, "runtime.c"), "-I", dir, "-lm")
			if out, err := exec.Command("gcc", args...).CombinedOutput(); err != nil {
				t.Fatalf("gcc: %v\n%s", err, out)
			}
			result := runBinary(binary)
			if result.failed != "" {
				t.Fatalf("%s\nstdout: %q", result.failed, result.stdout)
			}
			fields := strings.Fields(result.stdout)
			if len(fields) != 5 || fields[3] != "peak" {
				t.Fatalf("got %q", result.stdout)
			}
			// Two kept strings of a dozen bytes and the strings one iteration
			// builds: well under a hundred bytes, however long it ran.
			if peak, err := strconv.Atoi(fields[4]); err != nil || peak > 128 {
				t.Fatalf("the arena grew to %s bytes above where the loop started: %q", fields[4], result.stdout)
			}
		})
	}
}

// The Go backend has a collector and is generated as it always was.
func TestTheGoBackendCarriesNoArena(t *testing.T) {
	entry := writeArenaProject(t, arenaFirmware)
	root := filepath.Dir(filepath.Dir(entry))
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(parityManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cSource, err := os.ReadFile(filepath.Join(dirs[0], "app.c"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cSource), "rho_arena_mark()") {
		t.Error("expected the C program to release its strings")
	}
	entries, err := os.ReadDir(dirs[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range entries {
		if !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dirs[1], file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "rho_mark") || strings.Contains(string(data), "arena") {
			t.Errorf("%s mentions the arena", file.Name())
		}
	}
}
