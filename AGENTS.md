# AGENTS.md

## Test Commands

```sh
# Run all Go tests
go test ./...

# Build the rho CLI
go build -o ./bin/rho ./cmd/rho

# Build and install all tools
./bin/rho install --force
```

## Project Structure

- `cmd/rho` - Main CLI entry point
- `cmd/rho-fmt` - Formatter
- `cmd/rho-test` - Test runner
- `cmd/rho-deps` - Dependency manager
- `cmd/rho-doc` - Documentation generator
- `cmd/rho-flash` - Device flashing tool
- `cmd/rho-pkg/` - Package management (init, pack, unpack, inspect)
- `cmd/rho-module/` - Module management (publish, unpublish, search, etc.)
- `cmd/rho-lsp/` - Language server (standalone)
- `src/` - Compiler, runtime, AST, parser, codegen
- `internal/paths/` - Shared path utilities
- `modules/components/rho-lsp/` - LSP component built via rho module system
