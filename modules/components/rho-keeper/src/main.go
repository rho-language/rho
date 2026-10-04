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
	"strings"

	"github.com/rho/rho-keeper/src/keeper"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "rho keeper: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Said before anything is done, and never a failure. See `rho/src/privilege`.
	keeper.CheckNotRoot(os.Stderr)

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

	case "doctor":
		// What this machine has, against what the installed modules need. The same
		// report `rho doctor` prints, so the two cannot disagree about it.
		return keeper.Doctor(os.Stdout)

	case "sync", "fix", "install":
		options := keeper.SyncOptions{}
		for index := 0; index < len(args); index++ {
			switch strings.TrimLeft(args[index], "-") {
			case "dry-run", "n", "dry":
				options.DryRun = true
			case "yes", "y":
				options.Yes = true
			case "module", "component":
				if index+1 < len(args) {
					index++
					options.Module = args[index]
				}
			}
		}
		if len(args) > 1 {
			options.Module = args[len(args)-1]
		}
		switch command {
		case "sync":
			return keeper.Sync(options)
		case "fix":
			return keeper.Fix(options)
		default:
			// `install` is what a wizard in a browser runs, so the names come from
			// the path rather than from flags.
			names := []string{}
			for _, argument := range args {
				if !strings.HasPrefix(argument, "-") && !isFlagValue(args, argument) {
					names = append(names, argument)
				}
			}
			if len(names) < 2 {
				fmt.Fprintln(os.Stderr, "rho keeper install: name the module and the dependency")
				fmt.Fprintln(os.Stderr, "  rho keeper install <module> <dependency>")
				return fmt.Errorf("nothing to install")
			}
			return keeper.Install(names[0], names[1], options)
		}

	case "help", "-h", "--help":
		usage(os.Stdout)
		return nil

	default:
		usage(os.Stderr)
		return fmt.Errorf("unknown command %q", command)
	}
}

// isFlagValue is whether an argument is a flag's value rather than a name, so that
// `install --module rho-dbg openocd` does not read `--module`'s value as the
// module.
func isFlagValue(args []string, argument string) bool {
	for index, one := range args {
		if one == argument && index > 0 && strings.HasPrefix(args[index-1], "-") {
			return true
		}
	}
	return false
}

func usage(out *os.File) {
	fmt.Fprint(out, `usage: rho keeper [command]

  start      run the dashboard on a port (default 127.0.0.1:8477)
  status     what is installed and enabled, on one line per module
  doctor     what this machine has, against what the modules need (rho doctor)
  sync       install what the doctor found missing, after showing what it would run
  fix        the same, one thing at a time, asking about each
  install    install one dependency of one module

flags:
  -addr      the address to listen on; a bare number is a port
  --dry-run  say what would happen and do none of it
  --yes      answer every question in advance
  --module   narrow the fix to one module

Everything the dashboard can do to this machine is one of these commands, shown as
the command it runs: a page is a good place to read a command and a poor place to be
the only way to run one, because this installs packages with root.

The dashboard reads everything and changes two things: what is enabled. Every
change it makes is shown as the command that made it, so a change made here can
be repeated in a terminal.
`)
}
