/*
 * panic.c — what a panic costs.
 *
 * A message and a stop, and nothing else.
 *
 * `fprintf(stderr, "rho: %s\n", msg)` is how a panic reaches a person, and it is
 * also how stdio reaches the program: every `fprintf` is a `vfprintf`, and that
 * pulls the conversion tables, the buffer package, a stream and four locks into an
 * image that otherwise needs none of them. On a firmware that is the largest single
 * thing in the binary, and it is there for a path almost no program takes.
 *
 * So the message goes out the same way every other message does, and the program
 * stops where a debugger can find it: a breakpoint instruction on Cortex-M, where a
 * debugger attached stops with the call stack, and `wfi` with none attached, which
 * parks the core instead of running off the end of memory.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

static void panicf(const char* msg)
{
    rho_print(msg != NULL ? msg : "rho: panic\n");
#if defined(__arm__) || defined(__thumb__)
    for (;;) {
        __asm__ volatile ("bkpt #0");
        __asm__ volatile ("wfi");
    }
#elif defined(__riscv)
    for (;;) {
        __asm__ volatile ("ebreak");
        __asm__ volatile ("wfi");
    }
#else
    /* A hosted program has no debugger to stop for, and a panic that never returns
     * is a hung test rather than a failure. So the one place this runtime still
     * uses stdio is the one place a hosted build can afford it. */
    fflush(stdout);
    exit(1);
#endif
}