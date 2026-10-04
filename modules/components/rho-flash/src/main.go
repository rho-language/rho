//
// @file flash.go
// @brief Rho Flash — put a built binary on the board.
//
// What a board needs to be flashed is not a property of the flasher. OpenOCD,
// st-flash, picotool and dfu-util each want a different invocation, and which
// one is right depends on the part, the probe and what the board has soldered
// on it. So the platform says: a target spec carries a `flash` block naming the
// tool and the command, and this program runs it.
//
// It used to carry a switch over MCU names instead, with the tool and the load
// address written into the program. That was a board described twice — once in
// the spec the build used, and once in the flasher — and the two disagreed the
// first time a platform reached its board some other way. The address in
// particular was a constant here that the memory map already knew.
//
// Resolution is rho-target's, because that is where target specs are read and
// resolved for every build. Asking it rather than reading the JSON here is what
// keeps one description of a board: `rho target show -json <name>` is the same
// answer the compiler was given.
//
// @project Rho Programming Language
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
	"os/exec"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
)

// ComponentName is this component's own name, which is what a plugin has to say
// in order to be loaded by it.
const ComponentName = "rho-flash"

// HookFlash is the hook a platform plugin answers with how its boards are
// reached, for the cases a target spec cannot state.
const HookFlash = "flash"

// Flash is the part of a target spec this program needs.
//
// It is read from what rho-target resolved rather than from the spec file, so
// that a spec which inherits — a board inheriting its family — arrives with the
// parts it did not write itself.
type Flash struct {
	Tool      string   `json:"tool,omitempty"`
	Command   string   `json:"command,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
	Offset    string   `json:"offset,omitempty"`
	PostArgs  []string `json:"post_args,omitempty"`
	Comment   string   `json:"comment,omitempty"`
}

// declared reports whether the target says anything about reaching a board.
func (f Flash) declared() bool {
	return strings.TrimSpace(f.Command) != "" || len(f.Arguments) > 0
}

// invocation renders the command for a binary, as the program and its arguments.
//
// The command is split rather than handed to a shell, because it came out of a
// spec that a platform ships: a spec that could name a pipeline would be a spec
// that could run whatever a machine could be talked into.
func (f Flash) invocation(binary string) (string, []string, error) {
	argv := []string{}
	switch {
	case strings.TrimSpace(f.Command) != "":
		argv = splitCommand(f.Command)
	case len(f.Arguments) > 0:
		argv = append(argv, f.Arguments...)
	default:
		return "", nil, fmt.Errorf("the target declares no way to reach the board")
	}
	if len(argv) == 0 {
		return "", nil, fmt.Errorf("the target's flash command is empty")
	}

	out := make([]string, 0, len(argv)+len(f.PostArgs)+1)
	named := false
	for _, arg := range argv {
		if strings.Contains(arg, "{binary}") {
			arg = strings.ReplaceAll(arg, "{binary}", binary)
			named = true
		}
		out = append(out, arg)
	}
	// A command that never says where the file goes would leave the tool to its
	// default, which on some tools is the last build in a directory rather than
	// the one just built.
	if !named {
		out = append(out, binary)
	}
	for _, arg := range f.PostArgs {
		if arg == "{offset}" && f.Offset != "" {
			arg = f.Offset
		}
		out = append(out, arg)
	}
	return out[0], out[1:], nil
}

// splitCommand splits a command into arguments, honouring quotes so a path with
// a space in it survives as one argument.
func splitCommand(command string) []string {
	var (
		args    []string
		current strings.Builder
		quote   rune
		started bool
	)
	for _, r := range command {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			started = true
		case r == ' ' || r == '\t' || r == '\n':
			if started || current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
		}
	}
	if started || current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

/* -------------------------------------------------------------------------- */
/*  Resolution                                                                 */
/* -------------------------------------------------------------------------- */

// resolve asks rho-target what a target is, and returns how to reach it.
//
// The specs are read where every build reads them rather than being opened here,
// so that a board described once is described once. rho-target is a component, so
// it is run as one; a machine that has not installed it is told so rather than
// being handed a default that would be wrong for every board but one.
func resolve(name, project string) (Flash, error) {
	if project == "" {
		project = "."
	}
	args := []string{"target", "show", "-json", "-project", project, name}
	cmd := exec.Command("rho", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return Flash{}, fmt.Errorf("rho target show %s: %w", name, err)
	}

	var spec struct {
		Name  string `json:"name"`
		Flash Flash  `json:"flash"`
	}
	if err := json.Unmarshal(out, &spec); err != nil {
		return Flash{}, fmt.Errorf("rho target show %s returned something that is not a target spec: %w", name, err)
	}
	return spec.Flash, nil
}

/* -------------------------------------------------------------------------- */
/*  Flash                                                                     */
/* -------------------------------------------------------------------------- */

// flashRequest is what a platform plugin is asked: which board is being reached,
// for which project, with which binary, and what its target spec already says.
type flashRequest struct {
	Target  string `json:"target"`
	Project string `json:"project"`
	Binary  string `json:"binary"`
	Flash   Flash  `json:"flash"`
}

// flashResponse is what a plugin answers: the flash block to use in place of,
// or in addition to, what the target spec declared. An omitted field leaves the
// spec's own.
type flashResponse struct {
	Flash *Flash `json:"flash,omitempty"`
}

// platformFlash lets a platform plugin supply how a board is reached, which is
// for the cases a target spec cannot state. A plugin that fails is reported and
// the target spec's own answer stands.
func platformFlash(project, target, binary string, current Flash) Flash {
	found := plugins.Source{Tree: project, RhoHome: deps.RhoHome(), Platform: deps.Platform(project)}.Discover(ComponentName)
	if len(found) == 0 {
		return current
	}
	answer := plugins.RunHooks(found, ComponentName, HookFlash, flashRequest{
		Target: target, Project: project, Binary: binary, Flash: current,
	})
	for _, diagnostic := range answer.Diagnostics {
		fmt.Fprintf(os.Stderr, "rho flash: %s\n", diagnostic)
	}
	for _, raw := range answer.Payload {
		response := flashResponse{}
		if err := json.Unmarshal(raw, &response); err != nil || response.Flash == nil {
			continue
		}
		current = mergeFlash(current, *response.Flash)
	}
	return current
}

// mergeFlash overlays the fields a plugin set on the spec's own flash block, so
// a plugin can fill in what a spec left out without restating what it said.
func mergeFlash(base, over Flash) Flash {
	if over.Tool != "" {
		base.Tool = over.Tool
	}
	if over.Command != "" {
		base.Command = over.Command
	}
	if len(over.Arguments) > 0 {
		base.Arguments = over.Arguments
	}
	if over.Offset != "" {
		base.Offset = over.Offset
	}
	if len(over.PostArgs) > 0 {
		base.PostArgs = over.PostArgs
	}
	if over.Comment != "" {
		base.Comment = over.Comment
	}
	return base
}

/* -------------------------------------------------------------------------- */
/*  Flashing                                                                   */
/* -------------------------------------------------------------------------- */

// FlashFile writes a binary to a board, using whatever the target says.
//
// The target is named rather than a family of parts, because "STM32" is not an
// answer: the probe, the address and the reset sequence differ between two
// boards of the same family, and a constant picked for one of them is a fault
// on the other.
func FlashFile(binary string, targetName string, project string) error {
	if binary == "" {
		return fmt.Errorf("nothing to flash: name the binary")
	}
	if _, err := os.Stat(binary); err != nil {
		return fmt.Errorf("nothing to flash: %w", err)
	}

	flash, err := resolve(targetName, project)
	if err != nil {
		return err
	}
	// A platform may say how its boards are reached, for the cases a target
	// spec cannot state. The spec's own answer stands where the plugin is silent.
	flash = platformFlash(project, targetName, binary, flash)
	if !flash.declared() {
		return fmt.Errorf("the target %q declares no way to reach a board; add a flash block to its spec", targetName)
	}

	program, args, err := flash.invocation(binary)
	if err != nil {
		return fmt.Errorf("the target %q: %w", targetName, err)
	}
	if _, lookErr := exec.LookPath(program); lookErr != nil {
		return fmt.Errorf("the target %q reaches its board with %s, which is not installed: %w", targetName, program, lookErr)
	}

	if flash.Comment != "" {
		fmt.Fprintf(os.Stderr, "note: %s\n", flash.Comment)
	}

	cmd := exec.Command(program, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("flashing %s with %s\n", binary, program)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", program, err)
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  CLI                                                                        */
/* -------------------------------------------------------------------------- */

func main() {
	fs := flag.NewFlagSet("rho flash", flag.ExitOnError)
	target := fs.String("target", "", "the target to flash for: a board name from its spec")
	project := fs.String("project", "", "the project whose target to read (default: the working directory)")

	// Flags are read wherever they appear, because `rho flash firmware.elf
	// -target pico` is what a person types first. Only the two that take a
	// value are pulled out with it.
	argv := []string{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "-h" || arg == "--help":
			usage()
			os.Exit(0)
		case arg == "-target" || arg == "--target" || arg == "-project" || arg == "--project":
			if i+1 < len(os.Args) {
				argv = append(argv, arg, os.Args[i+1])
				i++
			}
		default:
			argv = append(argv, arg)
		}
	}
	if err := fs.Parse(argv); err != nil {
		os.Exit(2)
	}

	if *target == "" {
		fmt.Fprintln(os.Stderr, "rho flash: -target names the board to flash for")
		usage()
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "rho flash: name exactly one binary")
		usage()
		os.Exit(2)
	}

	if err := FlashFile(fs.Arg(0), *target, *project); err != nil {
		fmt.Fprintln(os.Stderr, "rho flash:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho flash -target <board> [-project dir] <file.bin>")
	fmt.Fprintln(os.Stderr, "       the board's own target spec says which tool writes to it")
}
