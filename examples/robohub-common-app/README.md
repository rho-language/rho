# RoboHub Common Dependency

This small project declares the local `robohub-common` C repository as a Rho
dependency. The dependency uses the absolute local path
`/home/miraur/projects/robohub-common`, matching the version declared in its
`module.yml` (`1.0.0`) and its exported include/source layout.

From the Rho repository root:

```bash
go run ./cmd/rho-deps sync
```

From this example directory, inspect the resolved dependency:

```bash
go run ../../cmd/rho-deps graph --json
go run ../../cmd/rho-deps verify
```

The example is intentionally focused on dependency resolution. The current
`robohub-common` platform sources require a board implementation for native
linking, so this project does not claim to be a standalone host executable.