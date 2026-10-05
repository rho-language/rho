# The language server

`rho lsp` speaks the Language Server Protocol on stdin and stdout, so an editor
gets completion, diagnostics and navigation for Rho sources without knowing
anything about Rho.

```sh
rho lsp < /dev/null      # the protocol is the input; an editor starts it
```

## What it answers

The server is a component, so it is installed and enabled like any other and
dispatched to by the command its manifest publishes. It reads what a project
declares — the manifest, the dependencies, the target — and answers from that,
so a completion is a completion the compiler would accept rather than a list of
words seen in the file.

It answers the requests an editor sends while a person types, and it reports
what the compiler found: a diagnostic from this server and one from a build are
the same diagnostic, which is the point of answering from the compiler's own
front end rather than from a second parser.

## How an editor is configured

`rho ide vscode` writes the launch configuration and the tasks an editor needs,
from what the project declares — so the debug session an editor starts and the
board its target spec describes cannot disagree.

```sh
rho ide vscode .
```

## Related

- {ref}`cli <documentation/cli>` — `rho ide`, and the rest of the CLI.
- {ref}`rho-dbg <rho-dbg/index>` — the debug adapter, which is a different
  protocol on the same idea: the editor's side is written from the project's
  own facts.
- {ref}`rho-fmt <rho-fmt/index>` and {ref}`rho-lint <rho-lint/index>` — the
  tools an editor runs alongside the server.
