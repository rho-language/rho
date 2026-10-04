//
// @file format.go
// @brief How a byte is written down.
//
// A firmware's flash is 2 MB, which is 2097152, and a budget written against
// 2097152 is a budget nobody can hold in their head while reading a build log.
// So every figure in a report is in the unit a person would say out loud, and the
// exact byte count is available behind `--bytes` for the cases where it matters —
// which are the cases where two numbers are being subtracted.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"fmt"
	"os/exec"
	"strings"
)

// DefaultWarnPercentValue is the share of a region at which it is reported as
// worth attention while still inside its budget.
const DefaultWarnPercentValue = 85.0

// execCommand runs a program and captures both streams, because a git failure's
// message is on stderr and a baseline lookup that reports "exit status 128" has
// told nobody anything.
func execCommand(root, name string, args ...string) *exec.Cmd {
	command := exec.Command(name, args...)
	command.Dir = root
	return command
}

// errOutput is whatever a failed command wrote, or nothing.
func errOutput(err error) []byte {
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.Stderr
	}
	return nil
}

// humanBytes is a size in the unit a person would say.
//
// Binary units, because the numbers being compared are memory sizes: a datasheet
// that says 2 MB means 2 MiB, and reporting "1.83 MB / 2.00 MB" for a region that
// is 2097152 bytes would be a report that disagreed with the board it is about.
func humanBytes(size uint64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, suffix := range []string{"KB", "MB", "GB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f TB", value/unit)
}

// humanPercent is a share of something, with one decimal when it is not whole.
func humanPercent(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%.0f%%", value)
	}
	return fmt.Sprintf("%.1f%%", value)
}

// bar is a usage bar, drawn to a width.
//
// Proportional to the *capacity* rather than to the largest thing in the report,
// because a bar that is always full tells a reader nothing: the whole question a
// bar answers is how much of the thing is left.
//
// Wider than the capacity draws full and says so, rather than drawing a bar of
// nonsense: an image over its budget is a fact the report states in words and in
// its exit code, and a bar that ran past the end of its own width would be
// decoration in place of that.
func bar(used, capacity uint64, width int) string {
	if width < 4 {
		width = 4
	}
	if capacity == 0 {
		// Brackets kept: a region with no capacity is still a region somebody is
		// asking about, and a row of dots without them reads as an accident.
		return "[" + strings.Repeat(".", width) + "]"
	}
	filled := int(float64(used) / float64(capacity) * float64(width))
	switch {
	case filled < 1 && used > 0:
		filled = 1
	case filled > width:
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat(".", width-filled) + "]"
}

// shortenObject is a path as short as it can be while still identifying the file.
//
// A build's object paths are absolute and long — the vendor tree alone is a
// hundred characters — and a list of them is unreadable. What identifies an object
// is its last two path elements: `Src/stm32h7xx_hal_eth.c` says which driver, and
// the driver is what a reader wants.
func shortenObject(path string) string {
	if path == "" {
		return ""
	}
	path = strings.TrimPrefix(path, "./")
	parts := strings.Split(path, "/")
	if len(parts) <= 2 {
		return path
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
