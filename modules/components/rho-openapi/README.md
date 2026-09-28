# rho-openapi

An OpenAPI 3.1 description of a Rho web service, and the reference page a person
reads it on — both written from the routes the compiler already reads.

```sh
rho openapi generate <project>        # the OpenAPI 3.1 document, as JSON
rho openapi page <project>            # the API reference page, as HTML
rho openapi spec <project>            # the route metadata, as JSON
rho openapi check <project>           # whether the description is sound
```

## Where the description comes from

Nothing is declared twice. A Rho service already declares its routes and its
handlers, and `rho-router` reads that declaration; this component writes it out.
There is no annotation to keep in step with the code, no specification file, and
no framework's vocabulary in the language.

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

That is a typed route, and the document says the path parameter is an integer,
because the handler asked for one.

```json
{
  "name": "version", "in": "path", "required": true,
  "schema": {"type": "integer", "format": "int64"}
}
```

## Two renderings, one reading

`generate` writes what a client generator, a portal or a contract test reads.
`page` writes what a person reads: the endpoints, what each takes, what each
answers. Both are written from the spec `rho-router` produced, so neither is
derived from the other and they cannot disagree.

A document is written by hand rather than through a marshaller, because a
document is something people read in a diff and a marshaller offers no say in
the order of the keys — which is the whole content of a diff. The same service
always produces the same bytes.

## One documentation site

`rho-doc-api`, in `modules/plugins`, is the plugin that puts the reference page
into a documentation build. Install it and every module that serves HTTP gets an
API page in the same sidebar as its module reference, under the same section —
one site, one search, one set of links. A module with no routes gets no page,
because most modules are libraries.

A service can also serve its own description. A target that asks for the
`openapiemit` port gets `openapi_document()` in Rho, and a route that answers it:

```rho
route GET "/openapi.json" => openapi_docs

fn openapi_docs(req: Request) Response {
    return json(status_ok(), openapi_document())
}
```

The document is written when the project is generated and embedded as a
constant, so the running service does not read Rho sources to describe itself. A
service whose description could not be written still serves, and answers with
the reason.

## What the description carries that a route does not

Two facts a description needs come from the project's descriptor rather than
from a handler, because neither is something a body can be read for:

```yaml
targets:
  - id: go-server
    emitter: go
    kind: server
    api:
      security: bearer
      security_description: "A publish token, obtained from POST /auth/token."
      public_paths: "/, /search, /healthz"
```

A credential is not a parameter, and whether an operation needs one is not
something a call in a body can be trusted to say: a check can be conditional, so
a handler that checks a credential is documented as *accepting* one — a security
entry with an empty alternative — while a pipeline that wraps the router in
`jwt` is documented as *requiring* one, because a wrapper refuses the request
before any handler sees it.

## Dependencies

`rho-router` for the reading, `rho-doc` for the pages. Neither is reimplemented:
a second reading of the sources would be a second dialect of Rho, and a second
page renderer would be a second definition of a page.

## Building

```sh
cd modules/components/rho-openapi && go build -o rho-openapi ./src
```
