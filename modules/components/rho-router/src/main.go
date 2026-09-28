//
// @file main.go
// @brief rho route: the HTTP surface of a Rho web service.
//
// A Rho application declares its routes and its handlers, which means it also
// declares what it exposes and what each operation takes. This reads that and
// prints it in the forms a person or a tool needs:
//
//	rho route list <project>            what the service answers
//	rho route show <project> <op>       one operation in full
//	rho route spec <project>            the metadata, as JSON
//	rho route openapi <project> -out f  the metadata as an OpenAPI 3.1 document
//	rho route accessors                 the typed accessors a handler calls
//	rho route -list-plugins             the plugins that extend the reading
//
// The reading is the same one a build would use, because it is the compiler's
// own parser: a route is read here exactly as it is compiled. The OpenAPI
// document and the metadata are two renderings of one reading, so they cannot
// disagree with each other or with the service.
//
// A build writes the document rather than shelling out to this command for it.
// The port supplies the router, the binding and the middleware as generated Go,
// so a generated project builds with the Go toolchain alone; this command is
// what a person reaches for, and what `rho build`'s OpenAPI step runs.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rholog "github.com/rho/rho-log/src/logger"
	"github.com/rho/rho-router/src/rhooas"
	"github.com/rho/rho-router/src/rhorouter"
)

// log is the command's own logger, so a run is identifiable in a shared log
// without every line having to say so.
var log = rholog.Default("rho-router")

func main() {
	os.Exit(run(os.Args[1:]))
}

// usage is what the command prints when asked.
const usage = `rho route - the HTTP surface of a Rho web service

usage:
  rho route list <project>              what the service answers
  rho route show <project> <operation>  one operation in full
  rho route spec <project>              the route metadata, as JSON
  rho route openapi <project>           the metadata as an OpenAPI 3.1 document
  rho route accessors                   the typed accessors a handler calls
  rho route -list-plugins               the plugins that extend the reading

flags:
  -out <path>       write to a file rather than to standard output
  -server <url>     the address the service answers at, for the document
  -base <prefix>    the prefix every path is served under
  -tag <name>       the group a plugin filters by, for -list-plugins
  -list-plugins     list the plugins that extend this component
  -plain            report in a form a script can read
`

// run dispatches one invocation and returns the process's exit status.
func run(args []string) int {
	flags := flag.NewFlagSet("route", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	out := flags.String("out", "", "write to a file rather than to standard output")
	server := flags.String("server", "", "the address the service answers at")
	base := flags.String("base", "", "the prefix every path is served under")
	title := flags.String("title", "", "the name the document calls the service")
	listPlugins := flags.Bool("list-plugins", false, "list the plugins that extend this component")
	plain := flags.Bool("plain", false, "report in a form a script can read")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *listPlugins {
		return reportPlugins(flags.Args(), *plain)
	}

	rest := flags.Args()
	if len(rest) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	command, arguments := rest[0], rest[1:]
	switch command {
	case "list":
		return reportList(arguments, *out, *plain)
	case "show":
		return reportShow(arguments, *out, *plain)
	case "spec":
		return reportSpec(arguments, *out, *plain)
	case "openapi":
		return reportOpenAPI(arguments, rhooas.OpenAPIOptions{
			Server: *server, BasePath: *base, Title: *title,
		}, *out, *plain)
	case "accessors":
		return reportAccessors(*out, *plain)
	case "help", "-h", "--help":
		fmt.Fprint(os.Stdout, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "rho route: unknown command %q\n", command)
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
}

/* -------------------------------------------------------------------------- */
/*  Reading                                                                    */
/* -------------------------------------------------------------------------- */

// read reads a project's routes, running the plugins that extend the reading.
func read(root string, usePlugins bool) (rhorouter.ReadResult, error) {
	if root == "" {
		root = "."
	}
	result, err := rhorouter.Read(rhorouter.ReadOptions{Root: root})
	if err != nil {
		return result, err
	}
	if !usePlugins {
		return result, nil
	}

	found := rhorouter.DiscoverPlugins(pluginSource(root))
	if len(found) == 0 {
		return result, nil
	}
	// The whole-service hook first, so an operation a plugin adds is read
	// through the per-operation hook like any other.
	spec, diagnostics := rhorouter.RunSpec(found, result.Spec)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	for i, operation := range spec.Operations {
		answered, diagnostics := rhorouter.RunOperation(found, spec, operation)
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		spec.Operations[i] = answered
	}
	result.Spec = spec
	return result, nil
}

// pluginSource is where plugins are found. The Rho home is where the toolchain
// installs them, which is the same place every other component looks, and the
// tree is the project being read. A tree's own plugins are not trusted by
// default: a plugin is a program, and one that came with a freshly cloned
// repository came from whoever wrote it.
func pluginSource(tree string) rhorouter.Source {
	home := os.Getenv("RHO_HOME")
	if home == "" {
		if userHome, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(userHome, ".rho")
		}
	}
	return rhorouter.Source{Tree: tree, RhoHome: home}
}

// reportDiagnostics writes what could not be read, which is a warning rather
// than a failure: one source that does not parse is not a reason to refuse to
// describe the others.
func reportDiagnostics(diagnostics []rhorouter.Diagnostic) {
	for _, item := range diagnostics {
		where := item.File
		if where != "" {
			where += ": "
		}
		level := item.Level
		if level == "" {
			level = "warning"
		}
		fmt.Fprintf(os.Stderr, "rho route: %s: %s%s\n", level, where, item.Message)
	}
}

/* -------------------------------------------------------------------------- */
/*  The commands                                                               */
/* -------------------------------------------------------------------------- */

// reportList prints what the service answers, one route per line.
func reportList(args []string, out string, plain bool) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "rho route: list needs a project directory")
		return 2
	}
	result, err := read(args[0], true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
		return 1
	}
	reportDiagnostics(result.Diagnostics)

	var b strings.Builder
	for _, operation := range result.Spec.Operations {
		summary := operation.Summary
		if summary == "" {
			summary = "-"
		}
		summary = strings.SplitN(summary, "\n", 2)[0]
		if plain {
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", operation.Method, operation.Path,
				operation.ID, operation.Tag, summary)
			continue
		}
		fmt.Fprintf(&b, "%-6s %-44s %-22s", operation.Method, operation.Path, operation.ID)
		if operation.Tag != "" {
			fmt.Fprintf(&b, " [%s]", operation.Tag)
		}
		if summary != "-" {
			fmt.Fprintf(&b, "  %s", summary)
		}
		b.WriteString("\n")
	}
	if len(result.Spec.Operations) == 0 {
		b.WriteString("no routes are declared\n")
	}
	return emit(out, b.String())
}

// reportShow prints one operation in full, which is what a person reads when a
// route is not behaving the way the sheet says.
func reportShow(args []string, out string, plain bool) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "rho route: show needs a project directory and an operation")
		return 2
	}
	result, err := read(args[0], true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
		return 1
	}
	reportDiagnostics(result.Diagnostics)

	for _, operation := range result.Spec.Operations {
		if operation.ID != args[1] && operation.Method+" "+operation.Path != args[1] {
			continue
		}
		encoded, err := json.MarshalIndent(operation, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
			return 1
		}
		return emit(out, string(encoded)+"\n")
	}
	fmt.Fprintf(os.Stderr, "rho route: no operation named %q\n", args[1])
	return 1
}

// reportSpec prints the metadata as it was read, which is the contract other
// tools consume.
func reportSpec(args []string, out string, plain bool) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "rho route: spec needs a project directory")
		return 2
	}
	result, err := read(args[0], true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
		return 1
	}
	reportDiagnostics(result.Diagnostics)

	encoded, err := json.MarshalIndent(result.Spec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
		return 1
	}
	return emit(out, string(encoded)+"\n")
}

// reportOpenAPI prints the metadata as an OpenAPI 3.1 document, which is what
// a client generator, a contract test or a portal consumes.
func reportOpenAPI(args []string, options rhooas.OpenAPIOptions, out string, plain bool) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "rho route: openapi needs a project directory")
		return 2
	}
	result, err := read(args[0], true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
		return 1
	}
	reportDiagnostics(result.Diagnostics)

	document := rhooas.OpenAPI(result.Spec, options).String()
	// The finished-document hook runs last, so a plugin sees exactly the text
	// that would otherwise be written.
	found := rhorouter.DiscoverPlugins(pluginSource(args[0]))
	if len(found) > 0 {
		var diagnostics []rhorouter.Diagnostic
		document, diagnostics = rhorouter.RunOpenAPI(found, result.Spec, document)
		reportDiagnostics(diagnostics)
	}
	return emit(out, document)
}

// reportAccessors prints the typed accessors a handler calls, which is the
// vocabulary the reading recognises and the one a document describes.
func reportAccessors(out string, plain bool) int {
	var b strings.Builder
	if plain {
		for _, name := range sorted(rhorouter.AccessorNames()) {
			b.WriteString(name + "\n")
		}
		return emit(out, b.String())
	}
	b.WriteString("A handler reads a request through these, and the type is part of the name:\n\n")
	for _, name := range sorted(rhorouter.AccessorNames()) {
		fmt.Fprintf(&b, "  %-14s %s\n", name, rhorouter.DescribeAccessor(name))
	}
	b.WriteString("\n  bind_ok(req)           whether every value read converted\n")
	b.WriteString("  bind_error(req)        which one did not, and why\n")
	return emit(out, b.String())
}

// reportPlugins lists the plugins that extend the reading.
func reportPlugins(args []string, plain bool) int {
	tree := ""
	if len(args) > 0 {
		tree = args[0]
	}
	found := rhorouter.DiscoverPlugins(pluginSource(tree))
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	if len(found) == 0 {
		if plain {
			return 0
		}
		fmt.Fprintf(os.Stdout, "rho route: no plugin extends %s\n", rhorouter.ComponentName)
		return 0
	}
	for _, plugin := range found {
		if plain {
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", plugin.Name, plugin.Version,
				strings.Join(plugin.Hooks, ","))
			continue
		}
		fmt.Fprintf(os.Stdout, "%-24s %-10s hooks: %s\n", plugin.Name, plugin.Version,
			strings.Join(plugin.Hooks, ", "))
		for _, command := range plugin.Commands {
			fmt.Fprintf(os.Stdout, "    %s -> %s (%s)\n", command.Hook, command.Run, command.Name)
		}
	}
	return 0
}

// emit writes a report to a file or to standard output. A file is written
// whole, so a reader never sees half a document.
func emit(out, content string) int {
	if out == "" {
		fmt.Fprint(os.Stdout, content)
		return 0
	}
	if directory := filepath.Dir(out); directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
			return 1
		}
	}
	if err := os.WriteFile(out, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "rho route: %v\n", err)
		return 1
	}
	log.Info("wrote " + out)
	return 0
}

// sorted returns a copy in order, so a listing is stable whatever order the
// source of the names was built in.
func sorted(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out
}
