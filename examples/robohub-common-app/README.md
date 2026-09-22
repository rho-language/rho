# RoboHub Common Dependency

This small project declares the local `robohub-common` C repository as a Rho
dependency. The dependency uses the absolute local path
`/home/miraur/projects/robohub-common`, matching the version declared in its
`module.yml` (`1.0.0`) and its exported include/source layout.

The same dependency also declares its Git source and `main` branch. If the
absolute local path is unavailable, the dependency manager clones that Git
repository into the declared path. A dependency may instead use `tag` to pin
a release; `branch` and `tag` are optional alternatives.

The `main: src/main.rho` entry in `module.yml` selects the application entry
point. The manifest enables `math.control`, while the application uses the
control module. Base `math` support is enabled automatically; specialized
standard modules are imported only when used.

From the Rho repository root, resolve and inspect the dependency:

```bash
(cd examples/robohub-common-app && go run ../../cmd/rho-deps sync)
(cd examples/robohub-common-app && go run ../../cmd/rho-deps graph --json)
(cd examples/robohub-common-app && go run ../../cmd/rho-deps verify)
```

Build the application project from the repository root:

```bash
go build -o ./rho ./cmd/rho
./rho build examples/robohub-common-app
./rho run examples/robohub-common-app
```

This generates `build/app.c`, `build/app.h`, `build/main.c`, and
`build/CMakeLists.txt` (plus `build/config.h` when the project has a config
directory). Because `robohub-common` contains platform hooks, its
native executable also needs a board implementation; the generated CMake
project is the integration point for that implementation.

The example is intentionally focused on dependency resolution. The current
`robohub-common` platform sources require a board implementation for native
linking, so this project does not claim to be a standalone host executable.