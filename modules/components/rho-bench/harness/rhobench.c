/*
 * rhobench.c — the harness a benchmark is measured by.
 *
 * Everything in here runs inside somebody's firmware. It is the whole of what
 * `rho bench` puts on a target: measure, print, exit. No statistics, no comparison,
 * no report formatting — those happen on a machine with memory, and duplicating them
 * here would be a second implementation of a median that nobody would keep in step
 * with the first.
 *
 * The JSON is written by hand rather than by a library because a firmware has no
 * library, and a benchmark that needed one would not run on the thing it exists to
 * measure. It is therefore deliberately small: numbers, strings and arrays, and a
 * check on every value that could be nonsense.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include "rho/bench.h"

#include <stdio.h>
#include <string.h>
#include <time.h>

/* A firmware with no benchmarks still links: an empty suite reports that it
 * measured nothing rather than failing to build. */
__attribute__((weak)) uint8_t rho_bench_stack_bottom[1];
__attribute__((weak)) uint8_t rho_bench_stack_top[1];

#if defined(RHO_BENCH_HAS_DWT)
volatile uint32_t *rho_bench_dwt_cyccnt = &DWT->CTRL;
static int rho_bench_dwt_ready = 0;
#endif

/* Add a benchmark to a suite. */
int rho_bench_add(rho_bench_suite_t *suite, const char *name,
                  void (*run)(rho_bench_t *state), const char *unit)
{
    if (suite == 0 || suite->count >= RHO_BENCH_MAX) {
        return -1;
    }
    suite->entries[suite->count].name = name;
    suite->entries[suite->count].unit = unit ? unit : "ns";
    suite->entries[suite->count].run = run;
    suite->count++;
    return 0;
}

/* Whether there is another iteration to run, advancing to it if so. */
int rho_bench_more(rho_bench_t *state)
{
    if (state->iter.warm < state->warmup) {
        state->timing_this = 0;
        state->iter.warm++;
        return 1;
    }
    if (state->iter.timed < state->iterations) {
        state->timing_this = 1;
        state->iter.timed++;
        return 1;
    }
    return 0;
}

/* Start an iteration's measurement. */
void rho_bench_begin(rho_bench_t *state)
{
    state->started_cycles = rho_bench_cycles();
    state->started_nanos = rho_bench_nanos();
}

/* Stop an iteration's measurement and keep it, unless it was a warm-up. */
void rho_bench_end(rho_bench_t *state)
{
    const uint64_t cycles = rho_bench_cycles();
    const uint64_t nanos = rho_bench_nanos();

    if (!state->timing_this) {
        /* A warm-up run. Its numbers are the cost of starting up and are not the
         * cost of the work, which is the whole reason this loop has two phases. */
        return;
    }
    state->measured = state->iter.timed;
    if (state->samples != 0) {
        state->samples[state->iter.timed - 1] = nanos - state->started_nanos;
    }
    if (state->cycles != 0) {
        /* Written once per slot, with no comparison against what was there.
         *
         * An earlier version kept the smallest count per slot, to reject a reading
         * an interrupt had spoiled. That is wrong in a way that only shows up with
         * more than one benchmark: every slot already held a value from the run
         * before, so a suite whose first benchmark was the cheapest recorded *its*
         * numbers for all the others. One write per slot, and an iteration that an
         * interrupt stretched shows up in the upper percentiles and the maximum —
         * which is what they are for. */
        state->cycles[state->iter.timed - 1] = cycles - state->started_cycles;
    }
}

/* Record a live allocation, and keep the peak. */
void rho_bench_heap_grow(rho_bench_t *state, size_t bytes)
{
    state->heap_allocs++;
    state->heap_live += bytes;
    if (state->heap_live > state->heap_peak) {
        state->heap_peak = state->heap_live;
    }
}

/* Record a free.
 *
 * Saturating at zero rather than going negative: a benchmark that frees something
 * it did not allocate has a bug, and a negative live count would read as a number
 * and hide it. */
void rho_bench_heap_shrink(rho_bench_t *state, size_t bytes)
{
    state->heap_live = (state->heap_live > bytes) ? (state->heap_live - bytes) : 0;
}

/* Prepare a benchmark to be run. */
void rho_bench_prepare(rho_bench_t *state, const char *unit, uint32_t iterations,
                       uint32_t warmup, uint64_t *samples, uint32_t capacity,
                       uint64_t *cycles)
{
    memset(state, 0, sizeof(*state));
    state->unit = unit ? unit : "cycles";
    state->clock = RHO_BENCH_CLOCK_NAME;
    state->iterations = iterations;
    state->warmup = warmup;
    state->capacity = capacity;
    state->samples = samples;
    state->cycles = cycles;
    state->stack_bytes = -1;
    state->stack_measured = 0;
    state->started_cycles = 0;
    state->started_nanos = 0;
    state->iter.warm = 0;
    state->iter.timed = 0;
    /* Zeroed rather than filled with a sentinel: every timed iteration writes its
     * own slot exactly once, so a slot is never compared against a stale value, and
     * a benchmark that measured fewer iterations than it asked for reports the ones
     * it took rather than reading whatever was in memory. */
    if (samples != 0 && iterations > 0) {
        memset(samples, 0, (size_t)iterations * sizeof(*samples));
    }
    if (cycles != 0 && iterations > 0) {
        memset(cycles, 0, (size_t)iterations * sizeof(*cycles));
    }

#if defined(RHO_BENCH_HAS_DWT)
    if (!rho_bench_dwt_ready) {
        rho_bench_dwt_enable();
        rho_bench_dwt_ready = 1;
    }
#endif
}

/* A monotonic clock, in nanoseconds. Declared in the header because the loop macros
 * call it: a benchmark measures both cycles and time, and the time is what makes a
 * cycle count mean something once the clock frequency is known. */
uint64_t rho_bench_nanos(void)
{
#if defined(CLOCK_MONOTONIC)
    struct timespec now;
    if (clock_gettime(CLOCK_MONOTONIC, &now) == 0) {
        return (uint64_t)now.tv_sec * 1000000000ull + (uint64_t)now.tv_nsec;
    }
#endif
    return 0;
}

/* -------------------------------------------------------------------------- */
/*  The report                                                                 */
/* -------------------------------------------------------------------------- */

static void rho_bench_escape(const char *text)
{
    if (text == 0) {
        return;
    }
    for (const char *at = text; *at; at++) {
        switch (*at) {
        case '"':  fputs("\\\"", stdout); break;
        case '\\': fputs("\\\\", stdout); break;
        case '\n': fputs("\\n", stdout); break;
        case '\r': fputs("\\r", stdout); break;
        case '\t': fputs("\\t", stdout); break;
        default:
            if ((unsigned char)*at < 0x20) {
                printf("\\u%04x", (unsigned char)*at);
            } else {
                fputc(*at, stdout);
            }
        }
    }
}

static void rho_bench_samples(const char *name, const uint64_t *values, uint32_t count)
{
    if (values == 0 || count == 0) {
        return;
    }
    printf("\"%s\":[", name);
    for (uint32_t i = 0; i < count; i++) {
        printf("%s%llu", i ? "," : "", (unsigned long long)values[i]);
    }
    fputs("]", stdout);
}

/* Print one benchmark's figures. */
static void rho_bench_print_one(rho_bench_t *state, rho_bench_entry_t *entry)
{
    printf("{\"name\":\"");
    rho_bench_escape(entry->name);
    printf("\",\"unit\":\"");
    rho_bench_escape(entry->unit ? entry->unit : state->unit);
    printf("\",\"clock\":\"%s\"", state->clock);
    printf(",\"iterations\":%u", (unsigned)state->measured);
    printf(",\"warmup_runs\":%u", (unsigned)state->warmup);
    printf(",");
    rho_bench_samples("samples", state->samples, state->measured);
    printf(",");
    rho_bench_samples("cycles", state->cycles, state->measured);

    printf(",\"stack_bytes\":%lld", (long long)state->stack_bytes);
    if (state->stack_measured) {
        printf(",\"stack_measured\":true");
    }
    if (state->heap_allocs || state->heap_peak) {
        printf(",\"heap_peak_bytes\":%llu", (unsigned long long)state->heap_peak);
        printf(",\"heap_live_bytes\":%llu", (unsigned long long)state->heap_live);
        printf(",\"heap_allocations\":%llu", (unsigned long long)state->heap_allocs);
    }
    fputs("}", stdout);
}

/* Run one benchmark. */
int rho_bench_run(rho_bench_t *state, rho_bench_entry_t *entry)
{
    if (entry == 0 || entry->run == 0) {
        return 1;
    }
    /* The run's position starts over: one state is reused for every benchmark in a
     * suite, and a second that began at the first one's last iteration would
     * measure nothing. */
    state->iter.warm = 0;
    state->iter.timed = 0;
    state->timing_this = 0;
    state->measured = 0;
    /* Cleared between benchmarks: one state is reused for the whole suite, and a
     * second benchmark that inherited the first's samples would report the first's
     * numbers. */
    if (state->samples != 0 && state->iterations > 0) {
        memset(state->samples, 0, (size_t)state->iterations * sizeof(*state->samples));
    }
    if (state->cycles != 0 && state->iterations > 0) {
        memset(state->cycles, 0, (size_t)state->iterations * sizeof(*state->cycles));
    }
    state->heap_peak = 0;
    state->heap_live = 0;
    state->heap_allocs = 0;

    if (state->stack_base != 0 && state->stack_span != 0) {
        rho_bench_fill_stack(state, (void *)state->stack_base, state->stack_span);
    }
    entry->run(state);
    if (state->stack_base != 0 && state->stack_span != 0) {
        rho_bench_stack_used(state);
    }
    return 0;
}

/* Run every benchmark in a suite and print one report.
 *
 * One pass, and one document covering all of them: a firmware that printed a report
 * per benchmark would be read as one broken report, and a firmware that allocated an
 * array of pointers to gather them would be spending the memory this whole exercise
 * is trying to be careful about. */
int rho_bench_run_all(rho_bench_t *state, rho_bench_suite_t *suite)
{
    if (suite == 0 || suite->count == 0) {
        /* A program with no benchmarks says so, rather than printing a report with
         * nothing in it that reads as a measurement of nothing. */
        fputs(RHO_BENCH_BEGIN, stdout);
        printf("{\"version\":%d,\"benchmarks\":[]}", RHO_BENCH_VERSION);
        fputs(RHO_BENCH_END "\n", stdout);
        fflush(stdout);
        return 1;
    }

    fputs(RHO_BENCH_BEGIN, stdout);
    printf("{\"version\":%d,\"suite\":\"", RHO_BENCH_VERSION);
    rho_bench_escape(state->name);
    printf("\",\"benchmarks\":[");
    for (int i = 0; i < suite->count; i++) {
        if (i) {
            fputc(',', stdout);
        }
        rho_bench_run(state, &suite->entries[i]);
        rho_bench_print_one(state, &suite->entries[i]);
    }
    fputs("]}", stdout);
    fputs(RHO_BENCH_END "\n", stdout);
    fflush(stdout);
    return 0;
}
