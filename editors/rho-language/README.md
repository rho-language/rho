# Rho for VS Code

This extension provides full language support for Rho:
- Syntax highlighting
- Language configuration (brackets, comments, etc.)
- **Built-in LSP client** — connects to `rho lsp` automatically

No separate Generic LSP Client extension needed.

## Install locally

From the repository root:

```sh
cd editors/rho-language
./package.sh
code --install-extension rho-0.2.0.vsix
```

This uses Python's standard library and does not require npm or `vsce` for packaging (npm is used only for compiling the TypeScript LSP client).

## Configuration

The LSP client starts `bin/rho lsp` by default (relative to workspace root). Configure the server path in VS Code settings:

```json
{
  "rho.lsp.serverPath": "bin/rho"
}
```

Use an absolute path if your rho binary is elsewhere.

## Build from source (development)

```sh
cd editors/rho-language
npm install
npm run compile
```