# Outsider test: findings from writing csv-report

This example was written for the beta plan's outsider test. The program is
deliberately unlike the home-automation project Rho grew up in: a command-line
CSV summariser. It was written from the documentation only — `docs/**`,
`README.md`, the `std/**/*.rho` sources (read as the API reference), `examples/`,
and the output of the installed `rho` 0.18.0. No compiler, component or test
source was read. In a few places the C that `rho build` generated for this
program was read, as a user debugging a slow program would; those places say so.

The program works. It builds for C and Go, the two print byte-identical output
for every case in `tests/parity.sh`, and `rho test` passes 20 cases. Getting
there took the workarounds below. Each finding is tracked to a fix before the
beta release; the **Status** line says where it went.

Severity: **blocker** — the task cannot be done with what the docs describe;
**workaround needed** — it can be done only by going around the language or the
toolchain; **doc gap** — the docs are missing something or say something wrong;
**papercut** — friction that costs minutes. **[BUG]** marks a toolchain defect.

| Severity | Count |
|---|---|
| blocker | 1 |
| workaround needed | 10 |
| doc gap | 7 |
| papercut | 12 |
| **total** | **30** |

13 are toolchain bugs: 1, 2, 3, 5, 6, 7, 8, 9, 11, 20, 21, 22 and 23.

## Toolchain bugs and blockers

### 1. [BUG] A struct field that is an array of structs does not compile on C — workaround needed
`struct Summary { stats: ColumnStats[MAX_FIELDS] }` passes the checker and gcc
fails: `unknown type name 'ColumnStats_32'` (`ColumnStats_32 stats[32];`). A
literal length fails the same way (`P[4]` → `P_4`); Go is right. Known bugs
#37/#38 fixed scalar elements only. Workaround: the caller owns the arrays and
the struct holds `Slice[ColumnStats]` and `Slice[Group]` over them.

### 2. [BUG] `rho test` overwrites the application's Go binary — workaround needed
`rho test` builds each test file for every target in `module.yml`, so the Go
build of the last test lands at `build/<name>`, the application's own binary;
`./build/csv_report sample.csv` then printed `[PASS] …`. `docs/tools/testing.md`
says tests are built through the C backend only. Workaround: `rho build .` after
`rho test`.

### 3. [BUG] `str_at` and `str_slice` are O(length) on C and O(1) on Go — workaround needed
Walking a 600 KB file with `str_at`/`str_slice` took 8.8 s on C and 0.02 s on Go;
the time is quadratic. The generated `runtime.c` calls `strlen(s)` on every call.
Workaround: read the file in 16 kB buffers with `std.os.fs` and `std.os.sys` and
build each line from bytes (C: 0.09 s for 40,000 rows).

### 4. The C backend cannot read its command line — blocker
`get_args()` and `rho_arg(i)` are Go-only (documented, and refused by name), and
nothing gives a C program its `argc`/`argv`, although a program "compiles to C
for a board or a host". Workaround: read `/proc/self/cmdline` (Linux only).

### 5. [BUG] Flags after the positional argument are refused, and the docs write them there — workaround needed
`rho build . --target go` and `rho new src/csv.rho --description "…"`, as written
in `tools/cli.md`, print only a usage line. Flags first works.

### 6. [BUG] A trailing comment in `tests/tests.yml` becomes part of the value, and the missing suite is reported as skipped — workaround needed
`- name: tests/csv   # the record parser` (the form `testing.md` shows) names a
suite that does not exist; it is reported as skipped and the run exits 0. A
suite that cannot be found should fail the run.

### 7. [BUG] `let` can be reassigned — doc gap hiding a bug
`syntax.md` says `let` binds a name that does not change, but `let i: u32 = 0`
then `i = i + 1` compiles, and `std/strings/string.rho` relies on it.

### 8. [BUG] A qualified generic call does not deduce from a closure — workaround needed
`slices.sort_by(groups, fn(a: Group, b: Group) -> bool {…})` is refused with
"has type fn(Group, Group) -> bool, want fn(T, T) -> bool"; `slices.sort_by[Group](…)`
says "Group is built into the language and is not a function value". The
unqualified call works.

### 9. [BUG] A line that starts with `(` continues the line above, and the error does not say so — doc gap
`var header: string[MAX_FIELDS] = fields` then `(*s).header = header` is read
as `fields(*s).header = …`: "expected an expression, got '='". `syntax.md` says
nothing about when a line continues.

## Gaps in what the language offers

### 10. Reaching a struct's field through a `Ptr[T]` is undocumented — doc gap
`s.n` on a `Ptr[St]` gives "member access requires a struct" with no hint (once
with no line or column). `(*s).n` and `s[0].n` work.

### 11. [BUG] No way to print a floating-point number — workaround needed
There is no `f64_to_str` or format function. Workaround: thousandths in an `i64`
with `milli_*`, plus a hand-written two-decimal formatter.

### 12. No way to write to standard error — workaround needed
`print` writes to stdout; `panic` adds `rho: `. Workaround:
`std.os.sys.write(2, buf[..n])`.

### 13. No conversion between `string` and `Slice[u8]` — workaround needed
Descriptor I/O is `Slice[u8]`, `std.os.fs.open` takes a NUL-terminated
`Ptr[u8]`, and text functions take `string`. Workaround: hand-written
`to_bytes` and `from_bytes`.

### 14. A string kept from a long loop keeps every string the loop builds — workaround needed
A read loop that stores a new group key keeps every line and field it parsed;
C memory grows with the file (7.5 MB for 40,000 rows).

### 15. Eight memory warnings on a correct program, some false — papercut
`input = args[i]` (a string that existed before the loop) is reported as kept;
`std.collections.slices.sort_by` warns for any struct holding a string; every
function filling a caller's `Slice[string]` and every bounded loop building an
output string warns that "its memory grows for as long as the loop runs".

### 16. No string-keyed map — papercut (documented)
`std.collections.map` hashes through `u32(key)`.

### 17. `rho_read_text` cannot tell a missing file from an empty one — papercut

## Tools

### 18. `rho run` cannot pass arguments, and `rho help run` fails — papercut
### 19. Where the binaries go is not documented — doc gap
With `c` and `go` targets the Go program is `build/<name>` and the C one
`build/output/<name>`; the build prints a CMake directory.
### 20. [BUG] `rho lint` reports false problems — papercut
`test fn` is "unexpected 'test' at the top level"; generated `build/` C is
linted; `rho-module-name` fires on what `rho init` writes; `rho-unused-import`
is false for bare calls; `rho-shadowed-binding` fires for a binding declared
after the loop.
### 21. [BUG] `rho fmt` cannot read test files, and drops comments (#49) — papercut
### 22. [BUG] `rho test` output surprises — papercut
A printed `[FAIL] x` line replaces the declared cases; `std.assert` `fail` prints
no newline; `--verbose --echo` prints lines twice; TAP `ok 3 - name` keeps the
number and dash.
### 23. [BUG] Noise on every C build — papercut
A `return discards 'const' qualifier` warning from the generated runtime, and an
unasked-for stack estimate on a host build.
### 24. Rho tests only run on C — doc gap
A Go-only bug or a C/Go difference is invisible to `rho test`; `tests/parity.sh`
fills the gap here.
### 25. The editor reported parse errors in std that the compiler accepts — papercut
Probably an older language server; nothing says how to check which one runs.
### 26. `rho init` details — papercut
`csv-report` silently became `csv_report`; the generated `@brief` ignores
`--description`; `--target c,go` writes `language: go` unexplained; `rho new
tests/x_test.rho` suggests including `main.rho`, whose `main` would clash.
### 27. `rho_arg` and `get_args` are underspecified — doc gap
### 28. A path for `std.os.fs.open` is a `Ptr[u8]` — papercut
### 29. A Romanian comment in std (`// ultimul segment` in `split`) — papercut
### 30. No way to say a function does not return — papercut

## What worked well

- The builtins page's C and Go columns, and refusals by name at the call.
- `rho init`, `rho new`, test discovery, the `[PASS] name` protocol and JUnit.
- `milli_*` decimals, identical on both backends.
- C/Go parity once it compiled, including stderr and exit status.
- Closures with `sort_by`, `Option`/`Result` with `match`, zero-initialised
  locals and the `Slice[T]` rules.
- Diagnostics: file:line:col and what to write instead.
- `known-bugs.md`, whose candour saved a `rho fmt -w`.
