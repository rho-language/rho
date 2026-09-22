# Rho Math Standard Library

The base `math` capability is enabled for every Rho module. Specialized
modules are split into independent `.rho` files so an application can import
only what it uses:

- `std.math.basic` - scalar min/max, clamp, map, deadband, and smoothstep
- `std.math.float` - floating-point functions and angle conversions
- `std.math.control` - PID, EMA, moving average, complementary, and Kalman filters
- `std.math.linear` - fixed-size vectors, matrices, and quaternions
- `std.math.fixed` - Q16.16 and Q8.8 fixed-point values
- `std.math.fast` - fast-math C hooks
- `std.math.embedded` - embedded scaling and ramp helpers

Enable a specialized capability in `rho.mod`:

```text
feature = "math.control"
```

Or enable it locally in a Rho module:

```rho
module controller
feature "math.control"
```

Feature flags automatically include the matching standard module and emit a
`RHO_FEATURE_*` definition in generated C. Explicit imports such as
`import std.math.linear` remain supported when a module wants direct control
over its dependencies.