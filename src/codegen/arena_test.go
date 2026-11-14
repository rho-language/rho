/*
 * arena_test.go — the string arena, as C.
 *
 * The arena is what every string in a C program comes from and what the
 * compiler releases at the end of a loop iteration, so its rules are tested as
 * the C they are: a mark is a place to come back to, a release comes back to
 * it, and a kept string is moved down to the mark and nothing else survives.
 * Both shapes are built — the fixed buffer a board has and the chunks a host
 * grows — and the chunks are made small so that a string crossing from one to
 * the next is the ordinary case rather than one a test never reaches.
 *
 * Built with AddressSanitizer when the compiler can, which poisons what is
 * released: a string read after the release that gave it back is a report,
 * not a value that happens to still be there.
 *
 * @project Rho Programming Language
 * @license Apache-2.0
 */

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const arenaTestProgram = `
#include <assert.h>

static int failures = 0;
#define CHECK(cond) do { if (!(cond)) { printf("FAIL line %d: %s\n", __LINE__, #cond); failures++; } } while (0)

int main(void) {
    /* A literal is not the arena's: keeping it moves nothing and releases. */
    size_t start = rho_arena_mark();
    const char* literal = "static";
    CHECK(rho_arena_keep(start, literal) == literal);
    CHECK(rho_arena_used() == start);

    /* A release returns to the mark. */
    size_t m = rho_arena_mark();
    const char* a = rho_concat("abc", "def");
    CHECK(strcmp(a, "abcdef") == 0);
    CHECK(rho_arena_used() > m);
    rho_arena_release(m);
    CHECK(rho_arena_used() == m);

    /* A kept string is moved down to the mark, and only it remains. */
    m = rho_arena_mark();
    const char* t1 = rho_concat("temp", "orary");
    const char* t2 = rho_concat(t1, "-and-more");
    const char* kept = rho_arena_keep(m, rho_concat(t2, "!"));
    CHECK(strcmp(kept, "temporary-and-more!") == 0);
    CHECK(rho_arena_used() == m + strlen("temporary-and-more!") + 1);

    /* A string from before the mark is not moved. */
    size_t m2 = rho_arena_mark();
    const char* again = rho_arena_keep(m2, kept);
    CHECK(again == kept);
    CHECK(rho_arena_used() == m2);
    rho_arena_release(m);
    CHECK(rho_arena_used() == m);

    /* An interior pointer — what str_trim_prefix returns — is kept from where
     * it points. */
    m = rho_arena_mark();
    const char* whole = rho_concat("prefix:", "value");
    const char* inner = rho_arena_keep(m, rho_str_trim_prefix(whole, "prefix:"));
    CHECK(strcmp(inner, "value") == 0);
    CHECK(rho_arena_used() == m + strlen("value") + 1);
    rho_arena_release(m);

    /* Strings across many chunks, the kept one in the last: it comes back down to
     * the mark, or to the first chunk after the mark that holds it. */
    m = rho_arena_mark();
    for (int i = 0; i < 40; i++) {
        (void)rho_concat("0123456789", "abcdefghij");
    }
    const char* far = rho_concat("far-away-", "string");
    const char* moved = rho_arena_keep(m, far);
    CHECK(strcmp(moved, "far-away-string") == 0);
    CHECK(rho_arena_used() <= m + 64 + strlen("far-away-string") + 1);
    const char* after = rho_concat("x", "y");
    CHECK(strcmp(moved, "far-away-string") == 0);
    CHECK(strcmp(after, "xy") == 0);
    rho_arena_release(m);
    CHECK(rho_arena_used() == m);

    /* A mark near the end of a chunk, and a string too long to fit after it. */
    size_t base = rho_arena_mark();
    while ((rho_arena_used() % 64) < 50) {
        (void)rho_str_alloc(1);
    }
    m = rho_arena_mark();
    const char* longer = rho_concat("a string of thirty-one bytes..", "");
    for (int i = 0; i < 10; i++) {
        (void)rho_concat("junk", "junk");
    }
    const char* tail = rho_arena_keep(m, rho_concat(longer, "!"));
    CHECK(strcmp(tail, "a string of thirty-one bytes..!") == 0);
    rho_arena_release(base);

    /* A loop that releases every iteration stays where it started. */
    m = rho_arena_mark();
    size_t peak = 0;
    for (uint32_t i = 0; i < 100000; i++) {
        const char* line = rho_concat(rho_concat("T=", rho_int_to_str(i)), "C");
        if (rho_arena_used() - m > peak) {
            peak = rho_arena_used() - m;
        }
        CHECK(line[0] == 'T');
        rho_arena_release(m);
    }
    CHECK(rho_arena_used() == m);
    CHECK(peak < 64);

    /* A condition's temporaries are released and its value kept. */
    m = rho_arena_mark();
    CHECK(rho_arena_release_cond(m, rho_str_contains(rho_concat("ab", "cd"), "bc")));
    CHECK(rho_arena_used() == m);

    /* Concatenation never answers NULL. */
    CHECK(strcmp(rho_concat(NULL, "x"), "x") == 0);
    CHECK(strcmp(rho_concat("y", NULL), "y") == 0);

    printf(failures == 0 ? "ok\n" : "failed\n");
    return failures == 0 ? 0 : 1;
}
`

// arenaRuntime is the runtime a program calling the arena and the strings gets.
func arenaRuntime() string {
	return "#include <stddef.h>\n#include <stdint.h>\n#include <stdbool.h>\n#include <string.h>\n#include <stdlib.h>\n#include <stdio.h>\n" +
		runtimeFor("rho_concat( rho_int_to_str( rho_str_trim_prefix( rho_str_contains( rho_arena_keep( rho_arena_used( rho_arena_release_cond(")
}

// buildArenaProgram compiles a program against the runtime with the given
// definitions, with the sanitizers when they work here.
func buildArenaProgram(t *testing.T, program string, defines ...string) string {
	t.Helper()
	compiler, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("no C compiler on the path")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "arena.c")
	if err := os.WriteFile(path, []byte(arenaRuntime()+program), 0o644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "arena")
	args := []string{"-std=c11", "-g", "-O0", "-Wall", "-Wextra"}
	if sanitizersAvailable(compiler, dir) {
		args = append(args, "-fsanitize=address,undefined", "-fno-sanitize-recover=undefined")
	}
	args = append(args, defines...)
	args = append(args, "-o", binary, path)
	if out, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
		t.Fatalf("gcc %s: %v\n%s", strings.Join(defines, " "), err, out)
	} else if strings.Contains(string(out), "warning:") {
		t.Errorf("the arena compiles with warnings:\n%s", out)
	}
	return binary
}

// sanitizersAvailable is whether this compiler builds and runs a sanitized program.
func sanitizersAvailable(compiler, dir string) bool {
	source := filepath.Join(dir, "probe.c")
	if err := os.WriteFile(source, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		return false
	}
	out := filepath.Join(dir, "probe")
	if err := exec.Command(compiler, "-fsanitize=address,undefined", "-o", out, source).Run(); err != nil {
		return false
	}
	return exec.Command(out).Run() == nil
}

func runArenaProgram(binary string) (string, error) {
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "ASAN_OPTIONS=detect_leaks=0")
	out, err := command.CombinedOutput()
	return string(out), err
}

// Both shapes keep the same rules: the chunks a host grows, made small so the
// tests cross them, and the fixed buffer a board has.
func TestTheStringArenaReleasesAndKeeps(t *testing.T) {
	for name, defines := range map[string][]string{
		"host chunks":  {"-DRHO_STRING_ARENA_GROWS", "-DRHO_STRING_ARENA_CHUNK=64u"},
		"board buffer": {"-DRHO_STRING_ARENA_BYTES=4096"},
	} {
		t.Run(name, func(t *testing.T) {
			binary := buildArenaProgram(t, arenaTestProgram, defines...)
			out, err := runArenaProgram(binary)
			if err != nil || strings.TrimSpace(out) != "ok" {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}

// A board's arena that runs out stops the program, saying so, rather than
// writing past the buffer.
func TestABoardsStringArenaStopsWhenItIsExhausted(t *testing.T) {
	binary := buildArenaProgram(t, `
int main(void) {
    for (uint32_t i = 0; i < 1000; i++) {
        (void)rho_concat("0123456789", rho_int_to_str(i));
    }
    printf("never\n");
    return 0;
}
`, "-DRHO_STRING_ARENA_BYTES=256")
	out, err := runArenaProgram(binary)
	if err == nil {
		t.Fatalf("expected the program to stop, it printed %q", out)
	}
	if !strings.Contains(out, "string arena exhausted") || strings.Contains(out, "never") {
		t.Fatalf("expected the exhaustion to be reported, got %q", out)
	}
}

// Under AddressSanitizer a string read after the release that gave it back is
// reported, which is what makes the parity tests a check of the compiler's
// releases and not only of what the programs print.
func TestAStringReadAfterItsReleaseIsReported(t *testing.T) {
	compiler, err := exec.LookPath("gcc")
	if err != nil {
		t.Skip("no C compiler on the path")
	}
	if !sanitizersAvailable(compiler, t.TempDir()) {
		t.Skip("this C compiler cannot build with the sanitizers")
	}
	binary := buildArenaProgram(t, `
int main(void) {
    size_t m = rho_arena_mark();
    const char* s = rho_concat("released ", "string");
    rho_arena_release(m);
    printf("%c\n", s[3]);
    return 0;
}
`, "-DRHO_STRING_ARENA_GROWS")
	out, err := runArenaProgram(binary)
	if err == nil || !strings.Contains(out, "use-after-poison") {
		t.Fatalf("expected AddressSanitizer to report the read, got %v\n%s", err, out)
	}
}

// A program that builds no string carries no arena: a firmware that toggles a
// pin does not pay for a buffer it never uses.
func TestAProgramThatBuildsNoStringCarriesNoArena(t *testing.T) {
	if strings.Contains(runtimeFor("rho_print( rho_print_u32("), "rho runtime: arena") {
		t.Error("printing a literal and a number pulled in the string arena")
	}
	if !strings.Contains(runtimeFor("rho_concat("), "rho runtime: arena") {
		t.Error("a concatenation did not pull in the string arena")
	}
}
