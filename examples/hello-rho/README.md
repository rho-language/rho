# Hello Rho

This example loads a name from `config/app.yml` and prints a greeting using the
Rho compiler and native x86 runner.

## Files

- `src/main.rho` - project entry point
- `src/greeting.rho` - included Rho source
- `src/messages.rho` - imported Rho module
- `config/app.yml` - YAML configuration read by the program
- `rho.mod` - project manifest

## Build and run on x86

From the project root:

```bash
go build -o ./rho ./cmd/rho
./rho build examples/hello-rho/src/main.rho
./rho run examples/hello-rho/src/main.rho
```

Expected output:

```text
Hello Alice
```
