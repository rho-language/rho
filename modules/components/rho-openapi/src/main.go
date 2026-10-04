//
// @file main.go
// @brief rho openapi: an API description, and the page a person reads it on.
//
// The reading is rho-router's, because two readings of one service are two
// dialects and they drift. What this command does is write the reading out in
// the two forms that are wanted:
//
//	rho openapi generate <project>        the OpenAPI 3.1 document, as JSON
//	rho openapi page <project>            the reference page, as HTML
//	rho openapi spec <project>            the contract, unchanged from rho-router
//	rho openapi check <project>           whether the description is sound
//
// `generate` is what a client generator, a portal or a contract test reads.
// `page` is what a person reads, and it is the same reading: a page and a
// document are two renderings of one description, so they cannot disagree.
//
// A rho-doc build runs the page rendering for itself through the plugin in
// modules/plugins/rho-doc-api, so an API reference joins the site the rest of
// the documentation is in rather than living beside it. This command is what a
// person reaches for, and what a pipeline that wants the JSON reaches for.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	rholog "github.com/rho/rho-log/src/logger"
	"github.com/rho/rho-openapi/src/rhoapiref"
	"github.com/rho/rho-openapi/src/rhoopenapi"
	"github.com/rho/rho-router/src/rhorouter"
)

// log is the command's own logger, so a run is identifiable in a shared log
// without every line having to say so.
var log = rholog.Default("rho-openapi")

func main() {
	os.Exit(run(os.Args[1:]))
}

// usage is what the command prints when asked.
const usage = `rho openapi - describe a Rho web service

usage:
  rho openapi generate <project>        the OpenAPI 3.1 document, as JSON
  rho openapi page <project>            the API reference page, as HTML
  rho openapi spec <project>            the route metadata, as JSON
  rho openapi check <project>           whether the description is sound

flags:
  -out <path>       write to a file rather than to standard output
  -server <url>     the address the service answers at
  -base <prefix>    the prefix every path is served under
  -title <name>     the name the description calls the service
  -target <id>      the descriptor target to describe, when a project has several
  -plain            report in a form a script can read
`

// run dispatches one invocation and returns the process's exit status.
func run(args []string) int {
	flags := flag.NewFlagSet("openapi", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	out := flags.String("out", "", "write to a file rather than to standard output")
	server := flags.String("server", "", "the address the service answers at")
	base := flags.String("base", "", "the prefix every path is served under")
	title := flags.String("title", "", "the name the description calls the service")
	target := flags.String("target", "", "the descriptor target to describe")
	plain := flags.Bool("plain", false, "report in a form a script can read")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	rest := flags.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	command, arguments := rest[0], rest[1:]
	options := rhoopenapi.OpenAPIOptions{Server: *server, BasePath: *base, Title: *title}
	switch command {
	case "generate":
		return reportDocument(arguments, *target, options, *out, *plain)
	case "page":
		return reportPage(arguments, *target, options, *out, *plain)
	case "spec":
		return reportSpec(arguments, *target, *out, *plain)
	case "check":
		return reportCheck(arguments, *target, *plain)
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "rho openapi: unknown command %q\n", command)
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                     */
/* -------------------------------------------------------------------------- */

// read reads a project's routes into the spec every rendering is written from.
func read(root, target string) (rhorouter.ReadResult, error) {
	if root == "" {
		root = "."
	}
	return rhorouter.Read(rhorouter.ReadOptions{Root: root, Target: target})
}

// reportDiagnostics writes what could not be read, which is a warning rather
// than a failure: one source that does not parse is not a reason to refuse to
// describe the others.
func reportDiagnostics(diagnostics []rhorouter.Diagnostic) int {
	failed := 0
	for _, item := range diagnostics {
		where := item.File
		if where != "" {
			where += ": "
		}
		level := item.Level
		if level == "" {
			level = "warning"
		}
		if level == "error" {
			failed = 1
		}
		fmt.Fprintf(os.Stderr, "rho openapi: %s: %s%s\n", level, where, item.Message)
	}
	return failed
}

/* -------------------------------------------------------------------------- */
/*  The commands                                                                */
/* -------------------------------------------------------------------------- */

// reportDocument writes the OpenAPI document, which is what a tool reads.
func reportDocument(args []string, target string, options rhoopenapi.OpenAPIOptions, out string, plain bool) int {
	argument, status := projectArgument(args, "generate")
	if status != 0 {
		return status
	}
	result, err := read(argument, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
		return 1
	}
	if failed := reportDiagnostics(result.Diagnostics); failed != 0 {
		return failed
	}
	return emit(out, rhoopenapi.OpenAPI(result.Spec, options).String())
}

// reportPage writes the reference page, which is what a person reads.
func reportPage(args []string, target string, options rhoopenapi.OpenAPIOptions, out string, plain bool) int {
	argument, status := projectArgument(args, "page")
	if status != 0 {
		return status
	}
	result, err := read(argument, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
		return 1
	}
	if failed := reportDiagnostics(result.Diagnostics); failed != 0 {
		return failed
	}
	return emit(out, rhoapiref.Render(result.Spec, rhoapiref.ReferenceOptions{
		Server: options.Server, Prefix: options.BasePath,
	}))
}

// reportSpec writes the contract unchanged, which is what a component that
// wants the reading without a rendering of it reads.
func reportSpec(args []string, target string, out string, plain bool) int {
	argument, status := projectArgument(args, "spec")
	if status != 0 {
		return status
	}
	result, err := read(argument, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
		return 1
	}
	if failed := reportDiagnostics(result.Diagnostics); failed != 0 {
		return failed
	}
	encoded, err := json.MarshalIndent(result.Spec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
		return 1
	}
	return emit(out, string(encoded)+"\n")
}

// reportCheck says whether the description is sound, which is what a pipeline
// wants rather than a document it then has to inspect.
//
// The checks are the ones a description can fail at without being wrong about
// the program: an operation with no identifier, two operations claiming one
// identifier, a path parameter nothing captures, a route that answers nothing.
func reportCheck(args []string, target string, plain bool) int {
	argument, status := projectArgument(args, "check")
	if status != 0 {
		return status
	}
	result, err := read(argument, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
		return 1
	}
	failed := reportDiagnostics(result.Diagnostics)

	problems := rhoopenapi.Check(result.Spec)
	for _, problem := range problems {
		fmt.Fprintf(os.Stderr, "rho openapi: %s: %s\n", problem.Operation, problem.Message)
	}
	if len(problems) > 0 {
		failed = 1
	}
	if len(problems) == 0 && failed == 0 {
		if plain {
			fmt.Fprintf(os.Stdout, "ok\t%d\n", len(result.Spec.Operations))
		} else {
			fmt.Fprintf(os.Stdout, "%d operations, no problems\n", len(result.Spec.Operations))
		}
	}
	return failed
}

/* -------------------------------------------------------------------------- */
/*  Plumbers                                                                    */
/* -------------------------------------------------------------------------- */

// projectArgument returns the project a command was given, or reports what it
// needed.
func projectArgument(args []string, command string) (string, int) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "rho openapi: %s needs a project directory\n", command)
		return "", 2
	}
	return args[0], 0
}

// emit writes a rendering to a file or to standard output. A file is written
// whole, so a reader never sees half a document.
func emit(out, content string) int {
	if out == "" {
		fmt.Fprint(os.Stdout, content)
		return 0
	}
	if directory := filepath.Dir(out); directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
			return 1
		}
	}
	if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "rho openapi: %v\n", err)
		return 1
	}
	log.Info("wrote " + out)
	return 0
}
