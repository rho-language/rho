# rho-mem

Where a firmware's memory went, and whether that is allowed.

`arm-none-eabi-size` prints one number per section. That number is what a budget
is written against, and it is not an answer: a build that reports "image is 14 kB
too large" has said nothing about which of the two hundred things in it to remove.
Everything else that fits in a terminal has the same problem — `nm --size-sort`
lists symbols without saying which region they cost, so the flash resident and the
RAM resident come out as one sorted list of numbers that cannot be added to
anything.

This reads what the linker already wrote and keeps the two facts together: a
symbol's size, and the memory region it was placed in.

```sh
rho mem                 # where it went
rho mem --top 15        # and what is in it
rho mem --diff main     # against the baseline on another branch
rho mem check           # fail if it does not fit
```

## What it reads

| Artifact | What it contributes |
|---|---|
| Linker map | The memory regions, exactly as the linker script names them, and each symbol with the object it came from |
| ELF | The allocated sections, and the zero-initialised ones — the one thing the map cannot express |

A build writes the map by default (`build.link_map`, or `--no-link-map` to turn it
off), so a project that has never heard of this component already has one.

## Two things the totals get right

**A `.data` section costs two regions.** Its initializer image is in flash and its
variables are in RAM, and both are paid for. Counting it once counts the wrong
region once.

**A region's usage is its span, not a sum of section sizes.** A link pads: between
two sections, and inside one to align a member. Those bytes are programmed. The
span from the lowest address charged to a region to the highest counts every byte
between them once — which is why these totals come out slightly *above*
`arm-none-eabi-size`, and are right to. On the `nucleo-h753zi` example that is 40
bytes of alignment padding between the vector table and the code, which `size`
does not mention and the flash does contain.

## Declaring budgets

```yaml
# module.yml
mem:
  regions:
    FLASH: { max_bytes: 2097152, warn_threshold: "85%" }   # 2 MB
    SRAM1: { max_bytes: 524288,  warn_threshold: "80%" }    # 512 KB
    DTCM:  { max_bytes: 131072,  warn_threshold: "90%" }    # 128 KB
  enforce:
    fail_on_overflow: true
    fail_on_growth_bytes: 10240    # fail if flash grows by more than 10 kB
```

Sizes take `K`/`KB`/`KiB`/`M`/`MB`/`G`, thresholds take `85%`, `85` or `0.85`, and
the region names are the ones the linker script uses — which is what `rho target
show -json` prints under `memory`.

Nothing in the manifest is required. A project with no `mem:` block gets a report
and a note saying there is nothing to check against, which is the state of a
project that has not decided yet.

## Growing is the question worth asking

A capacity says where the ceiling is. It does not say whether the thing under it is
getting bigger, and that is what a pull request actually raises: this firmware
fits, and it is 4 kB bigger than the one on the base branch.

So a run records what it found, and a later run compares:

```sh
rho mem --write-baseline      # once, and commit the file
rho mem --diff origin/main    # in a pipeline
```

The baseline is a file in the repository rather than something only a log holds,
because a diff on it is a diff anybody can read while reviewing the change that
caused it.

## Reading the report

```
FLASH   [##################..........]  1.12 MB / 2.00 MB  (56.0%)  [OK]
SRAM1   [#######################.....] 412.0 KB / 512.0 KB (80.4%)  [WARN]
DTCM    [####........................]  20.4 KB / 128.0 KB (15.9%)  [OK]
```

`WARN` is inside the budget and close enough to it to be worth acting on early — a
region at 99% is a region whose next feature does not fit, and nothing about that
shows up until a build fails. `--no-warn` turns the threshold off; the capacity is
still checked.

Regions that overlap — which a linker script does when a board's data really lives
in tightly coupled memory and the general-purpose region is declared across it —
are charged to the more specific one, and the overlap is reported rather than left
to be inferred from the numbers.

## Nothing here is specific to a board

There is no list of memory regions in this component, no per-vendor arithmetic, and
no per-family table. The regions are whatever the linker script said they were, so
the same component reports an STM32, an RP2040 and a host executable without being
told which it is looking at.

## Building it

```sh
go build -o rho-mem ./src
go test ./...
```
