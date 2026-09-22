# Hello Rho

This example loads a name from `config/app.yml` and prints a greeting using the
Rho compiler and native x86 runner.

## Files

- `src/main.rho` - project entry point
- `src/greeting.rho` - included Rho source
- `src/messages.rho` - imported Rho module
- `config/app.yml` - YAML configuration read by the program
- `rho.mod` - project manifest
- `build/` - generated C application project

## Build and run on x86

From the project root:

```bash
go build -o ./rho ./cmd/rho
./rho build examples/hello-rho
./rho run examples/hello-rho
```

The `main = "src/main.rho"` entry in `rho.mod` selects the application entry
point. Rho follows its imports and includes, then generates `app.c`, `app.h`,
`main.c`, `CMakeLists.txt`, and `config.h` under `build/`.

Expected output:

```text
Hello Alice
```
