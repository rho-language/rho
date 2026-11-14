//
// @file parity_std_math_test.go
// @brief std.math.basic, control, embedded and fixed on both backends.
//
// Arithmetic in Rho, with nothing behind it: the same numbers on both backends.
// Results are printed as thousandths through i64, so a difference in the last
// place of a double does not decide the comparison but a wrong formula does.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

func init() {
	parityCases = append(parityCases, stdMathParityCases...)
}

var stdMathParityCases = []parityCase{
	{
		name: "std.math.basic: min, max, clamp, map, deadband and smoothstep",
		source: `module app

import std.math.basic

fn milli(x: f64) string {
    return i64_to_str(i64(x * 1000.0))
}

fn main() void {
    print(int_to_str(min_u32(3, 7)) + " " + int_to_str(max_u32(3, 7)) + " " + int_to_str(clamp_u32(9, 2, 5)))
    print(" " + i64_to_str(i64(abs_i32(-5))))
    print(" " + milli(min_f64(1.5, -2.0)) + " " + milli(max_f64(1.5, -2.0)) + " " + milli(clamp_f64(7.0, 0.0, 5.0)))
    print(" " + milli(map_f64(5.0, 0.0, 10.0, 0.0, 100.0)) + " " + milli(deadband_f64(0.5, 1.0)) + " " + milli(deadband_f64(-3.0, 1.0)) + " " + milli(smoothstep(0.0, 1.0, 0.25)))
}
`,
		want: "3 7 5 5 -2000 1500 5000 50000 0 -2000 156",
	},
	{
		name: "std.math.control: PID, low-pass, moving average, complementary and Kalman",
		source: `module app

import std.math.control

fn milli(x: f64) string {
    return i64_to_str(i64(x * 1000.0))
}

fn main() void {
    var pid: PID = pid_init(2.0, 0.5, 0.1, -10.0, 10.0)
    pid = pid_set_derivative_filter(pid, 1.0)
    pid = pid_update(pid, 5.0, 3.0, 0.5)
    print("pid=" + milli(pid.integral) + "," + milli(pid.derivative) + "," + milli(pid_output(pid, 5.0, 3.0)))
    pid = pid_update(pid, 5.0, 4.0, 0.5)
    print("," + milli(pid_output(pid, 5.0, 4.0)) + "," + milli(pid_output(pid, 100.0, 0.0)))
    var lp: LowPass = low_pass_init(0.5, 0.0)
    lp = low_pass_update(lp, 10.0)
    lp = low_pass_update(lp, 20.0)
    print(" lp=" + milli(lp.value))
    var ma: MovingAverage4 = moving_average4_init()
    print(" ma=" + milli(moving_average4_value(ma)))
    ma = moving_average4_update(ma, 1.0)
    ma = moving_average4_update(ma, 2.0)
    print("," + milli(moving_average4_value(ma)))
    ma = moving_average4_update(ma, 3.0)
    ma = moving_average4_update(ma, 4.0)
    ma = moving_average4_update(ma, 9.0)
    print("," + milli(moving_average4_value(ma)))
    var cf: Complementary = complementary_init(0.9)
    cf = complementary_update(cf, 10.0, 2.0, 0.5)
    print(" cf=" + milli(cf.angle))
    var k: Kalman1D = kalman1d_init(0.0, 0.1, 1.0)
    k = kalman1d_update(k, 10.0)
    print(" k=" + milli(k.estimate) + "," + milli(k.covariance))
}
`,
		want: "pid=1000,4000,4900,2550,10000 lp=15000 ma=0,1500,4500 cf=1900 k=5238,523",
	},
	{
		name: "std.math.embedded: map_range, saturate, deadband, smoothstep01, angles",
		source: `module app

import std.math.embedded

fn milli(x: f64) string {
    return i64_to_str(i64(x * 1000.0))
}

fn main() void {
    print(milli(map_range(2.0, 0.0, 4.0, 10.0, 20.0)) + " " + milli(saturate(-1.0, 0.0, 1.0)) + " " + milli(saturate(0.5, 0.0, 1.0)) + " " + milli(deadband(0.2, 0.5)) + " " + milli(deadband(-0.7, 0.5)))
    print(" " + milli(smoothstep01(0.5)) + " " + milli(smoothstep01(2.0)) + " " + milli(deg_to_rad(180.0)) + " " + milli(rad_to_deg(1.0)))
}
`,
		want: "15000 0 500 0 -700 500 1000 3141 57295",
	},
	// q16_to_f64 divides an i32 by 65536.0: Go turned the literal into an int32 and
	// divided as integers, so the fraction was lost.
	{
		name: "std.math.fixed: Q16.16 and Q8.8 arithmetic",
		source: `module app

import std.math.fixed

fn milli(x: f64) string {
    return i64_to_str(i64(x * 1000.0))
}

fn main() void {
    let a: Q16_16 = q16_from_i32(3)
    let b: Q16_16 = q16_from_i32(-2)
    print(milli(q16_to_f64(a)) + " " + milli(q16_to_f64(q16_add(a, b))) + " " + milli(q16_to_f64(q16_sub(a, b))) + " " + milli(q16_to_f64(q16_mul(a, b))))
    let half: Q16_16 = Q16_16{ 32768 }
    print(" " + milli(q16_to_f64(q16_mul(a, half))) + " " + milli(q16_to_f64(q16_clamp(a, b, half))))
    let c: Q8_8 = q8_from_i32(5)
    let d: Q8_8 = q8_from_i32(2)
    print(" " + i64_to_str(i64(q8_add(c, d).raw)) + " " + i64_to_str(i64(q8_sub(d, c).raw)) + " " + i64_to_str(i64(q8_mul(c, d).raw)))
}
`,
		want: "3000 1000 5000 -6000 1500 500 1792 -768 2560",
	},
	// q16_mul and q8_mul multiplied two i32 raws in 32 bits: undefined behaviour in C
	// and a wrapped, wrong value in Go. They multiply in 64 bits now.
	{
		name: "std.math.fixed: a product wider than 32 bits",
		source: `module app

import std.math.fixed

fn main() void {
    // 200 * 300 in Q16.16: the raw product is past 2^47, which an i32 product
    // overflowed (undefined in C, wrapped in Go). The result itself fits.
    let a: Q16_16 = q16_from_i32(200)
    let b: Q16_16 = Q16_16{ 19661 }
    let p: Q16_16 = q16_mul(a, b)
    print(i64_to_str(i64(p.raw)) + " " + i64_to_str(i64(p.raw / 65536)))
    let c: Q8_8 = q8_from_i32(200)
    let d: Q8_8 = q8_from_i32(200)
    print(" " + i64_to_str(i64(q8_mul(c, d).raw)))
}
`,
		want: "3932200 60 10240000",
	},
	// C converts the integer to double; Go converted the constant to the integer type
	// (truncating, or refusing to build). The integer is converted explicitly now.
	{
		name: "a literal with a fraction meeting an integer is floating point on both backends",
		source: `module app

struct Reading { raw: i32 }

fn milli(x: f64) string {
    return i64_to_str(i64(x * 1000.0))
}

fn main() void {
    let r: Reading = Reading{ -3 }
    let n: u32 = 7
    let big: i64 = 5
    print(milli(r.raw / 2.0) + " " + milli(n * 0.5) + " " + milli(0.25 * big) + " " + milli(1.5 + n) + " " + milli(n - 0.5))
    if n > 6.5 { print(" gt") }
    if r.raw < -2.5 { print(" lt") }
    let k: f64 = n / 4.0
    print(" " + milli(k))
}
`,
		want: "-1500 3500 1250 8500 6500 gt lt 1750",
	},
}
