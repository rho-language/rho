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
 * SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
 */

static bool rho_text_space(char c)
{
    return c == ' ' || c == '\t' || c == '\r' || c == '\n';
}

/* Every string returned here is the arena's (arena.c). */
static char* rho_text_copy(const char* s, size_t n)
{
    char* out = rho_str_alloc(n + 1);
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
    char* out = rho_str_alloc(n * 3 + 1);
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
    char* out = rho_str_alloc(n + 1);
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

char* rho_i64_to_str(int64_t value)
{
    char buf[24];
    size_t j = sizeof(buf);
    buf[--j] = 0;
    uint64_t magnitude = value < 0 ? (uint64_t)0 - (uint64_t)value : (uint64_t)value;
    do {
        buf[--j] = (char)('0' + magnitude % 10);
        magnitude /= 10;
    } while (magnitude != 0);
    if (value < 0) {
        buf[--j] = '-';
    }
    return rho_text_copy(buf + j, sizeof(buf) - 1 - j);
}

/* A whole number with an optional sign and the space around it trimmed; 0 for
 * anything else, or one that does not fit. */
int64_t rho_i64_from_str(const char* text)
{
    if (text == NULL) {
        return 0;
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
    if (start == end || end - start > 18) {
        return 0;
    }
    int64_t value = 0;
    for (size_t i = start; i < end; i++) {
        if (text[i] < '0' || text[i] > '9') {
            return 0;
        }
        value = value * 10 + (text[i] - '0');
    }
    return negative ? -value : value;
}

static char* rho_text_case(const char* s, bool upper)
{
    if (s == NULL) {
        return "";
    }
    size_t n = strlen(s);
    char* out = rho_text_copy(s, n);
    for (size_t i = 0; i < n && out[0] != 0; i++) {
        if (upper && out[i] >= 'a' && out[i] <= 'z') {
            out[i] = (char)(out[i] - 32);
        } else if (!upper && out[i] >= 'A' && out[i] <= 'Z') {
            out[i] = (char)(out[i] + 32);
        }
    }
    return out;
}

char* rho_str_lower(const char* s) { return rho_text_case(s, false); }
char* rho_str_upper(const char* s) { return rho_text_case(s, true); }

/* The field at `index` (from 0) of a text split on `sep`, or "" past the last. */
char* rho_str_field(const char* s, const char* sep, uint32_t index)
{
    if (s == NULL || sep == NULL || sep[0] == 0) {
        return "";
    }
    size_t step = strlen(sep);
    const char* at = s;
    for (uint32_t i = 0; i < index; i++) {
        const char* next = strstr(at, sep);
        if (next == NULL) {
            return "";
        }
        at = next + step;
    }
    const char* end = strstr(at, sep);
    return rho_text_copy(at, end != NULL ? (size_t)(end - at) : strlen(at));
}

/* The text with every occurrence of `from` replaced by `to`; an empty `from`
 * replaces nothing. */
char* rho_str_replace(const char* s, const char* from, const char* to)
{
    if (s == NULL) {
        return "";
    }
    if (from == NULL || from[0] == 0) {
        return rho_text_copy(s, strlen(s));
    }
    if (to == NULL) {
        to = "";
    }
    size_t from_n = strlen(from), to_n = strlen(to), count = 0;
    for (const char* at = strstr(s, from); at != NULL; at = strstr(at + from_n, from)) {
        count++;
    }
    size_t n = strlen(s) + count * to_n - count * from_n;
    char* out = rho_str_alloc(n + 1);
    if (out == NULL) {
        return "";
    }
    char* w = out;
    const char* r = s;
    for (const char* at = strstr(r, from); at != NULL; at = strstr(r, from)) {
        memcpy(w, r, (size_t)(at - r));
        w += at - r;
        memcpy(w, to, to_n);
        w += to_n;
        r = at + from_n;
    }
    strcpy(w, r);
    return out;
}

/* Where `"key"` is followed by a colon, the first character of its value. */
static const char* rho_json_value_at(const char* doc, const char* key)
{
    if (doc == NULL || key == NULL) {
        return NULL;
    }
    size_t klen = strlen(key);
    const char* p = doc;
    while ((p = strchr(p, '"')) != NULL) {
        if (strncmp(p + 1, key, klen) == 0 && p[1 + klen] == '"') {
            const char* q = p + 2 + klen;
            while (*q == ' ' || *q == '\t' || *q == '\r' || *q == '\n') {
                q++;
            }
            if (*q == ':') {
                q++;
                while (*q == ' ' || *q == '\t' || *q == '\r' || *q == '\n') {
                    q++;
                }
                return q;
            }
        }
        p++;
    }
    return NULL;
}

char* rho_json_str_field(const char* doc, const char* key)
{
    const char* q = rho_json_value_at(doc, key);
    if (q == NULL || *q != '"') {
        return "";
    }
    q++;
    size_t n = 0;
    while (q[n] != 0 && q[n] != '"') {
        n += (q[n] == '\\' && q[n + 1] != 0) ? 2 : 1;
    }
    char* out = rho_str_alloc(n + 1);
    if (out == NULL) {
        return "";
    }
    size_t j = 0;
    for (size_t i = 0; i < n; i++) {
        char c = q[i];
        if (c == '\\' && i + 1 < n) {
            char e = q[++i];
            c = e == 'n' ? '\n' : e == 't' ? '\t' : e == 'r' ? '\r' : e;
        }
        out[j++] = c;
    }
    out[j] = 0;
    return out;
}

char* rho_json_num_field(const char* doc, const char* key)
{
    const char* q = rho_json_value_at(doc, key);
    if (q == NULL) {
        return "";
    }
    size_t n = 0;
    while (q[n] == '-' || q[n] == '+' || q[n] == '.' || q[n] == 'e' || q[n] == 'E' || (q[n] >= '0' && q[n] <= '9')) {
        n++;
    }
    return rho_text_copy(q, n);
}

/* Where the application keeps its data: what its supervisor says, else a folder
 * of its own under the user's home. */
char* rho_app_workspace(void)
{
    const char* named = getenv("RHO_APP_WORKSPACE");
    if (named == NULL || named[0] == 0) {
        named = getenv("ROBOHUB_APP_WORKSPACE");
    }
    if (named != NULL && named[0] != 0) {
        return rho_text_copy(named, strlen(named));
    }
    const char* home = getenv("HOME");
    const char* app = getenv("RHO_APP_NAME");
    if (home == NULL) home = ".";
    if (app == NULL || app[0] == 0) app = "app";
    size_t n = strlen(home) + strlen(app) + 16;
    char* out = rho_str_alloc(n);
    if (out == NULL) {
        return "";
    }
    snprintf(out, n, "%s/.rho/apps/%s", home, app);
    return out;
}
