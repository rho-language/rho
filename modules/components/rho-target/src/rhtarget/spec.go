//
// @file spec.go
// @brief Board and architecture specifications, and how they compose.
//
// A build for a dev board needs facts that belong to the board and not to the
// project: which compiler, which flags, where the flash is, how much RAM there
// is, and which pin is the LED. Those facts are the same for every project that
// targets the board, so they live in a spec rather than in every project.
//
// A spec is written once and refers to another: a board names the chip it
// carries, and a chip names the architecture it is built as.
//
//	stm32f4        the chip: core, toolchain, memory map
//	pico           the board: the chip, the vendor library, the pin map
//	esp32c3        a chip that is also its own board
//
// Resolution walks that chain and folds it into one spec, so a caller reads a
// single answer: a board with the toolchain of its chip and the flags of its
// architecture.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhtarget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/yaml"
)

// Kind is what a spec describes.
type Kind string

const (
	// KindArch is an architecture: the flags every core of it is built with.
	KindArch Kind = "arch"
	// KindMCU is a chip: a core, a toolchain and a memory map.
	KindMCU Kind = "mcu"
	// KindBoard is a board: a chip, the library that implements its interfaces,
	// and the pins a program names.
	KindBoard Kind = "board"
)

// Spec is one target: an architecture, a chip or a board.
type Spec struct {
	Kind    Kind   `json:"kind"`
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
	// Arch is the architecture a chip is built as, and the spec it inherits its
	// flags from.
	Arch string `json:"arch,omitempty"`
	// MCU is the chip a board carries.
	MCU string `json:"mcu,omitempty"`
	// ResolvedFrom names the specs this one was resolved from, nearest first, so
	// a report can say what a value actually came from.
	ResolvedFrom []string `json:"-"`

	// Family is the family of parts this spec is for, as the vendor names it —
	// `STM32H7`. It is what the family macro is derived from, so a spec states
	// which family it is once and everything downstream agrees rather than each
	// working it out from the part number.
	Family string `json:"family,omitempty"`
	// Core is the processor core, which is a fact about the chip that a program
	// may want to know.
	Core string `json:"core,omitempty"`
	// Toolchain is how the chip is compiled and linked.
	Toolchain Toolchain `json:"toolchain,omitempty"`
	// Memory is the address space a program is laid out in.
	Memory Memory `json:"memory,omitempty"`
	// Defines are the macros a build of this target always has.
	Defines []string `json:"defines,omitempty"`
	// Flags are added to the compiler's, after the architecture's.
	Flags []string `json:"flags,omitempty"`
	// LinkerScript names the script a project supplies, when the target needs one
	// that is not generated from the memory map.
	LinkerScript string `json:"linker_script,omitempty"`
	// VendorLibrary is the library that implements this board's interfaces, which
	// is what a build links.
	VendorLibrary string `json:"vendor_library,omitempty"`
	// Pins are the names a program uses for the pins of a board.
	Pins map[string]Pin `json:"pins,omitempty"`
	// Peripherals are the interfaces a board brings out, and the pins they are on.
	Peripherals map[string]Peripheral `json:"peripherals,omitempty"`
	// Flash is how a built program is written to the board.
	Flash Flash `json:"flash,omitempty"`
	// Source is the file the spec was read from, so a report can say where it
	// came from.
	Source string `json:"-"`
}

// Toolchain is the programs and flags a chip is built with.
type Toolchain struct {
	// Compiler, Linker, Objcopy and Size are the programs. A program named
	// without a path is found on PATH.
	Compiler string `json:"compiler,omitempty"`
	Linker   string `json:"linker,omitempty"`
	Objcopy  string `json:"objcopy,omitempty"`
	Size     string `json:"size,omitempty"`
	// Flags are the architecture's own: the ones that say what the core is.
	Flags []string `json:"flags,omitempty"`
}

// Memory is the address space of a chip.
type Memory struct {
	// Flash is where a program is stored and executed from.
	Flash Region `json:"flash,omitempty"`
	// RAM is where a program's data lives.
	RAM Region `json:"ram,omitempty"`
	// Extra are the further regions a chip has, such as the core-coupled memory
	// of an STM32 or the second bank of an RP2040, keyed by the name a program
	// refers to them by.
	Extra map[string]Region `json:"extra,omitempty"`
}

// Region is one region of memory: where it starts and how long it is, as the
// numbers a linker script is written with.
type Region struct {
	Origin string `json:"origin"`
	Length string `json:"length"`
	// Comment says what the region is for, when its name does not.
	Comment string `json:"comment,omitempty"`
	// Link says whether a program may be laid out in this region. A region that
	// is real memory is, and a window onto another region is not: a cache that
	// shows the flash at another address is not somewhere to put a section.
	// Absent means yes, which is what a region of memory is.
	Link *bool `json:"link,omitempty"`
}

// Linkable reports whether a program may be laid out in this region.
func (r Region) Linkable() bool { return r.Link == nil || *r.Link }

// Pin is one pin of a board.
type Pin struct {
	// Name is the board's own name for it, such as GP25, which is what a program
	// writes.
	Name string `json:"name,omitempty"`
	// Role is what the board wires it to, such as led or uart_tx. A pin with no
	// role is one the board brings out and does not use.
	Role string `json:"role,omitempty"`
	// Comment says what is on the board at that pin, such as an LED.
	Comment string `json:"comment,omitempty"`
}

// Peripheral is an interface a board brings out.
type Peripheral struct {
	// Instance is the hardware instance, such as i2c0, which is the name a board
	// registers with the platform.
	Instance string `json:"instance,omitempty"`
	// Pins are the pins it is on, by their role in the interface: sda, scl, tx,
	// rx, sck, mosi, miso.
	Pins map[string]string `json:"pins,omitempty"`
	// Comment says what it is connected to, such as an on-board sensor.
	Comment string `json:"comment,omitempty"`
}

// Flash is how a program reaches a board.
type Flash struct {
	// Tool is the program that writes a built binary: openocd, picotool,
	// esptool.py.
	Tool string `json:"tool,omitempty"`
	// Command is how the tool is invoked, with the binary named as {binary} and
	// the spec's own values available as {name} where a caller has one.
	Command string `json:"command,omitempty"`
	// Offset is where a program is written, for a tool that writes at an address
	// rather than into a named region.
	Offset string `json:"offset,omitempty"`
	// Comment says what has to be true for flashing to work, such as a boot mode.
	Comment string `json:"comment,omitempty"`
	// Arguments is the invocation as a list, for a platform that would rather
	// not write a command string with quoting inside it. It is the same thing:
	// a command is used when there is one, and arguments when there is not.
	Arguments []string `json:"arguments,omitempty"`
	// PostArgs are the arguments the offset goes between, for a tool that takes
	// the address after the file rather than before it. Written out, because a
	// tool's argument order is the one thing about flashing that is not
	// interchangeable, and a reader guessing it would flash to the wrong place.
	PostArgs []string `json:"post_args,omitempty"`
}

// Declared reports whether the target says anything about reaching a board.
func (f Flash) Declared() bool {
	return strings.TrimSpace(f.Command) != "" || len(f.Arguments) > 0
}

// Invocation renders the flash command for a binary, as the program to run and
// the arguments to run it with.
//
// The command is split on whitespace rather than handed to a shell. A spec is
// data that a platform ships, and a spec that could name a pipeline would be a
// spec that could run whatever a machine could be talked into; the tools that
// need quoting are served by honouring quotes here instead. `{binary}` is where
// the file goes, and a command that never says so has it appended — otherwise a
// tool with a default would flash whatever that default is rather than the file
// that was just built.
func (f Flash) Invocation(binary string) (string, []string, error) {
	argv := []string{}
	switch {
	case strings.TrimSpace(f.Command) != "":
		argv = SplitCommand(f.Command)
	case len(f.Arguments) > 0:
		argv = append(argv, f.Arguments...)
	default:
		return "", nil, fmt.Errorf("the target declares no way to reach the board: neither a command nor a list of arguments")
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

// SplitCommand splits a command into arguments, honouring quotes so that a path
// containing a space survives as one argument.
func SplitCommand(command string) []string {
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
/*  Reading                                                                    */
/* -------------------------------------------------------------------------- */

// Library is where the specs a component ships live.
type Library struct {
	// Root is the directory the specs are read from.
	Root string
	// specs are the specs that were read, by name.
	specs map[string]Spec
	// Diagnostics is what was wrong with them.
	Diagnostics []Diagnostic
}

// Diagnostic is a spec that could not be read, or a reference that does not
// resolve.
type Diagnostic struct {
	Path    string
	Message string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return d.Message
	}
	return d.Path + ": " + d.Message
}

// isSpecFile reports whether a path names a target spec.
//
// Both JSON and YAML are read. They say the same thing, and a target a person
// writes by hand is far more often written as YAML: a spec is mostly comments
// saying what a board is, and YAML lets those be written as YAML comments
// rather than as a quoted block that a JSON parser will not keep.
func isSpecFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	}
	return false
}

// decodeSpec reads a spec, in whichever of the two formats it was written.
func decodeSpec(data []byte, path string, spec *Spec) error {
	if strings.EqualFold(filepath.Ext(path), ".json") {
		return json.Unmarshal(data, spec)
	}
	// YAML is parsed and then handed to the same JSON decoder the JSON specs go
	// through, rather than being decoded field by field here. One decoder means
	// one set of rules about what a field is called and how it is spelled, so a
	// field cannot mean one thing in a .json spec and another in a .yaml one.
	document, err := yaml.Load(string(data))
	if err != nil {
		return err
	}
	converted, err := json.Marshal(yaml.ToAny(document))
	if err != nil {
		return err
	}
	return json.Unmarshal(converted, spec)
}

// Load reads every spec in a directory tree.
//
// A spec that cannot be read is reported and the rest are read: one board with a
// mistake in it must not cost a project the boards that are right.
func Load(root string) *Library {
	return LoadFrom([]string{root})
}

// LoadFrom reads every spec in each of several directory trees, in order.
//
// The first tree to declare a name is the one that stands. That is what makes an
// override an override: a target declared in the Rho home is read before the one
// the component ships, so a machine that needs a different compiler or a
// different memory map says so in its own directory and the shipped spec is
// reported as shadowed rather than silently fighting it.
//
// One tree is the ordinary case, and a project that keeps its own targets needs
// nothing else: it puts them in its own directory and names them.
func LoadFrom(roots []string) *Library {
	library := &Library{specs: map[string]Spec{}}
	seenRoot := false
	for _, root := range roots {
		if root == "" {
			continue
		}
		if !seenRoot {
			library.Root = root
			seenRoot = true
		}
		readInto(library, root)
	}
	return library
}

// AddFile reads one declared spec file into the library.
//
// A module names the spec file it carries — a board names its own, because its
// memory map and how it is reached are that board's — and that one file is read.
//
// Its directory is not searched. A board's folder also holds the board's
// manifest, its presets and its support package, and walking one reads all of
// them: the manifest arrives as a spec that has no compiler flags in it and is
// reported as naming nothing, which names neither the file nor the contract that
// put it there.
func (l *Library) AddFile(path string) {
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{Path: path, Message: err.Error()})
		return
	}
	spec := Spec{}
	if err := decodeSpec(data, path, &spec); err != nil {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{Path: path, Message: "cannot read the spec: " + err.Error()})
		return
	}
	if spec.Name == "" {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{Path: path, Message: "a spec names nothing"})
		return
	}
	spec.Source = path
	if existing, seen := l.specs[spec.Name]; seen && existing.Source != path {
		l.Diagnostics = append(l.Diagnostics, Diagnostic{
			Path:    existing.Source,
			Message: fmt.Sprintf("a spec named %q is already declared; the first one is used", spec.Name),
		})
		return
	}
	l.specs[spec.Name] = spec
}

// readInto reads one declared directory of specs into a library.
func readInto(library *Library, root string) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !isSpecFile(path) {
			return nil
		}
		library.AddFile(path)
		return nil
	})
}

// Effective are every spec, resolved, in name order: what a listing shows is
// what each target amounts to rather than what its own file happens to say.
func (l *Library) Effective() []Spec {
	resolved := make([]Spec, 0, len(l.specs))
	for _, name := range l.Names() {
		spec, diagnostics := l.Resolve(name)
		if len(diagnostics) > 0 {
			continue
		}
		resolved = append(resolved, spec)
	}
	return resolved
}

// Names are the specs that were read, sorted, which is what a listing shows.
func (l *Library) Names() []string {
	names := make([]string, 0, len(l.specs))
	for name := range l.specs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Get returns a spec by name.
func (l *Library) Get(name string) (Spec, bool) {
	spec, found := l.specs[name]
	return spec, found
}

// Boards are the specs that describe a board.
func (l *Library) Boards() []Spec {
	boards := []Spec{}
	for _, name := range l.Names() {
		if l.specs[name].Kind == KindBoard {
			boards = append(boards, l.specs[name])
		}
	}
	return boards
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                  */
/* -------------------------------------------------------------------------- */

// Resolve folds a board, the chip it carries and the architecture it is built as
// into one spec, which is what a generator reads.
//
// The chain is followed by name, and a name that does not resolve is reported
// rather than ignored: a board whose chip is missing is a board whose toolchain
// is missing, and a build with no compiler named is a build that fails later
// with a worse error.
func (l *Library) Resolve(name string) (Spec, []Diagnostic) {
	diagnostics := []Diagnostic{}
	spec, found := l.Get(name)
	if !found {
		return Spec{}, []Diagnostic{{Message: fmt.Sprintf("no spec named %q; this component knows %s",
			name, strings.Join(l.Names(), ", "))}}
	}

	chain := []Spec{}
	seen := map[string]bool{}
	for current := spec; ; {
		if seen[current.Name] {
			diagnostics = append(diagnostics, Diagnostic{
				Path: current.Source, Message: fmt.Sprintf("%q refers to itself; the chain is broken here", current.Name)})
			break
		}
		seen[current.Name] = true
		chain = append(chain, current)

		next := current.MCU
		if next == "" {
			next = current.Arch
		}
		if next == "" {
			break
		}
		parent, parentFound := l.Get(next)
		if !parentFound {
			// The chain names the part a spec is for, and no spec is named after
			// a part: a board is `nucleo-h753zi` and carries an `STM32H753ZI`, and
			// `STM32H753ZI` is the chip, not another spec. So the chain usually
			// ends here, which is not a problem.
			//
			// A spec that stops there has to be able to stand on its own. A board
			// spec does — it carries its own toolchain, memory map and flash
			// command — so it is left alone. One that cannot is incomplete, and
			// saying so names the spec that is short rather than the part it
			// mentioned.
			if !specIsSelfContained(current) {
				diagnostics = append(diagnostics, Diagnostic{
					Path: current.Source,
					Message: fmt.Sprintf("%q is for %q, which no spec declares, and it does not "+
						"carry a toolchain and memory map of its own to stand without one",
						current.Name, next),
				})
			}
			break
		}
		current = parent
	}

	// The chain is written child first, and a child's facts win: a board's flags
	// are added after its chip's, and the board's memory map is the one a program
	// is laid out in.
	resolved := chain[0]
	for _, parent := range chain[1:] {
		resolved = merge(parent, resolved)
	}
	resolved.ResolvedFrom = nil
	for _, one := range chain {
		resolved.ResolvedFrom = append(resolved.ResolvedFrom, one.Name)
	}
	resolved.Kind = spec.Kind
	resolved.Name = spec.Name
	resolved.Source = spec.Source
	return resolved, diagnostics
}

// merge folds a parent spec into a child, with the child's own facts winning.
func merge(parent, child Spec) Spec {
	if child.Core == "" {
		child.Core = parent.Core
	}
	if child.Arch == "" {
		child.Arch = parent.Arch
	}
	if child.MCU == "" {
		child.MCU = parent.MCU
	}
	if child.Toolchain.Compiler == "" {
		child.Toolchain.Compiler = parent.Toolchain.Compiler
	}
	if child.Toolchain.Linker == "" {
		child.Toolchain.Linker = parent.Toolchain.Linker
	}
	if child.Toolchain.Objcopy == "" {
		child.Toolchain.Objcopy = parent.Toolchain.Objcopy
	}
	if child.Toolchain.Size == "" {
		child.Toolchain.Size = parent.Toolchain.Size
	}
	if len(child.Toolchain.Flags) == 0 {
		child.Toolchain.Flags = parent.Toolchain.Flags
	} else {
		child.Toolchain.Flags = append(append([]string{}, parent.Toolchain.Flags...), child.Toolchain.Flags...)
	}
	if child.Memory.Flash.Origin == "" {
		child.Memory.Flash = parent.Memory.Flash
	}
	if child.Memory.RAM.Origin == "" {
		child.Memory.RAM = parent.Memory.RAM
	}
	if len(parent.Memory.Extra) > 0 {
		extra := map[string]Region{}
		for name, region := range parent.Memory.Extra {
			extra[name] = region
		}
		for name, region := range child.Memory.Extra {
			extra[name] = region
		}
		child.Memory.Extra = extra
	}
	child.Defines = append(append([]string{}, parent.Defines...), child.Defines...)
	child.Flags = append(append([]string{}, parent.Flags...), child.Flags...)
	if child.Flash.Tool == "" {
		child.Flash = parent.Flash
	}
	return child
}

// AllFlags are every flag a target is built with, in the order a compiler reads
// them: the architecture's, then the chip's.
func (s Spec) AllFlags() []string {
	return append(append([]string{}, s.Toolchain.Flags...), s.Flags...)
}

// Regions are every memory region a target has, in the order a linker script
// declares them: flash, then RAM, then the extra regions, sorted.
func (s Spec) Regions() []NamedRegion {
	regions := []NamedRegion{}
	if s.Memory.Flash.Origin != "" {
		regions = append(regions, NamedRegion{Name: "FLASH", Region: s.Memory.Flash, Kind: "flash"})
	}
	if s.Memory.RAM.Origin != "" {
		regions = append(regions, NamedRegion{Name: "RAM", Region: s.Memory.RAM, Kind: "ram"})
	}
	names := make([]string, 0, len(s.Memory.Extra))
	for name := range s.Memory.Extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		regions = append(regions, NamedRegion{
			Name: strings.ToUpper(name), Region: s.Memory.Extra[name], Kind: "extra"})
	}
	return regions
}

// NamedRegion is one memory region with the name a linker script gives it.
type NamedRegion struct {
	Name   string
	Region Region
	Kind   string
}

// LinkRegions are the regions a linker script may use, in the order a script
// declares them.
func (s Spec) LinkRegions() []NamedRegion {
	regions := []NamedRegion{}
	for _, region := range s.Regions() {
		if region.Region.Linkable() {
			regions = append(regions, region)
		}
	}
	return regions
}

// PinNames are a board's pins in the order a person reads them: by the number
// the board gives them, so GP2 comes before GP16.
func (s Spec) PinNames() []string {
	names := make([]string, 0, len(s.Pins))
	for name := range s.Pins {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		_, leftNumber := trailingNumber(names[i])
		_, rightNumber := trailingNumber(names[j])
		// Two pins of the same board are read by their numbers: a board names
		// them GP2 and GP16, and a person reads the second after the first.
		if leftNumber >= 0 && rightNumber >= 0 && leftNumber != rightNumber {
			return leftNumber < rightNumber
		}
		return names[i] < names[j]
	})
	return names
}

// trailingNumber is the number a pin name ends in, and where the name before it
// stops, or -1 when it does not end in one.
func trailingNumber(name string) (prefix string, number int) {
	index := len(name)
	for index > 0 && name[index-1] >= '0' && name[index-1] <= '9' {
		index--
	}
	if index == len(name) {
		return name, -1
	}
	value := 0
	for _, digit := range name[index:] {
		value = value*10 + int(digit-'0')
	}
	return name[:index], value
}

// Pin is the pin a board wires to a role, such as the one it calls led.
func (s Spec) Pin(role string) (Pin, bool) {
	for _, pin := range s.Pins {
		if pin.Role == role {
			return pin, true
		}
	}
	return Pin{}, false
}

// specIsSelfContained reports whether a spec carries what a build needs without a
// parent: how to compile it, and where it lives in memory.
func specIsSelfContained(spec Spec) bool {
	return spec.Toolchain.Compiler != "" && spec.Memory.RAM.Origin != "" && spec.Memory.Flash.Origin != ""
}
