//
// @file main.go
// @brief `rho monitor` — the target's output, without taking the board from a debugger.
//
//   rho monitor                       read it, whichever transport has one
//   rho monitor --backend=uart        read it over a particular transport
//   rho monitor --out=raw.log         and keep a copy
//
// Zero configuration is the design: a board plugged in with a USB console and a
// debug server offering RTT is read without being told anything, because the two
// backends that matter both work without help and neither costs a debugger
// anything. Everything on the command line exists for the cases where the defaults
// are wrong.
//
// The arguments are read by hand rather than with a flag package, for the reason
// rho-lint and rho-dbg read their own: both `-flag value` and `--flag=value`
// arrive, from an editor's launch configuration as much as from a person.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rho/rho-monitor/src/rhomonitor"

	rholog "github.com/rho/rho-log/src/logger"
)

// log is this component's own logger, so a monitor's own output — which transport
// it chose, what it could not open, when it stopped — is identifiable in a log
// that several components write to without every line having to say so.
//
// Deliberately not the target's output: that goes to the terminal the person is
// watching, unaltered, because a monitor that logged a board's output through the
// shared logger would be formatting somebody else's data twice.
var log = rholog.Default("rho-monitor")

// The exit codes, which a pipeline reads.
//
// Three, because "the target printed nothing" and "this run could not happen" are
// different failures and a pipeline told the second as the first stops reading the
// first.
const (
	// exitClean means the stream ended.
	exitClean = 0
	// exitUsage means the command was asked for something that cannot be done.
	exitUsage = 2
	// exitNoSource means nothing was there to read.
	exitNoSource = 3
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return monitor(args)
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage()
		return exitClean
	case "backends", "-list-backends", "--list-backends":
		return listBackends()
	case "ports":
		return listPorts()
	}
	return monitor(args)
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: rho monitor [project] [options]    read the target's output

options:
  --backend=NAME         rtt, uart, swo, or auto (the default)
  --channel=N            the RTT channel to read (default 0)
  --host=NAME            the RTT or SWO host (default localhost)
  --port=N               the RTT or SWO port (default 9090)
  --console=DEVICE       the serial console, instead of finding one
  --baud=N               the line speed (default 115200)
  --swo-file=PATH        read an ITM/SWO capture from a file
  --timestamps           prefix each line with when it arrived (the default)
  --no-timestamps        do not
  --no-color             do not colour levels
  --out=PATH             write a copy of everything here
  --duration=SECONDS     stop after this long, which is what a pipeline wants
  --quiet                print nothing; the exit code is the answer

  rho monitor backends            what this component can read
  rho monitor ports               the serial consoles on this machine`)
}

func listBackends() int {
	for _, kind := range rhomonitor.Backends() {
		fmt.Println(kind)
	}
	return exitClean
}

func listPorts() int {
	ports := rhomonitor.SerialPorts()
	if len(ports) == 0 {
		fmt.Fprintln(os.Stderr, "rho monitor: no serial console found")
		return exitNoSource
	}
	for _, port := range ports {
		fmt.Println(port)
	}
	return exitClean
}

// options is what a run was asked for.
type options struct {
	projectRoot string
	backend     string
	channel     int
	host        string
	port        int
	console     string
	baud        int
	swoFile     string
	outPath     string
	timestamps  bool
	color       bool
	duration    time.Duration
	quiet       bool
	// Set flags: a command line that said nothing must not override what the
	// project declared, and a boolean cannot tell "said no" from "said nothing".
	channelSet    bool
	timestampsSet bool
	colorSet      bool
}

func monitor(args []string) int {
	projectRoot, opts := parseArgs(args)

	settings := rhomonitor.ReadSettings()
	config := rhomonitor.ReadConfig(projectRoot)
	applySettings(&config, settings)
	apply(&config, opts)

	// Colour is a property of where the output is going, not of what the project
	// declared: a log redirected to a file has escape sequences in it that `grep`
	// will match against, and a person reading a pipe did not ask for colour.
	config.Formatting.Color = opts.color && isTerminal(os.Stdout)
	if opts.outPath != "" {
		config.Formatting.LogFile = opts.outPath
	}
	logPath := rhomonitor.LogFilePath(projectRoot, config.Formatting.LogFile)

	if opts.quiet {
		config.Formatting.Timestamps = false
		config.Formatting.ColorLevels = false
		config.Formatting.Color = false
	}

	// What a run decided and could not do, in the shared log as well as on
	// standard error: a monitor is usually the thing running while something else
	// is being debugged, and its account of itself belongs beside the rest.
	note := func(message string) {
		log.Info(message)
		if !opts.quiet {
			fmt.Fprintln(os.Stderr, "rho monitor:", message)
		}
	}

	source, err := rhomonitor.Detect(config, note)
	if err != nil {
		note(err.Error())
		return exitNoSource
	}

	stream, err := source.Open()
	if err != nil {
		note(err.Error())
		return exitNoSource
	}
	defer stream.Close()

	log.Info("reading target output",
		"transport", source.Kind().String(), "stream", stream.Describe(), "project", projectRoot)
	if !opts.quiet {
		fmt.Fprintf(os.Stderr, "rho monitor: reading %s\n", stream.Describe())
		if logPath != "" {
			fmt.Fprintf(os.Stderr, "rho monitor: writing a copy to %s\n", logPath)
		}
		if config.RunFor > 0 {
			fmt.Fprintf(os.Stderr, "rho monitor: stopping after %s\n", config.RunFor)
		}
	}

	// Counted rather than assumed: a run that read a board for a minute and
	// printed nothing is a run that went wrong, and the log entry saying "finished"
	// is not enough to tell it from a board that said nothing.
	counter := &rhomonitor.Counter{}
	formatter := rhomonitor.NewFormatter(os.Stdout, config.Formatting)
	formatter.Count(counter)
	if logPath != "" && !opts.quiet {
		if err := formatter.AttachLog(logPath); err != nil {
			note(err.Error())
		}
		defer formatter.Close()
	}

	// A person stopping a monitor presses Ctrl-C, and that has to end the run
	// rather than kill it mid-line: a target that prints continuously would
	// otherwise leave the terminal in whatever state the last escape sequence put
	// it in.
	//
	// Both the signal and the duration stop the run, and a monitor can be given
	// both, so the stop happens once — two of them closing the same channel is a
	// panic in a program whose whole job is to run until it is told otherwise.
	// Closing the stream as well is what unblocks a read that is waiting for a
	// target that has stopped printing: otherwise `--duration` on a silent board
	// would wait for output that is never coming.
	done := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			close(done)
			stream.Close()
		})
	}
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-interrupted
		stop()
	}()
	if config.RunFor > 0 {
		go func() {
			time.Sleep(config.RunFor)
			stop()
		}()
	}

	err = formatter.Tail(stream, done)
	if err != nil {
		note(err.Error())
		log.Error("monitor run failed", "error", err.Error())
		return exitUsage
	}
	log.Info("monitor run finished",
		"transport", source.Kind().String(), "lines", counter.Lines())
	return exitClean
}

// applySettings is what this component's own configuration says, over the
// defaults and under the project.
//
// Under the project, because a project's `monitor:` block is a decision about that
// firmware and this file is a decision about this installation; where they
// disagree the project is closer to the thing being run. Over the defaults,
// because that is what a settings file is.
func applySettings(config *rhomonitor.Config, settings rhomonitor.Settings) {
	if settings.Backend != "" {
		config.Backend = settings.Backend
	}
	if settings.BaudRate > 0 {
		config.UART.BaudRate = settings.BaudRate
	}
	config.Formatting.Timestamps = settings.Timestamps
	config.Formatting.ColorLevels = settings.ColorLevels
	if settings.LogFile != "" {
		config.Formatting.LogFile = settings.LogFile
	}
	if settings.Duration > 0 {
		config.RunFor = settings.Duration
	}
}

// apply is the command line over the project's declaration, which wins on
// everything the command line left alone.
func apply(config *rhomonitor.Config, opts options) {
	if opts.backend != "" {
		// Refused rather than carried through: a backend nobody reads would sit
		// in the config looking like a decision, and the run would then fall back
		// to `auto` and read something other than what was asked for. Said here
		// rather than silently ignored.
		if backend, known := rhomonitor.ParseBackend(opts.backend); known {
			config.Backend = backend
		} else {
			fmt.Fprintf(os.Stderr, "rho monitor: %q is not a transport this component reads; it reads %s\n",
				opts.backend, strings.Join(rhomonitor.KindNames(), ", "))
		}
	}
	if opts.host != "" {
		config.RTT.Host = opts.host
	}
	if opts.port > 0 {
		config.RTT.Port = opts.port
	}
	if opts.channel > 0 || opts.channelSet {
		config.RTT.Channel = opts.channel
	}
	if opts.console != "" {
		config.UART.Port = opts.console
		config.UART.AutoDetect = false
	}
	if opts.baud > 0 {
		config.UART.BaudRate = opts.baud
	}
	if opts.swoFile != "" {
		config.SWO.File = opts.swoFile
	}
	if opts.host != "" && opts.swoFile == "" {
		config.SWO.Host = opts.host
	}
	if opts.duration > 0 {
		config.RunFor = opts.duration
	}
	if opts.timestampsSet {
		config.Formatting.Timestamps = opts.timestamps
	}
	if opts.colorSet {
		config.Formatting.ColorLevels = opts.color
	}
}

// parseArgs reads a run's arguments, in both the spellings that arrive.
func parseArgs(args []string) (string, options) {
	opts := options{
		timestamps: true,
		color:      true,
	}
	rest := []string{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if !strings.HasPrefix(arg, "-") {
			rest = append(rest, arg)
			continue
		}
		trimmed := strings.TrimLeft(arg, "-")
		name, value, hasValue := trimmed, "", false
		if split := strings.Index(trimmed, "="); split >= 0 {
			name, value, hasValue = trimmed[:split], trimmed[split+1:], true
		}
		take := func() (string, bool) {
			if hasValue {
				return value, true
			}
			if index+1 < len(args) && !strings.HasPrefix(args[index+1], "-") {
				index++
				return args[index], true
			}
			return "", false
		}
		switch name {
		case "backend":
			if v, ok := take(); ok {
				opts.backend = v
			}
		case "channel":
			if v, ok := take(); ok {
				if parsed, err := strconv.Atoi(v); err == nil {
					opts.channel, opts.channelSet = parsed, true
				}
			}
		case "host":
			if v, ok := take(); ok {
				opts.host = v
			}
		case "port":
			if v, ok := take(); ok {
				if parsed, err := strconv.Atoi(v); err == nil {
					opts.port = parsed
				}
			}
		case "console", "uart":
			if v, ok := take(); ok {
				opts.console = v
			}
		case "baud":
			if v, ok := take(); ok {
				if parsed, err := strconv.Atoi(v); err == nil {
					opts.baud = parsed
				}
			}
		case "swo-file", "swo":
			if v, ok := take(); ok {
				opts.swoFile = v
			}
		case "out", "o", "log":
			if v, ok := take(); ok {
				opts.outPath = v
			}
		case "timestamps":
			opts.timestamps, opts.timestampsSet = !isFalse(value, hasValue), true
		case "no-timestamps":
			opts.timestamps, opts.timestampsSet = false, true
		case "color", "colour":
			opts.color, opts.colorSet = !isFalse(value, hasValue), true
		case "no-color", "no-colour":
			opts.color, opts.colorSet = false, true
		case "duration":
			if v, ok := take(); ok {
				if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
					opts.duration = time.Duration(seconds) * time.Second
				}
			}
		case "quiet":
			opts.quiet = !isFalse(value, hasValue)
		case "h", "help":
			usage()
			return "", opts
		default:
			fmt.Fprintf(os.Stderr, "rho monitor: unknown option %q\n", arg)
			fmt.Fprintln(os.Stderr, "rho monitor --help lists what this takes")
		}
	}
	root := ""
	if len(rest) > 0 {
		root = rest[0]
	}
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	return root, opts
}

// isTerminal is whether standard output is a terminal, which is what decides
// whether colour is written at all.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func isFalse(value string, hasValue bool) bool {
	if !hasValue {
		return false
	}
	switch strings.TrimSpace(value) {
	case "false", "0", "no", "off":
		return true
	}
	return false
}
