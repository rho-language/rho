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
- `src/codegen/` - C and Go code generation backends
- `src/projectgen/` - Generated-project descriptors and backends
- `src/schema/` - YAML schema validation
- `src/yaml/` - Minimal YAML loader
- `internal/paths/` - Shared path utilities
- `modules/components/rho-lsp/` - LSP component built via rho module system

## Build Targets

Rho applications generate C, Go, or Go web-service projects. The layout of
each target is data-driven by `src/projectgen/descriptor.default.yml`; a
project overrides it with its own `project.yml`. Applications pick backends via
`build.targets` in `module.yml`.

```sh
# Generate all declared targets (build/generated/<target>/)
./bin/rho build <project-dir>

# Build and run a generated Go project
cd <project-dir>/build/generated/go && go run .

# Build a generated C project
cd <project-dir>/build/generated/c && cmake -B out && cmake --build out
```
