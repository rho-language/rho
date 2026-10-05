/*
 * strings.c — the string and integer intrinsics.
 *
 * Every function here is a hand-rolled loop rather than a call into the C
 * library, and the reason is uniform: `snprintf("%u", n)` is `vfprintf`, and
 * `vfprintf` is the whole formatted-output machinery that print.c exists to keep
 * out of a firmware's flash. On a program that prints one integer per second,
 * that is 8 kB of flash bought by four digits.
 *
 * The platform made the same choice for the same reason: its fault printer
 * formats hex by hand into a stack buffer rather than calling
 * `printf("%08x")`. See `stm32_fault.c`.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

char* rho_concat(const char* a, const char* b)
{
    size_t len_a = strlen(a);
    size_t len_b = strlen(b);
    char* out = (char*)malloc(len_a + len_b + 1);
    if (out == NULL) {
        return NULL;
    }
    memcpy(out, a, len_a);
    memcpy(out + len_a, b, len_b + 1);
    return out;
}

char* rho_string_from_i32(int32_t value)
{
    /* Signed, because this is the one a program prints a number with, and the
     * magnitude is taken in a wider type so that the most negative value does not
     * overflow on the way to being positive. */
    char buffer[12];
    char* at = buffer + sizeof(buffer);
    uint32_t magnitude;
    *--at = 0;
    if (value < 0) {
        magnitude = (uint32_t)(0u - (uint32_t)value);
        *--at = '-';
    } else {
        magnitude = (uint32_t)value;
    }
    do {
        *--at = (char)('0' + (magnitude % 10u));
        magnitude /= 10u;
    } while (magnitude != 0);
    size_t len = (size_t)(buffer + sizeof(buffer) - at);
    char* out = (char*)malloc(len + 1);
    if (out == NULL) {
        return "";
    }
    memcpy(out, at, len);
    out[len] = 0;
    return out;
}

char* rho_str_from_byte(uint8_t b)
{
    char* out = (char*)malloc(2);
    if (out == NULL) {
        return "";
    }
    out[0] = (char)b;
    out[1] = 0;
    return out;
}

uint32_t rho_int_from_str(const char* s)
{
    if (s == NULL || *s == 0) {
        return 0;
    }
    uint32_t total = 0;
    for (const char* p = s; *p != 0; p++) {
        if (*p < '0' || *p > '9') {
            return 0;
        }
        total = total * 10 + (uint32_t)(*p - '0');
    }
    return total;
}

uint32_t rho_strlen(const char* s)
{
    return s != NULL ? (uint32_t)strlen(s) : 0;
}

bool rho_str_eq(const char* a, const char* b)
{
    return a != NULL && b != NULL && strcmp(a, b) == 0;
}

bool rho_str_starts_with(const char* s, const char* prefix)
{
    if (s == NULL || prefix == NULL) {
        return false;
    }
    size_t n = strlen(prefix);
    return strncmp(s, prefix, n) == 0;
}

const char* rho_str_trim_prefix(const char* s, const char* prefix)
{
    if (s == NULL || prefix == NULL) {
        return s;
    }
    size_t n = strlen(prefix);
    return strncmp(s, prefix, n) == 0 ? s + n : s;
}

bool rho_str_contains(const char* haystack, const char* needle)
{
    if (haystack == NULL || needle == NULL) {
        return false;
    }
    return strstr(haystack, needle) != NULL;
}

uint8_t rho_str_at(const char* s, uint32_t i)
{
    if (s == NULL) {
        return 0;
    }
    size_t n = strlen(s);
    return i < n ? (uint8_t)s[i] : 0;
}

int32_t rho_str_index_of(const char* haystack, const char* needle)
{
    if (haystack == NULL || needle == NULL) {
        return -1;
    }
    const char* found = strstr(haystack, needle);
    return found != NULL ? (int32_t)(found - haystack) : -1;
}

char* rho_str_slice(const char* s, uint32_t start, uint32_t length)
{
    if (s == NULL) {
        return "";
    }
    size_t n = strlen(s);
    if (start >= n) {
        return "";
    }
    size_t avail = n - start;
    size_t take = length < avail ? length : avail;
    char* out = (char*)malloc(take + 1);
    if (out == NULL) {
        return "";
    }
    memcpy(out, s + start, take);
    out[take] = 0;
    return out;
}