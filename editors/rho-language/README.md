# Rho Language Support for VS Code

This minimal extension registers the `rho` language id, associates `.rho`
files with Rho, and provides basic syntax highlighting.

## Install locally

From the repository root:

```sh
cd editors/rho-language
./package.sh
code --install-extension rho-language-0.1.0.vsix
```

This uses Python's standard library and does not require npm or `vsce`.

Reload VS Code. `Rho` will then appear in the language mode selector. The
workspace Generic LSP Client configuration launches `/home/miraur/projects/rho/bin/rho lsp` for this language.
