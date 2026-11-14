//
// @file parity_std_embedded_test.go
// @brief The embedded drivers on the host, over a board made of stubs.
//
// std.embedded's drivers call `*_hw_*` functions a board's platform implements in
// C, and they are for the C backend only (`@backends c`): no Go target has a
// platform that supplies them, and a Go build importing one is refused by name
// (backends_test.go). So they are tested where they run — a C build — on the
// host, with the platform replaced by stubs written here: a board in memory that
// records what a driver wrote and answers what it reads. The build is the
// parity build's, with AddressSanitizer and UndefinedBehaviorSanitizer, so a
// driver that writes past a buffer fails here rather than on a part.
//
// Stubs could be written in Go as well (a platform port is Go source), but the
// toolchain carries no Go platform for these functions, and the module's tag is
// what a build obeys; the Go half is the refusal.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const embeddedManifest = `module:
  name: parity
  type: rho
  version: 0.1.0

main: src/main.rho

build:
  language: c
  targets:
    - c
`

// embeddedBoard is the platform the drivers are linked against.
const embeddedBoard = `#include <stdint.h>
#include <string.h>

/* A board in memory: what the drivers wrote, and what they will read. */
static uint32_t pin_mode[64];
static uint32_t pin_level[64];
static uint32_t gpio_writes;
static uint32_t uart_baud;
static uint8_t uart_tx[256];
static uint32_t uart_tx_len;
static uint8_t uart_rx[256];
static uint32_t uart_rx_len;
static uint32_t delay_ms_total, delay_us_total;
static uint32_t i2c_bus, i2c_addr;
static uint8_t i2c_mem[16];
static uint32_t spi_ready;

void gpio_hw_init(uint32_t pin, uint32_t mode) { pin_mode[pin % 64] = mode; }
void gpio_hw_write(uint32_t pin, uint32_t value) { pin_level[pin % 64] = value; gpio_writes++; }
uint32_t gpio_hw_read(uint32_t pin) { return pin_level[pin % 64]; }

void uart_hw_init(uint32_t baud) { uart_baud = baud; }
void uart_hw_write(uint8_t* data, uint32_t len) {
    for (uint32_t i = 0; i < len && uart_tx_len < sizeof uart_tx; i++) uart_tx[uart_tx_len++] = data[i];
}
uint32_t uart_hw_read(uint8_t* out, uint32_t len) {
    uint32_t n = uart_rx_len < len ? uart_rx_len : len;
    memcpy(out, uart_rx, n);
    return n;
}

void timer_hw_delay_ms(uint32_t ms) { delay_ms_total += ms; }
void timer_hw_delay_us(uint32_t us) { delay_us_total += us; }

void i2c_hw_init(uint32_t bus) { i2c_bus = bus; }
uint32_t i2c_hw_write(uint32_t addr, uint8_t* data, uint32_t len) {
    i2c_addr = addr;
    for (uint32_t i = 0; i < len && i < sizeof i2c_mem; i++) i2c_mem[i] = data[i];
    return len;
}
uint32_t i2c_hw_read(uint32_t addr, uint8_t* out, uint32_t len) {
    if (addr != i2c_addr) return 0;
    for (uint32_t i = 0; i < len && i < sizeof i2c_mem; i++) out[i] = (uint8_t)(i2c_mem[i] + 1);
    return len;
}

void spi_hw_init(void) { spi_ready = 1; }
void spi_hw_transfer(uint8_t* tx, uint8_t* rx, uint32_t len) {
    for (uint32_t i = 0; i < len; i++) rx[i] = spi_ready ? (uint8_t)(tx[i] ^ 0xFF) : 0;
}

/* What a test asks the board afterwards. */
uint32_t stub_pin_mode(uint32_t pin) { return pin_mode[pin % 64]; }
uint32_t stub_gpio_writes(void) { return gpio_writes; }
uint32_t stub_uart_baud(void) { return uart_baud; }
uint32_t stub_uart_sent(uint32_t i) { return i < uart_tx_len ? uart_tx[i] : 0xFFFFFFFFu; }
uint32_t stub_uart_sent_len(void) { return uart_tx_len; }
void stub_uart_reply(uint32_t i, uint32_t byte) { if (i < sizeof uart_rx) { uart_rx[i] = (uint8_t)byte; if (i + 1 > uart_rx_len) uart_rx_len = i + 1; } }
void stub_uart_reset(void) { uart_tx_len = 0; uart_rx_len = 0; }
uint32_t stub_delay_ms(void) { return delay_ms_total; }
uint32_t stub_delay_us(void) { return delay_us_total; }
uint32_t stub_i2c_bus(void) { return i2c_bus; }
`

// embeddedBoardHeader declares what a test program asks the board, which the
// program declares as extern fn. The generated C names those functions without
// a prototype, as it does any foreign function a header is meant to declare, so
// the header is that header.
const embeddedBoardHeader = `#include <stdint.h>
uint32_t stub_pin_mode(uint32_t pin);
uint32_t stub_gpio_writes(void);
uint32_t stub_uart_baud(void);
uint32_t stub_uart_sent(uint32_t i);
uint32_t stub_uart_sent_len(void);
void stub_uart_reply(uint32_t i, uint32_t byte);
void stub_uart_reset(void);
uint32_t stub_delay_ms(void);
uint32_t stub_delay_us(void);
uint32_t stub_i2c_bus(void);
`

// runOnStubBoard builds a program for C with the stub board and runs it.
func runOnStubBoard(t *testing.T, source string) outcome {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		filepath.Join(root, "module.yml"): embeddedManifest,
		entry:                             source,
		filepath.Join(root, "board.c"):    embeddedBoard,
		filepath.Join(root, "board.h"):    embeddedBoardHeader,
	} {
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAllTargets failed: %v", err)
	}
	cDir := dirs[0]
	binary := filepath.Join(t.TempDir(), "board-c")
	build := exec.Command("gcc", "-std=c11", "-g", "-O0", "-fsanitize=address,undefined",
		"-fno-sanitize-recover=undefined", "-Werror=implicit-function-declaration",
		"-include", filepath.Join(root, "board.h"), "-o", binary,
		filepath.Join(cDir, "main.c"), filepath.Join(cDir, "app.c"), filepath.Join(cDir, "runtime.c"),
		filepath.Join(root, "board.c"), "-I", cDir, "-lm")
	if out, err := build.CombinedOutput(); err != nil {
		return outcome{failed: "C build failed: " + string(out)}
	}
	return runBinary(binary)
}

var embeddedCases = []struct {
	name, source, want string
}{
	{
		name: "std.embedded.gpio and std.embedded.timer: pins and delays",
		source: `module app

import std.embedded.gpio
import std.embedded.timer

extern fn stub_pin_mode(pin: u32) u32
extern fn stub_gpio_writes() u32
extern fn stub_delay_ms() u32
extern fn stub_delay_us() u32

fn main() void {
    init(13, 1)
    init(2, 0)
    write(13, true)
    print("mode=" + int_to_str(stub_pin_mode(13)) + "," + int_to_str(stub_pin_mode(2)))
    print(if read(13) { " high" } else { " low" })
    write(13, false)
    print(if read(13) { " high" } else { " low" })
    print(" writes=" + int_to_str(stub_gpio_writes()))
    delay_ms(250)
    delay_ms(5)
    delay_us(40)
    print(" ms=" + int_to_str(stub_delay_ms()) + " us=" + int_to_str(stub_delay_us()))
}
`,
		want: "mode=1,0 high low writes=2 ms=255 us=40",
	},
	{
		name: "std.embedded.uart, i2c and spi: bytes out and back",
		source: `module app

import std.core.ptr
import std.core.slice
import std.embedded.uart
import std.embedded.i2c
import std.embedded.spi

extern fn stub_uart_baud() u32
extern fn stub_uart_sent(i: u32) u32
extern fn stub_uart_sent_len() u32
extern fn stub_uart_reply(i: u32, byte: u32) void
extern fn stub_i2c_bus() u32

fn main() void {
    uart.init(115200)
    var out: u8[3] = [65, 66, 67]
    uart.write(Slice[u8]{ Ptr[u8](&out[0]), 3 })
    print("baud=" + int_to_str(stub_uart_baud()) + " sent=" + int_to_str(stub_uart_sent_len()) + ":" + int_to_str(stub_uart_sent(0)) + "," + int_to_str(stub_uart_sent(2)))
    stub_uart_reply(0, 7)
    stub_uart_reply(1, 8)
    var back: u8[4] = [0, 0, 0, 0]
    let n: u32 = uart.read(Slice[u8]{ Ptr[u8](&back[0]), 4 })
    print(" read=" + int_to_str(n) + ":" + int_to_str(u32(back[0])) + "," + int_to_str(u32(back[1])) + "," + int_to_str(u32(back[2])))

    i2c.init(1)
    var reg: u8[2] = [16, 32]
    let wrote: u32 = i2c.write(0x48, Slice[u8]{ Ptr[u8](&reg[0]), 2 })
    var got: u8[2] = [0, 0]
    let read: u32 = i2c.read(0x48, Slice[u8]{ Ptr[u8](&got[0]), 2 })
    let none: u32 = i2c.read(0x49, Slice[u8]{ Ptr[u8](&got[0]), 2 })
    print(" i2c bus=" + int_to_str(stub_i2c_bus()) + " w=" + int_to_str(wrote) + " r=" + int_to_str(read) + ":" + int_to_str(u32(got[0])) + "," + int_to_str(u32(got[1])) + " other=" + int_to_str(none))

    spi.init()
    var tx: u8[3] = [0, 15, 255]
    var rx: u8[3] = [1, 1, 1]
    spi.transfer(Slice[u8]{ Ptr[u8](&tx[0]), 3 }, Slice[u8]{ Ptr[u8](&rx[0]), 3 })
    print(" spi=" + int_to_str(u32(rx[0])) + "," + int_to_str(u32(rx[1])) + "," + int_to_str(u32(rx[2])))
}
`,
		want: "baud=115200 sent=3:65,67 read=2:7,8,0 i2c bus=1 w=2 r=2:17,33 other=0 spi=255,240,0",
	},
	{
		name: "std.embedded.rs485 and std.embedded.modbus: a read of holding registers",
		source: `module app

import std.core.ptr
import std.core.slice
import std.embedded.rs485
import std.embedded.modbus

extern fn stub_uart_baud() u32
extern fn stub_uart_sent(i: u32) u32
extern fn stub_uart_sent_len() u32
extern fn stub_uart_reply(i: u32, byte: u32) void
extern fn stub_uart_reset() void
extern fn stub_pin_mode(pin: u32) u32
extern fn stub_gpio_writes() u32

fn hex(v: u32) string {
    return int_to_str(v)
}

fn main() void {
    // The reference frame of the Modbus specification: read 10 holding
    // registers from address 0 of device 1, whose CRC is 0xCDC5 sent as C5 CD.
    var frame: u8[6] = [1, 3, 0, 0, 0, 10]
    let crc: u16 = crc16_modbus(Slice[u8]{ Ptr[u8](&frame[0]), 6 })
    print("crc=" + int_to_str(u32(crc)))

    let bus: RS485 = rs485.init(4, 5, 9600)
    print(" baud=" + int_to_str(stub_uart_baud()) + " de=" + int_to_str(stub_pin_mode(4)) + " re=" + int_to_str(stub_pin_mode(5)) + " gpio=" + int_to_str(stub_gpio_writes()))
    let m: Modbus = modbus.init(bus, 17)

    // The device answers two registers, 0x022B and 0x0001, with a good CRC.
    var reply: u8[9] = [17, 3, 4, 2, 43, 0, 1, 0, 0]
    let rc: u16 = crc16_modbus(Slice[u8]{ Ptr[u8](&reply[0]), 7 })
    reply[7] = u8(rc & 0xFF)
    reply[8] = u8(rc >> 8)
    var i: u32 = 0
    while i < 9 {
        stub_uart_reply(i, u32(reply[i]))
        i = i + 1
    }
    var regs: u16[2] = [0, 0]
    let ok: bool = read_holding(m, 107, 2, Slice[u16]{ Ptr[u16](&regs[0]), 2 })
    print(if ok { " ok" } else { " failed" })
    print(" regs=" + int_to_str(u32(regs[0])) + "," + int_to_str(u32(regs[1])))
    // The request: address, function 3, start 107 (0x006B), quantity 2, CRC.
    print(" sent=" + int_to_str(stub_uart_sent_len()) + ":")
    var j: u32 = 0
    while j < stub_uart_sent_len() {
        print(hex(stub_uart_sent(j)) + " ")
        j = j + 1
    }

    // A reply whose CRC is wrong is refused, and the registers are left alone.
    stub_uart_reset()
    stub_uart_reply(8, u32(reply[8]) ^ 1)
    i = 0
    while i < 8 {
        stub_uart_reply(i, u32(reply[i]))
        i = i + 1
    }
    var other: u16[2] = [9, 9]
    let bad: bool = read_holding(m, 107, 2, Slice[u16]{ Ptr[u16](&other[0]), 2 })
    print(if bad { "badcrc-accepted" } else { "badcrc-refused" })
    print(" " + int_to_str(u32(other[0])))

    // Too short a reply is refused.
    stub_uart_reset()
    stub_uart_reply(0, 17)
    stub_uart_reply(1, 3)
    print(if read_holding(m, 0, 1, Slice[u16]{ Ptr[u16](&other[0]), 2 }) { " short-accepted" } else { " short-refused" })
}
`,
		want: "crc=52677 baud=9600 de=1 re=1 gpio=2 ok regs=555,1 sent=8:17 3 0 107 0 2 183 71 badcrc-refused 9 short-refused",
	},
}

func TestEmbeddedDriversOnAStubBoard(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil {
		t.Skip("no C compiler available")
	}
	if !sanitizersWork(t) {
		t.Skip("this C compiler cannot build with the sanitizers")
	}
	for _, c := range embeddedCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := runOnStubBoard(t, c.source)
			if got.failed != "" {
				t.Fatalf("C: %s\nstdout so far: %q", got.failed, got.stdout)
			}
			if got.stdout != c.want {
				t.Fatalf("the drivers print\n%s\nwant\n%s", got.stdout, c.want)
			}
		})
	}
}

// Every std.embedded module says it is for C, which is what the refusal on a
// Go target reads.
func TestEveryEmbeddedModuleSaysItIsForC(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "std", "embedded", "*.rho"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no std.embedded sources: %v", err)
	}
	for _, file := range files {
		tag, ok := readStdBackendTag(file)
		if !ok {
			t.Errorf("%s is not read as a std module", file)
			continue
		}
		if strings.Join(tag.Backends, ",") != "c" {
			t.Errorf("%s says @backends %v, want c", file, tag.Backends)
		}
	}
}
