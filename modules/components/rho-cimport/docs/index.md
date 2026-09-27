# C import bridge

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
extern fn robohub_get_spi(name: Ptr[u8]) Ptr[RoboHubSPI]
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
  library: robohub-common
  headers:
    - registry.h
    - robohub/hal/spi.h
  include_dirs:
    - include
    - include/robohub/hal
  variants:
    stm32:
      target: robohub_stm32
    pico:
      target: robohub_pico
```

A build target a person names and a library a build links are different things,
and this is where they meet: `rho build --target stm32` generates the C target
with `robohub_stm32` linked behind it. The generated project carries the declared
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

The component takes plugins, over the same mechanism every component does:

| Hook | When it runs | What a plugin answers with |
|------|--------------|---------------------------|
| `parse` | Per header read, before its declarations are mapped | Declarations to add |
| `map` | Once, with everything that was read | Type mappings |
| `write` | Once, with the files that were written | Nothing yet; a step that observes |

## Building it

```sh
go build -o rho-cimport ./src
go test ./...
```
