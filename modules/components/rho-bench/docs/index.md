# Benchmarks

How long a program's code takes, how much stack and heap it uses while running, and
how that compares with the last time.

## What it is for, and what it is not

`rho mem` measures a program's size. `rho-test` says whether it passes. Neither says
how long it takes, and that is a different fact rather than a better one:

- A **size** is a property of a program at rest. Code can get faster without getting
  smaller.
- A **time** is a property of a program running. It moves with the clock, the cache,
  the branch predictor and whatever else the core is doing.
- A **stack figure** is neither, and is the one most tools never report at all: how
  deep a call actually went, which is the question behind every stack overflow and is
  invisible in a size.

So this is a separate component rather than a flag on one of them.

## The harness is the smallest part, on purpose

A benchmark is a function called many times, and everything in `rho/bench.h` is code
that runs inside somebody's firmware, on a part with 128 kB of RAM. So it measures
and prints, and it does not summarise.

```c
#include <rho/bench.h>

void bench_aes_encrypt(rho_bench_t *state)
{
    uint8_t key[16] = {0};
    uint8_t data[64] = {0};

    RHO_BENCH_LOOP(state) {
        aes128_encrypt(key, data);
    }
}
```

The median, the percentiles and the verdict are computed in `rho bench`, on a machine
with memory. A harness that also computed them would be a second implementation of a
median, and there would be no way to tell which of the two a number came from.

### Why the suite is a list and not a macro

A benchmark is added by writing it out:

```c
    rho_bench_suite_t suite = {0};
    RHO_BENCH_ADD(&suite, bench_aes_encrypt);
    RHO_BENCH_ADD(&suite, bench_json_parse);
```

The alternatives were both worse. A static initialiser cannot call a function, so a
macro that builds a table has to use a compiler extension; and a constructor's order
among other constructors is not defined, which would make the order a suite runs in
differ between two builds of the same source. A list is written in the order it runs,
by whoever wrote it — and the report lists benchmarks in that order too.

## What it measures

| Figure | Where it comes from | The question it answers |
|---|---|---|
| **Cycles** | `DWT->CYCCNT` on Cortex-M, `rdtsc` on a host | What a cycle budget is written against |
| **Time** | A monotonic clock | What a latency promise is written against |
| **Stack** | A fill, scanned back for the boundary | How deep a call actually went |
| **Heap** | The allocator's own calls | What is live at once, and what never came back |

### Both clocks, because either alone answers half

`DWT->CYCCNT` costs two instructions, so a benchmark on a Cortex-M measures almost
none of its own overhead. It also counts cycles, not seconds: without a clock
frequency it is a number nobody can read.

The host has the opposite problem, and it is worth being concrete about. `clock_gettime`
on a typical Linux machine resolves to about 100 ns. A 70-cycle function is roughly
24 ns. So the time column of a host benchmark legitimately reads `0 ns` while the
cycle column reads `70` — the same fact, measured by a clock too slow to see it, and
reported rather than hidden.

Every figure is reported with the clock it came from, and **a baseline recorded for
one target is refused against another** rather than compared. A host's cycles and a
board's cycles are different units, and the difference between them is a number with
no meaning.

### The stack figure

The only way to know how deep a call went after the fact is to look:

1. paint a bounded region of the stack with `0xAA` — chosen because it is not zero,
   not `0xFF`, and not a plausible pointer, so a legitimate local cannot look like
   untouched stack;
2. run the benchmark;
3. scan back from the low end for the first byte still holding the paint.

That byte is one below the deepest thing written. A stack that filled completely has
been overrun and the boundary does not exist, so that is reported as **not measured**
rather than as a number — a figure that understates the truth is worse than none.

### The heap figure

The harness wraps the allocator's calls rather than replacing it, so it works with a
firmware whose allocator is its own: the calls are the same two, in the same places,
whatever they do.

```c
    held[slot] = malloc(SLOT_BYTES);
    RHO_BENCH_HEAP_ALLOC(state, held[slot], SLOT_BYTES);
```

The harness counts rather than the benchmark, because a benchmark that counted its own
would have to be right about it, and forgetting one subtraction makes the live count
drift downwards and the peak look plausible — which is the failure a heap report has
to be able to catch. The **live count at the end** is what a leak looks like: a peak
that is stable and a live count that is not.

## Statistics

Runs are summarised rather than sampled from, because one run measures noise.

- **Median** is the middle run. Not the mean: one run in a thousand can be an order of
  magnitude out — a page fault, a timer that landed badly — and the mean is moved by
  it in a way that has nothing to do with the code. The example suite makes the point:
  an outlier moves the mean by an order of magnitude and the median not at all.
- **p95 and p99** are the tails, because latency is a promise about the worst case
  and a median hides it.
- **Standard deviation** is whether the runs agreed with each other. It is reported as
  a multiple of the median and capped, because it is not robust: one descheduled
  iteration turns it into hundreds of percent, which reads as a broken number rather
  than as what it is — a run with an outlier, which the p99 and the max beside it say
  properly.

Warm-up runs are discarded, because the first call into a function is not the steady
state: the instruction cache is cold, the branch predictor has nothing in it, and on a
host the code may not be faulted in yet. Measuring those would report the cost of
starting up as the cost of the work — which, on a host where faulting in a cache line
costs more than the function does, is the whole of the number.

## Budgets and regressions

```yaml
# module.yml
bench:
  default_iterations: 20000
  warmup_runs: 100
  stack_watermark: true
  budgets:
    max_stack_bytes: 4096
    max_heap_bytes: 8192
    max_cycles_per_iteration: 5000
    fail_on_regression_percent: 5.0
  targets:
    - host
    - stm32h743zi
```

`fail_on_regression_percent` is a limit rather than any change. Benchmarks move: a
rebuild shifts alignment, a scheduler shift moves a median by a percent, and a gate on
*any* change would be switched off within a month. The limit is what makes it a gate.

Every metric shares one direction of "better": lower cycles, lower time, lower stack,
lower heap. That is what lets one threshold mean the same thing for all of them, and
why a change that cuts cycles and grows the heap is reported as **two facts** rather
than averaged into a verdict describing neither.

```sh
rho bench --write-baseline      # once; commit the file
rho bench --diff origin/main    # in a pipeline
```

The baseline is a file in the repository rather than something only a log holds,
because a diff on it is a diff anybody can read while reviewing the change that caused
it.

## Composition

```
┌───────────────┐        ┌───────────────┐        ┌───────────────┐
│   rho-mem     │        │   rho-test    │        │  rho-monitor  │
│ static sizes  │        │ runs the      │        │ captures the  │
│ from map/ELF  │        │ benchmark     │        │ trace a run   │
└───────▲───────┘        └───────▲───────┘        └───────▲───────┘
        └───────── asked, not linked ──────────────────────┘
```

The static footprint is `rho-mem`'s and is asked for through `rho mem --json` — a
document, not a function signature. That is what lets either side be replaced by a
plugin, or by a version of the other that answers differently, without this component
changing. A component that linked another's library would be a copy of it that nobody
extends.

## What is verified, and what is not

- **Verified:** the harness, on a host. It compiles, runs, and produces correct
  figures — an empty body at 117 cycles, a 64-byte CRC at 1987, a 256-byte one at
  7821, and a malloc/free pair at 107 with a heap peak of one slot.
  `examples/crypto_bench.c` is the suite it was verified with.
- **Not verified here:** the Cortex-M `DWT` path and the stack fill on a real target.
  The code is written and guarded by the build tag it needs, but no board in this tree
  was measured through it.

Both are in the repository rather than in a branch, and the distinction is stated here
because the rest of this page is tested and that is not.

## The commands

| Command | What it does |
|---|---|
| `rho bench [project]` | Measure and report. Exits zero whatever it found |
| `rho bench --filter GLOB` | Measure only the benchmarks whose name matches |
| `rho bench --target NAME` | What to say the figures are for |
| `rho bench --diff REF` | Against the baseline as a git ref carries it |
| `rho bench --write-baseline` | Record this run's figures as the baseline |
| `rho bench check` | The same measurement, failing on a budget or a limit |
| `rho bench --json` | A document rather than a table |
| `rho bench --no-footprint` | Skip the static figure, which is `rho-mem`'s |

`check` is the same analysis with the outcome enforced, because a person reads the
text and a pipeline reads the exit code.