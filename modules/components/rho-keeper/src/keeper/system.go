//
// @file system.go
// @brief What this machine is, for somebody asking why it will not do something.
//
// A dashboard about an installation is read when something is not working: a
// build failed, a command is not found, a project will not resolve. The answers
// to those are mostly facts about the machine rather than about Rho, and a person
// having to ask for them one shell at a time is a person copying versions into a
// bug report by hand.
//
// So they are collected. Every one of them is read rather than assumed, and one
// that cannot be read says so rather than being left out: a diagnostic that
// quietly omits a missing compiler is a diagnostic that says the machine is
// ready when it is not.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package keeper

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"rho/src/deps"
)

// System is what this machine is, as a person asking why something failed needs
// to know it.
type System struct {
	// Rho is the toolchain: its version, and where it is installed to.
	Rho RhoInstall `json:"rho"`
	// Keeper is this dashboard's own version.
	Keeper string `json:"keeper"`
	// Go is the toolchain the generated projects are built with, because a
	// project that will not build is very often a project built with a Go that is
	// older than the one its go.mod asks for.
	Go string `json:"go,omitempty"`
	// OS is the platform and its version.
	OS string `json:"os"`
	// Arch is what it is built for.
	Arch string `json:"arch"`
	// Home is where the installation being reported on is.
	Home string `json:"home,omitempty"`
	// Drivers are the tools a build reaches for beyond the compiler, and what
	// each one says. A missing one is a fact about the machine, not an error.
	Drivers []Driver `json:"drivers,omitempty"`
	// Counts is the installation at a glance, which is what the overview shows
	// and what a person reads first here.
	Counts Counts `json:"counts"`
}

// RhoInstall is the Rho installation, as a diagnostic reports it.
type RhoInstall struct {
	Version  string   `json:"version,omitempty"`
	Binaries []Binary `json:"binaries,omitempty"`
	Missing  []string `json:"missing,omitempty"`
}

// Counts is what is installed and what of it is used.
type Counts struct {
	Components int `json:"components"`
	Plugins    int `json:"plugins"`
	Enabled    int `json:"enabled"`
	Contracts  int `json:"contracts"`
	Unfilled   int `json:"unfilled"`
}

// Driver is a tool a build reaches for.
type Driver struct {
	// Name is the tool, as a person would say it.
	Name string `json:"name"`
	// Version is what it says about itself, or what it could not be asked for.
	Version string `json:"version,omitempty"`
	// Found is whether it is there at all. A tool that is not installed is
	// reported rather than omitted, because its absence is the answer.
	Found bool `json:"found"`
	// Note is why a version is empty, when one is.
	Note string `json:"note,omitempty"`
}

// drivers are the tools worth naming. A C compiler is here because a project
// with a C target cannot be built without one, and a machine that has Go but no
// C compiler fails in a way that says nothing about why.
var drivers = []struct {
	name string
	// which is the flag that makes the tool print a version.
	which string
}{
	{"cc", "--version"},
	{"arm-none-eabi-gcc", "--version"},
	{"cmake", "--version"},
	{"git", "--version"},
}

var (
	systemOnce  sync.Once
	systemValue System
)

// ReadSystem is this machine, read once.
//
// It is read once because it does not change while a dashboard is open, and
// because running four programs to ask their versions on every request is a
// dashboard that is slow for no reason a person would thank it for.
func ReadSystem() System {
	systemOnce.Do(func() { systemValue = readSystem() })
	return systemValue
}

func readSystem() System {
	out := System{
		OS:     osDescription(),
		Arch:   runtime.GOARCH,
		Home:   deps.RhoHome(),
		Keeper: "unknown",
	}
	state := Read()
	out.Counts = Counts{
		Components: countKind(state.Modules, "component"),
		Plugins:    countKind(state.Modules, "plugin"),
		Contracts:  len(state.Contracts),
	}
	for _, module := range state.Modules {
		if module.Enabled {
			out.Counts.Enabled++
		}
	}
	for _, contract := range state.Contracts {
		if contract.Status != ContractOK {
			out.Counts.Unfilled++
		}
	}

	out.Keeper = state.Keeper.Version
	out.Rho = RhoInstall{Version: rhoVersion(), Binaries: state.Toolchain.Binaries, Missing: state.Toolchain.Missing}
	// The Go that will build a generated project is the one on the path, not the
	// one this dashboard was compiled with: a project is built by the machine, and
	// a dashboard reporting its own compiler would be answering a question
	// nobody asked.
	out.Go = firstLine(commandVersion("go", "version"))
	for _, d := range drivers {
		out.Drivers = append(out.Drivers, Driver{
			Name:    d.name,
			Version: firstLine(commandVersion(d.name, d.which)),
			Found:   foundOnPath(d.name),
		})
	}
	return out
}

// rhoVersion is the toolchain's version, read from what the installation
// recorded rather than asked of the binary.
//
// A toolchain that could be asked would be asked; this one has no `--version`,
// and a diagnostic that ran a command and pasted its refusal would report
// "Unknown command: --version" as though that were the version. So it is read
// from the install settings, which is where `rho install` wrote it.
func rhoVersion() string {
	home := deps.RhoHome()
	if home == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, "config", "settings.toml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "version") {
			_, value, found := strings.Cut(trimmed, "=")
			if found {
				return strings.Trim(strings.TrimSpace(value), `"`)
			}
		}
	}
	return ""
}

// commandVersion is what a tool says about itself, or "" when it will not say.
func commandVersion(program string, which string) string {
	path, err := exec.LookPath(program)
	if err != nil {
		// A program named by a path is asked by that path.
		if _, statErr := os.Stat(program); statErr != nil {
			return ""
		}
		path = program
	}
	// A tool is given a deadline, because a tool that hangs when asked what it
	// is would hang the first request of the page a person is waiting for.
	command := exec.Command(path, which)
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = command.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(systemDeadline):
		if command.Process != nil {
			_ = command.Process.Kill()
		}
		return ""
	}
	if runErr != nil && len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func foundOnPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// osDescription is the platform and its version, which a person asking why
// something failed usually wants together rather than as two fields.
func osDescription() string {
	name := runtime.GOOS
	switch runtime.GOOS {
	case "darwin":
		name = "macOS"
	case "linux":
		name = "Linux"
	case "windows":
		name = "Windows"
	}
	if info, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(info), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				pretty := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
				if pretty != "" {
					return pretty + " (" + runtime.GOARCH + ")"
				}
			}
		}
	}
	return name + " " + runtime.GOARCH
}

func firstLine(text string) string {
	if index := strings.Index(text, "\n"); index >= 0 {
		return strings.TrimSpace(text[:index])
	}
	return strings.TrimSpace(text)
}

func countKind(modules []Module, kind string) int {
	count := 0
	for _, module := range modules {
		if module.Kind == kind {
			count++
		}
	}
	return count
}

// systemDeadline is how long a tool gets to say what it is before it is
// recorded as silent. A tool that hangs on a version flag would otherwise hang
// the dashboard's first request, which is the one a person is waiting for.
const systemDeadline = 3 * time.Second
