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
// SPDX-License-Identifier: Apache-2.0
//

package rho

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

// The same loop keeping each line in a variable declared outside it cannot be
// released. On a host the build warns and the program runs out of a fixed
// arena; for a board the build is refused before anything is flashed.
func TestALoopThatKeepsItsStringsWarnsOnAHostAndIsRefusedForABoard(t *testing.T) {
	escaping := strings.Replace(arenaFirmware, `            state = "warm"`, `            state = line`, 1)
	entry := writeArenaProject(t, escaping)

	var binary string
	warnings := captureStderr(t, func() { binary = buildFirmwareOnHost(t, entry, "512") })
	if !strings.Contains(warnings, "warning:") || !strings.Contains(warnings, `"state"`) || !strings.Contains(warnings, "line 12") {
		t.Errorf("expected a warning naming the loop and the variable, got:\n%s", warnings)
	}
	result := runBinary(binary)
	// A panic is printed where everything else is, on standard output.
	if result.failed == "" || !strings.Contains(result.stdout, "string arena exhausted") {
		t.Fatalf("expected the loop to run out of the arena, got failed=%q stdout=%q", result.failed, result.stdout)
	}

	_, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", "pico")
	if err == nil {
		t.Fatal("expected a board build to be refused")
	}
	for _, want := range []string{"pico is a board", `"state"`, "line 12"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should say %q: %v", want, err)
		}
	}

	// A host — what the caller passes when the spec has no fixed arena — warns.
	if _, err := GenerateAllTargetsDetailed(entry, "", nil, nil, nil, "c", "", ""); err != nil {
		t.Errorf("a host build should only warn: %v", err)
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
