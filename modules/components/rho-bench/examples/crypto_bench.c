/*
 * crypto_bench.c — an example benchmark suite, and the shape of every one.
 *
 * Three functions, three different things worth measuring: a fixed amount of work
 * repeated, work whose size grows, and work that allocates. A benchmark that only
 * does the first tells you one thing about a program.
 *
 * Built and run on the host:
 *
 *     cc -O2 -I../../include crypto_bench.c ../../src/rhobench.c -o crypto_bench
 *     rho bench --binary ./crypto_bench
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include "rho/bench.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* Enough iterations to make the clock's own resolution irrelevant, and few enough
 * that a suite runs in a second. On a host a clock tick is 20–30 ns, so a
 * sub-microsecond function measured once is a measurement of `clock_gettime`. */
#define BENCH_ITERATIONS 20000
#define BENCH_WARMUP     100

/* One iteration: a multiply-and-xor over a fixed buffer. Not a real algorithm and
 * not meant to be one — what it is for is being the same work every time, so that
 * a change in the number is a change in the code. */
static uint32_t crc32_like(const uint8_t *data, size_t length)
{
    uint32_t value = 0xFFFFFFFFu;
    for (size_t i = 0; i < length; i++) {
        value ^= data[i];
        for (int bit = 0; bit < 8; bit++) {
            value = (value >> 1) ^ (0xEDB88320u & (uint32_t)(-(int32_t)(value & 1u)));
        }
    }
    return value;
}

/* Fixed work, repeated. The baseline every other benchmark is read against. */
void bench_crc32(rho_bench_t *state)
{
    uint8_t data[64] = {0};
    data[0] = 0xA5;

    RHO_BENCH_LOOP(state) {
        /* The result goes back into the state, so the compiler cannot remove the
         * call: a benchmark whose subject was optimised away measures the loop and
         * reports it as the code. */
        state->index ^= crc32_like(data, sizeof(data));
    }
}

/* Work that grows.
 *
 * The reason a mean is not enough: the per-call cost here goes up with the buffer,
 * so one figure would describe an average that no call ever had. One iteration is
 * therefore a whole sweep over four sizes, so the reported number is the cost of
 * that sweep — and each iteration gets its own slot rather than four sizes sharing
 * one, which would have left only the last size in the report.
 */
void bench_crc32_growing(rho_bench_t *state)
{
    uint8_t data[256] = {0};

    RHO_BENCH_CUSTOM(state) {
        for (uint32_t size = 16; size <= sizeof(data); size *= 4) {
            state->index ^= crc32_like(data, size);
        }
    }
}

/* A benchmark that touches the heap, so that the dynamic figures have something to
 * report. The peak is what a project budgets against; the live count at the end is
 * what a leak looks like. */
void bench_allocate_and_free(rho_bench_t *state)
{
    /* A small ring, so the peak is bounded and the numbers mean something. */
    enum { SLOTS = 16, SLOT_BYTES = 256 };
    void *held[SLOTS] = {0};

    RHO_BENCH_LOOP(state) {
        /* The slot comes from the iteration counter, which the compiler cannot see
         * past the clock read — so the free/malloc pair is a different one each
         * time and the allocator's fast path is not all that is measured. */
        const int slot = (int)(state->index % SLOTS);
        if (held[slot] != 0) {
            free(held[slot]);
            RHO_BENCH_HEAP_FREE(state, held[slot], SLOT_BYTES);
            held[slot] = 0;
        }
        held[slot] = malloc(SLOT_BYTES);
        RHO_BENCH_HEAP_ALLOC(state, held[slot], SLOT_BYTES);
    }

    /* Freed before the next benchmark runs, or one benchmark's heap would show up
     * as the next one's. */
    for (int i = 0; i < SLOTS; i++) {
        if (held[i] != 0) {
            free(held[i]);
            RHO_BENCH_HEAP_FREE(state, held[i], SLOT_BYTES);
        }
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

    /* The suite, in the order it runs. Written out rather than collected by a
     * macro at static-initialisation time, because that order is what the report
     * lists them in and a constructor's order is not defined. */
    rho_bench_suite_t suite;
    memset(&suite, 0, sizeof(suite));
    if (rho_bench_add(&suite, "bench_crc32", bench_crc32, "ns") != 0 ||
        rho_bench_add(&suite, "bench_crc32_growing", bench_crc32_growing, "ns") != 0 ||
        rho_bench_add(&suite, "bench_allocate_and_free", bench_allocate_and_free, "ns") != 0) {
        fprintf(stderr, "the suite is full; RHO_BENCH_MAX is %d\n", RHO_BENCH_MAX);
        return 1;
    }

    return rho_bench_run_all(&state, &suite);
}