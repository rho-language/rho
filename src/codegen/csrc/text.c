/*
 * text.c — what an application reads from a form and writes to a page.
 *
 * A value without the white space around it, a URL's escapes, one field of an
 * x-www-form-urlencoded body, and a decimal held as thousandths. Every RoboHub
 * module had its own copy of each; these are the one the Go runtime matches
 * byte for byte, which the parity tests hold it to.
 *
 * A decimal is read with a dot or a comma (2,5 is how Romanian writes 2.5),
 * digits past the third decimal are dropped rather than rounded, and a text
 * that is not a number reads as 0 — milli_valid says which it was.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

static bool rho_text_space(char c)
{
    return c == ' ' || c == '\t' || c == '\r' || c == '\n';
}

static char* rho_text_copy(const char* s, size_t n)
{
    char* out = (char*)malloc(n + 1);
    if (out == NULL) {
        return "";
    }
    memcpy(out, s, n);
    out[n] = 0;
    return out;
}

char* rho_str_trim(const char* s)
{
    if (s == NULL) {
        return "";
    }
    size_t start = 0;
    size_t end = strlen(s);
    while (start < end && rho_text_space(s[start])) {
        start++;
    }
    while (end > start && rho_text_space(s[end - 1])) {
        end--;
    }
    return rho_text_copy(s + start, end - start);
}

static int rho_text_hex(char c)
{
    if (c >= '0' && c <= '9') {
        return c - '0';
    }
    if (c >= 'a' && c <= 'f') {
        return c - 'a' + 10;
    }
    if (c >= 'A' && c <= 'F') {
        return c - 'A' + 10;
    }
    return -1;
}

char* rho_str_url_encode(const char* s)
{
    static const char digits[] = "0123456789ABCDEF";
    if (s == NULL) {
        return "";
    }
    size_t n = strlen(s);
    char* out = (char*)malloc(n * 3 + 1);
    if (out == NULL) {
        return "";
    }
    size_t j = 0;
    for (size_t i = 0; i < n; i++) {
        unsigned char c = (unsigned char)s[i];
        if ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
            c == '-' || c == '_' || c == '.' || c == '~') {
            out[j++] = (char)c;
        } else {
            out[j++] = '%';
            out[j++] = digits[c >> 4];
            out[j++] = digits[c & 15];
        }
    }
    out[j] = 0;
    return out;
}

/* A '+' is a space, as a form sends it; a '%' not followed by two hex digits is
 * kept as it is. */
static char* rho_text_decode(const char* s, size_t n)
{
    char* out = (char*)malloc(n + 1);
    if (out == NULL) {
        return "";
    }
    size_t j = 0;
    for (size_t i = 0; i < n; i++) {
        if (s[i] == '+') {
            out[j++] = ' ';
        } else if (s[i] == '%' && i + 2 < n &&
                   rho_text_hex(s[i + 1]) >= 0 && rho_text_hex(s[i + 2]) >= 0) {
            out[j++] = (char)(rho_text_hex(s[i + 1]) * 16 + rho_text_hex(s[i + 2]));
            i += 2;
        } else {
            out[j++] = s[i];
        }
    }
    out[j] = 0;
    return out;
}

char* rho_str_url_decode(const char* s)
{
    if (s == NULL) {
        return "";
    }
    return rho_text_decode(s, strlen(s));
}

/* rho_text_key_is reads an encoded key and compares it with a name, a byte at a
 * time, so looking for a field allocates nothing for the ones it passes. */
static bool rho_text_key_is(const char* s, size_t n, const char* name)
{
    size_t k = 0;
    for (size_t i = 0; i < n; i++) {
        char c = s[i];
        if (c == '+') {
            c = ' ';
        } else if (c == '%' && i + 2 < n && rho_text_hex(s[i + 1]) >= 0 && rho_text_hex(s[i + 2]) >= 0) {
            c = (char)(rho_text_hex(s[i + 1]) * 16 + rho_text_hex(s[i + 2]));
            i += 2;
        }
        if (name[k] == 0 || name[k] != c) {
            return false;
        }
        k++;
    }
    return name[k] == 0;
}

char* rho_form_field(const char* body, const char* name)
{
    if (body == NULL || name == NULL) {
        return "";
    }
    const char* p = body;
    while (*p) {
        const char* end = strchr(p, '&');
        size_t len = end != NULL ? (size_t)(end - p) : strlen(p);
        const char* eq = memchr(p, '=', len);
        size_t key_len = eq != NULL ? (size_t)(eq - p) : len;
        if (rho_text_key_is(p, key_len, name)) {
            if (eq == NULL) {
                return "";
            }
            return rho_text_decode(eq + 1, len - key_len - 1);
        }
        if (end == NULL) {
            break;
        }
        p = end + 1;
    }
    return "";
}

/* rho_text_milli reads a decimal as thousandths; it answers false for anything
 * that is not one: no digits, a second separator, a letter, more than fifteen
 * whole digits. */
static bool rho_text_milli(const char* text, int64_t* out)
{
    *out = 0;
    if (text == NULL) {
        return false;
    }
    size_t start = 0;
    size_t end = strlen(text);
    while (start < end && rho_text_space(text[start])) {
        start++;
    }
    while (end > start && rho_text_space(text[end - 1])) {
        end--;
    }
    bool negative = false;
    if (start < end && (text[start] == '-' || text[start] == '+')) {
        negative = text[start] == '-';
        start++;
    }
    int64_t whole = 0;
    int64_t fraction = 0;
    int whole_digits = 0;
    int fraction_digits = 0;
    bool separator = false;
    for (size_t i = start; i < end; i++) {
        char c = text[i];
        if ((c == '.' || c == ',') && !separator) {
            separator = true;
        } else if (c < '0' || c > '9') {
            return false;
        } else if (separator) {
            if (fraction_digits < 3) {
                fraction = fraction * 10 + (c - '0');
            }
            fraction_digits++;
        } else {
            if (whole_digits >= 15) {
                return false;
            }
            whole = whole * 10 + (c - '0');
            whole_digits++;
        }
    }
    if (whole_digits == 0 && fraction_digits == 0) {
        return false;
    }
    for (int d = fraction_digits; d < 3; d++) {
        fraction *= 10;
    }
    int64_t milli = whole * 1000 + fraction;
    *out = negative ? -milli : milli;
    return true;
}

bool rho_milli_valid(const char* text)
{
    int64_t value;
    return rho_text_milli(text, &value);
}

int64_t rho_milli_parse(const char* text)
{
    int64_t value;
    return rho_text_milli(text, &value) ? value : 0;
}

char* rho_milli_format(int64_t milli)
{
    char buf[32];
    size_t j = sizeof(buf);
    buf[--j] = 0;
    uint64_t value = milli < 0 ? (uint64_t)0 - (uint64_t)milli : (uint64_t)milli;
    uint64_t whole = value / 1000;
    uint32_t fraction = (uint32_t)(value % 1000);
    if (fraction != 0) {
        int places = 3;
        while (fraction % 10 == 0) {
            fraction /= 10;
            places--;
        }
        for (int d = 0; d < places; d++) {
            buf[--j] = (char)('0' + fraction % 10);
            fraction /= 10;
        }
        buf[--j] = '.';
    }
    do {
        buf[--j] = (char)('0' + whole % 10);
        whole /= 10;
    } while (whole != 0);
    if (milli < 0) {
        buf[--j] = '-';
    }
    return rho_text_copy(buf + j, sizeof(buf) - 1 - j);
}

/* rho_milli_round keeps `places` decimals of thousandths, the half away from
 * zero: 2.345 to two places is 2.35, -2.345 is -2.35. Three places or more
 * leaves the value as it is. */
int64_t rho_milli_round(int64_t milli, uint32_t places)
{
    if (places >= 3) {
        return milli;
    }
    int64_t step = places == 0 ? 1000 : places == 1 ? 100 : 10;
    int64_t magnitude = milli < 0 ? -milli : milli;
    int64_t rounded = (magnitude + step / 2) / step * step;
    return milli < 0 ? -rounded : rounded;
}
