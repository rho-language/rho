/*
 * bits.c — the float bit cast a wire protocol needs.
 *
 * A float on the wire is its IEEE-754 bits in little-endian order. Rho has no
 * cast between a word and a float, and the two ends of a link must agree on the
 * bits they write, so the cast lives here as one function each way rather than
 * being spelled differently in every generated file.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

uint32_t rho_f32_to_bits(float value)
{
    uint32_t bits;
    memcpy(&bits, &value, sizeof(bits));
    return bits;
}

float rho_f32_from_bits(uint32_t bits)
{
    float value;
    memcpy(&value, &bits, sizeof(value));
    return value;
}

uint64_t rho_f64_to_bits(double value)
{
    uint64_t bits;
    memcpy(&bits, &value, sizeof(bits));
    return bits;
}

double rho_f64_from_bits(uint64_t bits)
{
    double value;
    memcpy(&value, &bits, sizeof(value));
    return value;
}
