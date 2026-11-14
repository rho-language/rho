/*
 * checks.h — integer operations with one meaning on every backend.
 *
 * C leaves signed overflow, an over-wide shift, a division by zero and an index
 * past the end undefined. Rho defines them, and these are the definitions: the
 * program calls them where it would have used the operator.
 *
 * They are `static inline` and emitted beside the program rather than in the
 * runtime's translation unit, so a compiler sees each one at its call: with a
 * constant operand in range the check folds away and what is left is the
 * operator. Only the two ways of stopping are out of line, in checks.c.
 *
 * Arithmetic is done in an unsigned type at least 32 bits wide and converted
 * back. Unsigned arithmetic wraps by definition, so nothing here relies on
 * -fwrapv, and a narrow operand is not promoted to an `int` that a product can
 * overflow. Converting the result back to a signed type is modular on every
 * compiler Rho targets (GCC, Clang, avr-gcc, arm-none-eabi-gcc).
 *
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

#if defined(__GNUC__)
#define RHO_NORETURN __attribute__((noreturn))
#else
#define RHO_NORETURN
#endif

RHO_NORETURN void rho_panic_index(int64_t i, int64_t n);
RHO_NORETURN void rho_panic_div(void);
RHO_NORETURN void rho_panic_slice(int64_t lo, int64_t hi, int64_t n);
RHO_NORETURN void rho_panic_append(int64_t cap);

/* An index into something whose length is known: a fixed array or a Slice. */
static inline int64_t rho_index(int64_t i, int64_t n)
{
    if (i < 0 || i >= n) {
        rho_panic_index(i, n);
    }
    return i;
}

/* A sub-slice s[lo..hi] of something n long: 0 <= lo <= hi <= n, or the
 * program stops. The answer is the address of element lo — or p itself for an
 * empty cut, so no address past the storage is ever formed. rho_subslice names
 * the element type, which is how the address is counted in elements. */
static inline void* rho_subslice_at(void* p, int64_t lo, int64_t hi, int64_t n, size_t size)
{
    if (lo < 0 || lo > hi || hi > n) {
        rho_panic_slice(lo, hi, n);
    }
    return lo == hi ? p : (void*)((char*)p + (size_t)lo * size);
}

#define rho_subslice(T, p, lo, hi, n) ((T*)rho_subslice_at((void*)(p), (lo), (hi), (n), sizeof(T)))

/* The length of s[lo..hi], once rho_subslice has checked the bounds. Counted
 * unsigned, so a bound that has not been checked yet cannot overflow. */
static inline uint32_t rho_slice_len(int64_t lo, int64_t hi)
{
    return (uint32_t)((uint64_t)hi - (uint64_t)lo);
}

/* Where append writes: the slice's length, while it is below the capacity of
 * the fixed array the slice grows into. */
static inline uint32_t rho_append_at(uint32_t len, uint32_t cap)
{
    if (len >= cap) {
        rho_panic_append(cap);
    }
    return len;
}

/* The first element of a [T], which is the pointer it already is in C. */
#define rho_raw_ptr(p) ((void*)(p))

/* An array literal as storage of the block it is written in. */
#define rho_array_ptr(T, ...) ((T[]){__VA_ARGS__})

/* +, -, * and unary -: two's complement, at the type's width. */
#define RHO_WRAPPING(S, T, U)                                                   \
    static inline T rho_add_##S(T a, T b) { return (T)((U)a + (U)b); }          \
    static inline T rho_sub_##S(T a, T b) { return (T)((U)a - (U)b); }          \
    static inline T rho_mul_##S(T a, T b) { return (T)((U)a * (U)b); }          \
    static inline T rho_neg_##S(T a) { return (T)((U)0 - (U)a); }

RHO_WRAPPING(i8, int8_t, uint32_t)
RHO_WRAPPING(i16, int16_t, uint32_t)
RHO_WRAPPING(i32, int32_t, uint32_t)
RHO_WRAPPING(i64, int64_t, uint64_t)
RHO_WRAPPING(u8, uint8_t, uint32_t)
RHO_WRAPPING(u16, uint16_t, uint32_t)

/* A shift by the width of the type or more gives 0; a signed right shift gives
 * the sign. A left shift is done unsigned, so a signed one never overflows, and a
 * signed right shift complements twice, so a negative value is never shifted. */
#define RHO_SHIFTS_UNSIGNED(S, T, U, W)                                          \
    static inline T rho_shl_##S(T x, uint64_t n) { return n >= W ? (T)0 : (T)((U)x << n); } \
    static inline T rho_shr_##S(T x, uint64_t n) { return n >= W ? (T)0 : (T)((U)x >> n); }

#define RHO_SHIFTS_SIGNED(S, T, U, W)                                            \
    static inline T rho_shl_##S(T x, uint64_t n) { return n >= W ? (T)0 : (T)((U)x << n); } \
    static inline T rho_shr_##S(T x, uint64_t n)                                \
    {                                                                           \
        if (n >= W) {                                                           \
            return x < 0 ? (T)-1 : (T)0;                                        \
        }                                                                       \
        return x < 0 ? (T)~(~x >> n) : (T)(x >> n);                             \
    }

RHO_SHIFTS_UNSIGNED(u8, uint8_t, uint32_t, 8u)
RHO_SHIFTS_UNSIGNED(u16, uint16_t, uint32_t, 16u)
RHO_SHIFTS_UNSIGNED(u32, uint32_t, uint32_t, 32u)
RHO_SHIFTS_UNSIGNED(u64, uint64_t, uint64_t, 64u)
RHO_SHIFTS_SIGNED(i8, int8_t, uint32_t, 8u)
RHO_SHIFTS_SIGNED(i16, int16_t, uint32_t, 16u)
RHO_SHIFTS_SIGNED(i32, int32_t, uint32_t, 32u)
RHO_SHIFTS_SIGNED(i64, int64_t, uint64_t, 64u)

/* / and %: a zero divisor stops the program. The most negative value divided
 * by -1 wraps to itself, with a remainder of 0, as Go defines it; C leaves it
 * undefined, and x86 traps on it. */
#define RHO_DIVISION_UNSIGNED(S, T)                                              \
    static inline T rho_div_##S(T a, T b) { if (b == 0) { rho_panic_div(); } return (T)(a / b); } \
    static inline T rho_mod_##S(T a, T b) { if (b == 0) { rho_panic_div(); } return (T)(a % b); }

#define RHO_DIVISION_SIGNED(S, T, U)                                             \
    static inline T rho_div_##S(T a, T b)                                       \
    {                                                                           \
        if (b == 0) {                                                           \
            rho_panic_div();                                                    \
        }                                                                       \
        return b == -1 ? (T)((U)0 - (U)a) : (T)(a / b);                         \
    }                                                                           \
    static inline T rho_mod_##S(T a, T b)                                       \
    {                                                                           \
        if (b == 0) {                                                           \
            rho_panic_div();                                                    \
        }                                                                       \
        return b == -1 ? (T)0 : (T)(a % b);                                     \
    }

RHO_DIVISION_UNSIGNED(u8, uint8_t)
RHO_DIVISION_UNSIGNED(u16, uint16_t)
RHO_DIVISION_UNSIGNED(u32, uint32_t)
RHO_DIVISION_UNSIGNED(u64, uint64_t)
RHO_DIVISION_SIGNED(i8, int8_t, uint32_t)
RHO_DIVISION_SIGNED(i16, int16_t, uint32_t)
RHO_DIVISION_SIGNED(i32, int32_t, uint32_t)
RHO_DIVISION_SIGNED(i64, int64_t, uint64_t)
