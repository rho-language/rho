//
// @file svd.go
// @brief A chip's register map, read from the file a vendor ships.
//
// Debugging a microcontroller without its registers is debugging half a machine.
// The program is stopped at a line that reads a status register, the value is
// there, and there is nothing to call it: the person is reading `0x00000000` and
// has no way to ask whether that is "no interrupt pending" or "this peripheral is
// not clocked".
//
// A CMSIS-SVD file is what turns the address back into a name. It is XML, written
// by every vendor's tooling, and it is read here rather than handed to an editor
// as a file to parse because the same three questions are asked of it by every
// consumer: which peripherals exist, what is in one, and what each field means.
//
// Reading it is not the interesting part; what is interesting is what it is worth
// being strict about. A register at an address the chip does not have, or a field
// with a width nobody can parse, is reported rather than shown — because a
// peripheral tree with a wrong value in it is worse than one with a gap in it,
// since the wrong value is believed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodbg

import (
	"encoding/xml"
	"fmt"
	"math/bits"
	"os"
	"strconv"
	"strings"
)

// SVD is a chip's register map.
type SVD struct {
	// Device is the part the file describes.
	Device string `xml:"-"`
	// Peripherals are the interfaces the chip has, by name, in the order the file
	// lists them — which is grouped by the address ranges, so it reads as a memory
	// map rather than as an alphabet.
	Peripherals []Peripheral `xml:"peripherals>peripheral"`
}

// Peripheral is one interface the chip brings out.
type Peripheral struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	// BaseAddress is where it is. Rendered as a number when the file gives one that
	// parses, which is the only case where the register tree has anywhere to read.
	BaseAddress string     `xml:"baseAddress"`
	GroupName   string     `xml:"groupName"`
	Registers   []Register `xml:"registers>register"`
	Clusters    []Cluster  `xml:"registers>cluster"`
	DerivedFrom string     `xml:"derivedFrom,attr"`
}

// Cluster is a group of registers with an address offset of their own, which is
// how a large peripheral is described: the GPIO block has registers and clusters
// of registers, and flattening them into one list would lose the nesting that says
// which is which.
type Cluster struct {
	Name          string     `xml:"name"`
	Description   string     `xml:"description"`
	AddressOffset string     `xml:"addressOffset"`
	Registers     []Register `xml:"register"`
	Clusters      []Cluster  `xml:"cluster"`
	DerivedFrom   string     `xml:"derivedFrom,attr"`
}

// Register is one register.
type Register struct {
	Name          string `xml:"name"`
	Description   string `xml:"description"`
	AddressOffset string `xml:"addressOffset"`
	// Size is the register's width in bits, as the file writes it — which is
	// decimal or hexadecimal depending on the vendor, and `<size>0x20</size>` is
	// an ordinary thing to find in a file ST publishes. Read as text and parsed by
	// SizeBits rather than decoded into a number, because a decoder handed a
	// hexadecimal value for an integer field refuses the whole file, and one odd
	// register would then cost every register in it.
	Size        string  `xml:"size"`
	Access      string  `xml:"access"`
	ResetValue  string  `xml:"resetValue"`
	Fields      []Field `xml:"fields>field"`
	DerivedFrom string  `xml:"derivedFrom,attr"`
}

// Field is part of a register: the bits that mean something together.
type Field struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	BitOffset   string `xml:"bitOffset"`
	BitWidth    string `xml:"bitWidth"`
	LSB         string `xml:"lsb"`
	MSB         string `xml:"msb"`
	// BitRange is how a field is described when it is not an offset and a width,
	// which some files use instead.
	BitRange string `xml:"bitRange"`
	Access   string `xml:"access"`
}

// Bounds is a field's bits, and whether they were stated in a way that parses.
//
// Reported as a pair rather than as a position because a field whose bounds are
// missing is a field this cannot show, and one with bounds of zero width is a file
// that says something this cannot make sense of. Neither is worth guessing at: a
// shift by a width nobody stated is a wrong value in a register view.
func (f Field) Bounds() (offset int, width int, ok bool) {
	if f.LSB != "" && f.MSB != "" {
		low, lowErr := strconv.Atoi(strings.TrimSpace(f.LSB))
		high, highErr := strconv.Atoi(strings.TrimSpace(f.MSB))
		if lowErr == nil && highErr == nil && high >= low {
			return low, high - low + 1, true
		}
	}
	if f.BitRange != "" {
		// `msb:lsb`, the other way round.
		if high, low, found := strings.Cut(strings.TrimSpace(f.BitRange), ":"); found {
			highValue, highErr := strconv.Atoi(strings.TrimSpace(high))
			lowValue, lowErr := strconv.Atoi(strings.TrimSpace(low))
			if highErr == nil && lowErr == nil && highValue >= lowValue {
				return lowValue, highValue - lowValue + 1, true
			}
		}
		return 0, 0, false
	}
	if f.BitOffset != "" && f.BitWidth != "" {
		offsetValue, offsetErr := strconv.Atoi(strings.TrimSpace(f.BitOffset))
		widthValue, widthErr := strconv.Atoi(strings.TrimSpace(f.BitWidth))
		if offsetErr == nil && widthErr == nil && widthValue > 0 {
			return offsetValue, widthValue, true
		}
	}
	return 0, 0, false
}

// Address is a register's address, and whether the file said one that parses.
func (r Register) Address(base uint64) (uint64, bool) {
	offset, err := strconv.ParseUint(strings.TrimSpace(r.AddressOffset), 0, 64)
	if err != nil {
		return 0, false
	}
	return base + offset, true
}

// Width is the register's size in bytes: one when the file says 8 bits, two when
// it says 32, eight when it says 64, and two when it says nothing at all, because a
// 32-bit register is what an SVD file describes when it is describing a register.
func (r Register) Width() int {
	switch r.SizeBits() {
	case 8:
		return 1
	case 16:
		return 2
	case 64:
		return 8
	default:
		return 4
	}
}

// SizeBits is the register's width in bits, as the number the file wrote.
//
// Both spellings, because both occur in the files this is pointed at:
// `<size>32</size>` and `<size>0x20</size>` are the same width written by two
// vendors. A width that cannot be read at all falls back to 32, which is what a
// register map means when it describes a register.
func (r Register) SizeBits() int {
	text := strings.TrimSpace(r.Size)
	if text == "" {
		return 32
	}
	base := 10
	if lowered := strings.ToLower(text); strings.HasPrefix(lowered, "0x") {
		text, base = lowered[2:], 16
	}
	parsed, err := strconv.ParseInt(text, base, strconv.IntSize)
	if err != nil || parsed <= 0 {
		return 32
	}
	return int(parsed)
}

// Readable reports whether the register's contents can be shown.
//
// A write-only register has a value that reads back as whatever the hardware
// happens to drive, which is not the value that was written. Showing it as though
// it were would be showing noise as though it were state.
func (r Register) Readable() bool {
	return !strings.EqualFold(strings.TrimSpace(r.Access), "write-only") &&
		!strings.EqualFold(strings.TrimSpace(r.Access), "writeOnce")
}

// LoadSVD reads a register map from a file.
func LoadSVD(path string) (*SVD, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the register map: %w", err)
	}
	return ParseSVD(data, path)
}

// ParseSVD reads a register map from what a file holds.
//
// The file is read and then its `derivedFrom` references are followed, because
// that is how the vendors write one: every USART on an STM32 part is a peripheral
// with a base address and `derivedFrom` naming a peripheral that carries the
// registers. Read literally, USART3 has no registers at all — which is true of the
// file and not of the chip, and a peripheral tree that says so is worse than one
// that says nothing.
func ParseSVD(data []byte, path string) (*SVD, error) {
	device := struct {
		Version     string `xml:"version"`
		Name        string `xml:"name"`
		Description string `xml:"description"`
		SVDP        struct {
			Version string `xml:"version"`
		} `xml:"svd"`
		Peripherals []Peripheral `xml:"peripherals>peripheral"`
	}{}
	if err := xml.Unmarshal(data, &device); err != nil {
		return nil, fmt.Errorf("%s is not a register map: %w", path, err)
	}
	if len(device.Peripherals) == 0 {
		return nil, fmt.Errorf("%s describes no peripherals", path)
	}
	out := &SVD{Device: device.Name, Peripherals: resolvePeripherals(device.Peripherals)}
	return out, nil
}

// resolvePeripherals is every peripheral with what it derives filled in.
//
// In passes rather than recursively, because a file may derive in a cycle — one
// peripheral from another that derives from the first — and a cycle resolved
// recursively is a stack overflow on somebody's board. Each pass fills in what it
// can and a peripheral that stops changing is finished.
func resolvePeripherals(peripherals []Peripheral) []Peripheral {
	byName := map[string]Peripheral{}
	for _, peripheral := range peripherals {
		byName[strings.ToUpper(peripheral.Name)] = peripheral
	}
	resolved := append([]Peripheral{}, peripherals...)

	for pass := 0; pass < len(resolved); pass++ {
		changed := false
		for index := range resolved {
			filled, done := fillPeripheral(resolved[index], byName)
			if done && !samePeripheral(filled, resolved[index]) {
				resolved[index] = filled
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	for index := range resolved {
		resolved[index].Registers = mergeRegisters(nil, resolved[index].Registers)
		resolved[index].Clusters = mergeClusters(nil, resolved[index].Clusters)
	}
	return resolved
}

// fillPeripheral is one peripheral with its parent's registers in, and whether
// that is all it was waiting for.
//
// The parent is looked up by name, and a name with a dot in it names a register
// inside another peripheral — `derivedFrom="USART1.CR1"` — which is a register
// inheriting rather than a peripheral, so the dotted form is not a peripheral and
// the peripheral has nothing to fill in.
func fillPeripheral(peripheral Peripheral, byName map[string]Peripheral) (Peripheral, bool) {
	parent, found := byName[strings.ToUpper(parentNameOf(peripheral.DerivedFrom))]
	if !found || parent.Name == peripheral.Name {
		return peripheral, false
	}
	filled := peripheral
	if filled.Description == "" {
		filled.Description = parent.Description
	}
	if filled.BaseAddress == "" {
		filled.BaseAddress = parent.BaseAddress
	}
	filled.Registers = mergeRegisters(parent.Registers, peripheral.Registers)
	filled.Clusters = mergeClusters(parent.Clusters, peripheral.Clusters)
	filled.DerivedFrom = ""
	return filled, true
}

// parentNameOf is the element a `derivedFrom` names, without the dotted part that
// says which element inside it.
func parentNameOf(derived string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(derived), ".")
	return name
}

// mergeRegisters is a child's registers over a parent's, by name.
//
// The child's register replaces the parent's rather than joining with it, because
// that is what the file means: the child says what is different about this one and
// inherits the rest, and a register the child does not mention is the parent's
// unchanged.
func mergeRegisters(parent []Register, child []Register) []Register {
	if len(parent) == 0 {
		return child
	}
	byName := map[string]int{}
	out := append([]Register{}, parent...)
	for index, register := range out {
		byName[strings.ToUpper(register.Name)] = index
	}
	for _, register := range child {
		if index, found := byName[strings.ToUpper(register.Name)]; found {
			out[index] = register
			continue
		}
		byName[strings.ToUpper(register.Name)] = len(out)
		out = append(out, register)
	}
	return out
}

// mergeClusters is the same rule for clusters, applied to every level.
func mergeClusters(parent []Cluster, child []Cluster) []Cluster {
	if len(parent) == 0 {
		for index := range child {
			child[index].Registers = mergeRegisters(nil, child[index].Registers)
			child[index].Clusters = mergeClusters(nil, child[index].Clusters)
		}
		return child
	}
	byName := map[string]int{}
	out := append([]Cluster{}, parent...)
	for index, cluster := range out {
		out[index].Registers = mergeRegisters(nil, cluster.Registers)
		out[index].Clusters = mergeClusters(nil, cluster.Clusters)
		byName[strings.ToUpper(cluster.Name)] = index
	}
	for _, cluster := range child {
		cluster.Registers = mergeRegisters(nil, cluster.Registers)
		cluster.Clusters = mergeClusters(nil, cluster.Clusters)
		if index, found := byName[strings.ToUpper(cluster.Name)]; found {
			out[index] = cluster
			continue
		}
		byName[strings.ToUpper(cluster.Name)] = len(out)
		out = append(out, cluster)
	}
	return out
}

// samePeripheral is whether two peripherals say the same thing, which is what ends
// the passes.
func samePeripheral(left, right Peripheral) bool {
	return len(left.Registers) == len(right.Registers) &&
		len(left.Clusters) == len(right.Clusters) &&
		left.Description == right.Description &&
		left.BaseAddress == right.BaseAddress
}

// Find is a peripheral by name, case-insensitively.
//
// Case-insensitive because the files disagree about it: the same chip's registers
// appear as `USART1`, `Usart1` and `usart1` across vendors, and a person asking
// for one by name has the name their datasheet uses.
func (s *SVD) Find(name string) (Peripheral, bool) {
	for _, peripheral := range s.Peripherals {
		if strings.EqualFold(peripheral.Name, name) {
			return peripheral, true
		}
	}
	return Peripheral{}, false
}

// Peripheral.Address is where a peripheral is, and whether the file said so.
func (p Peripheral) Address() (uint64, bool) {
	return parseAddress(p.BaseAddress)
}

// AllRegisters are a peripheral's registers with the clusters flattened into them,
// in address order.
//
// Flattened because an editor shows a list: a cluster is how the *file* is
// organised, and a peripheral tree with three nested levels of grouping in it is a
// tree nobody scrolls. The cluster's offset is added so the order is the order the
// registers are at.
func (p Peripheral) AllRegisters() []Register {
	out := []Register{}
	out = append(out, p.Registers...)
	for _, cluster := range p.Clusters {
		out = append(out, cluster.registersAt(0)...)
	}
	sortRegisters(out)
	return out
}

func (c Cluster) registersAt(base uint64) []Register {
	// Its own offset added to the base it was reached through, which is what makes
	// a cluster inside a cluster land where the nesting says it does.
	own, err := strconv.ParseUint(strings.TrimSpace(c.AddressOffset), 0, 64)
	if err != nil {
		own = 0
	}
	offset := base + own
	out := []Register{}
	for _, register := range c.Registers {
		adjusted := register
		adjusted.AddressOffset = fmt.Sprintf("0x%x", offset+mustOffset(register.AddressOffset))
		out = append(out, adjusted)
	}
	for _, nested := range c.Clusters {
		out = append(out, nested.registersAt(offset)...)
	}
	return out
}

func mustOffset(value string) uint64 {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 0, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// sortRegisters puts registers in address order, and by name where two share one.
func sortRegisters(registers []Register) {
	for index := 1; index < len(registers); index++ {
		for position := index; position > 0; position-- {
			left, _ := registers[position-1].Address(0)
			right, _ := registers[position].Address(0)
			if left > right || (left == right && registers[position-1].Name > registers[position].Name) {
				registers[position-1], registers[position] = registers[position], registers[position-1]
				continue
			}
			break
		}
	}
}

// DecodeField reads one field out of a register's value.
//
// `width` is the register's width in bytes, and it is the reason this can refuse
// anything at all: a field that does not fit in the register it is in is a file
// describing something wider than the hardware has, and truncating it would show a
// plausible number that means nothing. The shift is done in 64 bits and masked with
// the field's own width, so a field that reaches past the register is reported as
// not readable rather than as a value.
func DecodeField(field Field, value uint64, width int) (uint64, bool) {
	offset, fieldWidth, ok := field.Bounds()
	if !ok || fieldWidth <= 0 || width <= 0 {
		return 0, false
	}
	limit := bits.UintSize * 8
	// Wider than the register: refused.
	if width < bits.UintSize/8 && offset >= width*8 {
		return 0, false
	}
	if fieldWidth > limit-offset {
		return 0, false
	}
	mask := uint64(1)<<uint(fieldWidth) - 1
	if bits.OnesCount64(mask) != fieldWidth {
		return 0, false
	}
	return (value >> uint(offset)) & mask, true
}

// Mask is a field's mask, for showing which bits it is.
func Mask(field Field) (uint64, bool) {
	offset, width, ok := field.Bounds()
	if !ok || width <= 0 || offset >= bits.UintSize*8 || width > bits.UintSize*8-offset {
		return 0, false
	}
	return (uint64(1)<<uint(width) - 1) << uint(offset), true
}

func parseAddress(value string) (uint64, bool) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 0, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
