//
// @file registers.go
// @brief A chip's register map as something other than an SVD file.
//
// An SVD file is XML, and reading it in every language that wants to show a
// peripheral is a second reader for the same facts in a language with no tests to
// hold them. So this reads it once — in the package that already does, with the
// tests that already check it — and answers with it as JSON.
//
// Which is what an editor's peripheral tree needs. It shows names, addresses and
// bit fields, and it has no way to read them out of a file. What it cannot have is
// the values: those come from a live session, over the debug protocol, where the
// register tree is answered from the board.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"rho/src/deps"
	"rho/src/targetspec"
)

// RegisterMap is a chip's registers, as JSON.
type RegisterMap struct {
	// Device is the part the file describes.
	Device string `json:"device"`
	// Source is the file it was read from, so a reader can say where the map it is
	// showing came from.
	Source string `json:"source,omitempty"`
	// Comment is what the map's own source said about it — a target spec's comment,
	// or a plugin's.
	Comment string `json:"comment,omitempty"`
	// Peripherals are the interfaces the chip brings out.
	Peripherals []MappedPeripheral `json:"peripherals"`
}

// MappedPeripheral is one interface and where it is.
type MappedPeripheral struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Address     string           `json:"address"`
	Registers   []MappedRegister `json:"registers"`
}

// MappedRegister is one register and where it is.
type MappedRegister struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Address     string        `json:"address"`
	Size        int           `json:"size"`
	Access      string        `json:"access,omitempty"`
	Readable    bool          `json:"readable"`
	Fields      []MappedField `json:"fields,omitempty"`
}

// MappedField is the bits of a register that mean something together.
type MappedField struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Offset and Width are where the field sits, so a reader can mask it.
	Offset int `json:"offset"`
	Width  int `json:"width"`
	// Bits is the same thing said the way a datasheet says it, because that is how
	// a person reads it while looking for a bit.
	Bits string `json:"bits"`
	// Mask is the field's bits, for a reader that would rather compare than shift.
	Mask string `json:"mask"`
}

// DescribeRegisters is a register map as JSON can be shown from.
//
// A field this cannot place is left out rather than guessed at: a shift by a width
// nobody stated produces a plausible number that means nothing, and a peripheral
// tree is read as fact.
func DescribeRegisters(load *SVD, source string, comment string) RegisterMap {
	out := RegisterMap{Device: load.Device, Source: source, Comment: comment}
	if load == nil {
		return out
	}
	out.Peripherals = make([]MappedPeripheral, 0, len(load.Peripherals))
	for _, peripheral := range load.Peripherals {
		base, parsed := peripheral.Address()
		mapped := MappedPeripheral{
			Name:        peripheral.Name,
			Description: peripheral.Description,
			Address:     peripheral.BaseAddress,
			Registers:   []MappedRegister{},
		}
		if !parsed {
			// No base address means no addresses: a peripheral described only by its
			// interrupts. Kept, because the name is still part of the map, and with
			// no registers rather than with wrong ones.
			out.Peripherals = append(out.Peripherals, mapped)
			continue
		}
		for _, register := range peripheral.AllRegisters() {
			address, addressed := register.Address(base)
			if !addressed {
				continue
			}
			mapped.Registers = append(mapped.Registers, describeRegister(register, address))
		}
		out.Peripherals = append(out.Peripherals, mapped)
	}
	sort.SliceStable(out.Peripherals, func(i, j int) bool {
		return out.Peripherals[i].Name < out.Peripherals[j].Name
	})
	return out
}

func describeRegister(register Register, address uint64) MappedRegister {
	mapped := MappedRegister{
		Name:        register.Name,
		Description: register.Description,
		Address:     hex(address),
		Size:        register.Width(),
		Access:      register.Access,
		Readable:    register.Readable(),
	}
	for _, field := range register.Fields {
		offset, width, ok := field.Bounds()
		if !ok {
			continue
		}
		mask, _ := Mask(field)
		mapped.Fields = append(mapped.Fields, MappedField{
			Name:        field.Name,
			Description: field.Description,
			Offset:      offset,
			Width:       width,
			Bits:        fmt.Sprintf("%d..%d", offset, offset+width-1),
			Mask:        hex(mask),
		})
	}
	return mapped
}

// hex is an address the way a datasheet writes one: `0x40011000`, with no leading
// zeros and no digit grouping.
func hex(value uint64) string { return "0x" + strconv.FormatUint(value, 16) }

// EncodeRegisters is a register map as the JSON a tree view reads.
func EncodeRegisters(map2 RegisterMap) ([]byte, error) {
	encoded, err := json.MarshalIndent(map2, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("render the register map: %w", err)
	}
	return append(encoded, '\n'), nil
}

/* -------------------------------------------------------------------------- */
/*  Finding the file                                                           */
/* -------------------------------------------------------------------------- */

// RegisterMapFile is where a board's register map is, for a reader with a target and
// a project to read from.
//
// The same order a session reads it in, so the tree and the session show the same
// chip's registers: an explicit path, then what the target named — beside the
// program, and in the project — and then what a plugin says the vendor keeps it at.
func RegisterMapFile(declared string, root string, program string, vendorRoots []string) (string, bool) {
	return RegisterMapPath(declared, root, program, vendorRoots)
}

// Found is the register map a board has, and where it came from.
type Found struct {
	// Path is the file, wherever it was found or fetched to.
	Path string
	// From is where it came from: the project, a platform's vendor tree, the cache,
	// or a plugin. Said in the failure as well as in a diagnostic, because "where
	// was it looked for" is what a person needs when it was not found.
	From string
	// Comment is what the source said about itself, when a plugin supplied it.
	Comment string
}

// FindRegisterMap is the register map a board has.
//
// Four places, in this order, and the first one ends the question:
//
//  1. what the project itself says, in its own manifest's `debug:` section — a name
//     somebody wrote down is an answer, and a session that looked past it would be
//     preferring a guess from a vendor's naming to a fact
//  2. a file the project carries, or a platform it depends on carries
//  3. wherever a plugin says this vendor keeps it
//  4. the cache, by working out from the part number which published file the
//     vendor writes it under
//
// Nothing here is a guess in the last sense of the word. A project that names a map
// gets it; a project that carries one gets it; and a part number is matched
// against what the vendor actually publishes, so a name that matches nothing is
// reported rather than approximated by the nearest thing to it.
//
// What is returned when none of them settles it is an error, and the error is what
// makes a session refuse to start: a debugging session on a microcontroller whose
// registers cannot be named is a session in which every register view is wrong
// without saying so.
func FindRegisterMap(request TargetFacts) (Found, error) {
	// 1. The project's own declaration for *this board*: a file, chosen by whoever
	// wired the board up. It is the answer when there is one, and nothing else is
	// consulted.
	if declared := request.Declared(); declared != "" {
		found, err := findDeclared(declared, request)
		if err == nil {
			found.From = "the project's own manifest"
			return found, nil
		}
		// A project that named a file which is not there has made a claim this cannot
		// honour. Falling through to the vendor's naming would debug a chip the
		// project did not ask about, so it is said instead.
		return Found{}, fmt.Errorf("the project names %s for %s, and it is not here: %w",
			declared, request.TargetName, err)
	}

	// 2. A file a repository carries.
	declared, source := "", SVDSource{}
	if request.Target != nil {
		declared = strings.TrimSpace(request.Target.Debug.SVD)
		if parsed, ok := ParseSVDSource(request.Target.Debug.SVDSource); ok {
			source = parsed
		}
	}
	if path, found := RegisterMapFile(declared, request.Root, request.Program, request.VendorRoots); found {
		return Found{Path: path, From: "the project or a platform it depends on"}, nil
	}
	if declared != "" {
		// The board's spec named a file and nothing here carries it. That is a
		// statement about where the map should be, and it is said rather than
		// worked around: carrying on to the vendor's naming would look for a
		// different map than the one the spec asked for, and succeed at it.
		return Found{}, fmt.Errorf(
			"the target names the register map %q, and nothing here carries it: "+
				"vendor it into the project, add it to a platform's vendor tree, or name the file "+
				"in the published set with the project's own `debug:` section", declared)
	}

	// 3. A plugin that knows where this vendor keeps them.
	if request.FromPlugin != nil {
		if path, comment := request.FromPlugin(); path != "" {
			if _, found := RegisterMapFile(path, request.Root, request.Program, request.VendorRoots); found {
				return Found{Path: path, From: "a plugin", Comment: comment}, nil
			}
		}
	}

	// 4. The part number, worked out against what the vendor publishes.
	if source.Path == "" {
		part := ""
		if request.Target != nil {
			part = strings.TrimSpace(request.Target.MCU)
		}
		if part == "" {
			return Found{}, ErrNoRegisterMap
		}
		resolved, _, err := ResolveDeviceForPart(part, request.Settings)
		if err != nil {
			return Found{}, err
		}
		source = resolved
	}
	if !request.Settings.Fetchable() {
		return Found{}, fmt.Errorf(
			"the register map for %s is not in the cache and this component will not fetch one (%s)",
			source.Path, defaultRepositoryName(request.Settings))
	}
	path, err := EnsureRegisterMap(source, request.Settings)
	if err != nil {
		return Found{}, fmt.Errorf("the register map for %s comes from %s: %w",
			source.Path, defaultRepositoryName(request.Settings), err)
	}
	return Found{Path: path, From: "the cache"}, nil
}

// TargetFacts is everything the search for a register map is a search *among*: the
// project's own declaration, the board it is built for, and where a file could be.
type TargetFacts struct {
	// Project is what the project's own manifest says about its boards, which is the
	// answer for the board this session is about.
	Project *DebugMapping
	// TargetName is the board being debugged, which is the key that project's
	// declaration is under.
	TargetName string
	// Target is the board the project is built for, which names a file of its own
	// and the part it carries.
	Target *targetspec.Target
	// Root is the project directory, and Program the built binary, which is where a
	// file a repository carries is looked for.
	Root    string
	Program string
	// VendorRoots are the vendor trees of the platforms the project depends on.
	VendorRoots []string
	// Settings is what this component is configured to fetch from and where.
	Settings Settings
	// FromPlugin is where a plugin says this vendor keeps its register maps.
	FromPlugin func() (string, string)
}

// DebugMapping is what a project says about its chip, as its manifest carries it.
//
// The manifest's own type, aliased rather than restated: a session reads what a
// project wrote, and a second declaration of it here is a second thing to keep in
// step with the first.
type DebugMapping = deps.DebugMapping

// ErrNoRegisterMap is a session with no board to describe: a program on this
// machine has no peripherals, which is a fact rather than a failure.
var ErrNoRegisterMap = errors.New("no register map is named and no part is known")

// findDeclared is the file a project named, read from where it says it is: in the
// project, or in the vendor tree of a platform it depends on.
func findDeclared(svd string, request TargetFacts) (Found, error) {
	declared := strings.TrimSpace(svd)
	if declared == "" {
		return Found{}, fmt.Errorf("the project names no file")
	}
	if path, found := RegisterMapPath(declared, request.Root, request.Program, request.VendorRoots); found {
		return Found{Path: path}, nil
	}
	return Found{}, fmt.Errorf("nothing carries %s", declared)
}

// ResolveDeviceForPart is the published file a part number means, by working the
// number out against what the vendor publishes.
//
// The index is fetched and cached on the first call on a machine, and read from the
// cache after that: a lookup is a question about a list, and the list is the same on
// every session.
func ResolveDeviceForPart(part string, settings Settings) (SVDSource, []string, error) {
	index, err := LoadIndex(settings, false)
	if err != nil {
		return SVDSource{}, nil, err
	}
	return FindDevice(index, part)
}

// ResolveDeviceForPartCached is the same answer without reaching for the network
// when the list is already here: for a caller that is *reporting* what a session
// would find, where fetching an index it does not need would be a surprise in a
// command whose name means "tell me".
func ResolveDeviceForPartCached(part string, settings Settings) (SVDSource, []string, error) {
	index, cached := readIndex(indexPath(settings))
	if !cached {
		return ResolveDeviceForPart(part, settings)
	}
	return FindDevice(index, part)
}

// partOf is the part number a board carries, and the namesTried are the spellings a
// search for it looked under.
func partOf(target *targetspec.Target) string {
	if target == nil {
		return ""
	}
	return strings.TrimSpace(target.MCU)
}

// Declared is the file the project named for this board, or nothing when it named
// none — which is the ordinary case for a board it was not built for.
func (t TargetFacts) Declared() string {
	if t.Project == nil {
		return ""
	}
	return t.Project.PathFor(t.TargetName)
}

// namesTried is what a search looked under, which is on the error because it is the
// part of the answer a person needs: it says whether their part number is the
// problem or whether the vendor has nothing for their chip.
func namesTried(err error) []string {
	var unknown *unknownDeviceError
	if errors.As(err, &unknown) {
		return unknown.tried
	}
	return nil
}

// A sentence rather than a failure about a file: nothing failed, and the thing that
// is missing is an answer. It names what was tried, because a person looking at it
// can see whether their part number is the problem or whether the vendor simply has
// nothing for their chip — and it names where to write the answer, which is a
// project they already have.
// CannotIdentify is the refusal a session gives when the naming does not settle it,
// naming the board so a person knows which key to write it under.
func CannotIdentify(target string, part string, tried []string, err error) error {
	// Read off the failure when the caller has none: the names a search tried are
	// part of what it found, and a refusal that omits them sends somebody to guess
	// which spelling was wrong.
	if len(tried) == 0 {
		tried = namesTried(err)
	}
	reason := "the naming does not settle it"
	if err != nil {
		reason = err.Error()
	}
	out := fmt.Sprintf("Cannot identify svd for target under debug: %s.", reason)
	if target != "" {
		out += fmt.Sprintf("\n  the board is %s", target)
	}
	if part != "" {
		out += fmt.Sprintf("\n  the part is %s, and the published set was searched under:", part)
		for _, candidate := range tried {
			out += "\n    " + candidate
		}
	}
	out += "\n\n  Point at the file for this board, in the project's own module.yml:"
	out += "\n\n    debug:"
	out += "\n      svd:"
	// The board's own name is the key — the part number is not, and a person
	// reading this has to be told which to write.
	key := target
	if key == "" {
		key = "<target>"
	}
	out += fmt.Sprintf("\n        %s: vendor/cmsis_svd/stm32h7xx.svd", key)
	out += "\n\n  A path per board — a project built for several boards has several chips, and"
	out += "\n  one of them's registers standing in for another's would be wrong without"
	out += "\n  saying so. What is named here is read as written, and nothing else is"
	out += "\n  consulted: not the board's spec, not the published set."
	return fmt.Errorf("%s", out)
}

// defaultRepositoryName is the repository a failure names, for a person who did
// not change it.
func defaultRepositoryName(settings Settings) string {
	if repository := strings.TrimSpace(settings.Repository); repository != "" {
		return repository
	}
	return DefaultRepository
}

// WriteRegisters writes a register map as JSON to a file, or to standard output when
// there is no file.
func WriteRegisters(out *os.File, path string, map2 RegisterMap) error {
	encoded, err := EncodeRegisters(map2)
	if err != nil {
		return err
	}
	if path == "" || path == "-" {
		_, err = out.Write(encoded)
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}
