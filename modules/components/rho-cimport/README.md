# rho-cimport

Generates a Rho foreign interface from C headers, and the C that Rho cannot
express. It is run once against a library, and what it writes is a bridge a Rho
program can call that library through.

```sh
rho cimport headers <project>      # what a project's C headers declare
rho cimport check <project>        # what would be generated, writing nothing
rho cimport generate <project>     # write the interface, and the C shim
```

## What it is for

A Rho program can call C, but only through a declaration it has:

```rho
extern fn sensor_open(name: Ptr[u8]) Ptr[Device]
```

Writing those by hand for a real library is a day of transcription that is wrong
by lunchtime, and the standard library's own C abstractions (`Ptr`, `Slice`) do
not map onto C's by themselves. So the declarations are generated from the
headers the library already publishes, which is also the only description of the
library that cannot drift from it.

## The three layers

A bridge is three layers, and each exists because of something the one below it
cannot do:

| Layer | What it is | Where it comes from |
|-------|------------|---------------------|
| 1. The build contract | Which headers, which library, and which vendor library each build target links | `c_connection` in `module.yml`, and the CMake the generator emits |
| 2. The foreign interface | One declaration per C function, structure and enumeration | Generated into `src/ffi.rho` and `src/ffi_shim.c` |
| 3. The idiomatic API | Handles with types, error codes as values, lifecycle in one place | Written by hand, in `src/lib.rho`, over Layer 2 |

### Layer 1: what the project links

```yaml
# module.yml
c_connection:
  library: sensor-lib
  headers:
    - registry.h
    - sensor/hal/spi.h
  include_dirs:
    - include
    - include/sensor/hal
  variants:
    stm32:
      target: vendor_stm32
    pico:
      target: vendor_pico
```

A build target a person names and a library a build links are different things,
and this is where they meet: `rho build --target stm32` generates the C target
with `vendor_stm32` linked behind it. The generated project carries the declared
headers and its own C sources, so it builds where it is generated.

### Layer 2: what the headers declare

```sh
rho cimport generate .
```

writes three files, all marked as generated:

| File | What it is |
|------|------------|
| `src/ffi.rho` | The declarations: `extern fn` per function, a comment per structure and enumeration |
| `src/ffi_shim.h` | The C that makes the calls Rho cannot, declared |
| `src/ffi_shim.c` | The same, made |

The shim is not a convenience. A C structure whose members are function pointers —
a HAL — is a table, and Rho has no type for a function pointer, so without
generated C to forward through, the table could be looked at from a binding and
never called.

Two rules the generator keeps to:

- **A C type is never redeclared in Rho.** A Rho struct with the name of a C one
  is a different type, and a pointer the library hands over would be read through
  the wrong layout. The library's types are named, not redefined, and the
  structure's members are written as documentation.
- **A declaration that cannot be read is reported.** A binding with a gap in it is
  worse than one that was not generated, because it looks complete.

### Layer 3: what a program writes

Layer 2 is C, carried into Rho: an error code is an `i32`, and a handle is a raw
pointer. Layer 3 is where that becomes something a program can use without
knowing it — a `Result`, a typed handle, one place that owns the lifecycle. It is
written by hand, because what reads well is a judgement and not a translation.

## Extending it

The component takes plugins, over the same mechanism every component does. It
declares the contract `platform` and offers one hook for it, `map`, which runs
once with the declarations that were read and answers with the C types a plugin
knows and what they are in Rho. A vendor type the header names and the language
has no primitive for is the one thing a generator cannot work out on its own.

```yaml
# the plugin's module.yml
serves: [zenith]

extends:
  - component: rho-cimport
    implements: [platform]
    hooks: [map]

commands:
  - name: vendor-types
    run: my-vendor-types
    hook: map
    component: rho-cimport
```

`extends` is a list because a platform speaks through more than one component:
the same platform states its C types here, its targets in rho-target and its
configuration in rho-kconfig, and each of those relations names the component it
belongs to. `serves` is what makes the mapping happen for one platform and not
for any other — see [the platform plugins](../../plugins/rho-robohub/) for a
plugin that fills four components' `platform` contracts at once.

A plugin that extends exactly one component may write `extends: rho-cimport` as
before, with `implements` and `hooks` beside it; both forms are the same thing.

The request and the answer are JSON envelopes, and the component's payload is
inside the envelope:

```json
{ "version": 1, "hook": "map", "component": "rho-cimport",
  "payload": { "headers": ["robot.h"],
               "declarations": [{ "kind": "alias", "name": "vendor_handle_t", "type": "int" }] } }
```

```json
{ "version": 1, "payload": { "types": { "vendor_handle_t": "i64" } } }
```

:::{note}
The payload is *inside* the envelope, not the whole document: a plugin that
decodes the whole request as its own payload finds every field empty and no
error to tell it so.
:::

A mapping a plugin supplies is used for the type it names and nothing else, and a
plugin that fails is reported and its mappings are left out.

## Building it

```sh
go build -o rho-cimport ./src
go test ./...
```
