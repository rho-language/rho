//
// @file main.go
// @brief rho keeper — a local dashboard for the installation.
//
// Two commands, and they are the same reading. `status` is what a person wants
// in a terminal, and `serve` is that text with a page around it. They read the
// same state through the same functions, so what one says and the other shows
// cannot come to disagree — which is the whole reason the reading lives in a
// library rather than in either command.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rho/rho-keeper/src/keeper"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "rho keeper: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	command := "start"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		command, args = args[0], args[1:]
	}

	switch command {
	case "start":
		flags := flag.NewFlagSet("start", flag.ContinueOnError)
		addr := flags.String("addr", "", "the address to listen on; a bare number is a port")
		if err := flags.Parse(args); err != nil {
			return err
		}
		return keeper.Start(keeper.Options{Addr: *addr})

	case "status":
		// What a person wants when they have not opened a browser: what is
		// installed, and which of it is used.
		fmt.Print(keeper.Summary(keeper.Read()))
		return nil

	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil

	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", command)
	}
}

func usage(out *os.File) {
	fmt.Fprint(out, `usage: rho keeper [command]

  start    run the dashboard on a port (default 127.0.0.1:8477)
  status   what is installed and enabled, on one line per module

flags for start:
  -addr    the address to listen on; a bare number is a port

The dashboard reads everything and changes two things: what is enabled. Every
change it makes is shown as the command that made it, so a change made here can
be repeated in a terminal.
`)
}
