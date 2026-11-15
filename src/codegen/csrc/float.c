/*
 * float.c — a floating-point number written as decimal text.
 *
 * f64_to_str(x, decimals) writes x in fixed notation with exactly `decimals`
 * digits after the point (at most 20), rounded to nearest from the exact
 * binary value, a tie to the even digit. That is what Go's
 * strconv.FormatFloat(x, 'f', decimals, 64) answers, which is the Go backend's
 * implementation, so the two backends print the same text for every value:
 * "NaN", "+Inf" and "-Inf" for the values that are not numbers, and a sign for
 * a negative value even when it rounds to zero ("-0.00").
 *
 * It is done exactly, with integers, rather than with snprintf: snprintf's
 * decimal point is the locale's, a board's C library may leave floating-point
 * formatting out, and the formatted-output machinery is what print.c exists to
 * keep out of a firmware.
 *
 * A double is m * 2^e with m below 2^53. Written with d decimals it is
 * round(m * 5^d * 2^(e+d)): a product of at most 1100 bits, held here in 32-bit
 * limbs, shifted, rounded, and written in base 10.
 *
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

#define RHO_FLOAT_MAX_DECIMALS 20u
/* 2^1024 * 5^20 is under 1100 bits. */
#define RHO_FLOAT_LIMBS 36

typedef struct {
    uint32_t limb[RHO_FLOAT_LIMBS]; /* least significant first */
    int used;
} rho_float_big;

static void rho_float_big_mul_small(rho_float_big* n, uint32_t factor)
{
    uint64_t carry = 0;
    for (int i = 0; i < n->used; i++) {
        uint64_t product = (uint64_t)n->limb[i] * factor + carry;
        n->limb[i] = (uint32_t)product;
        carry = product >> 32;
    }
    if (carry != 0 && n->used < RHO_FLOAT_LIMBS) {
        n->limb[n->used++] = (uint32_t)carry;
    }
}

static void rho_float_big_shl(rho_float_big* n, int bits)
{
    int words = bits / 32;
    int rest = bits % 32;
    if (n->used == 0) {
        return;
    }
    int top = n->used + words + 1;
    if (top > RHO_FLOAT_LIMBS) {
        top = RHO_FLOAT_LIMBS;
    }
    for (int i = top - 1; i >= 0; i--) {
        uint64_t value = 0;
        int from = i - words;
        if (from >= 0 && from < n->used) {
            value = (uint64_t)n->limb[from] << rest;
        }
        if (rest != 0 && from - 1 >= 0 && from - 1 < n->used) {
            value |= (uint64_t)n->limb[from - 1] >> (32 - rest);
        }
        n->limb[i] = (uint32_t)value;
    }
    n->used = top;
    while (n->used > 0 && n->limb[n->used - 1] == 0) {
        n->used--;
    }
}

/* Divides by `divisor` in place and returns the remainder. */
static uint32_t rho_float_big_div_small(rho_float_big* n, uint32_t divisor)
{
    uint64_t remainder = 0;
    for (int i = n->used - 1; i >= 0; i--) {
        uint64_t current = (remainder << 32) | n->limb[i];
        n->limb[i] = (uint32_t)(current / divisor);
        remainder = current % divisor;
    }
    while (n->used > 0 && n->limb[n->used - 1] == 0) {
        n->used--;
    }
    return (uint32_t)remainder;
}

/* The bit at position `bit`, counting from 0 at the least significant. */
static int rho_float_big_bit(const rho_float_big* n, int bit)
{
    int word = bit / 32;
    if (bit < 0 || word >= n->used) {
        return 0;
    }
    return (int)((n->limb[word] >> (bit % 32)) & 1u);
}

/* Whether any bit below position `bit` is set. */
static int rho_float_big_any_below(const rho_float_big* n, int bit)
{
    for (int i = 0; i < n->used && i * 32 < bit; i++) {
        uint32_t word = n->limb[i];
        int span = bit - i * 32;
        if (span < 32) {
            word &= (1u << span) - 1u;
        }
        if (word != 0) {
            return 1;
        }
    }
    return 0;
}

/* Shifts right by `bits`, rounding to nearest with a tie to even. */
static void rho_float_big_shr_round(rho_float_big* n, int bits)
{
    int half = rho_float_big_bit(n, bits - 1);
    int sticky = rho_float_big_any_below(n, bits - 1);
    int words = bits / 32;
    int rest = bits % 32;
    rho_float_big out = {{0}, 0};
    for (int i = 0; i + words < n->used; i++) {
        uint64_t value = (uint64_t)n->limb[i + words] >> rest;
        if (rest != 0 && i + words + 1 < n->used) {
            value |= (uint64_t)n->limb[i + words + 1] << (32 - rest);
        }
        out.limb[i] = (uint32_t)value;
        out.used = i + 1;
    }
    while (out.used > 0 && out.limb[out.used - 1] == 0) {
        out.used--;
    }
    if (half && (sticky || (out.used > 0 && (out.limb[0] & 1u)))) {
        /* Add one. */
        int i = 0;
        for (; i < out.used; i++) {
            if (++out.limb[i] != 0) {
                break;
            }
        }
        if (i == out.used && out.used < RHO_FLOAT_LIMBS) {
            out.limb[out.used++] = 1;
        }
    }
    *n = out;
}

char* rho_f64_to_str(double x, uint32_t decimals)
{
    uint64_t bits;
    memcpy(&bits, &x, sizeof(bits));
    int negative = (int)(bits >> 63);
    int exponent = (int)((bits >> 52) & 0x7ffu);
    uint64_t mantissa = bits & 0xfffffffffffffull;
    if (exponent == 0x7ff) {
        if (mantissa != 0) {
            return rho_str_dup("NaN");
        }
        return rho_str_dup(negative ? "-Inf" : "+Inf");
    }
    if (decimals > RHO_FLOAT_MAX_DECIMALS) {
        decimals = RHO_FLOAT_MAX_DECIMALS;
    }
    int e;
    if (exponent == 0) {
        e = -1074;
    } else {
        mantissa |= 1ull << 52;
        e = exponent - 1075;
    }
    /* n = m * 5^d */
    rho_float_big n = {{0}, 0};
    n.limb[0] = (uint32_t)mantissa;
    n.limb[1] = (uint32_t)(mantissa >> 32);
    n.used = n.limb[1] != 0 ? 2 : (n.limb[0] != 0 ? 1 : 0);
    for (uint32_t i = 0; i < decimals; i++) {
        rho_float_big_mul_small(&n, 5u);
    }
    int shift = e + (int)decimals;
    if (shift >= 0) {
        rho_float_big_shl(&n, shift);
    } else if (n.used > 0) {
        if (-shift > n.used * 32 + 1) {
            n.used = 0; /* below half of the last digit: rounds to zero */
        } else {
            rho_float_big_shr_round(&n, -shift);
        }
    }
    /* Digits, least significant first, in base 10. */
    char digits[400];
    int count = 0;
    while (n.used > 0 && count < (int)sizeof(digits) - 9) {
        uint32_t chunk = rho_float_big_div_small(&n, 1000000000u);
        for (int k = 0; k < 9; k++) {
            digits[count++] = (char)('0' + chunk % 10u);
            chunk /= 10u;
        }
    }
    while (count > 0 && digits[count - 1] == '0') {
        count--;
    }
    while (count < (int)decimals + 1) {
        digits[count++] = '0';
    }
    char out[440];
    size_t at = 0;
    if (negative) {
        out[at++] = '-';
    }
    for (int i = count - 1; i >= 0; i--) {
        out[at++] = digits[i];
        if (i == (int)decimals && decimals > 0) {
            out[at++] = '.';
        }
    }
    out[at] = 0;
    return rho_str_dup(out);
}

char* rho_f32_to_str(float x, uint32_t decimals)
{
    return rho_f64_to_str((double)x, decimals);
}
