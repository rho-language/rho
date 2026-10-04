//
// @file targetspec.go
// @brief What a target spec says, read from the report rho-target prints.
//
// One reader, shared: the editor configuration `rho ide` writes and the debugging
// session `rho dbg` runs both need to know which compiler a board is built with,
// which debugger reaches it and where its register map is. Two readers would be two
// lists to keep in step with the first, and the field one of them forgot is the
// field a session fails on.
//
// The facts are read from a target spec as that spec reports itself, which is what
// `rho target show -json` prints: the resolved spec, with a board's chip and
// architecture already folded in. So this reads that output rather than reaching
// into the component's own types — the component is dispatched to, not linked
// against, and which targets exist is a fact it holds and its plugins extend.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//
// It reads rather than imports because a component is dispatched to, not linked
// against: which targets exist is a fact the component carrying the specs holds,
// and a generator that had its own copy of the list would be a second list to
// keep in step with the first.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package targetspec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Target is what a generator needs to know about a target: which board it is,
// how it is built, how it is flashed, and how it is debugged.
//
// Only the fields an editor configuration can use are here. A resolved spec
// carries more — every pin, every region, every flag — and a debug configuration
// that quoted a memory map would be quoting a fact the editor already reads out
// of the program.
type Target struct {
	// Name is the target's name, as it was asked for.
	Name string `json:"name"`
	// Kind is what the spec describes: arch, mcu or board.
	Kind string `json:"kind"`
	// Core is the processor core, such as cortex-m0plus.
	Core string `json:"core"`
	// MCU is the part number the board carries, as the vendor writes it:
	// `STM32H753ZI`. It is the part's own name, with its package and temperature
	// code, and it is what a chip's register map is worked out from when nothing
	// else has said which one this is.
	MCU string `json:"mcu"`
	// Toolchain is how the target is compiled.
	Toolchain Toolchain `json:"toolchain"`
	// Memory is the address space, which is what a debugger's memory view reads.
	Memory Memory `json:"memory"`
	// Defines are the macros a build of this target always has. An editor parses
	// headers with them, and without them it parses out the branches a build
	// excludes — which are usually the ones that pick the board.
	Defines []string `json:"defines"`
	// Flash is how a built program is written to the target.
	Flash Flash `json:"flash"`
	// Debug is how a program's execution is inspected on it.
	Debug Debug `json:"debug"`
}

// Toolchain is the programs a target is built with.
type Toolchain struct {
	Compiler string `json:"compiler"`
	Linker   string `json:"linker"`
	Objcopy  string `json:"objcopy"`
	Size     string `json:"size"`
}

// Memory is a target's address space.
type Memory struct {
	Flash Region `json:"flash"`
	RAM   Region `json:"ram"`
}

// Region is one region of memory: where it starts and how long it is.
type Region struct {
	Origin string `json:"origin"`
	Length string `json:"length"`
}

// Flash is how a built program is written to a target.
type Flash struct {
	Tool    string `json:"tool"`
	Command string `json:"command"`
	Offset  string `json:"offset"`
}

// Debug is how a program is inspected on a target.
type Debug struct {
	// Tool is the debugger program. Empty means the debugger that goes with the
	// compiler, which DebuggerFor works out.
	Tool string `json:"tool"`
	// Server is the program that sits between the debugger and the board when a
	// probe is in the way. A target that declares none is debugged in place.
	Server string `json:"server"`
	// ServerArgs are the arguments that server is started with, before the port it
	// is told to listen on. Written as a list rather than a command string, because
	// these are arguments and a list is what a process is started with.
	ServerArgs []string `json:"server_args"`
	// Adapter names the debug adapter an editor drives this target with.
	Adapter string `json:"adapter"`
	// ServerType is what the debugger connects to, which is what tells one probe
	// server from another.
	ServerType string `json:"server_type"`
	// Interface is the probe, as the server names it.
	Interface string `json:"interface"`
	// Config are the configuration files the server is started with.
	Config []string `json:"config"`
	// Device is the probe as the vendor names it.
	Device string `json:"device"`
	// SVD is the register map, as a path: where a project or a platform that
	// carries one has it.
	SVD string `json:"svd"`
	// SVDSource is the same file in the published set `rho dbg` fetches from when
	// nothing here carries it, as that set writes it: `STMicro/STM32H7x3.svd`.
	//
	// Named separately because a path cannot say it. The file's name in a vendor's
	// published set is the vendor's and covers a device line — `STM32H7x3.svd` is
	// every part on that line — so the file to fetch is a fact about the part that
	// only a spec which knows the part can state.
	SVDSource string `json:"svd_source"`
	// Port is the port a probe server listens on.
	Port string `json:"port"`
	// Load is whether a session writes the program to the board at its start.
	Load bool `json:"load"`
	// Comment says what has to be true for a session to work.
	Comment string `json:"comment"`
}

// Declared reports whether the target says anything about debugging.
func (d Debug) Declared() bool {
	return strings.TrimSpace(d.Tool) != "" ||
		strings.TrimSpace(d.Server) != "" ||
		strings.TrimSpace(d.Adapter) != "" ||
		strings.TrimSpace(d.SVD) != ""
}

// Watcher is whether a probe has to be running before a debugger may connect.
//
// The difference between the two cases is why a launch that works on a laptop does
// nothing on the bench: a program on the host is started by the debugger, and a
// program on a board is started by whatever flashed it.
func (d Debug) Watcher() bool { return strings.TrimSpace(d.Server) != "" }

// ListenPort is the port the probe server is told to listen on.
func (d Debug) ListenPort() string {
	if port := strings.TrimSpace(d.Port); port != "" {
		return port
	}
	return "3333"
}

// DebuggerFor is the debugger that goes with a compiler.
//
// A toolchain's debugger is named after its compiler: the compiler part is
// dropped and `gdb` appended, so `arm-none-eabi-gcc` gives `arm-none-eabi-gdb`
// and a host `gcc` gives `gdb`. A target spec may therefore leave the debugger
// out and still be debuggable, which is why this is derived here rather than
// required in the spec.
func DebuggerFor(compiler string) string {
	compiler = strings.TrimSpace(compiler)
	if compiler == "" {
		return ""
	}
	if slash := strings.LastIndexAny(compiler, "/\\"); slash >= 0 {
		compiler = compiler[slash+1:]
	}
	for _, suffix := range []string{"-gcc", "-g++", "-clang++", "-clang", "-cc"} {
		if trimmed := strings.TrimSuffix(compiler, suffix); trimmed != compiler {
			return trimmed + "-gdb"
		}
	}
	switch compiler {
	case "gcc", "g++", "cc", "c++", "clang", "clang++":
		return "gdb"
	}
	return compiler + "-gdb"
}

// FindFile is a declared file, looked for in the places a project keeps one.
//
// A *list* of places rather than a search, and the distinction is the whole design:
// a search that finds `rp2040.svd` somewhere unexpected shows the registers of a
// chip this is not, on a board somebody has wired up and is staring at. A list is
// also something a reader can check, and something this function can be wrong
// about in one direction only.
//
// In that order: as an absolute path it is taken as given; a path with a separator in
// it is read from the project, and then from each platform it depends on, because a
// file a platform vendors lives beside that platform; and a bare file name is read
// beside the built program, at the project's root, in the project's own `vendor/`
// directories, and then in each dependency's.
//
// The `vendor/` directories are named rather than descended into, and only one level
// deep — which is where a vendored file actually is, and one level is a statement
// about where files live rather than a guess about where somebody put one.
func FindFile(declared string, projectRoot string, program string, roots []string) (string, bool) {
	if strings.TrimSpace(declared) == "" {
		return "", false
	}
	if filepath.IsAbs(declared) {
		return existing(declared)
	}
	if strings.ContainsAny(declared, `/\`) {
		if found, ok := existing(filepath.Join(projectRoot, declared)); ok {
			return found, true
		}
		// Not beside the program, and not at the project's root: a declared path with
		// a separator in it is a path, and the place it is read from is the place the
		// thing that declares it keeps its files.
		for _, root := range append([]string{projectRoot}, roots...) {
			if found, ok := existing(filepath.Join(root, declared)); ok {
				return found, true
			}
		}
		return existing(declared)
	}

	// A bare file name. Beside the built program first — a program that carries its
	// own register map keeps it where it builds.
	if program != "" {
		if found, ok := existing(filepath.Join(filepath.Dir(program), declared)); ok {
			return found, true
		}
	}
	places := []string{projectRoot}
	for _, root := range roots {
		places = append(places, root)
	}
	for _, place := range places {
		if found, ok := existing(filepath.Join(place, declared)); ok {
			return found, true
		}
		// And in the place's vendor directory, and in the directories inside it, one
		// level deep. Both shapes are real: `vendor/chip.svd` and
		// `vendor/cmsis_svd/chip.svd`, and a project that vendors a vendor's register
		// map uses whichever its tooling produced.
		vendor := filepath.Join(place, "vendor")
		if found, ok := existing(filepath.Join(vendor, declared)); ok {
			return found, true
		}
		for _, directory := range subdirectories(vendor) {
			if found, ok := existing(filepath.Join(directory, declared)); ok {
				return found, true
			}
		}
	}
	return "", false
}

// subdirectories is the directories inside a path, in name order.
func subdirectories(path string) []string {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			out = append(out, filepath.Join(path, entry.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// existing is a file that is there, or nothing.
func existing(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

// ParseTarget reads a resolved target spec as one of these.
//
// Decoded strictly rather than leniently: an output that is not a target spec is
// something that went wrong between here and the component, and a generator that
// treated it as an empty spec would write a configuration for a board it knows
// nothing about — which is a configuration that looks right and does nothing.
func ParseTarget(data []byte) (*Target, error) {
	target := &Target{}
	if err := json.Unmarshal(data, target); err != nil {
		return nil, fmt.Errorf("the target spec is not what a target spec looks like: %w", err)
	}
	if strings.TrimSpace(target.Name) == "" {
		return nil, fmt.Errorf("the target spec names no target")
	}
	return target, nil
}

// FirstNotEmpty is the first of its arguments that is not blank, which is how a
// generated field is given a default the spec did not state.
func FirstNotEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// StringsOf is a list of non-blank strings, and nil when there is none — which is
// what a field an editor should leave out entirely looks like.
func StringsOf(values []string) []any {
	out := []any{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
