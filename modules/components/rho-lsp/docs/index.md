# The language server

`rho lsp` speaks the Language Server Protocol on standard input and output, so an
editor gets diagnostics, completion, hover, go-to-definition, references and
semantic highlighting for Rho sources. It reads with the compiler's own lexer, parser and type checker, so a
squiggle in the editor and a build failure are the same finding at the same
position — there is no second grammar to keep in step with the language.

It is the one component `rho install` puts in place itself: the install builds
`rho-lsp` from the checkout, registers it and enables it, so `rho lsp` works on a
fresh machine without publishing anything.

```sh
rho lsp     # waits for the client's `initialize`; it prints no banner
```

## What it answers

| Request | What it gives |
|---|---|
| `textDocument/didOpen`, `didChange`, `didClose` | The open documents, kept in step as you type |
| `textDocument/publishDiagnostics` | What the compiler found, sent after each change |
| `textDocument/completion` | The symbols the workspace's symbol tables know |
| `textDocument/hover` | What the syntax node under the cursor is |
| `textDocument/definition` | Where it is declared |
| `textDocument/references` | Where it is used |
| `textDocument/semanticTokens/full` (and `/delta`) | Token types for highlighting |

## VS Code

The extension in `editors/rho-language` is the client: syntax highlighting, the
language server, and commands for building, running, testing and debugging. Its
current version is 0.4.0 (`package.json`). Package and install it from the
checkout:

```sh
cd editors/rho-language
./package.sh                            # npm install and compile when needed, then the .vsix
code --install-extension rho-0.4.0.vsix
```

It starts the server as `rho lsp`, finding `rho` by looking, in order, at the
`rho.lsp.serverPath` setting, `$RHO_BIN`, the Rho home (`~/.rho/bin/rho`, or
under `$RHO_HOME`) and `PATH`; nothing needs configuring after `rho install`. A
relative `rho.lsp.serverPath` is read from the Rho home. When the component is
installed but not enabled, a failed start offers the command that enables it.

## Configuring a project for an editor

`rho ide` writes the launch configurations, the tasks and a workspace file from
what the project declares, so the debug session an editor starts and the board
its target spec describes cannot disagree:

```sh
rho ide vscode .
rho ide neovim . -target pico
```

Neovim's DAP client reads the same `.vscode/launch.json`. Any LSP client can use
the server by running `rho lsp` for files with the `.rho` extension.

## Related

- {ref}`tools/cli` — `rho ide`, and the rest of the CLI.
- {ref}`rho-dbg <rho-dbg/index>` — the debug adapter: a different protocol on the
  same idea, the editor's side written from the project's own facts.
- {ref}`rho-fmt <rho-fmt/index>` and {ref}`rho-lint <rho-lint/index>` — the
  tools an editor runs alongside the server.
