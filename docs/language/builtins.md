# Built-in functions

The functions a program calls without writing them. Some the type checker knows
by name and a program calls without declaring; the rest are the runtime's
`rho_*` functions, which a program declares with `extern fn` and the backend
supplies. Both are listed here, with their signatures, what they answer at the
edges, and which backend has them.

A **C** or **Go** column says whether the backend supplies the function. A
function only one backend has still type-checks on the other, and fails at
the link (C) or the Go build, naming the symbol.

## How a built-in reaches a program

| Kind | Declared by the program? | Where it comes from |
|---|---|---|
| Intrinsic (`str_trim`, `len`, `getenv`) | No | The type checker knows its signature, and both backends lower it to the runtime's `rho_` function of the same name |
| Runtime function (`rho_read_text`, `rho_exec`) | Yes, `extern fn` with the signature below | The runtime fragment the backend emits when a program calls it |
| Service function (`param`, `auth_subject`, `template_fill`) | No | The generated Go service, or the port that provides it |
| `c_` name | Yes, `extern fn` | Nothing in the toolchain: the program or its platform links it |

A runtime fragment is emitted only when a program calls something in it, so a
firmware that never touches a file does not carry the file code.

A call to an undeclared name that begins with `c_` is accepted and typed `i32`.
Declare it with `extern fn` to give it its real type.

## Text

Strings are bytes. On C a string is NUL-terminated, so a zero byte ends it; on Go
it can hold one. That is why the cryptographic functions below take and return
hex.

| Signature | What it answers | C | Go |
|---|---|---|---|
| `strlen(s: string) u32` | The length in bytes | yes | yes |
| `len(value) u32` | The length of a string, a `[T]` or a fixed array. On C the type checker accepts it, but the C runtime has no `len`: use `strlen` for a string and the declared size for an array | no | yes |
| `str_eq(a: string, b: string) bool` | Whether two strings are equal. `==` between strings is the same call | yes | yes |
| `str_at(s: string, i: u32) u8` | The byte at `i`, or 0 past the end | yes | yes |
| `str_from_byte(b: u8) string` | A one-byte string | yes | yes |
| `str_slice(s: string, start: u32, length: u32) string` | `length` bytes from `start`, clamped to the string; `""` when `start` is past the end | yes | yes |
| `str_index_of(haystack: string, needle: string) i32` | Where `needle` first starts, or -1 | yes | yes |
| `str_contains(haystack: string, needle: string) bool` | Whether `needle` occurs | yes | yes |
| `str_starts_with(s: string, prefix: string) bool` | Whether `s` begins with `prefix` | yes | yes |
| `str_trim_prefix(s: string, prefix: string) string` | `s` without `prefix`, or `s` unchanged | yes | yes |
| `str_trim(s: string) string` | `s` without spaces, tabs, CR and LF at either end | yes | yes |
| `str_lower(s: string) string`, `str_upper(s: string) string` | ASCII letters changed; every other byte kept | yes | yes |
| `str_replace(s: string, from: string, to: string) string` | Every occurrence of `from` replaced by `to`, left to right. An empty `from` replaces nothing | yes | yes |
| `str_field(s: string, sep: string, index: u32) string` | Field `index` (from 0) of `s` split on `sep`. `""` past the last field, and `""` for an empty `sep` | yes | yes |
| `str_url_encode(s: string) string` | Percent-encoding of every byte except `A-Z a-z 0-9 - _ . ~` | yes | yes |
| `str_url_decode(s: string) string` | `+` becomes a space and `%XX` a byte. A `%` that is not followed by two hex digits is kept as written | yes | yes |
| `form_field(body: string, name: string) string` | The decoded value of one field of an `x-www-form-urlencoded` body; `""` when absent or written without `=` | yes | yes |
| `escape_html(value: string) string` | The value made safe for element text or an attribute | no | service |
| `string.from_i32(n: i32) string` | A signed number as text | yes | yes |

`+` joins two strings.

## Numbers as text

| Signature | What it answers | C | Go |
|---|---|---|---|
| `int_to_str(n: u32) string` | A `u32` in base ten | yes | yes |
| `int_from_str(s: string) u32` | A run of decimal digits. Anything else, a sign or a space included, is 0. A number too big for `u32` is 0 on Go and wraps on C | yes | yes |
| `i64_to_str(n: i64) string` | An `i64` in base ten, with a sign when negative | yes | yes |
| `i64_from_str(s: string) i64` | A whole number with an optional sign, the space around it trimmed. Text that is not one, or more than 18 digits, is 0 | yes | yes |

0 is also a valid answer. Text that may not be a number is checked first,
with `milli_valid` for a decimal, or by comparing `i64_to_str` of the result
with the input.

## Decimals as thousandths

A decimal is held as an `i64` of thousandths: `2.5` is `2500`. Both backends read
and write it identically, and parity tests check that they do.

| Signature | What it answers | C | Go |
|---|---|---|---|
| `milli_valid(text: string) bool` | Whether `text` is a decimal: optional sign, digits, at most one `.` or `,`, at most 15 whole digits, space around it allowed | yes | yes |
| `milli_parse(text: string) i64` | The decimal as thousandths. `2,5` and `2.5` are both 2500. Digits past the third decimal are dropped, not rounded. Text that is not a decimal is 0 | yes | yes |
| `milli_format(milli: i64) string` | The thousandths as a decimal, without trailing zeros: 2500 is `2.5`, 3000 is `3`, -1500 is `-1.5` | yes | yes |
| `milli_round(milli: i64, places: u32) i64` | Rounded to `places` decimals, half away from zero: 2345 to 2 places is 2350. Three places or more returns the value unchanged | yes | yes |

## Bits

The bit cast a wire format needs, since Rho has no cast between a float and a
word.

| Signature | What it answers | C | Go |
|---|---|---|---|
| `f32_to_bits(value: f32) u32` | The IEEE-754 bits of a float | yes | yes |
| `f32_from_bits(bits: u32) f32` | The float those bits are | yes | yes |
| `f64_to_bits(value: f64) u64` | The IEEE-754 bits of a double | yes | yes |
| `f64_from_bits(bits: u64) f64` | The double those bits are | yes | yes |

## JSON

These read a flat document as text. They do not parse it, and they find the
first occurrence of a key at any depth.

| Signature | What it answers | C | Go |
|---|---|---|---|
| `json_str_field(doc: string, key: string) string` | The first string value of `"key"`, with `\n`, `\t`, `\r` and other escaped characters unescaped. `""` when the key is absent or its value is not a string | yes | yes |
| `json_num_field(doc: string, key: string) string` | The first numeric value of `"key"`, as written (`-1.5e3`). `""` when absent | yes | yes |
| `json_string(value: string) string` | `value` as a JSON string literal, quotes and escapes included, so free text cannot break the document it is written into | no | service |

## Environment and the program

| Signature | What it answers | C | Go |
|---|---|---|---|
| `getenv(name: string) string` | The variable's value, or `""` when it is not set | yes | yes |
| `env_or(name: string, fallback: string) string` | The variable's value when it is set (even to `""`), otherwise `fallback` | yes | yes |
| `app_workspace() string` | Where the application keeps its data: `RHO_APP_WORKSPACE` when set, otherwise `$HOME/.rho/apps/<name>`. The name is `RHO_APP_NAME`; without it, C uses `app` and Go uses the program's file name | yes | yes |
| `get_args() string` | The command-line arguments joined by spaces | no | yes |
| `read_file(path: string) [u8]` | A whole file as bytes, or empty when it cannot be read | no | yes |
| `write_file(path: string, data: [u8])` | Writes a whole file (mode 0644); says nothing about failure | no | yes |
| `append(items: [T], item: T) [T]` | The array with one more element | no | yes |
| `extern fn rho_arg(index: i32) string` | One argument by position, from 0; `""` past the last | no | yes |
| `extern fn rho_setenv(name: string, value: string)` | Sets a variable for this process and what it runs | no | yes |

## Ending the program

| Signature | What it does | C | Go |
|---|---|---|---|
| `print(s: string)` | Writes to standard output with no newline added | yes | yes |
| `panic(msg: string)` | Writes the message and stops. On a hosted C build that is exit status 1; on a board it is the platform's stop (`RHO_PROGRAM_HALT`, or a breakpoint and `wfi` on Cortex-M and RISC-V). Go writes `rho: <msg>` to standard error and exits 1 | yes | yes |
| `abort()` | Stops with a non-zero status | yes | yes |
| `sizeof(T) u32` | The size of a type in bytes, decided at compile time | yes | yes |

## Files

Declared by the program:

```rho
extern fn rho_read_text(path: string) string
extern fn rho_write_text(path: string, text: string) bool
```

| Signature | What it answers | C | Go |
|---|---|---|---|
| `rho_read_text(path: string) string` | A whole file, or `""` when it cannot be read | yes | yes |
| `rho_write_text(path: string, text: string) bool` | Replaces the file (mode 0644), and whether every byte was written | yes | yes |
| `rho_append_text(path: string, text: string) bool` | Appends to the file, creating it, and whether it did | yes | yes |
| `rho_write_secret(path: string, text: string) bool` | Writes a file only its owner can read (0600). An existing file is narrowed to 0600 too. Go writes a temporary file beside the target and renames it over, so a reader never sees half of one. A board with no POSIX files writes it like any other | yes | yes |
| `rho_path_exists(path: string) bool` | Whether anything is at `path` | no | yes |
| `rho_list_dir(path: string) string` | The names in a directory, one per line; `""` when it cannot be read | no | yes |

`yaml.value`, `yaml.kind`, `yaml.keys` and `yaml.load` are called as members and
need no declaration. Each reads a YAML file by a dotted path:

| Signature | What it answers | C | Go |
|---|---|---|---|
| `yaml.value(path: string, key: string) string` | The value at `key`, such as `module.version` | yes | yes |
| `yaml.kind(path: string, key: string) string` | `string`, `number`, `boolean`, `list`, `map` or `missing` | yes | yes |
| `yaml.keys(path: string, key: string) string` | The keys under `key`, one per line | yes | yes |
| `yaml.load(path: string) string` | `user.name` from the file, or `unknown` | yes | yes |
| `extern fn rho_yaml_flat(path: string) string` | A document of nested maps as `a.b.c=value` lines; a key `_` stands for a map's own value | no | yes |
| `extern fn rho_yaml_write_flat(path: string, flat: string, header: string) bool` | Those lines written back as YAML, keys in order, values quoted, `header` as comments, replaced in one step | no | yes |

## Running programs

| Signature | What it answers | C | Go |
|---|---|---|---|
| `rho_exec(program: string, arguments: string, output_path: string) u32` | Runs `program` with `arguments` split on whitespace, honouring double quotes, and its standard output and error written to `output_path`. The answer is the exit code, or 255 when it could not be run | yes | yes |
| `rho_exec_args(program: string, arguments: string, output_path: string) u32` | The same, with one argument per line and nothing interpreted. An empty line is an empty argument, and a trailing newline ends the last one | yes | yes |
| `rho_exec_capture(program: string, arguments: string) string` | `rho_exec_args` whose output is returned: standard output and error when the program exited 0, `""` otherwise. No file another caller could read | yes | yes |
| `rho_sleep_ms(milliseconds: u32)` | Waits; on C a signal does not cut the wait short | yes | yes |
| `rho_lock(name: string)`, `rho_unlock(name: string)` | A named lock. A Go service runs requests on several goroutines, so a read-then-write is done under one. A C program has one thread and both do nothing | yes | yes |

No program is run through a shell. A value that came from a request, a file or a
user goes through `rho_exec_args` or `rho_exec_capture`, which pass it as exactly
one argument.

## Time

| Signature | What it answers | C | Go |
|---|---|---|---|
| `rho_now_s() u32` | Unix time in seconds | yes | yes |
| `rho_local_date() string` | Today's local date, `YYYY-MM-DD` | yes | yes |
| `rho_local_clock() string` | The local time of day, `HH:MM` | yes | yes |
| `rho_utc_offset_at(epoch: i64) i64` | The local zone's offset from UTC, in seconds, at that moment. A time from last winter gets last winter's offset | yes | yes |

## Cryptography

| Signature | What it answers | C | Go |
|---|---|---|---|
| `rho_hmac_sha256_hex(key_hex: string, message_hex: string) string` | HMAC-SHA256 of the message under the key, as 64 hex digits. `""` when either argument is not hex | yes | yes |
| `rho_random_hex(bytes: u32) string` | That many random bytes from the operating system, as hex. `""` for 0 or more than 1024 bytes, and when there is no source. There is no fallback to something predictable | yes | yes |

## Calling other services (Go)

A Go program that is a service calls others with the identity its supervisor
gave it: `RHO_CLIENT_ID`, `RHO_CLIENT_SECRET_FILE` (a file only its owner can
read) and `RHO_AUTH_URL`. No secret or token appears on a command line or in a URL.

| Signature | What it answers | C | Go |
|---|---|---|---|
| `rho_service_token(audience: string, scopes: string) string` | A token for one audience, from `POST <RHO_AUTH_URL>/auth/service-token`, cached until 80% of its lifetime has passed. `""` when there is no credential or the issuer refused | no | yes |
| `rho_http_request(method: string, url: string, body: string, content_type: string, audience: string, scopes: string, output_path: string) u32` | Sends one request, carrying a token when `audience` is not empty, and writes up to 4 MiB of the body to `output_path`. The answer is the HTTP status, or 0 when nothing answered or a token could not be had | no | yes |
| `rho_http_body(method: string, url: string, body: string, content_type: string, audience: string, scopes: string) string` | The same request, with the body returned. `""` for a status outside 2xx | no | yes |
| `rho_sse_follow(url: string, audience: string, scopes: string, path: string) bool` | Starts following another service's event stream into `path`, one `{"event","data"}` line per event, which is the shape `sse_events` serves. It returns at once and reconnects with a fresh token for as long as the program runs | no | yes |

## Handlers (Go service)

A handler in a generated Go service reads its request with these. They need no
declaration. {ref}`building/services` describes the service they belong to.

| Signature | What it answers |
|---|---|
| `param(req: Request, name: string) string` | A path wildcard |
| `path_str`, `query_str`, `header_str`, `body_str`, `field_str` `(req, name) string` | A value from one source of the request |
| `path_u32`, `path_i32`, `path_u64`, `path_f64`, `path_bool` and the same for `query_`, `header_`, `body_`, `field_` | The value converted. A failure is recorded on the request |
| `bind_ok(req) bool`, `bind_error(req) string` | Whether every conversion succeeded, and which one did not |
| `has_path`, `has_query`, `has_header`, `has_field` `(req, name) bool` | Whether a value is present, without converting it or recording a failure |
| `query_default(req, name: string, fallback: string) string` | A query value, or the fallback |
| `request_query_param(req, name: string) string` | One decoded value from the raw query string |
| `request_body(req) string`, `request_auth(req) string`, `request_path(req) string`, `request_method(req) string` | The raw body, the `Authorization` header, the path and the method |
| `openapi_document() string` | The service's own API description, written by the `openapiemit` port |

A response is built with `std.web.http` (`text`, `json`, `html`, `redirect`,
`with_headers`, `sse_file`, `sse_events`).

## Templates (Go service)

Templates are the files a target carries. `std.web.template` documents their
syntax and the `vars` encoding.

| Signature | What it answers |
|---|---|
| `template_get(name: string) string` | The template's source, or `""` |
| `template_has(name: string) bool` | Whether the target carries it |
| `template_fill(source: string, pairs: string) string` | The template rendered with an encoded list of values, escaped unless written `{{{ name }}}` |

## Authentication (Go service)

Supplied by the generated service's authentication code. The `header` argument
is a request's `Authorization` header.

| Signature | What it answers |
|---|---|
| `auth_bearer(header: string) string` | The token in a `Bearer` header, or `""` |
| `auth_subject(header: string) string` | The identity behind the request, or `""` when anonymous |
| `auth_is_authenticated(header: string) bool` | Whether the credential verifies |
| `auth_expires(header: string) u32` | The Unix time the credential runs out, or 0 |
| `auth_required() bool` | Whether the service requires authentication |
| `auth_open_tokens() bool` | Whether token issuance is open; closed by default |
| `auth_bootstrap(header: string) bool` | Whether the request carries the operator's bootstrap secret, which may mint a token for any subject |
| `auth_allowed(user: string) bool` | Whether a user is on the allowlist of those who may obtain a token |
| `auth_list_configured() bool` | Whether such a list is configured |
| `auth_issue(user: string, host: string, days: u32, scopes: string) string` | A new token, as the JSON body a client expects |

## Ports

These are supplied by a port the target asks for ({ref}`building/ports`). They
are listed so that a call to one is recognised:

| Function | Port |
|---|---|
| `emit(level: u32, subject: string, message: string)` | `logemit`, behind the `rho-log` library |
| `config_get(key: string, fallback: string) string` | `fsstore`: a registry setting, or the fallback |
| `clock_year() u32` | `fsstore` and `docstore`: the current year |
| `store_list`, `store_meta`, `store_detail`, `store_latest`, `store_data_dir`, `store_release` | `fsstore` |
| `registry_data_dir`, `registry_access`, `registry_title`, `registry_version`, `registry_site_url`, `registry_url`, `registry_token_days`, `registry_token_max_days`, `registry_open_tokens`, `registry_max_upload_mb` | `fsstore`, the registry application's configuration |

## Where to go next

- {ref}`stdlib <language/stdlib>` — the library written in Rho on top of these.
- {ref}`building/ports` — what a host supplies beyond the runtime.
- {ref}`building/services` — the Go service the handler functions belong to.
