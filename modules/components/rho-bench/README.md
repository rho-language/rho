# rho-bench

How long a program's code takes, how much stack and heap it uses while running, and
how that compares with the last time.

```sh
rho bench                       # measure what this project benchmarks
rho bench --filter crypto       # measure one group of them
rho bench --diff main           # against the baseline on another branch
rho bench check                 # fail if a budget or a threshold broke
```

## What it is for

`rho mem` measures a program's size and `rho-test` says whether it passes. Neither
says how long it takes. A size is a fact about a program at rest; a time is a fact
about a program running, and they answer different questions and move for different
reasons — code can get faster without getting smaller, and smaller without getting
faster.

## The harness is the smallest part on purpose

A benchmark is a function called many times, and everything in `rho/bench.h` runs
inside somebody's firmware on a part with 128 kB of RAM. So it measures and prints,
and it does not summarise. The median, the percentiles and the verdict are computed
here, on a machine with memory — a harness that also computed them would be a second
implementation of a median that nobody would keep in step with the first.

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

int main(void)
{
    static uint64_t samples[BENCH_ITERATIONS];
    static uint64_t cycles[BENCH_ITERATIONS];

    rho_bench_t state;
    rho_bench_prepare(&state, "ns", BENCH_ITERATIONS, BENCH_WARMUP,
                      samples, BENCH_ITERATIONS, cycles);
    state.name = "crypto";

    rho_bench_suite_t suite = {0};
    RHO_BENCH_ADD(&suite, bench_aes_encrypt);

    return rho_bench_run_all(&state, &suite);
}
```

Build it and measure it:

```sh
cc -O2 -I<rho-bench>/include bench.c <rho-bench>/harness/rhobench.c -o bench
rho bench --binary ./bench
```

## What it measures

| Figure | Where it comes from | What it answers |
|---|---|---|
| **Cycles** | `DWT->CYCCNT` on Cortex-M, `rdtsc` on a host | What a cycle budget is written against |
| **Time** | A monotonic clock | What a latency promise is written against |
| **Stack** | A fill, scanned back for the boundary | How deep a call actually went |
| **Heap** | The allocator's own calls | What is live at once, and what never came back |

Both cycles and time are recorded because either alone answers half the question:
cycles mean nothing without a clock frequency, and time at 100 ns resolution says
nothing about a 70-cycle function. **A host's `clock_gettime` is coarse — the
example suite below shows `0 ns` medians next to a 70-cycle median, which is the
same fact measured by a clock that is too slow to see it.** Both are reported with
the clock they came from, and a baseline recorded for one target is refused against
another rather than compared.

The stack figure is the only way to know how deep a call went after the fact: the
stack is painted with `0xAA`, the benchmark runs, and the lowest byte still holding
the paint is one below the deepest thing written. A stack that filled completely has
been overrun, and the boundary does not exist — so that is reported as *not measured*
rather than as a number, because a number that understates the truth is worse than
none.

## Declaring budgets

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

`fail_on_regression_percent` is a limit rather than "any change", because benchmarks
move: a rebuild shifts alignment, a scheduler shift moves a median by a percent, and
a gate on *any* change would be switched off within a month.

## Baselines

```sh
rho bench --write-baseline      # once; commit the file
rho bench --diff origin/main    # in a pipeline
```

The baseline is a file in the repository rather than something only a log holds,
because a diff on it is a diff anybody can read while reviewing the change that
caused it. What is recorded is each benchmark's figures, not a total and not a
verdict — "4% slower" is a question, and the answer is which benchmark, which metric,
and which direction.

Every metric shares one direction of "better": a lower cycle count, a lower time, a
lower stack figure and a lower heap figure are all improvements. That is what lets
one threshold mean the same thing for all of them, and it is why a change that cuts
cycles and grows the heap is reported as two facts rather than averaged into a
verdict describing neither.

## What is composed, and what is not

The static footprint is `rho-mem`'s and is asked for, not linked:

```
┌───────────────┐        ┌───────────────┐        ┌───────────────┐
│   rho-mem     │        │   rho-test    │        │  rho-monitor  │
│ static sizes  │        │ runs the      │        │ captures the  │
│ from map/ELF  │        │ benchmark     │        │ trace a run   │
└───────▲───────┘        └───────▲───────┘        └───────▲───────┘
        └───────── asked, not linked ──────────────────────┘
```

`rho mem --json` is the seam, and it is a documented document rather than a function
signature — so either side can be replaced by a plugin, or by a version of the other
that answers differently, without this component changing.

## Verified, and not

- **Verified:** the harness, on a host. It compiles, runs, and produces correct figures
  — an empty body at 117 cycles, a 64-byte CRC at 1987, a 256-byte one at 7821, and a
  malloc/free pair at 107 with a heap peak of one slot. `examples/crypto_bench.c` is
  the suite it was verified with.
- **Not verified here:** the Cortex-M `DWT` path and the stack fill on a real target.
  The code is written and guarded by the build tag it needs, but no board in this tree
  was measured through it. Everything else in this document is tested.

## Building it

```sh
go build -o rho-bench ./src          # the component
go test ./...                        # its tests
cc -O2 -Iinclude examples/crypto_bench.c harness/rhobench.c -o /tmp/crypto_bench
```