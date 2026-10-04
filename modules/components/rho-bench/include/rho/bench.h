/*
 * rho_bench.h — measuring code, on the target.
 *
 * A benchmark is a function called many times, and this header is the whole of what
 * a benchmark is written against. It is deliberately small: everything it does is
 * code that runs inside somebody's firmware, on a part with 128 kB of RAM and a
 * console slower than a modern disk. Everything else — the statistics, the
 * comparison against a baseline, the report, the verdict — happens in `rho bench`,
 * on a machine with memory to spare.
 *
 * So this measures and prints. It does not summarise: the summary is computed where
 * the meaning of a median is decided, and a harness that also computed one would be
 * a second implementation of it.
 *
 * A benchmark is three lines:
 *
 *     void bench_crc32(rho_bench_t *b) {
 *         RHO_BENCH_LOOP(b) {
 *             crc32(data, length);
 *         }
 *     }
 *     RHO_BENCH_REGISTER(bench_crc32);
 *
 * The harness measures two things where it can, and one where it must.
 *
 *   - **Cycles** from the core's own cycle counter — `DWT->CYCCNT` on Cortex-M,
 *     which costs one load and one subtraction per iteration and therefore measures
 *     almost nothing of its own. On a host, `rdtsc` where it is available.
 *   - **Time** from a monotonic clock, because cycles mean nothing without a
 *     frequency and a frequency is a fact about the clock tree.
 *   - **Stack** from a fill, which is the only way to know how deep a call went
 *     after the fact.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef RHO_BENCH_H
#define RHO_BENCH_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* The version of the report shape this harness writes. `rho bench` refuses a
 * report in a shape it does not read, and says which version it wanted, rather
 * than reading half of it into zeroes that look like measurements. */
#define RHO_BENCH_VERSION 1

/* The markers that delimit the report in a program's output. A firmware that also
 * logs to the same console is the normal case, so the report is delimited rather
 * than assumed to be the whole of what was printed. */
#define RHO_BENCH_BEGIN "@rho-bench:"
#define RHO_BENCH_END   "@rho-bench-end"

/* The run's position: how many warm-up and timed iterations are done.
 *
 * Kept in the state rather than in the loop's own scope because the closing macro
 * runs after the loop, and a counter declared in a `for`'s initialiser does not
 * survive to be closed. Both phases are counted in the same place so that a run
 * cannot end in one believing it is in the other. */
typedef struct {
    uint32_t warm;   /* warm-up runs started */
    uint32_t timed;  /* timed runs started */
} rho_bench_iter_t;

/* What to report, which is the state's whole content. It is public because the
 * macros below are the API and the macros are macros — a benchmark never names this
 * type, but a harness driver that is not using the macros needs it. */
typedef struct {
    const char *name;          /* the registered name, or NULL when anonymous */
    const char *unit;          /* "cycles", "ns", "us" — what the samples are in */
    const char *clock;         /* "dwt", "rdtsc", "chrono" — where they came from */
    uint32_t    iterations;    /* timed runs asked for */
    uint32_t    measured;      /* timed runs actually recorded */
    uint32_t    warmup;        /* runs discarded first */
    uint32_t    capacity;      /* samples this can hold */
    uint64_t   *samples;       /* wall-clock nanoseconds per run */
    uint64_t   *cycles;        /* core cycle counts per run */
    int64_t     stack_bytes;   /* most stack used, negative when not measured */
    int         stack_measured;
    uint64_t    heap_peak;     /* most heap live at once, when tracked */
    uint64_t    heap_live;     /* heap still live at the end */
    uint64_t    heap_allocs;   /* allocations made */
    uint32_t    index;         /* for a benchmark that must not fold; see below */
    /* The reads that bracket an iteration, kept here so that `begin` and `end` can
     * be real functions — which is what lets the body sit between them in the
     * source rather than inside a macro. */
    uint64_t    started_cycles;
    uint64_t    started_nanos;
    /* The run's position, kept here so the closing macro can reach it. */
    rho_bench_iter_t iter;
    /* Whether the iteration now running is a timed one, decided by `more` so that
     * `end` does not have to infer it from a counter that says the same thing in
     * both phases' first iteration. */
    int timing_this;
    /* The stack fill state, kept here so that filling the stack is something the
     * harness does once rather than something every benchmark does. */
    uintptr_t  *stack_base;    /* the bottom of the stack, when known */
    size_t      stack_span;    /* how far it can be filled */
} rho_bench_t;

/* -------------------------------------------------------------------------- */
/*  The clock                                                                  */
/* -------------------------------------------------------------------------- */

#if defined(__arm__) || defined(__thumb__)
/* Cortex-M: the Data Watchpoint and Trace unit counts core cycles in a register,
 * and reading it is two instructions. This is the only clock on a microcontroller
 * that costs nothing to read, which is what makes zero-overhead measurement
 * possible at all — a cycle counter read through a peripheral would cost more than
 * the code being measured.
 *
 * The unit has to be enabled first, which needs the cycle count register's own
 * address and the trace macrocell's power-on bit. It is done once, lazily, because
 * a benchmark that is never run should not spend the power. */
#define RHO_BENCH_HAS_DWT 1

extern volatile uint32_t *rho_bench_dwt_cyccnt;

static inline void rho_bench_dwt_enable(void)
{
    /* Core debug and the trace macrocell are off after reset on a Cortex-M, and
     * reading CYCCNT before they are on returns zero forever rather than failing. */
    CoreDebug->DEMCR |= CoreDebug_DEMCR_TRCENA_Msk;
    DWT->LAR = 0xC5ACCE55u;   /* the unlock key, or the register ignores writes */
    DWT->CYCCNT = 0u;
    DWT->CTRL |= DWT_CTRL_CYCCNTENA_Msk;
}

static inline uint32_t rho_bench_cycles(void)
{
    return DWT->CYCCNT;
}

#define RHO_BENCH_CLOCK_NAME "dwt"

#elif defined(__x86_64__) || defined(__i386__)
/* A host: the timestamp counter, which counts reference cycles rather than core
 * cycles. Slower per instruction than the DWT and a different unit, which is why a
 * host figure and a target figure are never compared directly — see
 * `rho bench`, which reports the clock each number came from. */
#define RHO_BENCH_HAS_RDTSC 1

static inline uint64_t rho_bench_cycles(void)
{
    uint32_t low, high;
    __asm__ __volatile__("rdtsc" : "=a"(low), "=d"(high));
    return ((uint64_t)high << 32) | low;
}

#define RHO_BENCH_CLOCK_NAME "rdtsc"

#else
/* Nothing to count with. The harness still runs and still measures time, because a
 * wall-clock number on an unknown target is better than no number, and it says so
 * through the clock name rather than pretending to have counted anything. */
static inline uint64_t rho_bench_cycles(void) { return 0; }
#define RHO_BENCH_CLOCK_NAME "none"
#endif

/* -------------------------------------------------------------------------- */
/*  The stack watermark                                                        */
/* -------------------------------------------------------------------------- */

/* The byte a filled stack holds.
 *
 * Any value works, and this one is not zero, not `0xFF`, and not a multiple of the
 * word size or of a plausible pointer — because the scan that undoes it looks for
 * the boundary by looking for the first byte that is not the pattern, and a pattern
 * that a legitimate local variable happens to equal would make the watermark read
 * low. `0xAA` is the conventional choice and none of the bad properties above. */
#define RHO_BENCH_STACK_PATTERN 0xAAu

/* Fill the stack and remember where the fill started.
 *
 * Filling is not "paint the whole stack and see what changed": the whole stack is
 * bounded by a linker symbol, and on a part whose stack is shared with an
 * interrupt's frame there is no such symbol at all. So the fill is done from the
 * current frame downward, a bounded amount, and the scan walks back up from the
 * low end of what was filled.
 *
 * The bound is a parameter rather than a constant because how deep firmware calls
 * go is a fact about the firmware. Filling past the end of the real stack would
 * corrupt whatever is below it, which on a part with no stack limit is the rest of
 * RAM. */
static inline void rho_bench_fill_stack(rho_bench_t *b, void *base, size_t span)
{
    b->stack_base = (uintptr_t *)base;
    b->stack_span = span;
    b->stack_bytes = -1;
    b->stack_measured = 0;
    if (base == NULL || span == 0) {
        return;
    }
    volatile uint8_t *bytes = (volatile uint8_t *)base;
    for (size_t i = 0; i < span; i++) {
        bytes[i] = RHO_BENCH_STACK_PATTERN;
    }
}

/* The deepest the stack went, by finding where the fill stopped.
 *
 * The stack grows down, so the fill starts at the top and the used part is where
 * the pattern first stops being there. Scanning from the low end of what was filled
 * is what makes this exact: the first byte that is still the pattern is one below
 * the deepest thing that was written.
 *
 * The result is a claim only when there was something to find. A stack that filled
 * completely has been overrun, and the highest unfilled byte not existing says "at
 * least this much" rather than "this much" — which is reported as not measured,
 * because reporting a number that understates the truth is worse than reporting
 * nothing. */
static inline int64_t rho_bench_stack_used(rho_bench_t *b)
{
    volatile uint8_t *bytes = (volatile uint8_t *)b->stack_base;
    if (bytes == NULL || b->stack_span == 0) {
        b->stack_bytes = -1;
        b->stack_measured = 0;
        return -1;
    }
    size_t i = 0;
    while (i < b->stack_span && bytes[i] == RHO_BENCH_STACK_PATTERN) {
        i++;
    }
    if (i == 0) {
        /* Nothing was left unfilled, so the stack was used past the region that
         * was filled. The number would be a lower bound and is not reported. */
        b->stack_bytes = -1;
        b->stack_measured = 0;
        return -1;
    }
    b->stack_bytes = (int64_t)(b->stack_span - i);
    b->stack_measured = 1;
    return b->stack_bytes;
}

/* -------------------------------------------------------------------------- */
/*  Benchmarking                                                               */
/* -------------------------------------------------------------------------- */

/* Whether there is another iteration to run, advancing to it if so.
 *
 * One call does both, because a `for` whose condition is the only place the counter
 * moves runs the body once: the advance has to happen *before* the body and the test
 * *has* to happen at the same time, or the loop either never advances or advances
 * after a test it has already passed.
 *
 * The warm-up phase runs first and its results are discarded, because the first call
 * into a function is not the steady state: the instruction cache is cold, the branch
 * predictor has nothing in it, and on a host the code may not be faulted in yet. A
 * benchmark that measured those would report the cost of starting up as the cost of
 * the work — which, on a host where faulting in a cache line costs more than the
 * function does, is the whole of the number. */
int rho_bench_more(rho_bench_t *state);

/* Start and stop an iteration's measurement. `end` records only a timed iteration,
 * which is what discards the warm-up.
 *
 * Both cycles and time, because either alone answers half the question. Cycles are
 * what a cycle budget is written against and cost almost nothing to read on a
 * Cortex-M; time is what a latency promise is written against, and needs a clock
 * frequency to turn cycles into anything a person can read. */
void rho_bench_begin(rho_bench_t *state);
void rho_bench_end(rho_bench_t *state);

/* Whether there is another iteration to run, having advanced the counters. */
int rho_bench_more(rho_bench_t *state);

/* Run a body `warmup + iterations` times, measuring the last `iterations` of them.
 *
 *     void bench_crc32(rho_bench_t *state) {
 *         uint8_t data[64] = {0};
 *         RHO_BENCH_LOOP(state) {
 *             crc32_like(data, sizeof(data));
 *         }
 *     }
 *
 * `more` is the loop's condition and `end` its increment, which is what puts the
 * three steps in the right order: advance and test before the body, read the clock
 * as the body starts, and record as it finishes. The closing is implicit — there is
 * no second macro — because a closing one would have to expand to something that
 * runs *inside* the `if` the body's brace closes, and a brace is already closed by
 * the time anything after it is expanded.
 *
 * `END` without a matching `LOOP` is not possible, which is the right way for a
 * benchmark to be broken. */
#define RHO_BENCH_LOOP(state)                                                      \
    for (; rho_bench_more(state); rho_bench_end(state))                            \
        if (rho_bench_begin(state), 1)

/* A benchmark whose unit is not one call: the body decides what to run and how to
 * count, and the harness only brackets it.
 *
 *     RHO_BENCH_CUSTOM(state) {
 *         for (int i = 0; i < 1024; i++) work(i);
 *     }
 *
 * The same loop with the count left to the body, which is what a benchmark over a
 * data structure — where one operation is a whole traversal — needs. */
#define RHO_BENCH_CUSTOM(state) RHO_BENCH_LOOP(state)

/* -------------------------------------------------------------------------- */
/*  The heap                                                                   */
/* -------------------------------------------------------------------------- */

/* Record a live allocation, and keep the peak.
 *
 * The harness tracks the heap rather than the benchmark because a benchmark that
 * counted its own would have to be right about it: forgetting one subtract makes the
 * live count drift downwards and the peak look plausible, which is the failure a
 * heap report has to be able to catch.
 *
 * Wrapping the allocator, rather than replacing it, is what makes this work for a
 * firmware whose allocator is its own — the calls are the same two, in the same
 * places, whatever they do. */
void rho_bench_heap_grow(rho_bench_t *state, size_t bytes);
void rho_bench_heap_shrink(rho_bench_t *state, size_t bytes);

/* Report an allocation and a free, in the shape a wrapped allocator would. */
#define RHO_BENCH_HEAP_ALLOC(state, pointer, bytes)                               \
    do {                                                                           \
        if ((pointer) != 0) {                                                      \
            rho_bench_heap_grow((state), (size_t)(bytes));                         \
        }                                                                          \
    } while (0)

#define RHO_BENCH_HEAP_FREE(state, pointer, bytes)                                \
    do {                                                                           \
        if ((pointer) != 0) {                                                      \
            rho_bench_heap_shrink((state), (size_t)(bytes));                       \
        }                                                                          \
    } while (0)

/* A monotonic clock, in nanoseconds.
 *
 * `CLOCK_MONOTONIC` rather than the wall clock, because the wall clock can step
 * backwards — a time correction mid-run — and a benchmark that measured a negative
 * interval would report it as an enormous one. */
uint64_t rho_bench_nanos(void);

/* -------------------------------------------------------------------------- */
/*  A suite                                                                   */
/* -------------------------------------------------------------------------- */

/* One benchmark, and how to run it. */
typedef struct {
    const char *name;   /* as registered, and as `rho bench --filter` matches it */
    const char *unit;   /* "ns", "cycles" — what its samples are in */
    void (*run)(rho_bench_t *state);
} rho_bench_entry_t;

/* How many benchmarks a suite holds by default.
 *
 * An array rather than a linked list built at run time, because a firmware that
 * needs `malloc` to start measuring is a firmware whose measurement perturbs what it
 * measures — and because a suite a person wrote out is a suite whose order they can
 * see. Overriding it costs one definition. */
#ifndef RHO_BENCH_MAX
#define RHO_BENCH_MAX 16
#endif

/* A suite: the benchmarks, in the order they will run.
 *
 * Registration is an explicit list rather than a macro that builds a table during
 * static initialisation, for two reasons. A static initialiser cannot call a
 * function, so the alternatives are either a compiler extension or a constructor —
 * and a constructor's order among others is not defined, which would make the order
 * a benchmark runs in differ between two builds of the same source. A list is
 * written in the order it runs, by whoever wrote it. */
typedef struct {
    rho_bench_entry_t entries[RHO_BENCH_MAX];
    int count;
} rho_bench_suite_t;

/* Add a benchmark to a suite. Returns 0, or -1 when the suite is full.
 *
 * -1 rather than silently dropping: a benchmark nobody measured is a benchmark
 * absent from the report for a reason nobody can see, which is the same failure as a
 * test that did not run. */
int rho_bench_add(rho_bench_suite_t *suite, const char *name,
                  void (*run)(rho_bench_t *state), const char *unit);

/* Add a benchmark whose unit is nanoseconds, which is nearly all of them. */
#define RHO_BENCH_ADD(suite, fn) rho_bench_add((suite), #fn, (fn), "ns")

/* Add a benchmark measured in something other than nanoseconds. */
#define RHO_BENCH_ADD_UNIT(suite, fn, unit) rho_bench_add((suite), #fn, (fn), (unit))

/* -------------------------------------------------------------------------- */
/*  The harness                                                                */
/* -------------------------------------------------------------------------- */

/* Where the stack is, when the project can say.
 *
 * A linker symbol is the only honest answer, and on a part whose stack is shared
 * with an interrupt's frame there is no such symbol — which is why this is a
 * parameter and not something the harness works out. A harness that guessed would
 * fill memory that is not a stack, and the first thing to notice would be an
 * unrelated variable changing its value. */
extern uint8_t  rho_bench_stack_bottom[];
extern uint8_t  rho_bench_stack_top[];

/* Prepare a benchmark to be run. */
void rho_bench_prepare(rho_bench_t *state, const char *unit, uint32_t iterations,
                       uint32_t warmup, uint64_t *samples, uint32_t capacity,
                       uint64_t *cycles);

/* Run one benchmark and record its figures in the state. Returns 0, or 1 when the
 * state has no room for another run's samples. */
int rho_bench_run(rho_bench_t *state, rho_bench_entry_t *entry);

/* Run every benchmark in a suite and print one report covering all of them.
 *
 * One document rather than one per benchmark: a benchmark that printed its own would
 * have to know how to merge, and a firmware that printed two JSON documents would be
 * read as one broken one. Returns the exit code a harness program should use. */
int rho_bench_run_all(rho_bench_t *state, rho_bench_suite_t *suite);

#ifdef __cplusplus
}
#endif

#endif /* RHO_BENCH_H */