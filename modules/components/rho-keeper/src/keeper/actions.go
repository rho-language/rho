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
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"fmt"
	"path/filepath"
	"strings"

	"rho/cmd/rho-module/core"
	"rho/src/deps"
	"rho/src/scaffold"
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

// Uninstall removes an installed module version from this machine.
//
// It is refused while the version is in use: enabling is the decision to run a
// module, and pulling the thing a run resolves to out from under it is not a
// change anyone asked for. Turning it off first is one click, and the refusal
// says so.
//
// The version the index selects is refused by the toolchain for the same reason,
// so the selection is stepped off first — that is part of uninstalling rather
// than a separate decision a page should make. What is removed is this machine's
// copy only; a registry is unchanged, which is why the module is offered again
// under the registry as soon as it is gone.
func Uninstall(name, version string) Action {
	name = strings.TrimSpace(name)
	kind := kindOf(name)
	if kind == "" {
		return Action{Command: command("module uninstall", name), Error: fmt.Sprintf("%s is not installed, so there is nothing to remove", name)}
	}
	if strings.TrimSpace(version) == "" {
		version = deps.InstalledVersions(kind, name)[0]
	}
	commandLine := command("module uninstall", name+"@"+version)

	if state, err := deps.LoadEnabled(); err == nil && state != nil {
		if state.IsEnabled(kind, name, version) {
			return Action{
				Command: commandLine,
				Error: fmt.Sprintf("%s %s@%s is in use; turn it off first, then uninstall",
					kind, name, version),
			}
		}
	}

	// `rho module uninstall` refuses the version the index selects; stepping off
	// it first is what that refusal asks for.
	_ = core.Deactivate(name, version)
	if err := core.UninstallLocal(name, version); err != nil {
		return Action{Command: commandLine, Error: err.Error()}
	}
	return Action{
		OK:      true,
		Command: commandLine,
		Message: fmt.Sprintf("%s %s@%s was removed from the local registry", kind, name, version),
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

// Init writes a new project, through the same function `rho init` uses.
//
// What comes out — a Rho application, or a driver and its example — is what the
// project's manifest declares, not a choice this makes. A dashboard that
// generated by its own means would be a second generator, and the two would
// disagree about what a project starts as.
func validateInitDir(dir string) (string, error) {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return "", fmt.Errorf("a directory to create the project in is required")
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid directory path")
	}
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("directory path must be absolute")
	}
	return cleaned, nil
}

func Init(project, dir, name, generator, category, description string, set map[string]string) Action {
	safeDir, err := validateInitDir(dir)
	cmd := command("init", dir)
	if err != nil {
		return Action{Command: cmd, Error: err.Error()}
	}
	cmd = command("init", safeDir)
	results, chosen, err := scaffold.Init(scaffold.Request{
		Project:     project,
		Dir:         safeDir,
		Name:        name,
		Generator:   generator,
		Category:    category,
		Description: description,
		Set:         set,
	})
	if err != nil {
		return Action{Command: cmd, Error: err.Error()}
	}
	paths := make([]string, 0, len(results))
	for _, result := range results {
		paths = append(paths, result.Dir)
	}
	if chosen == "" {
		return Action{OK: true, Command: cmd, Message: fmt.Sprintf("created a Rho application in %s", strings.Join(paths, ", "))}
	}
	return Action{OK: true, Command: cmd, Message: fmt.Sprintf("generated %s in %s", chosen, strings.Join(paths, ", "))}
}
