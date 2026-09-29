//
// @file actions.go
// @brief Changing what is installed and enabled, and saying what was done.
//
// An action here is the same operation the CLI performs, called through the same
// functions — not a second way of doing it. A dashboard that enabled a module by
// its own means would be a second implementation of a rule the toolchain already
// has, and the two would disagree about what enabling a version means.
//
// Every action answers with the command it performed, and the page shows it. That
// is not a nicety: a person who has just changed a machine should be able to see
// what changed it and repeat it, and a change made through a page that cannot be
// repeated in a terminal is a change nobody can undo from memory.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package keeper

import (
	"fmt"
	"strings"

	"rho/src/deps"
)

// Action is what a change did, as the page is told.
//
// The command is the important field. It is the same command a person would have
// typed, and it is shown whether it worked or not, because a person who has just
// been refused wants to know what to run instead.
type Action struct {
	// OK is whether it happened.
	OK bool `json:"ok"`
	// Command is what was run, always filled in, and is what the page shows.
	Command string `json:"command"`
	// Message is what happened, in a sentence a person can act on.
	Message string `json:"message,omitempty"`
	// Error is why it did not, when it did not.
	Error string `json:"error,omitempty"`
}

// kinds a request may name.
//
// A name is a name: a component and a plugin of the same name are different
// programs, and the page says which it means, so a request says too. A request
// that does not is answered for whichever kind has it, which is what a person
// who typed a name and found one thing was expecting.
var actionKinds = []string{deps.KindComponent, deps.KindPlugin}

// kindOf is the kind a name is installed as, or "" when it is installed as none.
func kindOf(name string) string {
	for _, kind := range actionKinds {
		if len(deps.InstalledVersions(kind, name)) > 0 {
			return kind
		}
	}
	return ""
}

// Enable turns a module on, at the newest installed version.
//
// The version is the newest because that is what turning something on means; a
// person pinning a version has a version to name, and the page takes one when the
// request carries it.
func Enable(name, version string) Action {
	kind := kindOf(name)
	if kind == "" {
		command := command("module enable", name)
		return Action{Command: command, Error: fmt.Sprintf("%s is not installed, so there is nothing to turn on", name)}
	}
	if version == "" {
		version = deps.InstalledVersions(kind, name)[0]
	}
	command := command("module enable", name+"@"+version)
	if err := deps.SetVersionEnabled(kind, name, version); err != nil {
		return Action{Command: command, Error: err.Error()}
	}
	return Action{
		OK:      true,
		Command: command,
		Message: fmt.Sprintf("%s %s is now on at %s", kind, name, version),
	}
}

// Disable turns a module off without removing it.
//
// It stays installed, so turning it back on does not mean finding it again —
// which is the difference between a module a person has withdrawn and one they
// have deleted, and a thing worth being able to undo.
func Disable(name string) Action {
	kind := kindOf(name)
	if kind == "" {
		command := command("module disable", name)
		return Action{Command: command, Error: fmt.Sprintf("%s is not installed, so there is nothing to turn off", name)}
	}
	command := command("module disable", name)
	if err := deps.Disable(kind, name); err != nil {
		return Action{Command: command, Error: err.Error()}
	}
	return Action{
		OK:      true,
		Command: command,
		Message: fmt.Sprintf("%s %s stays installed and is not used until it is turned on again", kind, name),
	}
}

// command is what a person would type, in one string.
//
// It is assembled here rather than stored, so it is the command as the toolchain
// spells it rather than a copy that would be right until one of them changed.
func command(verb, argument string) string {
	parts := []string{"rho", verb}
	for _, word := range strings.Fields(argument) {
		parts = append(parts, word)
	}
	return strings.Join(parts, " ")
}
