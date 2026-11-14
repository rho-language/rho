/*
 * math.c — the floating-point functions, from the C library.
 *
 * `std.math.float`, `std.math.fast` and the bare `sqrt`, `sin`, `cos`,
 * `atan2`, `floor`, `clamp_f64` and `lerp` call these. They are the C library's own, so a hosted
 * build links the maths library with the program (`-lm`): the generated CMake
 * says so whenever this fragment is emitted. The Go backend gives the same
 * answers from its `math` package.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <math.h>

double rho_sqrt(double x)
{
    return sqrt(x);
}

double rho_pow(double x, double y)
{
    return pow(x, y);
}

double rho_sin(double x)
{
    return sin(x);
}

double rho_cos(double x)
{
    return cos(x);
}

double rho_atan2(double y, double x)
{
    return atan2(y, x);
}

double rho_fabs(double x)
{
    return fabs(x);
}

double rho_floor(double x)
{
    return floor(x);
}

double rho_clamp_f64(double value, double low, double high)
{
    if (value < low) {
        return low;
    }
    if (value > high) {
        return high;
    }
    return value;
}

double rho_lerp(double a, double b, double t)
{
    return a + (b - a) * t;
}
