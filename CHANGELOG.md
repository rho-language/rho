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
