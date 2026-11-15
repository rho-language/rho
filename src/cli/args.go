//
// @file args.go
// @brief Flags wherever they are written: before a path, after it, or both.
//
// Go's flag package stops at the first argument that is not a flag, so
// `rho build . --target go` used to read `.` as the path and `--target go` as
// two more paths, and printed a usage line. The documentation writes the flags
// after the path, because that is how a person types them; this is the parse
// that reads them there.
//
// `--` still ends the flags: everything after it is a positional argument,
// whatever it looks like, which is how `rho run . -- -v` hands `-v` to the
// program rather than to rho.
//
// @project Rho Programming Language
// @date 2026-11-15
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package cli

import (
	"flag"
	"strings"
)

// ParseInterspersed parses a flag set's flags wherever they appear among the
// positional arguments, and returns the positional arguments in order.
//
// A flag is read as Go's flag package reads it — `-name`, `--name`,
// `-name=value`, `-name value` — so one dash and two mean the same. Everything
// after a `--` is positional and is returned in passthrough, separately, so a
// command that hands arguments on can tell them from its own.
func ParseInterspersed(fs *flag.FlagSet, args []string) (positional []string, passthrough []string, err error) {
	own, passthrough, terminated := splitAtTerminator(fs, args)
	for {
		if err := fs.Parse(own); err != nil {
			return positional, passthrough, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		own = rest[1:]
	}
	if !terminated {
		passthrough = nil
	}
	return positional, passthrough, nil
}

// ParseUntilFirst parses flags until the first positional argument, and returns
// that argument and everything after it untouched.
//
// It is the parse for a command whose first positional argument is its own and
// whose later ones belong to something else: `rho run <dir> args...` runs a
// program, and the program's `-v` is not rho's. A `--` before the first
// positional argument ends rho's flags as well, so `rho run -- -v` runs the
// project here with `-v`.
//
// A `--` right after the first positional argument is the separator people
// write out of habit (`rho run . -- -v`) and is dropped; any later one is the
// program's.
func ParseUntilFirst(fs *flag.FlagSet, args []string) (first string, rest []string, hasFirst bool, err error) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch {
		case argument == "--":
			if err := fs.Parse(args[:index]); err != nil {
				return "", nil, false, err
			}
			return "", append([]string{}, args[index+1:]...), false, nil
		case argument == "-" || !strings.HasPrefix(argument, "-"):
			if err := fs.Parse(args[:index]); err != nil {
				return "", nil, false, err
			}
			rest = append([]string{}, args[index+1:]...)
			if len(rest) > 0 && rest[0] == "--" {
				rest = rest[1:]
			}
			return argument, rest, true, nil
		case takesNextAsValue(fs, argument):
			index++
		}
	}
	if err := fs.Parse(args); err != nil {
		return "", nil, false, err
	}
	return "", nil, false, nil
}

// splitAtTerminator splits the arguments at the first `--` that is not the value
// of a flag. A `--` that follows `-o` is what `-o` was given, as Go's flag
// package reads it, so it is not the end of anything.
func splitAtTerminator(fs *flag.FlagSet, args []string) (own []string, after []string, found bool) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			return append([]string{}, args[:index]...), append([]string{}, args[index+1:]...), true
		}
		if takesNextAsValue(fs, argument) {
			index++
		}
	}
	return args, nil, false
}

// takesNextAsValue reports whether a flag argument consumes the next argument as
// its value: a flag the set declares, that is not boolean, written without `=`.
func takesNextAsValue(fs *flag.FlagSet, argument string) bool {
	if len(argument) < 2 || argument[0] != '-' || argument == "--" {
		return false
	}
	name := strings.TrimPrefix(strings.TrimPrefix(argument, "-"), "-")
	if name == "" || strings.Contains(name, "=") {
		return false
	}
	declared := fs.Lookup(name)
	if declared == nil {
		return false
	}
	if boolean, ok := declared.Value.(interface{ IsBoolFlag() bool }); ok && boolean.IsBoolFlag() {
		return false
	}
	return true
}

// WantsHelp reports whether an argument list asks for help: `-h`, `-help` or
// `--help` before any `--`.
func WantsHelp(args []string) bool {
	for _, argument := range args {
		switch argument {
		case "--":
			return false
		case "-h", "-help", "--help":
			return true
		}
	}
	return false
}
