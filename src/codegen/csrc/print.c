/*
 * print.c — where a program's output goes.
 *
 * The only thing in the runtime that is not an implementation of a language
 * feature, and the reason it exists separately is that it is the whole of the
 * difference between a Rho program and a C program on a board.
 *
 * `print("x")` used to compile to `printf("%s", "x")`, which is correct and
 * which costs 7–8 kB of flash: `printf` is the C library's formatted output, and
 * formatted output is a conversion table, a buffer package, a stream abstraction
 * and a set of locks around all three. On the `nucleo-h753zi` hello-world that
 * was more than the entire difference between the same program written in Rho and
 * written in C, bought by formatting a string that needed none.
 *
 * So this writes bytes and nothing else. The sink is weak, so a platform with a
 * console of its own replaces it with one call; the default is the write hook the
 * platform already implements.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

/* The output sink. Weak: a platform that has a console of its own defines this,
 * and the default below is not used. Returns the number of bytes taken, or a
 * negative number on failure. */
int rho_write(int fd, const char* data, uint32_t len) __attribute__((weak));

/* The default, when the platform has not written one.
 *
 * `_write` is the hook a newlib retarget provides and what a board's console
 * implements; `write` is the system call of the same name on a hosted build.
 * Naming the right one per target is the difference between a firmware that
 * prints and a firmware that links but prints nothing — which is what a retarget
 * providing only one of them does, and why the platform's own console implements
 * both.
 */
#if defined(__arm__) || defined(__thumb__) || defined(__riscv)
extern int _write(int fd, const char* data, int len);
/* _write has no warn_unused_result on bare-metal targets. */
#define RHO_DEFAULT_WRITE(fd, data, len) _write((fd), (data), (len))
#else
#include <unistd.h>
/* write() is declared __attribute__((warn_unused_result)) in glibc, and
 * (void)write(...) does not suppress it on all GCC versions. A static
 * wrapper that discards the return value once is the portable fix. */
static inline void rho_write_stdout(const char* data, int len)
{
    ssize_t r = write(1, data, (size_t)len);
    (void)r;
}
#define RHO_DEFAULT_WRITE(fd, data, len) rho_write_stdout((data), (len))
#endif

/* Everything `print` says, and nothing else. */
void rho_print(const char* s)
{
    if (s != NULL) {
        uint32_t len = 0;
        while (s[len] != 0) {
            len++;
        }
        if (rho_write != NULL) {
            rho_write(1, s, len);
        } else {
            (void)RHO_DEFAULT_WRITE(1, s, (int)len);
        }
    }
}
/* An integer, printed.
 *
 * This exists because `print(string.from_i32(n))` is the shape every program that
 * prints a number has, and that shape costs a heap allocation to build a string
 * nobody keeps: `rho_string_from_i32` asks `malloc` for four digits, formats into
 * it, hands it to `print`, and the caller throws it away. On a board that is the
 * allocator, the C library's `memcpy`, and a `_sbrk` that grows the heap — brought
 * into an image by four digits.
 *
 * So the compiler recognises the shape and emits this instead. Same output, no
 * allocation, and `rho_string_from_i32` itself leaves the image when nothing else
 * calls it.
 *
 * The digits are built on the stack and written in **one** call. Writing them one
 * at a time is the obvious version and it is 452 bytes rather than 120: a write
 * per digit inlines the platform's whole console path once per digit, and the
 * inlining is what costs the flash. Formatting first and writing once is also
 * faster, which on a console is the difference between a heartbeat that keeps time
 * and one that drifts.
 */
__attribute__((optimize("Os")))
void rho_print_i32(int32_t value)
{
    /* Twelve bytes covers every 32-bit value with a sign, which is all there is. */
    char reversed[12];
    char forward[12];
    uint32_t magnitude;
    int count = 0;
    int at = 0;

    if (value < 0) {
        forward[at++] = '-';
        /* Negated in an unsigned type, because the most negative value has no
         * positive counterpart and negating it in a signed one overflows. */
        magnitude = (uint32_t)(0u - (uint32_t)value);
    } else {
        magnitude = (uint32_t)value;
    }
    do {
        reversed[count++] = (char)('0' + (magnitude % 10u));
        magnitude /= 10u;
    } while (magnitude != 0u);

    while (count > 0) {
        forward[at++] = reversed[--count];
    }
    if (rho_write != NULL) {
        rho_write(1, forward, (uint32_t)at);
    } else {
        (void)RHO_DEFAULT_WRITE(1, forward, (int)at);
    }
}

/* An unsigned integer, printed. Separate because a program counting seconds has an
 * unsigned one, and casting it to signed to print it is a conversion whose result
 * is wrong for every value above half the range. */
__attribute__((optimize("Os")))
void rho_print_u32(uint32_t value)
{
    char reversed[12];
    char forward[12];
    int count = 0;
    int at = 0;

    do {
        reversed[count++] = (char)('0' + (value % 10u));
        value /= 10u;
    } while (value != 0u);

    while (count > 0) {
        forward[at++] = reversed[--count];
    }
    if (rho_write != NULL) {
        rho_write(1, forward, (uint32_t)at);
    } else {
        (void)RHO_DEFAULT_WRITE(1, forward, (int)at);
    }
}
