package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/internal/paths"
	"rho/src/integration"
	"rho/src/plugins"
	"rho/src/protocol"
)

// platformBinding is the file of one binding kind the platform's integration
// plugin provides (see src/integration).
func platformBinding(platform, kind string) (string, error) {
	home, _ := paths.GetRhoHome()
	cwd, _ := os.Getwd()
	return integration.Binding(plugins.Source{Tree: cwd, RhoHome: home, Platform: platform}, kind)
}

// protocolCLI turns a Rho schema into the C and Go code both ends of a link
// need. It is a command of its own rather than a build target because a schema
// is written once and generated for sides that are built apart — a node
// toolchain and a server toolchain that never see each other.
func protocolCLI(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: rho protocol generate [--target c|go|rho] [--out DIR] [--package NAME] [--bus FILE] [--transport FILE] [--platform NAME] <file-or-dir>...")
		return 2
	}
	switch args[0] {
	case "generate":
		return protocolGenerate(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown protocol subcommand:", args[0])
		return 2
	}
}

func protocolGenerate(args []string) int {
	fs := flag.NewFlagSet("protocol generate", flag.ContinueOnError)
	target := fs.String("target", "c", "the code to generate: c, go or rho")
	out := fs.String("out", "", "where to write it (default build/protocol/<target>)")
	packageName := fs.String("package", "protocol", "the Go package name")
	busFile := fs.String("bus", "", "a bus binding file: generates the event-bus files for the c target")
	transportFile := fs.String("transport", "", "a framing file: adds the wire framing to the rho target")
	platform := fs.String("platform", "", "a platform whose integration plugin provides the bindings")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	paths := fs.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "protocol generate: a file or directory is required")
		return 2
	}

	files, err := schemaFiles(paths)
	if err != nil {
		fmt.Fprintln(os.Stderr, "protocol generate:", err)
		return 1
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "protocol generate: no .rho files found")
		return 1
	}

	// Every file is read into one schema, and every schema it includes with it:
	// a schema may be split across files — the shared identity in one, a level's
	// messages in another — and the wire does not care which file a declaration
	// was written in. A name declared in two of them is one declaration.
	merged := &protocol.Schema{}
	for _, file := range files {
		schema, parseErr := protocol.LoadFile(file)
		if parseErr != nil {
			fmt.Fprintln(os.Stderr, "protocol generate:", parseErr)
			return 1
		}
		protocol.Merge(merged, schema)
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join("build", "protocol", *target)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "protocol generate:", err)
		return 1
	}

	var written []string
	switch *target {
	case "c":
		header, source := merged.EmitC()
		written = []string{
			filepath.Join(outDir, "protocol.h"),
			filepath.Join(outDir, "protocol.c"),
		}
		contents := []string{header, source}
		// The bus files name a runtime's own types and functions, so they are
		// generated only for a binding the platform ships; with none, the bytes
		// are framed and nothing is assumed about who carries them.
		bindingPath := *busFile
		if bindingPath == "" && *platform != "" {
			found, findErr := platformBinding(*platform, integration.BindingProtocolBusC)
			if findErr != nil {
				fmt.Fprintln(os.Stderr, "protocol generate:", findErr)
				return 1
			}
			bindingPath = found
		}
		if bindingPath != "" {
			bus, loadErr := protocol.LoadBus(bindingPath)
			if loadErr != nil {
				fmt.Fprintln(os.Stderr, "protocol generate:", loadErr)
				return 1
			}
			busHeader, busSource := merged.EmitCBus(bus)
			written = append(written, filepath.Join(outDir, "protocol_bus.h"), filepath.Join(outDir, "protocol_bus.c"))
			contents = append(contents, busHeader, busSource)
		}
		for i, path := range written {
			if err := os.WriteFile(path, []byte(contents[i]), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "protocol generate:", err)
				return 1
			}
		}
	case "rho":
		transport := ""
		framing := *transportFile
		if framing == "" && *platform != "" {
			found, findErr := platformBinding(*platform, integration.BindingProtocolTransportRho)
			if findErr != nil {
				fmt.Fprintln(os.Stderr, "protocol generate:", findErr)
				return 1
			}
			framing = found
		}
		if framing != "" {
			data, readErr := os.ReadFile(framing)
			if readErr != nil {
				fmt.Fprintln(os.Stderr, "protocol generate:", readErr)
				return 1
			}
			transport = string(data)
		}
		source, emitErr := merged.EmitRho(transport)
		if emitErr != nil {
			fmt.Fprintln(os.Stderr, "protocol generate:", emitErr)
			return 1
		}
		written = []string{filepath.Join(outDir, "protocol.rho")}
		if err := os.WriteFile(written[0], []byte(source), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "protocol generate:", err)
			return 1
		}
	case "go":
		source := []byte(merged.EmitGo(*packageName))
		// Run it through the Go formatter, so the generated file is one a `gofmt`
		// check passes without a separate step.
		if formatted, formatErr := format.Source(source); formatErr == nil {
			source = formatted
		}
		written = []string{filepath.Join(outDir, "protocol.go")}
		if err := os.WriteFile(written[0], source, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "protocol generate:", err)
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "protocol generate: unknown target", *target)
		return 2
	}

	for _, file := range written {
		fmt.Println(file)
	}
	return 0
}

// schemaFiles lists the .rho files under the given paths, in a stable order.
func schemaFiles(paths []string) ([]string, error) {
	files := []string{}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, path)
			continue
		}
		err = filepath.Walk(path, func(candidate string, entry os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(candidate, ".rho") {
				files = append(files, candidate)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}
