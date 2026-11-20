# Changelog

Notable changes to the Rho toolchain, one section per release. The section for
the version being published is what a registry shows against that version, so
this is the file the documentation site's overview reads.

## [0.1.0] - 2026-10-12

First release of the Rho toolchain: the compiler, the C and Go backends, the
standard library, the module and package tools, and the component set — the
formatter, linter, documentation generator, test runner, debugger, target specs
and the `rho keeper` dashboard.

Added the module registry and its official deployment at
https://registry.rho-lang.org, where components and plugins are published and
installed from, and the documentation site service that serves these pages.

Modules gained a release history: a `CHANGELOG.md` beside the manifest, carried
in the package, whose section for a published version is recorded against that
release.

## [0.1.1] - 2026-10-20

The C runtime's symbols are callable from a program again. A fragment of the
runtime is emitted into the runtime's own translation unit and called from the
program's, and the prototypes the program was given said `static` — so a
program that called `panic()`, read an environment variable or loaded a YAML
file was compiled with a call to a symbol its own translation unit did not
define, and the link failed on a name that had been declared right there. The
prototypes and the definitions are external now, and the rule is a test.

A program's stop is the platform's, the way its console already was. A platform
names the function in `RHO_PROGRAM_HALT`, and a panic on a part ends there
rather than in the C library's `exit` — which is a firmware carrying an exit
path it has nowhere to go from. A platform that says nothing keeps the hosted
behaviour, and the platforms the runtime knows keep their own.

A pointer to a pointer is a pointer to the pointer: `Ptr_Ptr_Thing` is `Thing**`
in C, where it stopped at one star and named a different type than the program
meant.

Two different generic applications that mangle to the same name are reported by
the backend rather than one of them dropped: a program that compiles and means
something other than it says is worse than one that fails to compile.

The handbook has a page on platforms: what one is, the target spec that says
what the part is, and the console and the stop a program on a part is given.

A generated project names its program before the target file is read, so that
the target file can say how the board is reached in terms of the program it
builds: the `flash` target a build carries now names the file it wrote, and
splits the command into arguments rather than handing the tool the whole
command as one name it cannot parse.

## [0.1.2] - 2026-10-21

A loop can `continue`: the statement that leaves the innermost loop and goes
on with the next thing the loop has to do, which is what a guard inside a loop
body has always meant. A `continue` outside a loop is refused, as a `break`
outside a loop is.

The step of a `for` loop now comes before the body rather than after it, and it
is a real assignment rather than a piece of text. Two things follow. A `continue`
in a `for` no longer skips the step, so a loop whose body continues is a loop
that ends. And a `for` loop advances at all: the step was written in a form the
backends did not read, so the index never moved and the loop repeated its first
element for ever.

The native YAML reads more of a document: `yaml.value` takes a dotted path, so
`board.connectors` is the connectors of the board; `yaml.kind` says what a
value is — a list, a map, a string — because a program that reads a document
needs to know what it read; and `yaml.keys` says what a document holds.

A program can read a file and write one — `rho_read_text` and
`rho_write_text` — and the arguments of a program it runs are split on
whitespace honouring quotes, so a name with a space in it is one argument and
not two.

The C runtime answers the same as the Go one, which it did not. Its YAML
walked a document by its top-level keys and nothing else; it now walks a dotted
path the way the Go runtime does, and says what a value is and what a document
holds. A program's configuration is read the same on either backend too: the
name under the `user:` section, which the Go runtime was reading from the top
level instead.

The C runtime can run a program as well. The program is run directly rather
than through a shell, and what it said is written where the caller asked for
it, so a program that orchestrates other programs means the same thing on
either backend.

## [0.1.3] - 2026-11-01

A generated service can stream a file as Server-Sent Events with
`web.http.sse_file`. Each line is one event, a `__DONE__:<status>` line closes
the stream with a `done` event, and the stream follows a file that is still
being written. The generated Go server keeps the connection open for the length
of the action and sends keepalives; a long build or flash no longer trips a
write deadline, and the read deadline no longer closes a stream after a minute.

The `gateway` port — serial, UDP and clock access for a program that relays a
device's frames — gains `gateway_wall_time_ms`, the host's Unix time in
milliseconds. A device's `timestamp_ms` is its own uptime; the host can now also
record when it received the frame, which is what places a reading on a calendar.

Module manifests may declare `capabilities` and `deployment`. A board says which
roles it can take (`gateway`, `relay`, `node`, `edge`, `host`), what it has
(`wireless`, `ethernet`, `links`, `buses`, `deploy`), and how a deployment
reaches it (`method`, `arch`, `kconfig`, `apps_dir`). A driver says it is a
sensor, its bus and what it measures, and names the example, image, Kconfig and
boards a deployment builds. The module schema validates these blocks, including
`board.summary`, and a deployment wizard's description has a schema of its own.

A generated Go runtime emitted `rhoGetenv` twice when a program read an
environment variable, which was a redeclaration in the generated runtime. It is
emitted once now, and a test holds it.

## [0.1.4] - 2026-11-02

A module manifest may declare the plugins an application loads:

```yaml
plugins:
  - name: alert-log
    enabled: true
    config:
      path: alert.log
  - name: email-alert
    enabled: false
    config:
      to_address: ops@example.internal
```

The list is validated: a plugin needs a name, `enabled` is a decision rather
than a word, and `config` is a map whose keys belong to the plugin. The schema
says what an application asks for; the plugin registry and its manager are the
application's own.

The Go backend also learned two shapes an application needed: a fixed-size
array parameter is emitted with one dimension rather than two, and a `Ptr[T]`
built from a reference carries the address in the field the generated struct
has. Both have tests in `src/codegen/goexpr_test.go`.

The SQL port now splits a schema on statement boundaries that respect line and
block comments as well as string literals, so a semicolon in a comment does not
end a statement early. The version stays 0.1.4; the binary is rebuilt from the
same source.

The documentation site lists the newest release first, so its overview shows the
current version rather than the first four releases ever written.

The `gateway` port provides `gateway_http_get` and `gateway_http_post`, the HTTP
requests a program needs to poll and switch a device on the network.

## [0.1.5] - 2026-11-04

The documentation site image is now a multi-stage build: the live docsite
service runs in a Go builder and is mirrored into static pages, so the runtime
is a slim static server with the same landing page, menu, release notes and
generated documentation as before.

Release notes on the documentation site preserve changelog paragraph breaks
instead of flattening a release into one block.

Generated servers can carry an `auth` middleware: ES256 session cookies verified
with a pinned public key, direct and reverse-proxy origins, one-use login
tickets, local sessions, and HTTPS-aware cookies. `rho auth` gained `keygen`,
`ticket` and `verify`, and generated responses can carry headers, which is what
lets a login set a cookie and redirect.

## [0.2.0] - 2026-11-05

Time series are now a toolchain feature rather than an application convention.

- `sql_exec_many` runs one prepared statement once per row in a single
  transaction, so a service can write a batch instead of committing every row.
  The sql port now writes its affected count through the out-parameter instead
  of into a copy, and no longer takes the handle-table lock twice in a statement.
- A `telemetry` port ingests a sequenced outbox of readings into a project's
  store: it polls the outbox, writes a page in one transaction, serves latest
  and time-range queries, and rolls completed minutes and hours up before
  retention. A service can serve latest values and series over the schema its
  project declares.
- The `gateway` port lists the generic sensor fields a firmware publishes, so a
  listing carries every `m<index>.<field>` a board reports, and records sensor
  frames in its history.
- The `telemetry` port excludes fault readings from latest values, rollups and
  series, walks a JSON path for HTTP device descriptors, applies the
  descriptor's scale before storing the value, and records a reading a service
  read itself.
- The `gateway` port sends a bearer token when the deployment names one.

## [0.3.0] - 2026-11-06

Security hardening across the services and ports that carry device readings.

- `rho auth` now mints scoped tokens and tickets (`--scopes`), and can revoke a
  ticket by id into a list the generated middleware reads locally.
- The generated auth middleware enforces required scopes, injects the verified
  subject, scopes and audience into request headers, and refuses revoked tokens.
- Generated servers serve TLS when `RHO_TLS_CERT` and `RHO_TLS_KEY` are set.
- The `gateway` port's uplink honours `UPLINK_BIND`, so a relay listener can be
  confined to one interface.
- `sql_exec_many` and the `telemetry` port (outbox ingestion, latest, series,
  rollups, retention) shipped in 0.2.0 are documented here as released.

## [0.4.0] - 2026-11-08

A web, time-series, rollout and provisioning release.

- The template engine nests blocks of the same kind, supports `{{else}}`,
  `{{#each}}`, `{{#with}}`, whitespace control and `{{t "key"}}` translation.
  Flat JSON locale catalogs are embedded at build time, with a build-time
  report of the keys a catalog lacks.
- HTMX and Server-Sent Events are supported: pages answer fragments when
  `HX-Request` is set, and named SSE streams carry HTML fragments; the template
  renderer gained the tests that keep these contracts from drifting.
- The `telemetry` port bounds the WAL, batches retention, supports incremental
  and full vacuum, reports storage status, and stores actionable faults in a
  dedicated table with list, clear, prune and live-event paths.
- Added `rho-artifact` for signed host and firmware artifacts and `rho-bootstrap`
  for first-boot identity, permissions and enrollment with an enrollment server.
  Both are published to the remote registry.
- Flash and deploy verify signed artifacts before touching a board or target.
- Includes across modules use bare names resolved by the compiler; an
  ecosystem's dependencies are declared once, as a named group under `deps`, and
  checked at build time.
- `scripts/check.sh` is idempotent when a version is already published.

## [0.5.0] - 2026-11-09

Structured logging for services, and a formatter that keeps what it prints.

- The `logemit` port now filters levels, writes text or JSON, rotates its own
  file, and is configured at runtime with `log_configure`; the environment
  supplies defaults for level, format, path, size, retention and stderr.
- `rho.log` gained `configure` and `fatal`; the component's Go logger gained
  `Fatal` without terminating the process.
- `rho fmt` preserves `const` declarations, fixed array lengths and dimensions,
  `[value; count]`, `Ptr[T](&x)`, local `var`, `continue`, `for ... in` with
  `reverse` and `step`, and `match`.
- Added port, rotation, filtering, format and fatal tests.

## [0.5.1] - 2026-11-10

Security fix for archive extraction.

- The artifact verifier and `rho-artifact` recognize the manifest and payload
  by their exact names and write the payload under the constant payload name;
  the payload extractor uses `os.Root` and refuses absolute, parent and volume
  paths before writing.
- `rho keeper`'s tar and zip unpackers use `os.Root` and validate every entry
  before writing. The host-tar fallback validates the archive listing before
  extraction instead of extracting first.
- Removed the post-extraction “cleanup” that could delete any path named in a
  malicious archive listing.
- Manifest loading and the scaffold generator open project files through
  `os.Root`; a `generate:` file, template path or output path cannot leave the
  project or the destination. Added tests for escaping generate, template and
  output paths.
- Added tests for escaping tar, zip and payload members.

## [0.6.0] - 2026-11-13

Modules become units with their own scope, exports and interfaces; services get
an identity; the C and Go backends agree on three more programs; and the runtime
answers the clock and a program's output as values.

**Modules and interfaces**

- A module owns its types, constants and tables: `a.Type` names another
  module's, and a module that declares `sql:` has its tables and statements
  generated into the applications that use it, the module's version winning.
- A module says what it exports and is imported through its declared
  dependencies only. Each published version records its exports, and
  `rho module check` (and publishing) refuses a breaking change without a new
  major version.
- Interfaces are bound to providers in the manifest (`bind:`), with settings a
  running installation can change; a dependency may use an interface its
  application does not bind (its calls answer nothing, and the build says so);
  an interface imported twice is bound once.
- Modules carry files and are included by name; `module.id` is a declared
  identity, with `previous_ids` for a module that was renamed.

**Dependencies**

- The lock pins a module by its version, dependency groups name a repository,
  and lockfiles are workspace-relative and portable (`${RHO_WORKSPACE}`,
  `${RHO_HOME}`).

**Runtime**

- `rho_now_s`, `rho_local_date`, `rho_local_clock` and `rho_utc_offset_at` read
  the clock from the process, on both backends. A time is shown with the offset
  it had, across a change to summer time.
- `rho_exec_capture` returns a program's output as a value — one argument per
  line, no shell, no file another caller could be reading — and
  `rho_http_body` (Go) returns a response's body the same way. Together they
  replace the `date`-into-a-file and report-file pattern whose concurrent
  callers could read each other's answers.
- HMAC-SHA256 and random bytes, `rho_sleep_ms`, `rho_exec_args`,
  `rho_write_secret`, a service identity with an HTTP client and a stream
  follower, and flat YAML read and write.

**Auth**

- Client credentials and service tokens, issuer discovery and key fetch, a key
  provided by the issuer, and a fix to whom a relying party trusts.

**Code generation**

- A nested negation is two negations: `-(-b)` was emitted as C's `--b`, which
  compiled and decremented `b`. Every cast and unary operand is parenthesised,
  so a widened operand keeps its sign on Go (`(int64)-b` was not Go).
- An if expression evaluates only the branch taken on Go, as in C:
  `if n > 0 { t / n } else { 0 }` divided by zero there. Arms of two widths are
  cast to the expression's type.
- `~` is Go's `^`. A negative literal after a cast is cast whole. The Go
  runtime's YAML reader unquotes a value only when it is quoted as a whole.
- The SQL read port writes no more rows than the caller's slice holds, reports
  a row it cannot scan as a failure rather than a row of zeros, and narrows a
  number from whichever storage class SQLite returns.
- A conditional written inside a string literal is left as text; unused locals
  no longer break the Go build; a module-level binding cannot be assigned.
- All of the above are in the parity suite, which runs each program on both
  backends and compares what they print.

**Projects, CLI and architecture**

- `project.yml` says only what differs from the built-in descriptor;
  templates have a folder of their own; ports are declared by modules.
- `rho new` writes the file header; an application can be published;
  `rho deploy` builds and pushes images; `rho test` runs C host tests;
  `rho module publish --local` installs without uploading.
- An ecosystem's names are out of the core: platform ports are provided by an
  integration plugin, and transport framing is out of the generator.

**Security**

- Manifest and scaffold paths are confined with `os.Root`.
- The `same_origin` middleware refuses a write a browser says came from another
  origin.

## [0.7.0] - 2026-11-14

The text an application reads from a form and writes to a page is the
toolchain's, the same on both backends: `str_trim`, `str_url_encode`,
`str_url_decode`, `form_field(body, name)`, and decimals held as thousandths —
`milli_valid`, `milli_parse`, `milli_format` and `milli_round(milli, places)`.
A decimal is read with a dot or a comma, digits past the third decimal are
dropped, and a text that is not a number is 0, which `milli_valid` tells apart
from a real 0. Applications carried their own copy of each, and the copies had
drifted: some read a comma, some a sign, some neither.

A parity case can say what both backends must print, not only that they agree.

## [0.7.1] - 2026-11-14

Safer defaults and fixes from a security and usability review.

- A generated service listens on 127.0.0.1:8080 when `RHO_HTTP_ADDR` does not say otherwise, where it listened on every interface; a service meant for the network says `RHO_HTTP_ADDR=:<port>`, as a supervisor that deploys services already does for them.
- `redirect()` refuses a location with a line break (it would add a header of its own), and the sign-in callback returns only to a path on its own service: no `/\\host`, no control characters.
- A program's report file (`rho_exec`, `rho_exec_args`) is written owner-only, in an owner-only folder: a program's output may carry a secret.
- `rho install --force` removes the toolchain — programs, standard library, components, plugins, local registry, caches — and keeps the credentials, the configuration, application state and the fetched toolchains, where it removed all of `~/.rho`.
- `rho init` works in the folder it is run in (the default `--project .` was refused).

## [0.7.2] - 2026-11-14

Usability.

- A generated service started by hand listens on loopback at the port its manifest names (`module.port`, else `deploy.port`), so a service comes up where its documentation says it does.
- `rho help <command>` shows that command's help; `rho build -h` says what it builds; the command list describes `build`, `run`, `install`, `protocol` and `env` as they are.
- `rho pkg … --help` prints help instead of creating a directory named `--help`.
- A folder with no `module.yml` is told so, and how to start one, instead of naming the legacy `rho.mod`.
- rho-doc 0.1.4 reads manifests with a `sql:` block, so modules that declare one are documented.

## [0.8.0] - 2026-11-14

More of what every module carried a copy of is the toolchain's, the same on both
backends: `i64_to_str` and `i64_from_str` (whole numbers of any width — `int_to_str`
takes a u32), `str_lower` and `str_upper` (ASCII), `str_field(s, sep, index)` (one
field of a separated line), `json_str_field` and `json_num_field` (the first value
of a key in a flat JSON document), and `app_workspace()` — where an application
keeps its data: `RHO_APP_WORKSPACE`, else an ecosystem's own application
workspace variable, else `~/.rho/apps/<RHO_APP_NAME or the program's name>`, one
rule instead of one per module.

## [0.8.1] - 2026-11-14

A supervisor can serve its applications behind its own origin: the `app_proxy`
middleware forwards `/app/<name>/…` to the application's loopback port (listed in
a file the supervisor writes, `name port` per line), streams the answer so an
event feed is relayed as written, never forwards the supervisor's own cookie
(`strip_cookie`), scopes the application's cookies to its path, and rewrites the
root-relative links of its pages, its redirects and its stylesheets onto the
prefix. Cookies are scoped by host and not by port, so this is what keeps the
supervisor's session out of the applications' requests.

The auth middleware builds its return address and callback from
`X-Forwarded-Prefix` and `X-Forwarded-Host` when they come from the loopback,
where such a proxy connects from, so an application signs in through the
supervisor's origin as well as on its own port.

## [0.8.2] - 2026-11-14

A project's own tree may carry the ports it is built with: a plugin in it that
integrates its platform and provides ports (`serves:` and `integration.provides.ports`,
no `extends:` needed) is read when the project is generated. A port is Go source
compiled into the project, as its sources are, so an application can keep one
beside its code and version it with it, rather than in a platform plugin it shares
with every other application of that platform.

## [0.8.3] - 2026-11-14

A build says when a file calls a function of another module in the program that the module does not list in `provides.exports`. An included module shares the program's namespace, so the language cannot keep its private functions private; the warning shows where a module's declared boundary is crossed, wherever in the program the call is. A name the calling module defines itself is its own and is not reported. The warning that told every include of such a module to import it instead is gone: a module that calls the application's hooks has to be included, and the calls that do cross its boundary are now named.

`str_replace(text, from, to)` replaces every occurrence of one text with another, on both backends; an empty `from` replaces nothing. Modules carried copies of it.

`rho module publish` given a relative path read no exports from a module whose main only includes its other files, and refused the release as removing all of them.

## [0.8.4] - 2026-11-14

`std.math.float`, `std.math.fast` and `std.math.linear` build and agree on both backends: the math they call is the runtime's own (`rho_sqrt` and the rest, libm on C, the math package on Go), and a C project that uses it links libm. `std.os` builds on C — the clock, the environment, exit, and file descriptors through the runtime rather than raw syscall numbers that held only on x86-64; on Go the descriptor calls stop with a message saying they need the C backend. `setenv` takes the value and overwrites.

A service whose module declares its own `GET /healthz` answers it itself, where registering the path twice stopped the service at startup.

`rho protocol`: a `string[N]` field is N bytes on the wire for every target — the C size macro and the Go struct had counted N strings of 64 bytes — and the Rho target accepts it. Decoded strings end at their first NUL.

`rho module install` installs a provider; `rho module enable` finds an installed library module, which resolves as a dependency only once enabled. A plugin that only provides an ecosystem's data is held to the integration contract. `rho module activate` says a version is required.

## [0.9.0] - 2026-11-14

Strings in a C program no longer leak. Every string the runtime builds comes from a string arena, and the compiler releases what a loop iteration built at the end of that iteration (and before `continue`), and what a statement built when nothing keeps it; a string a variable keeps is compacted down. On a board — a target with a RAM region and no `deploy` block, or one that sets `linker.string_arena_bytes` — the arena is a fixed buffer, so strings need no `malloc`, and running out stops with "string arena exhausted". On a host it grows in chunks. A loop that keeps what it builds in something declared outside it cannot be released: a board build refuses it and names the loop and the variable, a host build warns. C code that keeps a string it was given copies it. `rho_concat` never returns NULL, and `print(int_to_str(x))` and `print(i64_to_str(x))` build no string.

`request_remote(req)` is the address of the connection a request came on, which no header can change.

An unsigned value plus an expression made only of written numbers (`up + 2 * N`) keeps the unsigned type on the Go backend, where it was widened to a signed one that Go refused to assign.

## [0.10.0] - 2026-11-14

`Ptr[T]` is one type on both backends: `T*` in C and `unsafe.Pointer` in Go, which the Go collector follows. A cast from an integer or another pointer, `to_u64`, indexing, `*p`, `p + n` and `&x` mean the same on both, and `std.os` file descriptors (`open`, `read`, `write`, `close`) work on Go too. `std/core/ptr.rho` declares no struct any more, and a program may not declare one named `Ptr`. A generic struct's pointer field keeps its element type.

C declarations come in dependency order, so `std.strings.string` builds on C; an alias to a generic type brings that specialisation with it. A function a program declares wins over a built-in of the same name when the built-in does not apply to the argument (`len` of a `String`). The built-in `len` works on C for a string and a fixed-size array; `len` of a dynamic array stops a C build and names the call.

## [0.11.0] - 2026-11-14

Every module of the standard library is tested on the backends it is for, and says which they are (`// @backends`, `// @port`); a program that uses a module or a built-in its target cannot supply is refused by name before anything is generated, instead of failing at link time or with an undefined Go name. `docs/tools/known-bugs.md` lists what was found and fixed.

What an integer operation means is the same on both backends. Signed `+ - *` and negation wrap (two's complement) at every width. A shift by a constant of the type's width or more is refused; at run time an unsigned shift that wide gives 0, a signed right shift gives the sign, and a signed left shift is done on the unsigned type. Division or remainder by zero, and an index past the end of a fixed array or a `Slice`, stop the program with the same message on both (`rho: division by zero`, `rho: index 7 out of range [0,4)`); a constant that would do so is refused. A constant that does not fit the type it becomes is refused (`constant -3 overflows u32`). `[T]` and `Ptr[T]` stay unchecked pointers.

A panic on a hosted C program goes to stderr as `rho: <message>`, as on Go, and `abort()` is a panic. A program's own `extern fn` gets a C prototype. Standard-library function bodies are type-checked when their module is loaded. `std.collections.bitset` is built from a pointer and a size (`bitset(&words[0], n)`); `std.sql.ok` returns `Result[T, string]`. Fixed along the way: float literals meeting integers, fixed-point multiplication in 64 bits, value parameters of generics (`Array[u32, 4]`), arrays of generic structs, `std.core.range`, std externs without C prototypes, a Go server without templates, narrow `-`/`~`, and shift promotion.

### Breaking

These were refused or changed at once, without a deprecation first, because the old behaviour was wrong code on at least one backend.

- `std.collections.bitset`: `BitSet[N]` holding its own `u64[N]` is gone; a bitset is built over words the caller owns. `let b: BitSet[4] = ...` becomes `var words: u64[4]` and `let b: BitSet = bitset(&words[0], 4)`, and `set[N](b, i)`, `clear[N](b, i)`, `get[N](b, i)` become `set(b, i)`, `clear(b, i)`, `get(b, i)`. (0.14.0 changed the constructor again; see there.)
- `std.sql.ok[T, E](value, code, message)` is `ok[T](value, code, message)` and returns `Result[T, string]`: drop the second type argument, and declare what it returns as `Result[T, string]`.
- A constant that does not fit the type it becomes is refused (`constant -3 overflows u32`): write a value the type holds (`let b: u8 = 44` for `300`), or do the arithmetic in a wider type and convert the result.
- A shift by a constant of the type's width or more is refused (`shift count 32 overflows type u32`): write the result (`0`), or shift a wider type: `u64(x) << 32`.
- A division or remainder by a constant zero, and a constant index outside a fixed array, are refused: remove the expression; it stopped or misbehaved at run time.
- An index past the end of a fixed array or a `Slice` stops the program with `rho: index i out of range [0,n)` instead of reading past it: check the index before using it.
- A panic on a hosted C program, and `abort()` on both backends, write `rho: <message>` to standard error and exit with status 1 (`abort()` used to stop C with SIGABRT, status 134): a script that tested for 134 tests for 1, and reads the message from standard error.
- `std.core.builtin` no longer declares the pointer, memory and arithmetic helpers (`ptr_null`, `ptr_add`, `load_u8`, `add_i32`, `str_concat`, …), none of which compiled or linked on any backend: use `Ptr[T]` and the operators.

## [0.12.0] - 2026-11-14

Errors point at the mistake. Every position keeps the file it came from, through `include` and `import`, so an error in an included file, an imported module or the standard library is reported at that file's line and column — the column of the offending expression. Every independent error in a program is reported in one run (up to 20 shown, "and N more"), without cascades: an undefined name once, nothing that follows from a failed `let`. A parse error recovers at the next declaration.

One diagnostic, everywhere: `file:line:col: error|warning: message`, with notes (`X is declared here`, `helper.rho is included here`, what a generic was specialised with). `rho build`, `rho lint` (a new `rho-check` rule) and the language server report the same thing at the same place; the editor checks a file as part of its project, with its unsaved text. The twenty most common messages say what is wrong and what to do, with a "did you mean" where a name is close to one in scope.

Fixed: columns were one too far right on every line after the first; the language server published every diagnostic one line low.

### Breaking

- `rho build` and `rho run` print a program's mistakes as `file:line:col: error: message`, with paths relative to the working directory, instead of `rho build: <message>`: a script that matched the `rho build:` prefix on a compile error matches `: error: ` instead. The exit status is unchanged.

## [0.13.0] - 2026-11-14

Enums, with or without data. `enum Shape { Circle(i32), Rect { w: i32, h: i32 }, Empty }` and plain enums with numbers (`enum Mode { Off = 0, Auto = 2 }`); generic enums with default type arguments. A value is built as `Shape.Circle(3)` and taken apart with `match` or `if let`: variants bind their payloads, patterns nest, literals compare, and a `match` that misses a variant is refused with the missing ones named (an arm that can never be reached is a warning). Plain enums are the narrowest integer on both backends and convert to and from numbers (`Mode(n)` stops the program if `n` is not a value); data enums are a tag and a union, copied as values, and follow the string arena's rules. `==` works on plain enums.

`Option` and `Result` are language enums now (`enum Option[T] { None, Some(T) }`, `enum Result[T, E = Unit] { Err(E), Ok(T) }`), with their helpers; the field-style reads (`.is_some`, `.value`, `.is_ok`, `.ok`, `.err`) and struct-style literals keep working, and `Option[Ptr[T]]` is still a bare pointer in C. A protocol schema or a SQL column takes a plain enum as a number and refuses a data enum.

Warnings reach `rho build` and the editor. Fixed: Go if-expressions over a call, `Option[Ptr[T]].value`'s type, nested prelude generics, and `rho fmt` dropping enums.

### Breaking

- `==` and `!=` between two `Option` or two `Result` values are refused, since each is an enum that carries data (on Go they compared field by field; on C they did not compile): test the variant with `match` or `if let`, as in `if let Option.Some(x) = a { ... }`, or compare `a.is_some` and `a.value`.

## [0.14.0] - 2026-11-14

`Slice[T]` is the everyday sequence, on both backends. A fixed array `T[N]` becomes a `Slice[T]` over all N elements wherever one is wanted (a `let`, an assignment, an argument, a return, a struct field), and still decays to `[T]`. A slice has `s.len` and `len(s)`, checked indexing, cuts `s[a..b]`, `s[..b]`, `s[a..]` and `s[..]` (checked `0 <= a <= b <= len`, stopping with `rho: slice [a..b] out of range [0,len]`), `for x in s` with `reverse` and `step`, and `Slice.from_raw(p, n)` over a `Ptr[T]` or a `[T]`. `append` grows a slice into the fixed array it was cut from: `var buf: u8[64]`, `var s = buf[..0]`, `s = append(s, v)`; past the array it stops with `rho: append past capacity 64` on both backends. A slice that would point at a function's own storage after it returns is refused.

`[T]` stays a bare pointer: a cut of it is refused, and `len`, `for` and `append` of it are refused on C with a pointer to `Slice[T]` and `Slice.from_raw`.

A C local declared without a value starts at zero (`0`, `""`, `NULL`, `{0}`), as on Go.

### Breaking

- `std.os.sys`: `write(fd, buf, len)` and `read(fd, buf, len)` take `buf: Slice[u8]` instead: `write(fd, buf[..n])`.
- `std.collections.bitset`: `bitset(&words[0], n)` is `bitset(words)`, with `words` a `Slice[u64]` or a `u64[N]`.

Fixed: an element of a `string[N]` was typed `u8`; `var x: T` in a generic body kept the name `T`; `Slice[T]` was not deduced from a fixed-array argument.

## [0.15.0] - 2026-11-15

A `[T]` means the same on every backend: it is a bare pointer, so `len`, `for` and `append` on one are refused on Go as they are on C, with the place named and the `Slice[T]` to use. `read_file` returns a `Slice[u8]` and `write_file` takes one (a `u8[N]` or a cut of one is passed as it is).

A fixed array's length may be a module constant: with `const MAX: u32 = 32`, `u32[MAX]` is `u32[32]` in a local, a parameter or a struct field, so it can be cut and becomes a `Slice[T]`. A struct field of a fixed-array type compiles on C (`uint32_t items[4];`).

### Breaking

- `len(p)`, `for x in p` and `p = append(p, v)` on a `[T]` no longer build for Go. Declare a fixed array and cut it: `var buf: u8[4096]`, `var p = buf[..0]`, `p = append(p, v)`; or take a `Slice[T]` parameter.
- `read_file(path)` is a `Slice[u8]`: `let data: [u8] = read_file(path)` becomes `let data: Slice[u8] = read_file(path)`; `extern fn read_file(path: string) -> [u8]` becomes `-> Slice[u8]`, and `write_file`'s `data: [u8]` becomes `data: Slice[u8]`.

Fixed: a fixed array sized by a constant was not a fixed array (#37); a struct field of array type did not compile on C (#38).

## [0.16.0] - 2026-11-15

Functions are values, on both backends. A function type is written `fn(A, B) -> R` (or `fn(A)`); a named function, an imported one, or a literal `fn(x: i32) -> i32 { return x + k }` is a value of it, and any expression of a function type can be called — a local, a field, an element of a `Slice` or an array (`(fn(u32) -> u32)[4]`), an `Option` payload, a call's result. Generic functions deduce through function types (`map_into(s, out, f: fn(T) -> U)`), and a generic function is a value where a function type says which specialisation is meant.

A literal captures the names it uses by value, when it runs. On C a function value is a function pointer and an environment; the environment lives in the string arena and follows its rule — released with the loop iteration or statement that made it, kept by a `let` or a return, refused on a board where a loop would keep one per iteration. Calling a value that holds no function stops the program with `rho: call of a function value that holds no function` on both backends.

Refused, each with what to write instead: assigning to or taking the address of a captured name; capturing a `Ptr`, `[T]`, `Slice[T]` or a value holding one (pass it as a parameter); capturing a fixed array; fixed arrays in a function type; comparing or printing a function value; a built-in used as a value.

### Breaking

None. The refusals above apply to function values, which are new; `return None` for `None()` (#41) never built, and now says so at the line.

Fixed: type arguments containing commas were split wrongly, so `Slice[fn(u32) -> u32]` and nested applications got the wrong element type (#39); a bare generic function as a value (`return None`) produced invalid C and Go and is now refused with the fix (#41). rho-lint no longer reports an import used only in a type as unused (#40).

## [0.17.0] - 2026-11-15

Generic functions can say what their type parameters must support: `fn sum[T: Number](xs: Slice[T]) T`, `[T: Ordered + Equal]`. The constraints are `Integer`, `Float`, `Number`, `Ordered` (numbers and strings) and `Equal` (numbers, bool, strings, plain enums); each implies the ones it contains. Inside the body the operators a constraint allows are checked once, and every specialisation lowers them exactly as the concrete type would — wrapping, checked division and the same panics on both backends. A type argument that does not satisfy a constraint is refused at the call, naming both.

Type parameters are deduced through `Slice[T]`, fixed arrays, `Option[T]`, function types, nested generic structs and the expected type, with literals consulted last (`max(5, x)` for a `u8` x is `max[u8]`). Type arguments may be written (`f[u32](x)`), and a generic function with its arguments written is a value (`apply(doubled[u32], 21)`). A deduction that fails names the parameters and suggests `f[A, B](...)`.

New in std: `std.core.cmp` (`min`, `max`, `clamp`) and `std.collections.slices` (`sum`, `contains`, `index_of`, `map_into`, `filter_into`, `sort`, `sort_by`), on every backend.

### Breaking

- `<`, `<=`, `>`, `>=` on an unconstrained type parameter are refused: write `[T: Ordered]`. `==` and `!=` on one are still accepted.
- A function named like a C type word (`fn double`, `fn float`) is refused at its declaration, as a parameter or a local of that name already was: rename it.
- `==` and `!=` between two structs are refused on every backend (C could not compile them; Go compared field by field): compare the fields.

Fixed: string ordering compared pointers on C (#42); `x as T` and `T` locals in a specialisation kept the name `T` (#43, #44), which made an `i32` `+` in a generic plain C `+`; a nested generic call deduced `add_T` (#45); a fixed array of a plain enum did not become a `Slice` (#46); #47 and #48 above.

## [0.18.0] - 2026-11-15

A written promise about change: `docs/stability.md` says what is stable (the language as documented, std modules tagged stable, the `module.yml` keys, the documented CLI and its exit codes, the diagnostic format), what is experimental, and how a change arrives — a deprecation warning in one minor release, then a `### Breaking` section with the exact edit in a later one; a patch release never breaks a program, and wrong code or unsound behaviour is refused at once with the fix spelled out. Every release note from 0.11.0 has a Breaking section.

`// @deprecated use X instead` in the doc comment of a function, struct, enum, variant, constant or alias — or in a module's header — makes every use from another module a warning that names the replacement and points at the declaration (`file:line:col: warning: old is deprecated: use new instead`), in `rho build`, `rho lint` and the editor, which strikes the use through. The build still succeeds.

Every std module says how stable it is (`// @stability stable|experimental`); the API reference shows it on each page and in its index, and `docs/language/stdlib.md` lists every module with its tag and the reason.

### Breaking

None.

Known: `rho fmt` drops comments, tags included (#49); it is fixed before beta.

## [0.19.0] - 2026-11-15

What an outsider needed to write a command-line tool from the docs alone (`examples/csv-report`, with its findings in `FINDINGS.md`), and what a firmware-shaped soak showed.

A loop that keeps its latest string in a variable declared outside it — a string, a struct, an enum, an `Option` or a fixed array of them, up to 32 strings — now runs in bounded memory on a host and on a board: each iteration releases what it built except what those variables hold, which is moved down (`acc = acc + x` keeps only the newest `acc`). A loop storing into a `Slice` element, through a pointer, or keeping a function value is still not released, and says so.

A C program reads its command line: `rho_arg(i)`, `get_args()` and the new `arg_count()` work on both backends (on a board they are `""` and 0). New built-ins on both: `f64_to_str(x, decimals)` and `f32_to_str` (ties to even, identical digits on C and Go), `milli_format_fixed(m, decimals)`, `eprint(s)` to standard error, `str_to_bytes(s, out)` and `str_from_bytes(b)`; `rho_path_exists` works on C. `p.f` reaches a field through a `Ptr` to a struct. A struct literal is an expression inside an `if`/`while` header's parentheses and arguments. A qualified generic call (`slices.sort_by(xs, f)`, `mod.f[T](x)`) deduces like an unqualified one. A line starting with `(`, `[` or an operator starts a new statement, as in Go. `str_at`, `str_slice` and `len` no longer measure the whole string on C each call (a 1.3 MB scan: 78 s → 30 ms). `std.strings.utf8.first_codepoint` decodes UTF-8.

The tools: flags may follow the path (`rho build . --target go`, as the docs write it); `rho run <dir> args…` passes arguments and runs the program in the directory `rho` was started from; `rho help <command>` works for every command; `rho` exits with a component's own status. `rho test` builds its programs under `build/test/` and never overwrites the application's binaries, a missing or empty suite fails the run, and printed `[PASS]`/`[FAIL]` lines report declared cases rather than replace them. `rho lint` reads test files, skips generated files under `build/` (`--include-generated` reads them), and no longer reports an import used by a bare call, a binding declared after a loop as shadowed, or the module name `rho init` writes. `rho fmt` keeps every comment, prints `test fn`, `namespace`, `message`, `rpc` and bit widths, and parenthesises a cast's operand (it printed `((n*37)%600) as i32` as `n * 37 % 600 as i32`). `rho init` uses `--description` and says when it renames a directory into a module name. A C build names its binary; the stack estimate is shown only for a board. YAML comments after list items are comments.

Fixed: #49–#66 in `docs/tools/known-bugs.md`, among them array-of-struct fields on C, a Go-only build calling runtime functions under the wrong name, `arr[u32(i)]` read as a type argument, `ring[i % 4] = v` failing on Go, rho's own docsite refused by the backend check, and a `const` warning on every C build.

### Breaking

- Assigning to a `let` binding is deprecated and warns at the assignment, naming the binding; it becomes an error in the next minor release. Declare it with `var`.
- A continuation line that starts with an operator, `(` or `[` is now a new statement (an operator there is an error): end the previous line with the operator instead.
- `rho run` runs the program in the current directory, not in `build/` or the project root: pass paths relative to where you run it.
- `rho` exits with a component's own status instead of 1.
- `rho lint` no longer reads files under `build/`: pass `--include-generated` to keep the old behaviour.
- A test suite that names no test file, or a file with no `test fn`, fails the run instead of being skipped.
- A program function named `eprint`, `f64_to_str`, `f32_to_str`, `milli_format_fixed`, `str_to_bytes`, `str_from_bytes` or `arg_count` gives way to the built-in when called with the built-in's arguments: rename it.

## [0.19.1] - 2026-11-15

In a container or a pipeline, where root is the default, the "running as root" line is printed only to a terminal: a program that runs `rho` and reads its output — the keeper reading a sign-in ticket or the doctor's TSV — no longer finds the line inside its answer. `rho keeper sync` runs each install step as a program with its arguments (it ran `sudo apt-get install -y gcc-avr` as one program name, so every sync on a CI runner failed while the plan it printed read correctly); a fetch step runs `rho keeper fetch`.

### Breaking

None.

## [0.19.2] - 2026-11-19

A generated Go service given only one of `RHO_TLS_CERT` and `RHO_TLS_KEY` refuses to start, saying both must be set; it used to serve plain HTTP, so a deployment that meant TLS and mistyped one variable sent everything unencrypted.

`rho doc` reads a module's pages from `docs/doc/` when it has one, and leaves its requirements and architecture decisions to the rho-doc-req plugin; `rho req` (component rho-req) checks, traces, renders and edits requirements and ADRs across a workspace, modules nested in a repository included; `rho test` writes `build/test-reports/results.json` on every run.

`rho build --emit-c --test-report a,b` is a test build: each named function prints `[PASS] <name>` every time it returns (its body moves to `<name>__rho_test_body`; arguments and return value unchanged), and an ordinary build is untouched. `rho test` (rho-test 0.4.0) builds every Rho test file this way, so a `test fn` nothing reported did not run and fails — "never ran: main() does not call it", or "did not run: the program stopped before it" — and a failed case's reason is printed beside it. A test that `main` forgot used to pass whenever the program printed no result lines.

### Breaking

- A service started with only one of `RHO_TLS_CERT` / `RHO_TLS_KEY` exits 1: set both, or neither.
- A `test fn` that `main` never calls now fails `rho test`; call it from `main`. A test no longer needs to print its own `[PASS]` line.

## [0.19.3] - 2026-11-19

`request_query(req)` gives a go-server handler the request's raw query string, without its `?`. A handler that sends a request on elsewhere — the keeper redirecting its old `/app/<name>/…` addresses to an application's own port — had no way to keep the query: `req.query` passed the type checker but was emitted under a name Go's Request does not have.

### Breaking

None.

## [0.19.4] - 2026-11-20

A Server-Sent Events response carries the headers its handler added (`with_headers(sse_file(...), ...)`), as every other response does: the generated Go server wrote them only for responses that were not streams, so a stream could not answer a page on another port with `Access-Control-Allow-Origin`.

## [0.20.0] - 2026-11-20

A generated Go service is ready to be supervised by systemd. A signal to stop ends its open Server-Sent Events streams at once: every request's context derives from one base context that `SIGINT`/`SIGTERM` cancels before `Shutdown` waits, so a restart with a page open takes about a second instead of the full 30 s grace (a stream still waiting for its file to appear also watches its context now). Under a unit with `NOTIFY_SOCKET` the service sends `READY=1` once it listens, `WATCHDOG=1` every half `WATCHDOG_USEC` while its own `/healthz` answers `200` (a hung service stops feeding the watchdog and is restarted), and `STOPPING=1` when it is told to stop; abstract sockets (`@…`) are supported, and without `NOTIFY_SOCKET` nothing changes. The binary carries the marker `rho-sd-notify-v1`, which a supervisor reads to know it may ask for `Type=notify` and a watchdog.

The watchdog's own probes of `/healthz` carry `X-Rho-Watchdog` and are not written to the request log, so a supervised service does not log a line every 15 s.

Fixed: `rho module validate .` (or `../<name>`) refused a collection whose platform is reached through a sibling dependency, while the same folder named from its parent validated — the platform walk kept a `..` that the manifest reader refuses (known bug 67).

### Breaking

- The service opens its listener itself and serves on it (`Serve` / `ServeTLS`) rather than through `ListenAndServe`; an address that cannot be listened on is reported by `StartServer` before anything is served, as before.
- A handler's request context is cancelled as soon as the stop signal arrives, not when the grace period ends: a hand-written extension that ties work to `r.Context()` sees it cancelled at shutdown.
