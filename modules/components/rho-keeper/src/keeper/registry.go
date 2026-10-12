//
// @file registry.go
// @brief The remote registry, read and installed from, through the toolchain.
//
// The dashboard shows what this machine has. This adds the other half of the
// question: what the registry it is pointed at offers, and which of that is not
// here yet. It is fetched rather than derived, because a remote registry is
// another machine and the only true list of what it holds is the one it serves.
//
// What is offered and where it comes from are read the way the CLI reads them —
// the same `remote` package `rho module search` and `rho module install` use —
// so a module a page offers to install is a module the command line would
// install, and neither can offer something the other cannot. Installing runs the
// same `rho module install` a person would type, for the same reason: a dashboard
// that installed by its own means would be a second installer, and the two would
// disagree the first time an install changed.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rho/cmd/rho-module/remote"
	"rho/src/deps"
)

// RegistryModule is one module the remote registry offers, as a page shows it.
type RegistryModule struct {
	Name string `json:"name"`
	// Version is the newest version the registry holds, which is the one an
	// install with no version named would fetch.
	Version     string `json:"version"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	// Author is the module's own claim about who wrote it; Owner is the identity
	// the registry recorded. They are shown apart because one is a claim and the
	// other is a fact about a credential.
	Author string `json:"author,omitempty"`
	Owner  string `json:"owner,omitempty"`
	// Installed is whether this machine already holds the module at all, and
	// InstalledVersions is what it holds, newest first. A page that says only
	// "installed" hides the case that matters most: the registry has a newer one.
	Installed         bool     `json:"installed"`
	InstalledVersions []string `json:"installedVersions,omitempty"`
	// Enabled is whether one of the installed versions is the one a run uses.
	Enabled bool `json:"enabled"`
}

// Newer is whether the registry's version is one this machine does not hold.
// It is a separate answer from Installed, because a machine can have an older
// version of a module and still be behind.
func (m RegistryModule) Newer() bool {
	for _, installed := range m.InstalledVersions {
		if installed == m.Version {
			return false
		}
	}
	return m.Installed
}

// Registry is the remote registry, as the page shows it.
type Registry struct {
	// URL is the registry this reading is about, so a page says which one it is
	// rather than leaving a person to guess.
	URL string `json:"url,omitempty"`
	// Configured is whether a remote registry is set at all. A machine with none
	// is not an error, and the page says so rather than showing a failure.
	Configured bool             `json:"configured"`
	Modules    []RegistryModule `json:"modules"`
	// Available counts the modules not held here, which is the one number a
	// person opens this page for.
	Available int `json:"available"`
	// Error is why the registry could not be read, when it could not. It is a
	// sentence a person can act on, not a status code.
	Error string `json:"error,omitempty"`
}

// ReadRegistry reads what the configured remote registry offers, and for each
// module whether this machine already holds it.
//
// Reading is public, so no credential is needed to list; the client uses the
// issued token when there is one, which is what lets an owner see their own
// private modules here as well as in a terminal.
func ReadRegistry() Registry {
	cfg, err := remote.LoadConfig()
	if err != nil {
		return Registry{Error: err.Error()}
	}
	out := Registry{URL: cfg.Remote.URL}
	if strings.TrimSpace(cfg.Remote.URL) == "" {
		return out
	}
	out.Configured = true

	found, err := remote.SearchRemote(remote.NewClient(cfg), "")
	if err != nil {
		out.Error = err.Error()
		return out
	}

	// What is installed here is read once, through the same functions the rest of
	// the dashboard reads it with, so a module a row calls installed is one the
	// modules list would also show.
	state, _ := deps.LoadEnabled()
	out.Modules = make([]RegistryModule, 0, len(found))
	// The registry's search answers one entry per version, oldest first, so a
	// module with several releases would otherwise appear once per release. One
	// row per module is what a person is choosing between; the newest version is
	// the one an install with no version named would fetch.
	at := map[string]int{}
	for _, info := range found {
		module := RegistryModule{
			Name:        info.Name,
			Version:     info.Version,
			Type:        info.Type,
			Description: info.Description,
			Author:      info.Author,
			Owner:       info.Owner,
		}
		module.InstalledVersions = installedUnder(info.Type, info.Name)
		module.Installed = len(module.InstalledVersions) > 0
		for _, version := range module.InstalledVersions {
			if state != nil && state.IsEnabled(kindFor(info.Type, info.Name), info.Name, version) {
				module.Enabled = true
			}
		}
		if index, seen := at[info.Name]; seen {
			// Later entries are newer, so the row is replaced rather than added
			// to: the count of what is not installed is decided once, on the
			// first sighting, and does not move when a module gains a version.
			out.Modules[index] = module
			continue
		}
		at[info.Name] = len(out.Modules)
		if !module.Installed {
			out.Available++
		}
		out.Modules = append(out.Modules, module)
	}
	return out
}

// installedUnder is the versions of a module present in this Rho home.
//
// The registry says which kind a module is, and that is the kind it is installed
// as. When it says nothing, both kinds are checked, because a name that is
// installed is installed whatever the registry called it.
func installedUnder(kind, name string) []string {
	if kind != "" {
		return deps.InstalledVersions(kind, name)
	}
	if versions := deps.InstalledVersions(deps.KindComponent, name); len(versions) > 0 {
		return versions
	}
	return deps.InstalledVersions(deps.KindPlugin, name)
}

// kindFor is the kind a module is installed under, for asking whether a version
// is enabled.
func kindFor(kind, name string) string {
	if kind != "" {
		return kind
	}
	if len(deps.InstalledVersions(deps.KindComponent, name)) > 0 {
		return deps.KindComponent
	}
	return deps.KindPlugin
}

// installDeadline bounds an install. A module is a package over a network, so it
// can take a while; it must not take forever with a page watching.
const installDeadline = 5 * time.Minute

// InstallModule installs a module from the remote registry.
//
// It runs the same `rho module install` a person would type, rather than calling
// into an installer directly, because the answer a page shows is meant to be a
// command that can be repeated in a terminal. The output of that command is the
// message, so a failure explains itself in the toolchain's own words.
func InstallModule(name, version string) Action {
	name = strings.TrimSpace(name)
	if name == "" {
		return Action{Error: "a module name is needed"}
	}
	reference := name
	if version = strings.TrimSpace(version); version != "" {
		reference = name + "@" + version
	}
	commandLine := command("module install", reference)

	binary := rhoBinary()
	if binary == "" {
		return Action{Command: commandLine, Error: "no Rho toolchain was found to install with"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), installDeadline)
	defer cancel()
	run := exec.CommandContext(ctx, binary, "module", "install", reference)
	output, err := run.CombinedOutput()
	message := lastLines(string(output), 12)
	if ctx.Err() == context.DeadlineExceeded {
		return Action{Command: commandLine, Error: "the install did not finish in time", Message: message}
	}
	if err != nil {
		action := Action{Command: commandLine, Error: err.Error(), Message: message}
		if message != "" {
			action.Error = message
		}
		return action
	}
	if message == "" {
		message = fmt.Sprintf("installed %s", reference)
	}
	return Action{OK: true, Command: commandLine, Message: message}
}

// rhoBinary is the installed toolchain, or "" when there is none. It is the same
// binary `rho module install` in a terminal would be, found the same way.
func rhoBinary() string {
	home := deps.RhoHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "bin", "rho")
}

// lastLines keeps the tail of a command's output.
//
// The end of an install's output is the part that says what happened; the start
// is a download counting up. A page shows a bounded message rather than a
// transcript, so the part that is an answer is the part kept.
func lastLines(text string, count int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
