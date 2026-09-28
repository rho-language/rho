# HTTP routing and API metadata

A Rho application declares its routes and its handlers. That declaration is
already everything a machine needs to know about the service's HTTP surface, so
nothing here asks the project to say it twice.

This page covers the four things a target can ask for, and the command that
reads a service's metadata for anything else that needs it.

(router:command)=
## The `rho route` command

```sh
rho route list <project>              # what the service answers
rho route show <project> <operation>  # one operation in full
rho route spec <project>              # the route metadata, as JSON
rho route openapi <project>           # the metadata as an OpenAPI 3.1 document
rho route accessors                   # the typed accessors a handler calls
rho route -list-plugins               # the plugins that extend the reading
```

`-out` writes to a file rather than to standard output. `-server` and `-base`
say where the service answers and what prefix it is mounted under, because a
manifest cannot know either: they are facts about a deployment.

`spec` is the contract. Everything else in the tree that needs to know what a
service exposes reads that shape, so there is one reading and one document.

## Declaring a route

```rho
route GET  "/modules/{name}"     => show_module
route POST "/modules"             => create_module
route GET  "/modules/{name}/docs/{path...}" => docs_page
```

The path is Go's `http.ServeMux` pattern syntax, unchanged from the source to
the mux. A `{name}` segment captures one element; a `{name...}` segment captures
every remaining element as one path, which is what a documentation page, its
stylesheet and a version's directory of pages all need.

## Binding

(parameter-binding)=
### Zero-reflection parameter binding

A handler reads a request through typed accessors, and the accessor is where a
value's type is decided:

```rho
fn show_module(req: Request) Response {
    let id: u32       = path_u32(req, "id")
    let q: string     = query_str(req, "q")
    let tenant: string = header_str(req, "X-Tenant")
    let name: string  = field_str(req, "name")
    if !bind_ok(req) {
        return error_json(400, bind_error(req))
    }
    return json(200, render(id, q, tenant, name))
}
```

Every conversion is a call the standard library spells out. The generated
service ends in `strconv.ParseUint(raw, 10, 32)`, and a reader can follow it
from the Rho call site to the `strconv` call without going through anything at
run time. There is no `reflect` on the path, no `interface{}` to decode into,
and no tag to read.

The four sources are:

| Prefix | Reads |
| --- | --- |
| `path_` | a wildcard the router captured |
| `query_` | one query parameter |
| `header_` | one header, matched without regard to case |
| `body_` | the whole request body |
| `field_` | one field of a JSON body |

and the six types are `str`, `i32`, `u32`, `u64`, `f64` and `bool`, so a
`u32` reads as a 32-bit unsigned integer and a `f64` as a number.

(binding-failure)=
### What a failed conversion does

A REST API has to answer a path that is not a number, and it has to answer with
a status. So each accessor records through the request, and the handler asks
once:

```rho
if !bind_ok(req) {
    return error_json(400, bind_error(req))
}
```

`bind_error` names the parameter and what was wrong with it — `the path
parameter "id" is not a 32-bit unsigned integer` — so a client is told which
part of its request to fix. The first failure is the one reported, so the
message names the accessor the handler wrote rather than whatever ran last.

The state travels on a pointer inside the request, because a Rho handler
receives the request by value and a value it keeps is a copy. A copy shares the
pointer, so a failure recorded by an accessor is still there to be read.

### Optional values

An optional value cannot be read through a typed accessor: it would record a
failure for a request that is perfectly correct, and the failure would sit on
the request for the rest of the handler. So an optional value is asked about
first, and read only if it is there.

```rho
let limit: u32 = 20
if has_query(req, "limit") {
    limit = query_u32(req, "limit")
}
if !bind_ok(req) {
    return error_json(400, bind_error(req))
}
```

`has_path`, `has_query`, `has_header` and `has_field` record nothing, and
`query_default` reads a query parameter or falls back without recording. A
parameter present but empty is present: a client that sent the key sent a value.

`import rho.router` is the named form of these, with the common cases composed
over them: `page_path` for a remainder that may be empty, `page_limit` for a
capped page size, `search_query` for a filter that may be absent.

## Routing

(routing)=
### Standard library first

```yaml
targets:
  - id: go-server
    emitter: go
    kind: server
    router: serveMux
    files:
      - path: app.go
        role: implementation
      - path: server.go
        role: server
```

Naming a router asks for the routing port; there is no second line declaring
it. The standard library is the default because a generated project should
build with `go build` and nothing fetched, and Go 1.22 already carries the
patterns a REST API is written in.

### Adapters

`chi`, `gin` and `fiber` are available, and each is an adapter rather than an
alternative: a registration change and a way to read a captured value, never a
second router. The handlers, the binding and the middleware are the same
whatever dispatches them, so a project that moves between routers moves code
rather than rewriting it. The adapter's module requirement is written into the
generated `go.mod` for you.

(middleware)=
## The middleware pipeline

```yaml
    router: serveMux
    middleware:
      - name: request_id
      - name: cors
        options:
          origin: "https://app.rho.dev"
      - name: rate_limit
        options:
          requests: "100"
          window: "1m"
      - name: jwt
        options:
          issuer: "https://auth.rho.dev"
          audience: "registry"
          skip_paths: "/healthz, /auth/token"
```

The first name is the outermost wrapper, so a request meets the request-id
stamp, then the cross-origin headers, then the rate limit, then the
authentication, and only then the router. The composition is the Go one and the
wrappers are the Go shape:

```go
func rhoPipeline(next http.Handler) http.Handler {
	next = withJWT(next)
	next = withRateLimit(next)
	next = withCORS(next)
	next = withRequestID(next)
	return next
}
```

There is no Rho-side pipeline to learn and no registration to sequence. A
wrapper the port does not supply is one function with that signature and one
more name in the list — which is the whole extension mechanism, and the reason
a project is not limited to the four supplied here.

The four supplied:

| Name | Does |
| --- | --- |
| `request_id` | takes the identifier the client sent or mints one, echoes it back, and puts it where a log line and a client report can both reach it |
| `cors` | answers the preflight and sets the cross-origin headers from the same settings |
| `rate_limit` | a fixed window per client, counted in a map rather than by a token bucket, because a counter is something a reader can check by hand |
| `jwt` | verifies an HS256 bearer token with the standard library and refuses the request without one that is valid |

Every option has a default, so a project names a wrapper and overrides what it
cares about. A setting that is not a count or a duration falls back to the
default rather than to a limit the service cannot honour, and the signing key
is named rather than written: a key in a source file is a key in every clone of
it. A service with no key refuses every token rather than accepting every one,
which is the failure that cannot be discovered in production.

## The metadata

(metadata)=
### What a reading produces

`rho route spec` writes the contract: the operations, their parameters and
types, the bodies they accept, the statuses they answer with, and the comments
that describe them.

```json
{
  "version": 1,
  "module": "registry",
  "operations": [
    {
      "operationId": "download_module",
      "method": "GET",
      "path": "/modules/{name}/{version}/rho-pkg",
      "summary": "download_module serves the stored package for one version.",
      "parameters": [
        {"name": "name", "in": "path", "type": "string", "required": true},
        {"name": "version", "in": "path", "type": "string", "required": true}
      ],
      "responses": [
        {"status": 200, "description": "OK", "contentType": "application/gzip"},
        {"status": 404, "description": "Not Found"}
      ]
    }
  ]
}
```

Two things are worth saying about how it is read.

The reading uses the compiler's own lexer, parser and AST, so a route is read
exactly as the compiler read it. A second dialect of Rho would be a second
opinion about what a program says, and it would eventually disagree with the
build that ships it.

The types come from the handler rather than from an annotation. There is no
annotation in Rho, and adding one for this would put a web framework's
vocabulary into the language. The type is the one the handler asked to receive
the value as, which is also the type the generated service converts to — so the
document describes what the program does rather than what a separate
declaration says it might.

Comments are read from the source text by position, because the lexer discards
them: a comment is not a value and a program does not run on one, but a
documentation tool has no use for a route that cannot describe itself. The
first line of a comment is the heading and the rest is the prose under it.

(openapi)=
### OpenAPI 3.1

`rho route openapi` writes the contract as a document:

```sh
rho route openapi applications/registry -server https://registry.rho.dev \
  -out build/registry-openapi.json
```

3.1 rather than 3.0, because a document produced from a program may state facts
the older version has nowhere to put: a type is a type rather than a string
naming one, and a value may be any shape at all.

The document is written by hand rather than through a marshaller. A document is
something people read in a diff, and a marshaller offers no say in the order of
the keys, which is the whole content of a diff. It is also reflection again,
and a file whose whole point is that nothing on the path is should not start
using it at the end. The same service always produces the same bytes.

### For the next component

The metadata is a contract, not a document. Nothing in `rhorouter` mentions
OpenAPI, so `rho-openapi` depends on this component for the reading and writes
whatever it likes from the spec:

```go
import "github.com/rho/rho-router/src/rhorouter"

result, err := rhorouter.Read(rhorouter.ReadOptions{Root: project})
document := result.Spec
```

A generated client, a mock, a contract test and an OpenAPI document are four
renderings of one reading, and none of them should parse Rho sources again.

(plugins)=
## Plugins

| Hook | Asked | For |
| --- | --- | --- |
| `spec` | once, over the service | adding an operation no source declares |
| `operation` | once per route | filling in a description, a tag or a response |
| `openapi` | once, over the document | reordering, filtering or extending it |

A plugin adds to what was read rather than replacing it, and an operation
already present is left alone — which is how a plugin that wants to fill a gap
fills the gap and a plugin that wants to add an operation adds one. A plugin
that fails is a diagnostic rather than an error: one broken extension must not
cost a reader the API of everything else.

`rho route -list-plugins` shows what is loaded. A tree's own plugins are not
trusted by default, because a plugin is a program and one that arrived with a
freshly cloned repository arrived from whoever wrote it.
