//
// @file cli.go
// @brief What this toolchain can be asked to do, read from the toolchain.
//
// A page that lists the commands is a page that goes stale: the list is written
// once and the toolchain moves on, and a dashboard listing commands a machine no
// longer has is worse than one that does not list them at all.
//
// So the list is not written. It is asked for, from the binaries that are
// installed, by asking each of them for its own help. What comes back is what
// this machine can actually do, on this machine, right now — which is the only
// list that can be true.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rho/src/deps"

	clicore "rho/src/cli"
)

// Command is one command a toolchain accepts.
type Command struct {
	// Tool is the binary it belongs to, so a page can group by what was asked.
	Tool string `json:"tool"`
	// Invocation is what a person types, as the tool spells it. It is what
	// follows the tool, so a page renders the tool's name in front of it once.
	Invocation string `json:"invocation"`
	// Description is what the command is for, as the module that publishes it
	// states. A component's own words, rather than this page's.
	Description string `json:"description,omitempty"`
	// Kind is what the command comes from: "core" for the toolchain's own,
	// "tool" or "group" for what was read from a binary, and a component's name
	// for a command that component publishes.
	Kind string `json:"kind,omitempty"`
	// Help is what the tool says about it, which is its own words rather than
	// this page's.
	Help string `json:"help"`
	// Available is whether the tool is on this machine. A command that is not is
	// listed as such rather than omitted, because its absence is the answer a
	// person reading this page came for.
	Available bool `json:"available"`
	// Path is where the binary is, for a command that cannot be run.
	Path string `json:"path,omitempty"`
}

// ReadCommands is what this toolchain accepts, read from the toolchain.
//
// The list is the same one `rho --help` prints, read by running it, because that
// is the list this machine can act on: the toolchain's own commands, and the
// component commands that are installed and turned on here. A page that listed
// its own idea of them would be a second list, and a second list is how a page
// comes to offer a command that is not there.
//
// A machine with no toolchain has no commands to read, and the page says so
// rather than showing a list of things that are not installed.
func ReadCommands() []Command {
	core, components := clicore.All()
	out := []Command{}

	// What the toolchain says about itself, which is the whole list in its own
	// words and already reflects this machine.
	if help := toolchainHelp(); help != "" {
		out = append(out, Command{Tool: "rho", Kind: "toolchain", Available: true, Help: help})
	} else {
		// Nothing could be read, so the list is built from the inventory rather
		// than left empty: a person looking at this page can still be told what
		// the toolchain has, which is a fact about the toolchain and not about
		// whether it could be asked.
		for _, command := range core {
			out = append(out, Command{
				Tool: "rho", Invocation: command.Invocation, Kind: "core", Description: command.Description,
			})
		}
	}

	// The component commands, from the same reading the toolchain's help used,
	// so the page and the terminal cannot disagree about this machine.
	for _, command := range components {
		out = append(out, Command{
			Tool:        "rho",
			Invocation:  command.Invocation,
			Description: command.Description,
			Kind:        command.Source,
			Available:   true,
		})
	}
	return out
}

// toolchainHelp is what the installed toolchain says about itself, or "".
func toolchainHelp() string {
	home := deps.RhoHome()
	if home == "" {
		return ""
	}
	return helpOf(filepath.Join(home, "bin", "rho"))
}

// helpOf is what a tool says about itself, or "" when it will not say.
//
// A tool is given a deadline, because a help that hangs would hang the page that
// is asking for it, and both streams are read because a tool that answers on
// standard error has still answered.
func helpOf(binary string, args ...string) string {
	if binary == "" {
		return ""
	}
	if len(args) == 0 {
		args = []string{"--help"}
	}
	command := exec.Command(binary, args...)
	var out, errors bytes.Buffer
	command.Stdout = &out
	command.Stderr = &errors
	done := make(chan error, 1)
	go func() { done <- command.Run() }()
	select {
	case err := <-done:
		if err != nil && out.Len() == 0 && errors.Len() == 0 {
			return ""
		}
	case <-time.After(systemDeadline):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		return ""
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		text = strings.TrimSpace(errors.String())
	}
	return text
}
