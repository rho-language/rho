/*
 * crypto.c — HMAC-SHA256 and random bytes, over hex strings.
 *
 * A program that verifies a frame or a token needs a keyed hash, and one that
 * starts a session needs a nonce nobody can predict. Both work on hex text, which
 * is what a C string can carry: a binary value with a zero in it is not a string.
 *
 * SHA-256 is FIPS 180-4, written out here with no allocation and no dependency,
 * so the same code runs on a host and on a part. Random bytes come from the
 * operating system; a target with none answers "" and a caller refuses.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    uint32_t state[8];
    uint64_t length;
    uint8_t block[64];
    size_t used;
} rho_sha256;

static const uint32_t rho_sha256_k[64] = {
    0x428a2f98u, 0x71374491u, 0xb5c0fbcfu, 0xe9b5dba5u, 0x3956c25bu, 0x59f111f1u, 0x923f82a4u, 0xab1c5ed5u,
    0xd807aa98u, 0x12835b01u, 0x243185beu, 0x550c7dc3u, 0x72be5d74u, 0x80deb1feu, 0x9bdc06a7u, 0xc19bf174u,
    0xe49b69c1u, 0xefbe4786u, 0x0fc19dc6u, 0x240ca1ccu, 0x2de92c6fu, 0x4a7484aau, 0x5cb0a9dcu, 0x76f988dau,
    0x983e5152u, 0xa831c66du, 0xb00327c8u, 0xbf597fc7u, 0xc6e00bf3u, 0xd5a79147u, 0x06ca6351u, 0x14292967u,
    0x27b70a85u, 0x2e1b2138u, 0x4d2c6dfcu, 0x53380d13u, 0x650a7354u, 0x766a0abbu, 0x81c2c92eu, 0x92722c85u,
    0xa2bfe8a1u, 0xa81a664bu, 0xc24b8b70u, 0xc76c51a3u, 0xd192e819u, 0xd6990624u, 0xf40e3585u, 0x106aa070u,
    0x19a4c116u, 0x1e376c08u, 0x2748774cu, 0x34b0bcb5u, 0x391c0cb3u, 0x4ed8aa4au, 0x5b9cca4fu, 0x682e6ff3u,
    0x748f82eeu, 0x78a5636fu, 0x84c87814u, 0x8cc70208u, 0x90befffau, 0xa4506cebu, 0xbef9a3f7u, 0xc67178f2u,
};

#define RHO_ROTR(x, n) (((x) >> (n)) | ((x) << (32 - (n))))

static void rho_sha256_compress(rho_sha256* c, const uint8_t* p)
{
    uint32_t w[64];
    for (int i = 0; i < 16; i++) {
        w[i] = ((uint32_t)p[4 * i] << 24) | ((uint32_t)p[4 * i + 1] << 16) |
               ((uint32_t)p[4 * i + 2] << 8) | (uint32_t)p[4 * i + 3];
    }
    for (int i = 16; i < 64; i++) {
        uint32_t s0 = RHO_ROTR(w[i - 15], 7) ^ RHO_ROTR(w[i - 15], 18) ^ (w[i - 15] >> 3);
        uint32_t s1 = RHO_ROTR(w[i - 2], 17) ^ RHO_ROTR(w[i - 2], 19) ^ (w[i - 2] >> 10);
        w[i] = w[i - 16] + s0 + w[i - 7] + s1;
    }
    uint32_t a = c->state[0], b = c->state[1], cc = c->state[2], d = c->state[3];
    uint32_t e = c->state[4], f = c->state[5], g = c->state[6], h = c->state[7];
    for (int i = 0; i < 64; i++) {
        uint32_t t1 = h + (RHO_ROTR(e, 6) ^ RHO_ROTR(e, 11) ^ RHO_ROTR(e, 25)) + ((e & f) ^ (~e & g)) + rho_sha256_k[i] + w[i];
        uint32_t t2 = (RHO_ROTR(a, 2) ^ RHO_ROTR(a, 13) ^ RHO_ROTR(a, 22)) + ((a & b) ^ (a & cc) ^ (b & cc));
        h = g; g = f; f = e; e = d + t1; d = cc; cc = b; b = a; a = t1 + t2;
    }
    c->state[0] += a; c->state[1] += b; c->state[2] += cc; c->state[3] += d;
    c->state[4] += e; c->state[5] += f; c->state[6] += g; c->state[7] += h;
}

static void rho_sha256_init(rho_sha256* c)
{
    static const uint32_t iv[8] = {
        0x6a09e667u, 0xbb67ae85u, 0x3c6ef372u, 0xa54ff53au, 0x510e527fu, 0x9b05688cu, 0x1f83d9abu, 0x5be0cd19u,
    };
    memcpy(c->state, iv, sizeof(iv));
    c->length = 0;
    c->used = 0;
}

static void rho_sha256_update(rho_sha256* c, const uint8_t* data, size_t n)
{
    c->length += (uint64_t)n * 8u;
    while (n > 0) {
        size_t take = 64 - c->used;
        if (take > n) {
            take = n;
        }
        memcpy(c->block + c->used, data, take);
        c->used += take;
        data += take;
        n -= take;
        if (c->used == 64) {
            rho_sha256_compress(c, c->block);
            c->used = 0;
        }
    }
}

static void rho_sha256_final(rho_sha256* c, uint8_t out[32])
{
    uint64_t bits = c->length;
    uint8_t pad = 0x80;
    rho_sha256_update(c, &pad, 1);
    uint8_t zero = 0;
    while (c->used != 56) {
        rho_sha256_update(c, &zero, 1);
    }
    uint8_t len[8];
    for (int i = 0; i < 8; i++) {
        len[i] = (uint8_t)(bits >> (56 - 8 * i));
    }
    /* The length is the message's, not the padding's: write it without counting it. */
    memcpy(c->block + 56, len, 8);
    rho_sha256_compress(c, c->block);
    for (int i = 0; i < 8; i++) {
        out[4 * i] = (uint8_t)(c->state[i] >> 24);
        out[4 * i + 1] = (uint8_t)(c->state[i] >> 16);
        out[4 * i + 2] = (uint8_t)(c->state[i] >> 8);
        out[4 * i + 3] = (uint8_t)c->state[i];
    }
}

static int rho_hex_value(char c)
{
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

/* rho_hex_decode decodes hex into a new buffer, or returns NULL when the text is
 * not an even number of hex digits. */
static uint8_t* rho_hex_decode(const char* hex, size_t* length)
{
    size_t n = strlen(hex);
    if (n % 2 != 0) {
        return NULL;
    }
    uint8_t* out = (uint8_t*)malloc(n / 2 + 1);
    if (out == NULL) {
        return NULL;
    }
    for (size_t i = 0; i < n / 2; i++) {
        int hi = rho_hex_value(hex[2 * i]);
        int lo = rho_hex_value(hex[2 * i + 1]);
        if (hi < 0 || lo < 0) {
            free(out);
            return NULL;
        }
        out[i] = (uint8_t)((hi << 4) | lo);
    }
    *length = n / 2;
    return out;
}

static char* rho_hex_encode(const uint8_t* data, size_t n)
{
    static const char digits[] = "0123456789abcdef";
    char* out = (char*)malloc(2 * n + 1);
    if (out == NULL) {
        return strdup("");
    }
    for (size_t i = 0; i < n; i++) {
        out[2 * i] = digits[data[i] >> 4];
        out[2 * i + 1] = digits[data[i] & 0x0f];
    }
    out[2 * n] = '\0';
    return out;
}

/* rho_hmac_sha256_hex is HMAC-SHA256 (RFC 2104) of a hex message under a hex key,
 * as 64 hex digits, or "" when either is not hex. */
char* rho_hmac_sha256_hex(const char* key_hex, const char* message_hex)
{
    size_t key_length = 0, message_length = 0;
    uint8_t* key = rho_hex_decode(key_hex == NULL ? "" : key_hex, &key_length);
    uint8_t* message = rho_hex_decode(message_hex == NULL ? "" : message_hex, &message_length);
    if (key == NULL || message == NULL) {
        free(key);
        free(message);
        return strdup("");
    }
    uint8_t block[64];
    memset(block, 0, sizeof(block));
    if (key_length > 64) {
        rho_sha256 c;
        rho_sha256_init(&c);
        rho_sha256_update(&c, key, key_length);
        rho_sha256_final(&c, block);
    } else {
        memcpy(block, key, key_length);
    }
    uint8_t inner_pad[64], outer_pad[64], inner[32], mac[32];
    for (int i = 0; i < 64; i++) {
        inner_pad[i] = block[i] ^ 0x36;
        outer_pad[i] = block[i] ^ 0x5c;
    }
    rho_sha256 c;
    rho_sha256_init(&c);
    rho_sha256_update(&c, inner_pad, 64);
    rho_sha256_update(&c, message, message_length);
    rho_sha256_final(&c, inner);
    rho_sha256_init(&c);
    rho_sha256_update(&c, outer_pad, 64);
    rho_sha256_update(&c, inner, 32);
    rho_sha256_final(&c, mac);
    free(key);
    free(message);
    return rho_hex_encode(mac, 32);
}

/* rho_random_hex is `bytes` random bytes from the operating system, as hex, or ""
 * when there is no such source. A nonce or a key that fell back to something
 * predictable would be one an attacker could compute, so there is no fallback. */
char* rho_random_hex(unsigned int bytes)
{
    if (bytes == 0 || bytes > 1024) {
        return strdup("");
    }
    uint8_t* raw = (uint8_t*)malloc(bytes);
    if (raw == NULL) {
        return strdup("");
    }
    FILE* source = fopen("/dev/urandom", "rb");
    if (source == NULL) {
        free(raw);
        return strdup("");
    }
    size_t got = fread(raw, 1, bytes, source);
    fclose(source);
    if (got != bytes) {
        free(raw);
        return strdup("");
    }
    char* out = rho_hex_encode(raw, bytes);
    free(raw);
    return out;
}
