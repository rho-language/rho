/*
 * checks.c — how a checked operation stops the program.
 *
 * The checks themselves are inline, beside the program (checks.h); what they
 * call when one fails is here, once. The message is the one the Go build gives,
 * so a program stops with the same words on both backends.
 *
 * The number is formatted by hand: this is the path a firmware takes when it has
 * gone wrong, and it should not be the reason the image carries printf.
 *
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

static char* rho_checks_decimal(char* end, int64_t value)
{
    uint64_t magnitude = value < 0 ? (uint64_t)0 - (uint64_t)value : (uint64_t)value;
    do {
        *--end = (char)('0' + (int)(magnitude % 10u));
        magnitude /= 10u;
    } while (magnitude != 0u);
    if (value < 0) {
        *--end = '-';
    }
    return end;
}

static char* rho_checks_append(char* out, const char* text)
{
    while (*text != '\0') {
        *out++ = *text++;
    }
    return out;
}

void rho_panic_index(int64_t i, int64_t n)
{
    char message[96];
    char digits[24];
    char* out = message;
    out = rho_checks_append(out, "index ");
    digits[23] = '\0';
    out = rho_checks_append(out, rho_checks_decimal(&digits[23], i));
    out = rho_checks_append(out, " out of range [0,");
    out = rho_checks_append(out, rho_checks_decimal(&digits[23], n));
    out = rho_checks_append(out, ")");
    *out = '\0';
    panicf(message);
    for (;;) {
    }
}

void rho_panic_div(void)
{
    panicf("division by zero");
    for (;;) {
    }
}
