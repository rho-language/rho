//
// @file uart.go
// @brief Reading a serial console.
//
// A USB console is its own interface. On a board with an ST-LINK that composite
// device is a debug interface and a virtual serial port side by side, so opening
// the serial port takes nothing from a debugger and contends with nothing — which
// is why this backend is second in the detection order rather than an afterthought.
//
// The line discipline is set here rather than left to whatever the system had,
// because the failure when it is wrong is silent in the worst way: the port opens,
// the baud rate is wrong, and every byte the target sends is discarded by the
// driver before anybody sees it. A console that prints nothing looks exactly like a
// target that printed nothing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The places a serial console is found, most likely first.
//
// The glob is the whole of the detection: a USB CDC console is a character device
// whose name the kernel assigns in enumeration order, so there is nothing to match
// on except what it is called. `/dev/ttyACM*` first because a board's own console
// is nearly always CDC — an ST-LINK's virtual serial port, a DAPLink's — and
// `/dev/ttyUSB*` after it because that is a UART bridge and belongs to whatever
// plugged it in.
var serialGlobs = []string{
	"/dev/ttyACM*",
	"/dev/ttyUSB*",
	// The macOS names for the same devices, which are not the same names.
	"/dev/cu.usbmodem*",
	"/dev/cu.usbserial*",
	"/dev/cu.SLAB*",
}

// NewUARTSource is a serial console at a device node.
//
// Exported because "a console at this path" is something a caller can hold: the
// detection above it is a convenience, and a caller that was told which device to
// use should not have to go back through the scan to get it.
func NewUARTSource(port string, baud int, autoDetect bool) Source {
	options := UARTOptions{Port: port, BaudRate: baud, AutoDetect: autoDetect && port == ""}
	return &uartSource{options: options}
}

// NewRTTSource is an RTT stream at an endpoint, which is the same arrangement for
// the same reason.
func NewRTTSource(host string, port, channel int, timeout time.Duration) Source {
	return &rttSource{
		options: RTTOptions{Host: host, Port: port, Channel: channel},
		timeout: timeout,
	}
}

// uartSource is a serial console.
type uartSource struct {
	options UARTOptions
	// open is how a device is opened, replaceable so that the detection and
	// reading logic can be exercised without hardware.
	open func(path string, baud int) (*os.File, error)
}

func (s *uartSource) Kind() Kind { return KindUART }

// Describe is the device it will open, and which one was chosen out of several.
func (s *uartSource) Describe() string {
	if s.options.Port != "" {
		return fmt.Sprintf("%s at %d baud", s.options.Port, s.options.BaudRate)
	}
	return fmt.Sprintf("the first serial console found, at %d baud", s.options.BaudRate)
}

// Available is whether a console can be opened, and which one.
func (s *uartSource) Available() error {
	port := s.options.Port
	if port == "" {
		found, err := DetectSerialPort()
		if err != nil {
			return err
		}
		port = found
	}
	// Opened and closed: a device node that exists is not a console anybody can
	// read, and the difference is only visible by trying.
	file, err := s.opener()(port, s.options.BaudRate)
	if err != nil {
		return fmt.Errorf("cannot open %s: %w", port, err)
	}
	file.Close()
	return nil
}

// Open opens the console for reading.
func (s *uartSource) Open() (Stream, error) {
	port := s.options.Port
	if port == "" {
		found, err := DetectSerialPort()
		if err != nil {
			return nil, err
		}
		port = found
	}
	file, err := s.opener()(port, s.options.BaudRate)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", port, err)
	}
	return &uartStream{file: file, port: port}, nil
}

// opener is the real serial opener, or the one a test supplied.
func (s *uartSource) opener() func(string, int) (*os.File, error) {
	if s.open != nil {
		return s.open
	}
	return openSerial
}

// DetectSerialPort is the console on this machine, or why there is not one.
//
// Every candidate is named in the failure, because "no console found" on a machine
// with three of them is a statement about the search rather than about the
// machine, and the person reading it has to be able to tell which.
func DetectSerialPort() (string, error) {
	candidates := []string{}
	for _, glob := range serialGlobs {
		matches, _ := filepath.Glob(glob)
		candidates = append(candidates, matches...)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no serial console found. Looked for %s; "+
			"a console is a USB interface beside the debug one, and it appears when the board is plugged in",
			strings.Join(serialGlobs, ", "))
	}
	sort.Strings(candidates)
	return candidates[0], nil
}

// SerialPorts is every console node found, which is what `--port` lists when a
// machine has more than one.
func SerialPorts() []string {
	candidates := []string{}
	for _, glob := range serialGlobs {
		matches, _ := filepath.Glob(glob)
		candidates = append(candidates, matches...)
	}
	sort.Strings(candidates)
	return candidates
}

// uartStream is an open serial console.
type uartStream struct {
	file   *os.File
	port   string
	closed bool
}

func (s *uartStream) Describe() string { return s.port }

func (s *uartStream) Read(p []byte) (int, error) { return s.file.Read(p) }

func (s *uartStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.file.Close()
}

/* -------------------------------------------------------------------------- */
/*  The line discipline                                                       */
/* -------------------------------------------------------------------------- */

// openSerial opens a device node as a console and sets its line discipline.
//
// `raw`, because a console is a byte stream and a terminal driver will helpfully
// interpret it: echo what the target prints back at it, translate its newlines,
// and buffer a line that never arrives. On a firmware that prints without newlines
// — a heartbeat on one line — that is the difference between seeing output and
// seeing nothing at all.
func openSerial(path string, baud int) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	if err := configureSerial(file, baud); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

// configureSerial puts a console into raw mode at a line speed.
func configureSerial(file *os.File, baud int) error {
	if !isTerminal(file) {
		// Not a terminal — a pipe, a test's file, a pty on a system where the
		// discipline cannot be set. There is nothing to configure and nothing to
		// fail: the bytes are already what they are.
		return nil
	}
	speed, ok := baudConstant(baud)
	if !ok {
		return fmt.Errorf("%d is not a line speed this component knows", baud)
	}
	// `ICANON` off, `ECHO` off, `ISIG` off: a console is not a terminal someone
	// is typing at. With canonical mode on, the driver waits for a newline the
	// target may never send, and a firmware that prints a heartbeat without one
	// produces nothing at all — which looks exactly like a board that is silent.
	//
	// `CS8` is named rather than left at zero because on Linux the character-size
	// bits are a value and zero is *five* bits. A termios built field by field
	// without it opens the port successfully and receives nothing: every byte is
	// read at the wrong width, which a driver discards without a word.
	termios := syscall.Termios{
		Iflag: syscall.IGNPAR,
		Cflag: syscall.CREAD | syscall.CLOCAL | syscall.CS8 | speed,
		Lflag: 0,
	}
	termios.Cc[syscall.VMIN] = 1
	termios.Cc[syscall.VTIME] = 0
	if err := ioctl(file.Fd(), syscall.TCSETS, unsafe.Pointer(&termios)); err != nil {
		return fmt.Errorf("set the console to %d baud: %w", baud, err)
	}
	return nil
}

// isTerminal is whether a file is a character device, which is what can have a
// line discipline at all.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// ioctl is the one call this needs, named so the platform dependency is in one
// place rather than spread through the file.
func ioctl(fd uintptr, request uintptr, argument unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(argument))
	if errno != 0 {
		return errno
	}
	return nil
}

// baudConstant maps a line speed to the constant the driver wants.
//
// The standard set, and no more: a console runs at a speed somebody chose from a
// table, and a speed not in the table is a speed this component should say it does
// not know rather than approximate — a console at the wrong speed is silent, and
// silence is the worst thing to hand somebody.
func baudConstant(baud int) (uint32, bool) {
	speeds := map[int]uint32{
		1200:    syscall.B1200,
		2400:    syscall.B2400,
		4800:    syscall.B4800,
		9600:    syscall.B9600,
		19200:   syscall.B19200,
		38400:   syscall.B38400,
		57600:   syscall.B57600,
		115200:  syscall.B115200,
		230400:  syscall.B230400,
		460800:  syscall.B460800,
		921600:  syscall.B921600,
		1000000: syscall.B1000000,
	}
	speed, ok := speeds[baud]
	return speed, ok
}
