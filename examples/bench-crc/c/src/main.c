/*
 * main.c — CRC-32 benchmark, written in plain C.
 *
 * Identical in behaviour to examples/bench-crc/rho/src/main.rho:
 * computes CRC-32 over a 1 KiB buffer 10000 times and prints the digest
 * so the compiler cannot optimise the work away.
 *
 * Build with:  rho build
 * Measure:    rho bench --binary build/output/bench-crc-c
 *            rho mem --binary build/output/bench-crc-c
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <stdint.h>
#include <stdio.h>

#define CRC32_ITERATIONS 10000u
#define CRC32_BUFFER_SIZE 1024u
#define CRC32_POLY 0xEDB88320u

static uint32_t crc32_byte(uint32_t crc, uint8_t byte)
{
    crc ^= byte;
    for (int i = 0; i < 8; i++) {
        crc = (crc >> 1) ^ (CRC32_POLY & (uint32_t)(-(int32_t)(crc & 1u)));
    }
    return crc;
}

static uint32_t crc32_buffer(const uint8_t *data, size_t length)
{
    uint32_t crc = 0xFFFFFFFFu;
    for (size_t i = 0; i < length; i++) {
        crc = crc32_byte(crc, data[i]);
    }
    return crc ^ 0xFFFFFFFFu;
}

int main(void)
{
    uint8_t data[CRC32_BUFFER_SIZE];
    uint32_t result = 0;

    /* Fill the buffer with deterministic data. */
    for (size_t i = 0; i < CRC32_BUFFER_SIZE; i++) {
        data[i] = (uint8_t)(i & 0xFF);
    }

    /* Run the benchmark. The result is printed so the compiler
     * cannot eliminate the loop. */
    for (uint32_t n = 0; n < CRC32_ITERATIONS; n++) {
        result ^= crc32_buffer(data, CRC32_BUFFER_SIZE);
    }

    printf("CRC32 digest: 0x%08X\n", result);
    return 0;
}
