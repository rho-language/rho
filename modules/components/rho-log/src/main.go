//
// @file main.go
// @brief Rho log component.
//
// Renders log records through the shared formatting core, so the output a
// device writes to a serial console and a server writes to stdout are defined
// in one place.
//
// Usage:
//
//	rho log -format text -level info -subject registry -message started -field port=8080
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rho/rho-fmt/src/logfmt"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("rho log", flag.ExitOnError)
	format := fs.String("format", "text", "output format: text or json")
	levelName := fs.String("level", "info", "level: trace, debug, info, warn, error, fatal")
	subject := fs.String("subject", "", "subject, usually a module or subsystem")
	message := fs.String("message", "", "the message")
	stamp := fs.String("stamp", "", "a pre-rendered timestamp, for a source with no clock")
	fields := fs.String("field", "", "a field as key=value, repeatable with commas")
	noLevel := fs.Bool("no-level", false, "omit the level, for a compact line")
	fs.Parse(args)

	level, ok := logfmt.ParseLevel(*levelName)
	if !ok {
		fmt.Fprintf(os.Stderr, "rho log: unknown level %q\n", *levelName)
		return 2
	}

	record := logfmt.Record{
		Level:     level,
		Subject:   *subject,
		Message:   *message,
		StampText: *stamp,
		Fields:    parseFields(*fields),
	}
	if *stamp == "" {
		record.Stamp = logfmt.Now()
	}

	var formatter logfmt.Formatter
	switch *format {
	case "text":
		options := logfmt.DefaultOptions()
		options.ShowLevel = !*noLevel
		formatter = logfmt.NewText(options)
	case "json":
		formatter = logfmt.DefaultJSON()
	default:
		fmt.Fprintf(os.Stderr, "rho log: unknown format %q\n", *format)
		return 2
	}

	os.Stdout.Write(formatter.Format(record))
	os.Stdout.Write([]byte("\n"))
	return 0
}

// parseFields reads a comma separated list of key=value pairs.
func parseFields(list string) []logfmt.Field {
	if strings.TrimSpace(list) == "" {
		return nil
	}
	fields := []logfmt.Field{}
	for _, pair := range strings.Split(list, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, found := strings.Cut(pair, "=")
		if !found {
			fmt.Fprintf(os.Stderr, "rho log: field %q is not key=value\n", pair)
			continue
		}
		fields = append(fields, logfmt.Field{Key: strings.TrimSpace(key), Value: value})
	}
	return fields
}
