# OpenAPI descriptions and API references

A Rho service declares its routes and its handlers. That declaration is already
everything a machine needs to know about the service's HTTP surface, and this
component writes it out — as a document a tool reads, and as a page a person
reads.

(openapi:command)=
## The `rho openapi` command

```sh
rho openapi generate <project>        # the OpenAPI 3.1 document, as JSON
rho openapi page <project>            # the API reference page, as HTML
rho openapi spec <project>            # the route metadata, as JSON
rho openapi check <project>           # whether the description is sound
```

`-out` writes to a file rather than to standard output. `-server` and `-base`
say where the service answers and what prefix it is mounted under, because a
manifest cannot know either: they are facts about a deployment. `-target` names
the descriptor target to describe when a project serves more than one.

(openapi:document)=
## The document

`generate` writes an OpenAPI 3.1 document. 3.1 rather than 3.0, because a
description produced from a program may state facts the older version has
nowhere to put: a type is a type rather than a string naming one, and a value
may be any shape at all.

```json
{
  "openapi": "3.1.0",
  "info": {"title": "Catalogue", "version": "2.1.0"},
  "paths": {
    "/items/{id}": {
      "get": {
        "operationId": "show_item",
        "summary": "show_item answers one item, read by its identifier.",
        "parameters": [
          {"name": "id", "in": "path", "required": true,
           "schema": {"type": "integer", "format": "int64"}}
        ],
        "responses": {
          "200": {"description": "OK"},
          "404": {"description": "Not Found"}
        }
      }
    }
  }
}
```

Every path is written as the service declares it — `{name}` for one segment,
`{name...}` for the rest of the path — because that is the syntax the router
matches and a description that rewrote it would be a description of a different
service.

The document is written by hand rather than through a marshaller. A document is
something people read in a diff, and a marshaller offers no say in the order of
the keys, which is the whole content of a diff. It is also reflection again, and
a tool whose whole point is that nothing on the path is should not start using
it at the end. The same service always produces the same bytes.

(openapi:reference)=
## The reference page

`page` writes the same description as HTML: the endpoints grouped, the
parameters in a table, the statuses beside them. It is what a person reads when
they want to know what an endpoint takes, and it is written from the spec rather
than from the document, so the two cannot disagree.

In practice you do not run this by hand. `rho-doc-api` is the plugin that adds
the page to a documentation build, so the API reference is a page of the same
site as everything else.

(openapi:serving)=
## Serving the service's own description

A service that publishes a contract should publish it where a client can reach
it. A target that asks for the `openapiemit` port gets one function in Rho:

```rho
route GET "/openapi.json" => openapi_docs

fn openapi_docs(req: Request) Response {
    return json(status_ok(), openapi_document())
}
```

The document is written when the project is generated, from the same reading the
build used, and embedded as a constant: the running service does not read Rho
sources to describe itself, and it cannot fail to start because a source moved.
A build host without the component gets a service that answers with the reason
rather than one that cannot start.

(openapi:security)=
## Credentials

A credential is not a parameter, and whether an operation needs one is not
something a call inside a body can be trusted to say — a check can be
conditional. So a service declares it, once, in the descriptor of the target
that serves it:

```yaml
    api:
      security: bearer
      security_description: "A publish token, obtained from POST /auth/token."
      public_paths: "/, /search, /healthz"
```

The two halves of the answer are different, and a document says both:

| Where it comes from | What the document says |
| --- | --- |
| `api.security` | the service declares the scheme |
| the `jwt` middleware | every operation requires it, except `skip_paths` |
| a handler that checks a credential | that operation accepts it, and may require it |

"Accepts it, and may require it" is written as a security entry with an empty
alternative — `[{"bearerAuth": []}, {}]` — which is exactly the shape of a
conditional check, and an empty alternative is what makes it a description a
client can act on rather than one it has to guess about.

(openapi:check)=
## Checking a description

```sh
rho openapi check applications/registry
```

`check` reports what a description cannot express, which is what a document
becomes unusable for rather than what is merely absent: an operation with no
identifier, two operations claiming one identifier, a path wildcard nothing
declares, a declared path parameter the route has nowhere to put.

An operation that names no status is deliberately not a problem. A handler can
delegate past the level a reading follows, and a document says that with the
catch-all response: the service answers something, and the description says it
does not know what.
