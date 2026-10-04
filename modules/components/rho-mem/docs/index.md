# Memory budgets

Where a firmware's memory went, and whether that is allowed.

## The problem it answers

`arm-none-eabi-size` prints one number per section. That number is what a budget
is written against, and it is not an answer: a build that reports "image is 14 kB
too large" has said nothing about which of the two hundred things in it to remove.

`nm --size-sort` lists symbols largest first, which is closer, and still does not
say which memory region each one costs. Without that, the flash resident and the
RAM resident are one sorted list of numbers that cannot be added to anything — and
"reduce flash" and "reduce RAM" are different tasks with different answers.

So the two facts are kept together: a symbol's size, and the region it was placed
in.

```
               build artifacts (.map, and the ELF beside it)
                                  │
                    ┌─────────────┴─────────────┐
                    ▼                           ▼
          linker map                   ELF headers
          Memory Configuration          allocated sections
          output sections               SHT_NOBITS sections
          symbols + the object          symbol table
            each came from
                    │           │
                    └─────┬─────┘
                          ▼
              regions, and what landed in each
                          │
        ┌─────────────────┼──────────────────┐
        ▼                 ▼                  ▼
   breakdown         top symbols          budgets
   per region        by region and size    fail or warn
```

## The two accounting decisions

Everything else is a format to read or a report to print. Two things decide whether
the numbers mean anything.

**A `.data` section costs two regions.** Its initializer image lives in flash and
its variables live in RAM, and the firmware pays for both: the flash holds the
bytes that are copied out at startup, and the RAM holds them afterwards. A tool
that counts the section once has counted the wrong region once.

The map prints a *load address* for every section that has one, including `.bss` —
which stores nothing at all. So the zero-initialised sections are identified from
the ELF's `SHT_NOBITS`, because that is the one question a linker map has no way
of expressing, and charging a `.bss` to flash overstates the image by however large
it is.

**A region's usage is a span, not a sum.** A link pads: between two sections, and
inside one to align a member. Those bytes are in the image. The map writes them
two ways — as `*fill*` entries inside a section that already counts them, and as
gaps between sections that nothing counts — so summing section sizes double-counts
the first and drops the second.

The span, from the lowest address charged to a region to the highest, is right by
construction: it counts every byte between them once, which is exactly the bytes
that region holds.

This is why these totals come out slightly **above** `arm-none-eabi-size`. On the
`nucleo-h753zi` example the difference is 40 bytes — the alignment padding between
the vector table and the code — and `size` does not mention it while the flash
does contain it.

## Overlapping regions

A linker script may declare two regions over the same addresses, and does on a part
whose data really lives in tightly coupled memory: the general-purpose RAM is
declared across the DTCM, both at the same origin. "The region containing this
address" then has more than one answer.

The more specific region — the smaller one, since a larger region declared across a
smaller one is a description in more general terms — is charged, and the overlap is
reported. A budget written against the wrong one of two aliased regions measures
the wrong thing, and a reader who has to infer the rule from the numbers cannot.

## Declaring budgets

```yaml
# module.yml
mem:
  map_file: build/firmware.map       # optional; a build writes the map itself
  elf_file: build/firmware.elf       # optional
  regions:
    FLASH: { max_bytes: 2097152, warn_threshold: "85%" }   # 2 MB
    SRAM1: { max_bytes: 524288,  warn_threshold: "80%" }    # 512 KB
    DTCM:  { max_bytes: 131072,  warn_threshold: "90%" }    # 128 KB
  enforce:
    fail_on_overflow: true
    fail_on_growth_bytes: 10240    # fail if flash grows by more than 10 kB
```

Sizes take `K`, `KB`, `KiB`, `M`, `MB`, `G`. Thresholds take `85%`, `85` or
`0.85`. Region names are the ones the linker script uses, which is what
`rho target show -json` prints under `memory`.

A region with no budget is reported and not failed: a project that budgets flash
and not the backup SRAM has decided the backup SRAM is not a thing to worry about.

A budget for a region the build placed nothing in is reported too, because it is
either a typo in the name or a feature that was switched off, and both are silent
otherwise.

## Growing is the question worth asking

A capacity says where the ceiling is. It does not say whether the thing under it is
getting bigger — and a firmware that grows 300 bytes a week is 10 kB over in a
year, with every one of those builds passing.

```sh
rho mem --write-baseline      # once; commit the file
rho mem --diff origin/main    # in a pipeline
```

The baseline is a file in the repository rather than something only a log holds,
because a diff on it is a diff anybody can read while reviewing the change that
caused it. A baseline recorded for one board is not compared against a build for
another: the regions and their sizes differ, so the number would mean nothing.

## The commands

| Command | What it does |
|---|---|
| `rho mem [project]` | The breakdown and the largest symbols. Exits zero whatever it found |
| `rho mem --top N` | How many symbols to attribute. `0` for none |
| `rho mem check` | The same analysis with the outcome enforced; non-zero when a budget is exceeded |
| `rho mem --diff REF` | Against the baseline as a git ref carries it |
| `rho mem --write-baseline` | Records this run's figures |
| `rho mem --bytes` | Exact byte counts beside the readable ones |
| `rho mem --no-warn` | Drops the warn threshold; the capacity is still checked |

`check` enforces the overflow whether or not the project declared it, because a
command whose whole purpose is to fail a build that does not fit should not exit
zero because the project has not yet written down how much room it has.

## Where the map comes from

A build writes one by default:

```yaml
build:
  link_map: true      # the default; `rho build --no-link-map` for one run
```

It is written with `--cref` beside it, so the map carries which object referred to
which symbol. A symbol's size says what it costs; the cross reference says who
pulled it in — and a driver that was not meant to be linked at all is a different
problem from one that is simply large.

## Nothing here is specific to a board

There is no list of memory regions in this component, no per-vendor arithmetic and
no per-family table. The regions are whatever the linker script said they were, so
the same component reports an STM32, an RP2040 and a host executable without being
told which it is looking at.

A project can also point this at a platform's own conventions the same way it points
any other component at them: a plugin. Which analyzers a project runs is a fact
about the project rather than about the tool — the same argument that makes
`rho lint`'s analyzers plugins applies to anything that has to read a build.
