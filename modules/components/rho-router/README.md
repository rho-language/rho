# rho-router

The HTTP surface of a Rho web service: the router, the typed parameter
binding, the middleware pipeline, and the route metadata every tool that needs
to know what a service exposes reads.

```sh
rho route list <project>              # what the service answers
rho route show <project> <operation>  # one operation in full
rho route spec <project>              # the route metadata, as JSON
rho route openapi <project>           # the metadata as an OpenAPI 3.1 document
rho route accessors                   # the typed accessors a handler calls
rho route -list-plugins               # the plugins that extend the reading
```

## What it is for

A Rho application declares its routes and its handlers, which means it also
declares what it exposes and what each operation takes. Nothing has to be said
twice: there is no annotation to keep in step with the code, no separate
specification file, and no framework's vocabulary in the language.

```rho
route GET "/modules/{name}/{version}" => show_module

fn show_module(req: Request) Response {
    let version: u32 = path_u32(req, "version")
    if !bind_ok(req) {
        return error_json(400, bind_error(req))
    }
    return json(200, render(name, version))
}
```

That is a typed route, and `rho route openapi` turns it into a document that
says the path parameter is an integer, because the handler asked for one.

## The four parts

**Zero-reflection parameter binding.** A handler reads a request through typed
accessors, and every value is converted by a statement the compiler wrote. There
is no `reflect` on the path, no `interface{}` to decode into, and no tag to
read: `path_u32` ends in `strconv.ParseUint`. A conversion that fails is
reported through the request, and the handler asks once whether the binding
held, so a bad path is a 400 rather than a panic.

**Standard library first.** Go 1.22 gave `http.ServeMux` the method-and-wildcard
patterns a REST API is written in, and `{name}` and `{name...}` are the syntax a
Rho route is declared with. A generated project therefore routes its requests
with nothing fetched. `chi`, `gin` and `fiber` are adapters, not alternatives:
each is a registration change and a way to read a captured value, never a
second router, so the handlers, the binding and the middleware are the same
whatever dispatches them.

**Middleware as Go middleware.** Every wrapper is a
`func(http.Handler) http.Handler`, composed in the order the target declared
them, outermost first. There is no Rho-side pipeline to learn and no
registration to sequence; a wrapper the port does not supply is one function
with that signature and one more name in the list.

**Route metadata.** The routes, their parameters and their types, the bodies
they accept and the statuses they answer with, read out of the sources with the
compiler's own parser. It is a published contract, so a client generator, a
contract test and an OpenAPI document all read the same thing and cannot
disagree.

## How a project uses it

Routing is a target's decision, so it is a target's setting:

```yaml
targets:
  - id: go-server
    emitter: go
    kind: server
    router: serveMux          # or chi, gin, fiber
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
          skip_paths: "/healthz, /auth/token"
```

Naming a router asks for the routing port; there is no second line declaring
it. The port is generated into the project, so `go build` and `go run .` work in
the generated directory with nothing fetched and nothing installed.

## rho-openapi

The metadata this component reads is a published contract, not an OpenAPI
document wearing a contract's clothes. `rho-openapi` is the component that
turns it into a document, and it depends on this one for the reading:

```go
import "github.com/rho/rho-router/src/rhorouter"

result, err := rhorouter.Read(rhorouter.ReadOptions{Root: project})
document := result.Spec   // whatever is wanted is written from this
```

Nothing in `rhorouter` knows about OpenAPI, so the contract is not tied to one
rendering of it. The `rhooas` package in this component is a proof that the
contract is sufficient — it writes a complete 3.1 document from the spec alone
— and it moves to `rho-openapi` as a package when that component is built.

## Plugins

A plugin answers one of three hooks, and adds to what was read rather than
replacing it, so a broken extension costs one operation rather than the API of
everything else.

| Hook | Asked | For |
| --- | --- | --- |
| `spec` | once, over the service | adding an operation no source declares |
| `operation` | once per route | filling in a description, a tag or a response |
| `openapi` | once, over the document | reordering, filtering or extending it |

## Building

```sh
cd modules/components/rho-router && go build -o rho-router ./src
```

## Documentation

`docs/index.md` is the page this component publishes about itself, written in
the same doc-as-code as the rest of the tree.
