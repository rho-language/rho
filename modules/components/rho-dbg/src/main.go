//
// @file main.go
// @brief `rho dbg` — a debugging session an editor can drive.
//
// The command an editor runs and speaks to. There is one mode that matters: with
// `--dap` this program becomes a debug adapter, reads the Debug Adapter Protocol
// on standard input, starts a debugger and a probe, and answers with what it says.
//
// What a session runs on is decided from facts rather than from a launch
// configuration: the board's target spec says which debugger and which probe, an
// enabled plugin says when a board is better served by a debugger this component
// has never heard of, and a person who disagrees can name one on the command line.
// Everything else this file does is read that decision's flags and report what this
// machine has — because the difference between "this board cannot be debugged" and
// "openocd is not installed" is a fact a person can act on, and neither is visible
// from a session that failed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	modcore "rho/cmd/rho-module/core"

	"rho/src/deps"
	"rho/src/plugins"
	"rho/src/targetspec"

	rholog "github.com/rho/rho-log/src/logger"

	"github.com/rho/rho-dbg/src/rhodbg"
)

// log is this component's own logger, so a session is identifiable in a log that
// several components write to without every line having to say so.
var log = rholog.Default("rho-dbg")

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	// `--dap` rather than a subcommand, because this is how an editor starts a
	// debug adapter: it runs the program with the flag and speaks to its standard
	// input. A subcommand would be one more thing for an editor's configuration to
	// have to get right before anything works.
	switch args[0] {
	case "--dap", "dap":
		return dap(args[1:])
	case "engines":
		return engines(args[1:])
	case "check", "doctor":
		return check(args[1:])
	case "registers", "peripherals":
		return registers(args[1:])
	case "svd":
		return svd(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	}
	fmt.Fprintf(os.Stderr, "rho dbg: unknown command %q\n", args[0])
	usage()
	return 2
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho dbg --dap [options]      serve the Debug Adapter Protocol on stdin/stdout")
	fmt.Fprintln(os.Stderr, "       rho dbg engines [-target n]  what this machine can debug a board with")
	fmt.Fprintln(os.Stderr, "       rho dbg check [-target n]    which of those are installed")
	fmt.Fprintln(os.Stderr, "       rho dbg registers [--target n] [--svd file] [-out file]")
	fmt.Fprintln(os.Stderr, "                                 the chip's register map, as JSON")
	fmt.Fprintln(os.Stderr, "       rho dbg svd [--target n] [--source Vendor/Device.svd] [--refresh]")
	fmt.Fprintln(os.Stderr, "                         put a chip's register map in the cache")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "options a session may be started with:")
	fmt.Fprintln(os.Stderr, "  --program PATH        the built binary (an editor's `program`)")
	fmt.Fprintln(os.Stderr, "  --target NAME         the board, as its target spec names it")
	fmt.Fprintln(os.Stderr, "  --engine NAME         a debugger this machine has (see `rho dbg engines`)")
	fmt.Fprintln(os.Stderr, "  --cwd DIR             the project directory")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "The board's target spec says which debugger and which probe to use, and a")
	fmt.Fprintln(os.Stderr, "plugin may offer another. `rho ide vscode` writes the launch configuration")
	fmt.Fprintln(os.Stderr, "an editor needs.")
}

// options are what a session may be started with.
//
// Read by hand rather than with a flag package, because this command is started by
// an editor rather than by a person: both `-flag value` and `--flag=value` arrive,
// and a flag package that refused the second would fail on configurations that are
// perfectly ordinary — which is every one an editor writes itself.
type options struct {
	program     string
	target      string
	cwd         string
	engine      string
	debugger    string
	serverPath  string
	serverArgs  []string
	device      string
	iface       string
	configFiles []string
	svd         string
	port        string
	malloc      string
	load        bool
	loadSet     bool
	attach      bool
	stopAtEntry bool
}

func parse(args []string) (options, error) {
	chosen := options{}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "-") {
			return chosen, fmt.Errorf("unexpected argument %q", argument)
		}
		name := strings.TrimLeft(argument, "-")
		value := ""
		hasValue := false
		if equals := strings.Index(name, "="); equals >= 0 {
			name, value, hasValue = name[:equals], name[equals+1:], true
		}
		// A flag that takes a value consumes the next argument when it was not given
		// as `name=value`.
		takeValue := func() (string, error) {
			if hasValue {
				return value, nil
			}
			if index+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", argument)
			}
			index++
			return args[index], nil
		}

		var err error
		switch name {
		case "program", "exec":
			chosen.program, err = takeValue()
		case "target":
			chosen.target, err = takeValue()
		case "cwd":
			chosen.cwd, err = takeValue()
		case "engine":
			chosen.engine, err = takeValue()
		case "gdb", "gdbPath", "gdb-path":
			chosen.debugger, err = takeValue()
		case "server", "serverpath":
			chosen.serverPath, err = takeValue()
		case "serverArgs", "server-args":
			var joined string
			joined, err = takeValue()
			chosen.serverArgs = append(chosen.serverArgs, splitArguments(joined)...)
		case "device":
			chosen.device, err = takeValue()
		case "interface", "iface":
			chosen.iface, err = takeValue()
		case "configFiles", "config":
			var joined string
			joined, err = takeValue()
			chosen.configFiles = append(chosen.configFiles, splitArguments(joined)...)
		case "svd", "svdFile":
			chosen.svd, err = takeValue()
		case "port":
			chosen.port, err = takeValue()
		case "malloc":
			chosen.malloc, err = takeValue()
		case "load":
			chosen.load, chosen.loadSet = true, true
		case "no-load":
			chosen.load, chosen.loadSet = false, true
		case "attach":
			chosen.attach = true
		case "stopAtEntry", "stop-at-entry":
			chosen.stopAtEntry = true
		default:
			return chosen, fmt.Errorf("unknown option %q", argument)
		}
		if err != nil {
			return chosen, err
		}
	}
	return chosen, nil
}

// splitArguments is a list of arguments written in one flag, split on whitespace
// with quotes honoured.
//
// Honoured because a config file path on a Windows machine is the one case where
// the argument contains a space, and splitting it wrong produces a probe server
// that cannot find its own configuration.
func splitArguments(value string) []string {
	out := []string{}
	current := &strings.Builder{}
	quote := rune(0)
	for _, c := range value {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
				continue
			}
			current.WriteRune(c)
		case c == '\'' || c == '"':
			quote = c
		case c == ' ' || c == '\t':
			if current.Len() > 0 {
				out = append(out, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  The mode an editor runs                                                    */
/* -------------------------------------------------------------------------- */

// dap serves the protocol on standard input and output.
func dap(args []string) int {
	chosen, err := parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho dbg:", err)
		return 2
	}

	codec := rhodbg.NewCodec(os.Stdin, os.Stdout)
	session, err := open(codec, chosen)
	if err != nil {
		// Said on the console as well as on stderr, because the editor is waiting
		// for a session that is not going to arrive and a message in its debug
		// console is the only place a person is looking.
		fmt.Fprintln(os.Stderr, "rho dbg:", err)
		_ = codec.Output(rhodbg.OutputStderr, "rho dbg: "+err.Error()+"\n")
		return 1
	}
	defer session.Close()

	// The session's own beginning, in its console: every later message is read next
	// to it, and a session that says what it is can have its later oddity traced
	// back to a choice it made here.
	for _, line := range session.Opening() {
		_ = codec.Output(rhodbg.OutputConsole, line+"\n")
	}
	log.Info("debugging session started", "target", chosen.target, "program", chosen.program)

	if err := session.Serve(); err != nil {
		fmt.Fprintln(os.Stderr, "rho dbg:", err)
		return 1
	}
	return 0
}

// open is a whole session, from the options an editor was started with.
func open(codec *rhodbg.Codec, chosen options) (*rhodbg.Session, error) {
	root := chosen.cwd
	if root == "" {
		root = "."
	}

	request := rhodbg.OpenRequest{
		Program:     chosen.program,
		Root:        root,
		VendorRoots: vendorRoots(root),
		TargetName:  chosen.target,
		Engine:      chosen.engine,
		Debugger:    chosen.debugger,
		ServerPath:  chosen.serverPath,
		ServerArgs:  chosen.serverArgs,
		ConfigFiles: chosen.configFiles,
		Device:      chosen.device,
		Interface:   chosen.iface,
		Malloc:      chosen.malloc,
		Port:        chosen.port,
		Plugins:     plugins.Source{Tree: root, RhoHome: deps.RhoHome()},
	}
	if chosen.loadSet {
		request.Load = &chosen.load
	}

	// The board, if there is one. A session that named no board is a session on this
	// machine, which needs no probe and has no register map — said rather than
	// guessed, because treating a host program as a board would try to start a
	// probe server and wait twenty seconds for it to listen.
	if chosen.target != "" {
		spec, err := lookupTarget(chosen.target, root)
		if err != nil {
			return nil, err
		}
		request.Target = spec
	} else {
		request.TargetName = "native"
	}

	return rhodbg.Open(request, codec)
}

/* -------------------------------------------------------------------------- */
/*  What this machine has                                                      */
/* -------------------------------------------------------------------------- */

func engines(args []string) int {
	target := namedArg(args, "-target")
	available := rhodbg.Discover(pluginSource(""))

	// JSON, for a caller that has to choose rather than read. An editor's debug
	// dropdown offers one entry per engine a machine has, and a program cannot
	// choose from a table written for a person.
	if len(args) > 0 && args[0] == "-json" {
		answer := struct {
			Engines []struct {
				Name    string `json:"name"`
				Kind    string `json:"kind"`
				Comment string `json:"comment,omitempty"`
				Program string `json:"program,omitempty"`
			} `json:"engines"`
		}{}
		answer.Engines = append(answer.Engines, struct {
			Name    string `json:"name"`
			Kind    string `json:"kind"`
			Comment string `json:"comment,omitempty"`
			Program string `json:"program,omitempty"`
		}{Name: rhodbg.BuiltinGDB, Kind: rhodbg.KindDriver, Comment: builtinComment})
		for _, spec := range available.FromPlugins {
			comment := spec.Comment
			if comment == "" {
				comment = "provided by a plugin"
			}
			answer.Engines = append(answer.Engines, struct {
				Name    string `json:"name"`
				Kind    string `json:"kind"`
				Comment string `json:"comment,omitempty"`
				Program string `json:"program,omitempty"`
			}{Name: spec.Name, Kind: spec.Kind, Comment: comment, Program: spec.Program})
		}
		encoded, err := json.MarshalIndent(answer, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg engines:", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}

	fmt.Println("Debuggers available:")
	fmt.Printf("  %-22s %s\n", rhodbg.BuiltinGDB, builtinComment)
	for _, spec := range available.FromPlugins {
		line := spec.Comment
		if line == "" {
			line = "provided by a plugin"
		}
		fmt.Printf("  %-22s %s\n", spec.Name, line)
	}
	if len(available.FromPlugins) == 0 {
		fmt.Println("  (no plugin offers another debugger)")
		fmt.Println("  a module in modules/plugins declaring `extends: rho-dbg` with the `engine`")
		fmt.Println("  contract adds one: the debugger's own adapter, run and relayed to")
	}
	if len(available.Sources) > 0 {
		fmt.Println()
		fmt.Println("Asked:", strings.Join(available.Sources, ", "))
	}

	if target != "" {
		spec, err := available.Choose(target, "")
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg engines:", err)
			return 1
		}
		fmt.Println()
		fmt.Printf("For %s: %s\n", target, spec.Name)
	}
	return 0
}

// check says which of the programs a board's session needs are installed.
//
// Because from inside a session the three ways it can fail look the same: a board
// that is not connected, a debugger that is not installed, and a probe server that
// is not installed all end as a connection that was refused. This is where they are
// told apart, before anything has been started.
// builtinComment is what the built-in engine is called in a listing.
const builtinComment = "GDB/MI, carried by this component"

func check(args []string) int {
	target := namedArg(args, "-target")
	if target == "" {
		target = "native"
	}
	root := namedArg(args, "-project")
	if root == "" {
		root = namedArg(args, "-cwd")
	}
	if root == "" {
		root = "."
	}
	spec, err := lookupTarget(target, root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho dbg check:", err)
		return 1
	}

	debugger := spec.Debug.Tool
	if debugger == "" {
		debugger = targetspec.DebuggerFor(spec.Toolchain.Compiler)
	}
	missing := 0
	// A debugger is reported as the program it resolved to, not as the one that was
	// asked for: a machine that has gdb-multiarch and not arm-none-eabi-gdb can
	// debug this board, and saying "not installed" would send somebody to install
	// something they already have.
	report := func(name string, needed bool, debugger bool) {
		if !needed {
			return
		}
		found, err := rhodbg.ResolveProgram(name)
		if debugger {
			found, err = rhodbg.ResolveDebugger(name)
		}
		if err != nil {
			fmt.Printf("  %-24s not installed\n", name)
			missing++
			return
		}
		if filepath.Base(found) == name {
			fmt.Printf("  %-24s installed\n", name)
			return
		}
		fmt.Printf("  %-24s not installed; using %s, which is\n", name, filepath.Base(found))
	}

	fmt.Printf("%s:\n", target)
	if debugger == "" {
		fmt.Printf("  %-24s the target names no debugger, and its compiler is not known either\n", "(no debugger)")
		missing++
	} else {
		report(debugger, true, true)
	}
	report(spec.Debug.Server, spec.Debug.Watcher(), false)

	// The register map, and where it is: in the project's own manifest, in a file a
	// repository carries, or in the cache this machine fetched once. Said either
	// way, because a peripheral view that comes up empty is the one failure a
	// session does not report.
	label, where := registerMapWhere(root, target, spec)
	fmt.Printf("  %-24s %s\n", label, where)
	if len(availableEngines()) > 0 {
		names := []string{}
		for _, name := range availableEngines() {
			names = append(names, name)
		}
		fmt.Printf("  %-24s %s\n", "also available", strings.Join(names, ", "))
	}

	fmt.Println()
	if missing == 0 {
		fmt.Printf("%s can be debugged here.\n", target)
		return 0
	}
	fmt.Printf("%s cannot be debugged here yet: %d of the programs it needs are not installed.\n", target, missing)
	return 1
}

// svd puts a chip's register map in this machine's cache.
//
// On purpose rather than because a session needed one: a session fetches a map
// itself when nothing has it, and this is for the case where a machine should hold
// it before somebody presses F5 — an image that is going on the machine anyway, or
// a project whose sessions should not be the thing that reaches for the network.
func svd(args []string) int {
	root := namedArg(args, "-cwd")
	if root == "" {
		root = namedArg(args, "-project")
	}
	if root == "" {
		root = "."
	}
	refresh := false
	for _, argument := range args {
		if argument == "--refresh" || argument == "-refresh" {
			refresh = true
		}
	}

	settings := rhodbg.ReadSettings()
	declared := namedArg(args, "-source")
	if declared == "" {
		declared = namedArg(args, "-svdSource")
	}
	target := namedArg(args, "-target")
	if declared == "" && target != "" {
		spec, err := lookupTarget(target, root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg svd:", err)
			return 1
		}
		declared = spec.Debug.SVDSource
	}

	// The project's own entry for this board wins outright, as it does everywhere
	// else: a project that has named the file for the board it is built for has said
	// which one is right, and a command that went looking for another would be
	// arguing with it.
	if project := projectDebug(root); project.PathFor(target) != "" && declared == "" {
		found, err := rhodbg.FindRegisterMap(rhodbg.TargetFacts{
			Project:     project,
			TargetName:  target,
			Root:        root,
			Program:     namedArg(args, "-program"),
			VendorRoots: vendorRoots(root),
			Settings:    settings,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg svd:", err)
			return 1
		}
		fmt.Println(found.Path)
		return 0
	}

	// A part number, worked out against what the vendor publishes. The same search a
	// session does, so `rho dbg svd` and a session on the same board cannot end up
	// with different answers to which file this chip's registers are in.
	if declared == "" && target != "" {
		spec, err := lookupTarget(target, root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg svd:", err)
			return 1
		}
		part := partOfSpec(spec)
		if part == "" {
			fmt.Fprintf(os.Stderr, "rho dbg svd: %s carries no part number, so there is nothing to look up\n", target)
			fmt.Fprintln(os.Stderr, "  name the file with --source, or point at it in the project's module.yml")
			return 2
		}
		source, tried, err := rhodbg.ResolveDeviceForPart(part, settings)
		if err != nil {
			// Said as what it is: a part the published set does not have, a name two
			// vendors use, or a list that could not be read. All three are refusals
			// with different next steps, and the message tells them apart.
			fmt.Fprintln(os.Stderr, "rho dbg svd:", rhodbg.CannotIdentify(target, part, tried, err))
			return 1
		}
		declared = source.Path
	}

	source, ok := rhodbg.ParseSVDSource(declared)
	if !ok {
		fmt.Fprintln(os.Stderr,
			"rho dbg svd: name the file in the published set, as Vendor/Device.svd")
		fmt.Fprintf(os.Stderr, "  where it comes from is %s\n", settings.Repository)
		fmt.Fprintln(os.Stderr, "  a project's own module.yml says it with `debug:`,")
		fmt.Fprintln(os.Stderr, "  a target's `svd_source:` says it for a board, and --source says it here")
		return 2
	}

	if refresh {
		if err := rhodbg.ForgetRegisterMap(source, settings); err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg svd:", err)
			return 1
		}
	}

	if cached, found := rhodbg.CachedRegisterMap(source, settings); found && !refresh {
		fmt.Println(cached)
		fmt.Fprintf(os.Stderr, "rho dbg svd: already in the cache from %s\n", settings.Repository)
		return 0
	}

	// A fetch asked for on purpose is a fetch even when the setting says not to:
	// somebody typed it.
	forced := settings
	forced.Fetch = true
	path, err := rhodbg.EnsureRegisterMap(source, forced)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho dbg svd:", err)
		return 1
	}
	fmt.Println(path)
	fmt.Fprintf(os.Stderr, "rho dbg svd: fetched %s into %s\n", source.Path, settings.CacheDirectory())
	return 0
}

// registers answers with a chip's register map as JSON.
//
// Not a dump of the SVD file: the file is XML, and everything that wants to show a
// peripheral — an editor's tree, a script, a test — would otherwise need its own
// reader for it. Read once here, where the reader is tested, and answered in the
// shape those callers already read.
func registers(args []string) int {
	root := namedArg(args, "-cwd")
	if root == "" {
		root = namedArg(args, "-project")
	}
	if root == "" {
		root = "."
	}
	out := namedArg(args, "-out")
	declared := namedArg(args, "-svd")
	if declared == "" {
		declared = namedArg(args, "-svdFile")
	}
	target := namedArg(args, "-target")

	var spec *targetspec.Target
	if target != "" {
		found, err := lookupTarget(target, root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho dbg registers:", err)
			return 1
		}
		spec = found
	}

	found, err := rhodbg.FindRegisterMap(rhodbg.TargetFacts{
		Project:     projectDebug(root),
		TargetName:  target,
		Target:      spec,
		Root:        root,
		Program:     namedArg(args, "-program"),
		VendorRoots: vendorRoots(root),
		Settings:    rhodbg.ReadSettings(),
		FromPlugin: func() (string, string) {
			return rhodbg.RegisterMapFromPlugins(pluginSource(root), target, declared)
		},
	})
	if err != nil {
		if errors.Is(err, rhodbg.ErrNoRegisterMap) {
			fmt.Fprintf(os.Stderr, "rho dbg registers: %s carries no register map and names no part\n", describeName(target))
			return 1
		}
		fmt.Fprintln(os.Stderr, "rho dbg registers:", rhodbg.CannotIdentify(target, partOfSpec(spec), nil, err))
		return 1
	}
	if found.Path == "" {
		// Said rather than answered with an empty map: an empty one is a chip with
		// no peripherals, which is a different claim from a chip whose register map
		// this machine has not been given.
		fmt.Fprintf(os.Stderr,
			"rho dbg registers: %s carries no register map, and nothing says where one is.\n"+
				"  A CMSIS-SVD file from the vendor, named by the target's debug block, is what is read.\n",
			describeName(target))
		return 1
	}

	load, err := rhodbg.LoadSVD(found.Path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho dbg registers:", err)
		return 1
	}
	if out == "" || out == "-" {
		fmt.Fprintf(os.Stderr, "rho dbg registers: %s, from %s\n", found.Path, found.From)
	}
	if err := rhodbg.WriteRegisters(os.Stdout, out, rhodbg.DescribeRegisters(load, found.Path, found.Comment)); err != nil {
		fmt.Fprintln(os.Stderr, "rho dbg registers:", err)
		return 1
	}
	if out != "" && out != "-" {
		fmt.Fprintln(os.Stderr, "rho dbg registers: wrote", out)
	}
	return 0
}

// describeName is what a target was asked for, said as a noun a sentence can use.
func describeName(target string) string {
	if strings.TrimSpace(target) == "" {
		return "this machine"
	}
	return fmt.Sprintf("the target %q", target)
}

// registerMapWhere says where a board's register map is, and what it is called —
// which are the two questions a person asking this actually has, and both of which
// are answered by the same search a session would do.
//
// Nothing is fetched here: this is a report about what a session would find, and a
// report that downloaded four megabytes to answer it would be a surprising thing
// for a command whose name means "tell me".
func registerMapWhere(root string, target string, spec *targetspec.Target) (string, string) {
	settings := rhodbg.ReadSettings()
	label := "(register map)"

	if project := projectDebug(root); project.PathFor(target) != "" {
		label = project.PathFor(target)
		if path, err := rhodbg.FindRegisterMap(rhodbg.TargetFacts{
			Project: project, TargetName: target, Root: root,
			VendorRoots: vendorRoots(root), Settings: settings,
		}); err == nil {
			return label, fmt.Sprintf("named by the project, and read from %s", path)
		}
		return label, "named by the project, and not there"
	}
	if declared := strings.TrimSpace(spec.Debug.SVD); declared != "" {
		label = declared
		if _, found := rhodbg.RegisterMapPath(declared, root, "", vendorRoots(root)); found {
			return label, "carried by the project or by a platform it depends on"
		}
		return label, "named by the target, and not here"
	}
	if declared := strings.TrimSpace(spec.Debug.SVDSource); declared != "" {
		label = declared
	}
	if part := partOfSpec(spec); part != "" {
		label = part
		source, tried, err := rhodbg.ResolveDeviceForPartCached(part, settings)
		switch {
		case err == nil:
			if _, cached := rhodbg.CachedRegisterMap(source, settings); cached {
				return source.Path, fmt.Sprintf("the published set; %s is in the cache (%s)", part, settings.CacheDirectory())
			}
			if settings.Fetchable() {
				return source.Path, fmt.Sprintf("the published set; fetched once into %s", settings.CacheDirectory())
			}
			return source.Path, "the published set; this component will not fetch one"
		case len(tried) > 0:
			// The list was read and the part is not in it: a fact about the vendor,
			// and one a person needs in full because it is not a fact about their
			// spelling.
			return label, fmt.Sprintf("not in the published set under any of %s",
				strings.Join(tried, ", "))
		default:
			// The list itself could not be read, which is a fact about this machine
			// and says nothing about the part. Saying otherwise would send somebody
			// looking for a chip that is published.
			return label, fmt.Sprintf("the published set could not be read: %v", err)
		}
	}
	return label, "none named and no part known, so the peripheral view will be empty"
}

func availableEngines() []string {
	return rhodbg.Discover(pluginSource("")).FromPlugins2Names()
}

// namedArg is the value a flag was given, spelled either way.
//
// Both spellings because a person types both and the help says the double one:
// `--target pico` is the ordinary way to write it, and refusing it would make the
// documented form the one that does not work.
func namedArg(args []string, name string) string {
	bare := strings.TrimLeft(name, "-")
	for index, argument := range args {
		if !strings.HasPrefix(argument, "-") {
			continue
		}
		name := strings.TrimLeft(argument, "-")
		// `--flag=value`, which is how an editor writes one and is the only form
		// with no next argument to be missing.
		if flag, value, found := strings.Cut(name, "="); found {
			if flag == bare {
				return value
			}
			continue
		}
		if name == bare && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  Resolving what a board is                                                  */
/* -------------------------------------------------------------------------- */

// lookupTarget asks rho-target about a board.
//
// A component call rather than a copy of the specs, because which probe a board
// has and where its register map lives are facts rho-target holds and its plugins
// extend, and a session that carried its own list would be a second list to keep in
// step with the first.
func lookupTarget(name string, project string) (*targetspec.Target, error) {
	module, err := modcore.LoadInstalledModule("rho-target")
	if err != nil {
		return nil, fmt.Errorf("a debugging session needs the rho-target component, which is not installed: %w", err)
	}
	entry := module.Entry()
	if entry == nil {
		return nil, fmt.Errorf("the rho-target component has no entry point")
	}

	// Both streams are captured rather than connected to this process's own.
	// Standard output here is the protocol an editor is reading, and standard error
	// is the toolchain's — where a line saying which component failed would arrive
	// beside a session and read as though the session had failed.
	// The project is named, because a board a project declares is not in this
	// component's own list: `rho target show` reads a project's platform
	// declarations when it is told which project, and the toolchain that built this
	// session is the one that knows.
	args := []string{"show", "-json", name}
	if strings.TrimSpace(project) != "" {
		args = []string{"show", "-json", "-project", project, name}
	}

	said := &bytes.Buffer{}
	// Standard output is captured because it is the answer. Standard error goes to
	// this process's own, where it belongs: the component is saying what it thinks
	// about the board, and a session that printed nothing of it would be hiding a
	// component's own complaint.
	runErr := entry.Run(args, os.Stdin, writeOnly(said))
	if runErr != nil {
		message := strings.TrimSpace(said.String())
		if message == "" {
			return nil, fmt.Errorf("rho target show %s: %w", name, runErr)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return targetspec.ParseTarget(said.Bytes())
}

// writeOnly is an os.File a component's output can be read through.
//
// The entry point takes files, which is right for a program whose output is a
// person reading it, and wrong for this one: a component dispatched by a debugging
// adapter writes into a protocol another program is speaking, and a line arriving
// in the middle of a session is a message the editor will read as part of it.
func writeOnly(buffer *bytes.Buffer) *os.File {
	read, write, err := os.Pipe()
	if err != nil {
		return nil
	}
	go func() {
		defer read.Close()
		_, _ = io.Copy(buffer, read)
	}()
	return write
}

// vendorRoots are the vendor trees of the platforms a project depends on, which is
// where a register map a platform carries lives.
// projectDebug is what the project's own manifest says about its chip, which is
// the answer when it says anything.
func projectDebug(root string) *rhodbg.DebugMapping {
	manifest, err := deps.LoadManifest(deps.ManifestPath(root))
	if err != nil || manifest == nil || manifest.Debug == nil {
		return nil
	}
	return manifest.Debug
}

// partOfSpec is the part number a board carries.
func partOfSpec(spec *targetspec.Target) string {
	if spec == nil {
		return ""
	}
	return spec.MCU
}

func vendorRoots(root string) []string {
	manifest, err := deps.LoadManifest(deps.ManifestPath(root))
	if err != nil || manifest == nil {
		return nil
	}
	return manifest.VendorRoots(root)
}

// pluginSource is where the debuggers a session may use are found.
func pluginSource(root string) plugins.Source {
	if root == "" {
		root = "."
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	return plugins.Source{Tree: root, RhoHome: deps.RhoHome()}
}
