# Generated services

The `go-server` target turns a Rho module into a Go HTTP service. The module
declares its routes and writes its handlers in Rho; the backend writes the
server around them: the listener, the request and response types, the router,
the middleware a project asked for, the page templates and the translations.
Nothing in the generated project is written by hand, and all of it can be read.

This page follows a small example, `acme-notes`, a service that lists and adds
notes.

## The project

```text
acme-notes/
  module.yml
  project.yml          how the server target differs from the built-in one
  src/main.rho
  templates/notes.html
  locales/en.json
  locales/ro.json
```

```yaml
# module.yml
module:
  name: acme-notes
  type: rho
  version: 0.1.0
  port: 8710
description: A small notes service
main: src/main.rho

build:
  language: go
  targets:
    - go-server
```

```yaml
# project.yml
version: 1
targets:
  - id: go-server
    router: serveMux
    middleware:
      - name: request_id
      - name: same_origin
    templates:
      - dir: templates
    default_locale: en
    locales:
      - locale: en
        file: locales/en.json
      - locale: ro
        file: locales/ro.json
```

`project.yml` is read on top of the built-in descriptor, so the target keeps the
built-in files and kind and adds only what is this service's own; see
{ref}`building/targets`.

```sh
rho build                 # writes build/generated/go-server/ and build/acme-notes
./build/acme-notes        # rho acme_notes listening on 127.0.0.1:8710
```

## Routes

A route binds a method and a path to a handler, at the top level of a module:

```rho
module acme_notes

import std.web.http
import std.web.template

route GET  "/notes"      => list_notes
route GET  "/notes/{id}" => get_note
route POST "/notes"      => add_note
```

The methods are `GET`, `POST`, `PUT`, `DELETE`, `PATCH` and `HEAD`. The path is a
Go `ServeMux` pattern: `{id}` captures one segment and `{path...}` the rest of
the path. With the standard library's router, `"/"` also answers every path no
other route matched, so a handler on it reads `request_path(req)` to tell the
root from a page that does not exist.

A handler takes a `Request` and returns a `Response`:

```rho
fn add_note(req: Request) Response {
    let body: string = request_body(req)
    return text(status_created(), body)
}
```

Every generated service also answers `GET /healthz` with `{"status":"ok"}`. It is
registered before the module's routes, so a module does not declare its own: with
the standard library's router, a second `GET /healthz` is a conflict the service
panics on at startup.

### Reading a request

| Call | What it returns |
|------|-----------------|
| `param(req, "id")` | A captured path wildcard, or `""` |
| `request_path(req)`, `request_method(req)` | The path and the method |
| `request_body(req)` | The body, as text |
| `request_auth(req)` | The raw `Authorization` header |
| `request_query_param(req, "q")` | One query parameter, decoded |

The body is capped at 16 MB; `RHO_HTTP_MAX_BODY` (in bytes) changes the cap when
the service starts.

A target that names a `router` or lists any `middleware` gets the **routing
port** (`httproute`), and with it typed accessors. Each converts one value from
one source — `path_`, `query_`, `header_`, `body_` (the whole body) or `field_`
(a top-level field of a JSON body) — to one type: `str`, `i32`, `u32`, `u64`,
`f64` or `bool`. A conversion that fails records why on the request, and the
handler asks once:

```rho
fn get_note(req: Request) Response {
    let id: u32 = path_u32(req, "id")
    if !bind_ok(req) {
        return text(status_bad_request(), bind_error(req))
    }
    return json(status_ok(), "{\"id\":" + int_to_str(id) + "}")
}
```

`GET /notes/x` answers `the path parameter "id" is not a 32-bit unsigned
integer`. An optional value is read with `has_path`, `has_query`, `has_header` or
`has_field`, or with `query_default(req, "lang", "en")`, none of which records a
failure.

The router is `serveMux` (Go's `http.ServeMux`, the default) or an adapter —
`chi`, `gin` or `fiber` — whose module requirement is written into `go.mod` for
the project.

### Writing a response

`std.web.http` builds the common responses:

| Call | What it is |
|------|------------|
| `text(status, body)` | `text/plain; charset=utf-8` |
| `json(status, body)` | `application/json` |
| `html(status, body)` | `text/html; charset=utf-8` |
| `redirect(location)` | A `302` with a `Location` header; a location with a line break goes to `/` instead |
| `with_headers(resp, "Name: value")` | The same response with more header lines, for a cookie or an HTMX header |

and names its statuses: `status_ok()`, `status_created()`, `status_no_content()`,
`status_found()`, `status_bad_request()`, `status_unauthorized()`,
`status_forbidden()`, `status_not_found()`, `status_conflict()`,
`status_payload_too_large()`, `status_internal()`.

## Templates

Markup lives in template files, not in Rho strings. A target carries them under
`templates:` — a `name` and a `file`, or a `dir` whose every file is carried
under its file name without the extension — and they are compiled into the
binary. A program that asks for a template by a literal name the target does not
carry is refused at build time, rather than rendering an empty page.

```rho
fn list_notes(req: Request) Response {
    let lang: string = query_default(req, "lang", "en")
    return html(status_ok(), fill(template_get("notes"),
        vars("notes", "milk\nbread") + vars("__locale", lang)))
}
```

`template_get(name)` is the template's text, and `fill(source, pairs)` (or the
intrinsic it calls, `template_fill`) renders it with values joined from `vars`.
One encoded list rather than a parameter per value means a template gains a
placeholder without any signature changing.

```html
<!-- templates/notes.html -->
<h1>{{t "title"}}</h1>
<ul>
  {{#each notes}}<li>{{@index1}}. {{.}}</li>{{else}}<li>{{t "empty"}}</li>{{/each}}
</ul>
```

| Syntax | What it does |
|--------|--------------|
| `{{ name }}` | A value, HTML-escaped |
| `{{{ name }}}` | A value as it is, for markup the program built |
| `{{ name ?? fallback }}` | A default for a missing or empty value (also written with a pipe and a quoted default, below) |
| `{{#if name}}…{{else}}…{{/if}}`, `{{#unless name}}…{{/unless}}` | A branch by presence |
| `{{#each name}}…{{else}}…{{/each}}` | The body once per line of `name`; `{{.}}` is the line, `{{@0}}`, `{{@1}}`… its tab-separated fields, and `{{@index}}`, `{{@index1}}`, `{{@first}}`, `{{@last}}`, `{{@count}}` describe the loop |
| `{{#with name}}…{{/with}}` | Look names up under `name.` inside the body |
| `{{> name }}` | Another carried template |
| `{{t "key"}}`, `{{t "key" val=name}}` | A translation; `{{val}}` in it is replaced with an escaped value |
| `{# note #}` | A comment, removed |
| `{{- name -}}` | Absorb the whitespace before or after the tag |

A quoted default can also follow a pipe: `{{ name | "none yet" }}`.

`escape_html(text)` escapes a value a handler places in markup itself.

### Translations

A catalog is a flat JSON object of keys to strings. A target lists its own
under `locales:` with a `default_locale` (`en` when it names none), and a module
the program depends on can ship catalogs in its own `module.yml` under
`locales:`, in the same `locale`/`file` shape. The modules' catalogs are merged
first and the project's last, so a project can say a module's word its own way.
The catalogs are compiled into the binary; the service reads no file to
translate a page.

The language of one rendering is the reserved `__locale` value:
`vars("__locale", "ro")`. A key missing in that language falls back to the
default language and then to the key itself. A build reports a catalog that is
missing keys the default has, or has keys it does not, as a warning:

```text
rho i18n: ro is missing 3 keys (first: notes.empty)
```

## Static files and assets

A request under `/static/` is served from the `static` directory in the
service's working directory, before routing, so a static file never shadows a
route. The built-in target creates an empty `static/` in the generated project;
a project puts files there with `copy:` entries (`from` a project file, `to` a
path in the generated project) or ships the directory beside the binary.

A file a program reads rather than serves is an **asset**: a module lists files
or directories under `assets:` in its `module.yml`, and they are compiled into
the binary. A program declares `extern fn rho_asset(module: string, path:
string) string` and reads one by its module's name and path, so a module never
reads its own source tree at run time.

## Server-sent events

Two responses stream rather than answer once, and both follow a file:

```rho
// One event per line of the file; a line beginning __DONE__: ends the stream
// with a `done` event.
sse_file(path, 250)

// Named HTML fragments: each line is {"event":"…","data":"…"}. The snapshot
// file, when it names one, is sent from its start first, so a page that
// connects sees the current state before later changes.
sse_events(path, snapshot, 250)
```

The last argument is how many milliseconds the server waits before looking
again when the file has nothing new. This is how a long action reports
progress: the action writes a file, and the page listens to it. The server has
no write deadline, so a stream stays open for as long as the action it reports.

## Starting, stopping and background work

A module that defines `fn init() void` has it run before the listener starts. A
module that defines `fn background() void` has it run on its own goroutine after
`init()` and beside the listener, for a service that also does something other
than answer requests — polling a sensor, watching a directory.

The service stops on `SIGINT` or `SIGTERM`, giving requests in flight thirty
seconds to finish.

| Variable | What it does |
|----------|--------------|
| `RHO_HTTP_ADDR` | Where the service listens. Unset, it is `127.0.0.1:<port>` when `module.yml` names a port (`module.port`, else `deploy.port`), and `127.0.0.1:8080` otherwise — this machine only. A service meant for the network says so: `RHO_HTTP_ADDR=:8710` |
| `RHO_TLS_CERT`, `RHO_TLS_KEY` | Both set: the service serves TLS with them. Otherwise it is plain HTTP, and a reverse proxy or a private link is how a deployment ends TLS |
| `RHO_HTTP_MAX_BODY` | The largest request body accepted, in bytes (16 MB by default) |

Every service also writes one log line per request, and refuses a request that
would change something when a browser says it came from another site: a method
other than `GET`, `HEAD` and `OPTIONS`, or a `GET` with a `confirm` parameter,
whose `Sec-Fetch-Site` or `Origin` names somewhere else. A command-line client
sends neither header and is allowed.

## Middleware

A target lists middleware in `project.yml`, outermost first. Each is a
`func(http.Handler) http.Handler` written into the generated project with its
settings as constants, so the project shows what it allows without being run.
Every option has a default; a target names a middleware and overrides what it
cares about:

```yaml
middleware:
  - name: request_id
  - name: cors
    options:
      origin: "https://notes.example.com"
  - name: rate_limit
    options:
      requests: "240"
      window: "1m"
```

| Middleware | What it does | Options (defaults) |
|------------|--------------|--------------------|
| `request_id` | Takes the identifier a client sent, mints a 128-bit one when there is none, and echoes it back | `header` (`X-Request-Id`) |
| `cors` | Answers a preflight and sets the cross-origin headers; a specific origin is echoed back rather than answered with `*` | `origin` (`*`), `methods`, `headers` (`Authorization, Content-Type, X-Request-Id`), `max_age` (`600`) |
| `rate_limit` | A fixed window per client, answered with `429` and `Retry-After`. The client is `X-Real-IP`, else the rightmost `X-Forwarded-For` entry, else the connection's address | `requests` (`100`), `window` (`1m`), `header` (`X-RateLimit-Limit`) |
| `jwt` | Verifies an HMAC-SHA256 bearer token and refuses a request without a valid one | `secret_env` (`RHO_JWT_SECRET`), `issuer`, `audience`, `header` (`Authorization`), `scheme` (`Bearer`), `skip_paths` |
| `same_origin` | Refuses a write (any method but `GET`, `HEAD`, `OPTIONS`) whose `Sec-Fetch-Site` or `Origin` says it came from another origin. An `Origin` matching `X-Forwarded-Host` counts as this one | `skip_paths`: paths, or prefixes ending in `/`, that take a cross-origin write on purpose |
| `auth` | Verifies a signed session and sends a browser without one to sign in — see below | see below |
| `app_proxy` | Serves `/<prefix>/<name>/…` from the application `<name>` on its loopback port, so a supervisor and the applications it runs share one origin | `prefix` (`/app/`), `apps_file` (`~/.rho/keeper/proxy.txt`), `strip_cookie` |

The per-site check every service makes runs outside this list, so a path in
`same_origin`'s `skip_paths` still has to pass it.

`app_proxy` reads `apps_file`, one `name port` per line, again whenever it
changes, so an application added is reachable without a restart. It rewrites
root-relative links in HTML, a `Location` header and a cookie's `Path` onto the
prefix, never forwards the `strip_cookie` cookie, tells the application where it
is reached with `X-Forwarded-Prefix`, `-Host` and `-Proto`, and streams the
response, so an event stream is relayed as it is written.

### The `auth` middleware

`auth` accepts a session issued by an authorization server, verified against
that server's public key (ES256), so the service holds no signing key and no
password. A request is admitted when it carries any of:

- an `Authorization: Bearer` token the key verifies;
- a session cookie holding such a token;
- a session cookie naming a session this service opened itself.

A service opens its own session at `/auth/callback?ticket=…`: the ticket is
verified, used once, and exchanged for an eight-hour session in an `HttpOnly`,
`SameSite=Lax` cookie. A request with no valid session is answered with a JSON
`401` when its path contains `/api/` or it accepts `application/json`, and a
browser is redirected to the login URL, with the callback and the page it wanted
in the query.

What the session says is passed to the handler as headers — `X-Rho-Subject`,
`X-Rho-Scopes` and `X-Rho-Aud` — which a handler reads with
`header_str(req, "X-Rho-Subject")`. The middleware removes those headers from
every incoming request first, so the only values a handler sees are ones it
verified. A session whose scopes all end in `:read` may look and may not change
anything, and a token id listed in a revocation file is refused.

| Option | Default | What it is |
|--------|---------|------------|
| `issuer` | none | The issuer a token must name, when set |
| `audience` | `deploy.service`, else the module name | The audience a token must name |
| `public_key` | `~/.rho/auth/public.pem` | The issuer's public key; `RHO_AUTH_PUBLIC_KEY` overrides it at run time |
| `cookie` | `rho_session_<audience>` | The session cookie, named per service because a cookie is scoped by host and not by port |
| `login_url` | `/auth/login?audience=<audience>` | Where a browser is sent to sign in |
| `skip_paths` | `/healthz,/auth/login,/auth/callback,/static` | Paths, and everything under them, served without a session |
| `scopes` | none | Scopes every session must hold; `*` holds them all |
| `mode`, `public_url`, `base_path` | `direct`, none, none | Behind a reverse proxy: `mode: proxy` with the public origin and the path the service is under |
| `trust_forwarded` | `false` | Believe `X-Forwarded-Host` and `-Proto` from any client |
| `scheme` | `auto` | `http` or `https` to force the scheme of a return address and the cookie's `Secure` flag |
| `return_param` | `return_to` | The query parameter the callback address travels in |
| `session_file`, `keeper_port` | none, `19090` | Where sessions are kept; the issuer's port when it runs on this machine |

Where the issuer is, and how a service runs without one, is the environment's:

| Variable | What it does |
|----------|--------------|
| `RHO_AUTH_URL` | The issuer. A relative `login_url` is sent there, and when the key file is not on this machine the key is fetched from `<RHO_AUTH_URL>/auth/public-key` and a copy kept, so a service started while the issuer is down recovers |
| `RHO_AUTH_STANDALONE` | `1`, `true` or `yes`: there is no issuer. The service answers requests from this machine only — a loopback address and no forwarding header — as the subject `local` with every scope |
| `RHO_APP_WORKSPACE` | The directory sessions and the key copy are kept in (`auth/` under it); without it, `~/.rho/auth-app/<audience>/` |

Forwarded headers are believed only from the loopback. `X-Forwarded-Prefix`,
`-Host` and `-Proto` say where a front proxy on the same machine reaches the
service, and the middleware builds its return addresses from them only when the
request arrived from a loopback address; from anywhere else they are ignored,
unless `trust_forwarded` says otherwise.

## Describing the service

A target's `api:` section says what its routes cannot: `security` (`none` or
`bearer`), `security_description`, and the `public_paths` served without a
credential. The `openapiemit` port asks the `rho-openapi` component for an
OpenAPI description of the routes and that section when the project is
generated, embeds it, and a handler serves it with `openapi_document()`. A build
host without the component gets a service whose description says so, and that
still serves everything else. See {ref}`building/ports`.
