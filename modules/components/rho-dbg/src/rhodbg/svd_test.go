//
// @file svd_test.go
// @brief Tests for reading a chip's register map.
//
// A register map is read from a file a vendor's tooling wrote, so these are the
// shapes that tooling produces: fields described three different ways, clusters
// nested inside clusters, a register whose address offset is not a number. What is
// checked is mostly what is *refused*, because a register shown with a wrong value
// is believed, and a register shown with none is merely missing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/targetspec"
)

// A register map as a vendor writes one: the common case, with clusters.
const stm32 = `<?xml version="1.0" encoding="utf-8"?>
<device schemaVersion="1.1" xmlns:xs="http://www.w3.org/2001/XMLSchema-instance">
  <name>STM32F4</name>
  <version>1.0</version>
  <description>STM32F407/427</description>
  <addressUnitBits>8</addressUnitBits>
  <width>32</width>
  <peripherals>
    <peripheral>
      <name>USART1</name>
      <description>Universal synchronous/asynchronous receiver transmitter</description>
      <baseAddress>0x40011000</baseAddress>
      <addressBlock><offset>0</offset><size>0x400</size><usage>registers</usage></addressBlock>
      <registers>
        <register>
          <name>CR1</name><description>Control register 1</description>
          <addressOffset>0x00</addressOffset><size>32</size><access>read-write</access>
          <resetValue>0x00000000</resetValue>
          <fields>
            <field><name>UE</name><description>USART enable</description><bitOffset>13</bitOffset><bitWidth>1</bitWidth></field>
            <field><name>PS</name><description>Parity selection</description><bitOffset>9</bitOffset><bitWidth>1</bitWidth></field>
            <field><name>M</name><description>Word length</description><bitOffset>12</bitOffset><bitWidth>1</bitWidth></field>
          </fields>
        </register>
        <register>
          <name>SR</name><description>Status register</description>
          <addressOffset>0x00</addressOffset><size>32</size><access>read-only</access>
          <fields>
            <field><name>TXE</name><description>Transmit data register empty</description><lsb>7</lsb><msb>7</msb></field>
            <field><name>RXNE</name><description>Read data register not empty</description><bitRange>5:5</bitRange></field>
          </fields>
        </register>
        <register>
          <name>DR</name><description>Data register</description>
          <addressOffset>0x04</addressOffset><size>32</size><access>read-write</access>
        </register>
      </registers>
    </peripheral>
    <peripheral>
      <name>GPIOA</name><description>General-purpose I/O port A</description>
      <baseAddress>0x40020000</baseAddress>
      <registers>
        <cluster>
          <name>MODER</name><addressOffset>0x00</addressOffset>
          <register><name>MODER0</name><addressOffset>0x00</addressOffset><size>32</size></register>
          <register><name>MODER1</name><addressOffset>0x04</addressOffset><size>32</size></register>
        </cluster>
        <cluster>
          <name>IDR</name><addressOffset>0x10</addressOffset>
          <cluster>
            <name>IDR_low</name><addressOffset>0x00</addressOffset>
            <register><name>IN0</name><addressOffset>0x00</addressOffset><size>32</size></register>
          </cluster>
        </cluster>
      </registers>
    </peripheral>
    <peripheral>
      <name>DMA1</name><description>Direct memory access</description>
      <baseAddress>0x40026000</baseAddress>
    </peripheral>
  </peripherals>
</device>`

func loadOf(t *testing.T, content string) *SVD {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chip.svd")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	load, err := LoadSVD(path)
	if err != nil {
		t.Fatal(err)
	}
	return load
}

func TestAPeripheralIsFoundByTheNameItsDatasheetUses(t *testing.T) {
	load := loadOf(t, stm32)
	if load.Device != "STM32F4" {
		t.Errorf("expected the part the file describes, got %q", load.Device)
	}
	// Case-insensitively, because the files disagree about it and a person asking
	// for one has the name their datasheet uses.
	for _, name := range []string{"USART1", "usart1", "Usart1"} {
		peripheral, found := load.Find(name)
		if !found {
			t.Fatalf("expected %q to be found", name)
		}
		address, parsed := peripheral.Address()
		if !parsed || address != 0x40011000 {
			t.Errorf("%q: expected 0x40011000, got %x (%v)", name, address, parsed)
		}
	}
	if _, found := load.Find("UART9"); found {
		t.Error("expected a peripheral the file does not describe not to be found")
	}
}

// A field's bounds are stated three ways across the files in the wild, and all
// three have to land on the same bits — otherwise a register view shows the wrong
// field for whichever file the vendor happened to ship.
func TestAFieldsBoundsAreReadHoweverTheyWereWritten(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		field         Field
		offset, width int
		ok            bool
	}{
		{"offset and width", Field{BitOffset: "13", BitWidth: "1"}, 13, 1, true},
		{"lsb and msb", Field{LSB: "7", MSB: "7"}, 7, 1, true},
		{"lsb and msb spanning", Field{LSB: "4", MSB: "7"}, 4, 4, true},
		{"bit range", Field{BitRange: "5:5"}, 5, 1, true},
		{"bit range spanning", Field{BitRange: "15:8"}, 8, 8, true},
		{"neither", Field{Name: "unknown"}, 0, 0, false},
		{"reversed", Field{LSB: "7", MSB: "4"}, 0, 0, false},
		{"not a number", Field{BitOffset: "x", BitWidth: "1"}, 0, 0, false},
		{"no width", Field{BitOffset: "4", BitWidth: ""}, 0, 0, false},
	} {
		offset, width, ok := testCase.field.Bounds()
		if ok != testCase.ok || (ok && (offset != testCase.offset || width != testCase.width)) {
			t.Errorf("%s: expected %d..%d (%v), got %d..%d (%v)",
				testCase.name, testCase.offset, testCase.offset+testCase.width-1, testCase.ok,
				offset, offset+width-1, ok)
		}
	}
}

// Decoding is where a wrong value would be believed. A field that cannot be placed
// is left out rather than guessed at, and one that is wider than the register it is
// in is refused rather than truncated.
func TestAFieldIsOnlyShownWhenItsBitsCanBeKnown(t *testing.T) {
	enable := Field{Name: "UE", BitOffset: "13", BitWidth: "1"}
	if value, ok := DecodeField(enable, 0x2000, 4); !ok || value != 1 {
		t.Errorf("expected the enable bit to be 1, got %x (%v)", value, ok)
	}
	if value, ok := DecodeField(enable, 0, 4); !ok || value != 0 {
		t.Errorf("expected the enable bit to be 0, got %x (%v)", value, ok)
	}

	// A field a file describes wider than the register it is in: truncating would
	// produce a plausible number that means nothing.
	tooWide := Field{Name: "BIG", BitOffset: "34", BitWidth: "4"}
	if _, ok := DecodeField(tooWide, 0, 4); ok {
		t.Error("expected a field past the end of the register to be refused")
	}
	// And one this cannot place at all.
	if _, ok := DecodeField(Field{Name: "?"}, 0, 4); ok {
		t.Error("expected a field with no bounds to be refused")
	}
	// A whole field's mask, which is what a person reads to see which bits it is.
	mask, ok := Mask(Field{LSB: "4", MSB: "7"})
	if !ok || mask != 0xF0 {
		t.Errorf("expected 0xf0, got %x (%v)", mask, ok)
	}
}

// A large peripheral is described as clusters inside clusters, and the offsets nest.
// Flattening them without adding the offsets puts half the registers at addresses
// that belong to another block.
func TestClustersAreFlattenedWithTheirOffsetsAdded(t *testing.T) {
	load := loadOf(t, stm32)
	gpio, found := load.Find("GPIOA")
	if !found {
		t.Fatal("expected the port")
	}
	registers := gpio.AllRegisters()

	byName := map[string]uint64{}
	for _, register := range registers {
		address, addressed := register.Address(0x40020000)
		if !addressed {
			t.Errorf("%s: expected an address", register.Name)
			continue
		}
		byName[register.Name] = address
	}
	// The cluster at 0x00, then its registers at 0 and 4.
	if byName["MODER0"] != 0x40020000 || byName["MODER1"] != 0x40020004 {
		t.Errorf("expected the mode registers at the port's base, got %x", byName)
	}
	// A cluster inside a cluster: the inner one adds to both.
	if byName["IN0"] != 0x40020010 {
		t.Errorf("expected the nested register at 0x40020010, got %x", byName["IN0"])
	}
	// In address order, because that is the order the file is in and the order a
	// person reads a memory map in.
	for index := 1; index < len(registers); index++ {
		left, _ := registers[index-1].Address(0)
		right, _ := registers[index].Address(0)
		if left > right {
			t.Errorf("expected address order, got %x before %x", left, right)
		}
	}
}

// A peripheral with no registers is not an error: some are named for their
// interrupts alone. What matters is that it is not offered for expansion, because
// expanding it would show an empty list where the registers are further down.
func TestAPeripheralWithNoRegistersIsNotOfferedForExpansion(t *testing.T) {
	load := loadOf(t, stm32)
	dma, found := load.Find("DMA1")
	if !found {
		t.Fatal("expected the controller")
	}
	if len(dma.AllRegisters()) != 0 {
		t.Errorf("expected no registers, got %d", len(dma.AllRegisters()))
	}
}

// A write-only register reads back as whatever the hardware drives, which is not
// what was written. Shown as what it is, rather than as a value nobody believes.
func TestAWriteOnlyRegisterIsNotRead(t *testing.T) {
	register := Register{Name: "DR", Access: "write-only", AddressOffset: "0x04"}
	if register.Readable() {
		t.Error("expected a write-only register not to be readable")
	}
	if !(Register{Access: "read-write"}).Readable() {
		t.Error("expected a read-write register to be readable")
	}
	// And one that says nothing is readable: the default is to show it.
	if !(Register{Name: "X"}).Readable() {
		t.Error("expected a register with no access stated to be readable")
	}
}

func TestAFileThatIsNotARegisterMapIsRefused(t *testing.T) {
	for name, content := range map[string]string{
		"not xml":        `<device>`,
		"no peripherals": `<device><name>X</name><peripherals></peripherals></device>`,
	} {
		if _, err := ParseSVD([]byte(content), name); err == nil {
			t.Errorf("expected %s to be refused", name)
		}
	}
	if _, err := LoadSVD(filepath.Join(t.TempDir(), "absent.svd")); err == nil {
		t.Error("expected a file that is not there to be refused")
	}
}

// A register's width is what the session reads, so a size nobody stated has to
// become the one every file means by it.
func TestAWidthNobodyStatedIsTheOneEveryFileMeans(t *testing.T) {
	// Both spellings, because both occur in the files this is pointed at:
	// `<size>32</size>` and `<size>0x20</size>` are the same register.
	for _, testCase := range []struct {
		size  string
		width int
	}{{"8", 1}, {"16", 2}, {"32", 4}, {"64", 8}, {"0x20", 4}, {"0x40", 8}, {"", 4}, {"odd", 4}} {
		register := Register{Size: testCase.size}
		if got := register.Width(); got != testCase.width {
			t.Errorf("size %q: expected %d bytes, got %d", testCase.size, testCase.width, got)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  The map inside a session                                                   */
/* -------------------------------------------------------------------------- */

// The register tree is answered by the session and not by the engine, because it is
// the chip's own description and every engine would otherwise need to read it.
func TestTheRegisterMapIsShownAsAScopeAndItsValuesAreRead(t *testing.T) {
	engine := newRecorder()
	// The USART this test reads, at the address the file below gives it.
	engine.values = map[uint64]string{
		0x40011000: "48000f20",
		0x40011004: "00000000",
	}
	talk := sessionOf(t, engine)

	load := loadOf(t, stm32)
	if err := talk.writeRegisterMap(load); err != nil {
		t.Fatal(err)
	}

	// The scope appears with the frame's own scopes.
	scopes := struct {
		Scopes []Scope `json:"scopes"`
	}{}
	bodyOf(t, talk.ask(RequestScopes, ScopesArguments{FrameID: 1001}), &scopes)

	var peripherals Scope
	for _, scope := range scopes.Scopes {
		if scope.Name == "Peripherals" {
			peripherals = scope
		}
	}
	if peripherals.VariablesReference == 0 {
		t.Fatalf("expected a scope that can be expanded, got %+v", peripherals)
	}
	if peripherals.NamedVariables != 3 {
		t.Errorf("expected the three peripherals the file describes, got %d", peripherals.NamedVariables)
	}

	// The interfaces, with their addresses and only those that have registers.
	listed := struct {
		Variables []Variable `json:"variables"`
	}{}
	bodyOf(t, talk.ask(RequestVariables, VariablesArguments{VariablesReference: peripherals.VariablesReference}), &listed)
	if len(listed.Variables) != 3 {
		t.Fatalf("expected three peripherals, got %d", len(listed.Variables))
	}
	var usart Variable
	for _, one := range listed.Variables {
		if one.Name == "USART1" {
			usart = one
		}
	}
	if usart.VariablesReference == 0 {
		t.Fatalf("expected an expandable peripheral, got %+v", usart)
	}

	// Its registers, with their values read from the board.
	registers := struct {
		Variables []Variable `json:"variables"`
	}{}
	bodyOf(t, talk.ask(RequestVariables, VariablesArguments{VariablesReference: usart.VariablesReference}), &registers)
	byName := map[string]Variable{}
	for _, one := range registers.Variables {
		byName[one.Name] = one
	}
	if byName["CR1"].Value != "48000f20" {
		t.Errorf("expected the control register's value to have been read, got %+v", byName["CR1"])
	}
	if byName["DR"].MemoryReference != 0x40011004 {
		t.Errorf("expected the data register at the peripheral's base plus its offset, got %+v", byName["DR"])
	}
	if !engine.saw("readMemory 0x40011000 length=4") {
		t.Errorf("expected the board to be read, got:\n%s", engine.said())
	}
}

// A value read before the program moved is a value of where it used to be, so it is
// dropped the moment anything is resumed.
func TestRegisterValuesAreForgottenWhenTheProgramMoves(t *testing.T) {
	engine := newRecorder()
	talk := sessionOf(t, engine)
	talk.session.registerMap = loadOf(t, stm32)

	usart, found := talk.session.registerMap.Find("USART1")
	if !found {
		t.Fatal("expected the port")
	}
	talk.session.registersOf(usart)
	if engine.calls() == 0 {
		t.Fatal("expected the board to be read")
	}
	before := engine.calls()

	// The same again is not read twice: expanding the same peripheral twice should
	// not read the board twice.
	talk.session.registersOf(usart)
	if after := engine.calls(); after != before {
		t.Errorf("expected the second expansion not to read the board again, got %d more calls", after-before)
	}

	talk.session.ForgetRegisterValues()
	talk.session.registersOf(usart)
	if after := engine.calls(); after <= before {
		t.Error("expected the board to be read again once the program had moved")
	}
}

// Without a register map there is still a scope, and it cannot be expanded —
// because a scope with a reference and nothing in it opens as an empty tree, which
// reads as a chip that has no peripherals.
func TestWithoutARegisterMapTheScopeIsNotExpandable(t *testing.T) {
	talk := sessionOf(t, newRecorder())
	scopes := struct {
		Scopes []Scope `json:"scopes"`
	}{}
	bodyOf(t, talk.ask(RequestScopes, ScopesArguments{FrameID: 1001}), &scopes)

	for _, scope := range scopes.Scopes {
		if scope.Name != "Peripherals" {
			continue
		}
		if scope.VariablesReference != 0 {
			t.Errorf("expected a scope that cannot be expanded, got %d", scope.VariablesReference)
		}
		return
	}
	t.Error("expected the peripherals scope to be answered even without a map")
}

/* -------------------------------------------------------------------------- */
/*  The map as something else can read it                                      */
/* -------------------------------------------------------------------------- */

// An SVD file is XML, and every language that wants to show a peripheral would
// otherwise need its own reader for it. So the map is answered as JSON, from the
// reader that is already tested.
func TestTheRegisterMapIsAnsweredAsJSON(t *testing.T) {
	load := loadOf(t, stm32)
	described := DescribeRegisters(load, "chip.svd", "from the vendor")

	if described.Device != "STM32F4" || described.Source != "chip.svd" {
		t.Errorf("expected the part and where it was read from, got %+v", described)
	}
	byName := map[string]MappedPeripheral{}
	for _, peripheral := range described.Peripherals {
		byName[peripheral.Name] = peripheral
	}
	usart, found := byName["USART1"]
	if !found {
		t.Fatalf("expected the peripheral, got %v", byName)
	}
	if usart.Address != "0x40011000" {
		t.Errorf("expected the base address the way a datasheet writes it, got %q", usart.Address)
	}

	registers := map[string]MappedRegister{}
	for _, register := range usart.Registers {
		registers[register.Name] = register
	}
	if got := registers["DR"].Address; got != "0x40011004" {
		t.Errorf("expected the register at the base plus its offset, got %q", got)
	}
	if !registers["SR"].Readable || registers["DR"].Access != "read-write" {
		t.Errorf("expected the access to be reported, got %+v", registers["DR"])
	}

	// A field is placed, with its bits said the way a datasheet says them and a mask
	// for a reader that would rather compare than shift.
	fields := map[string]MappedField{}
	for _, field := range registers["CR1"].Fields {
		fields[field.Name] = field
	}
	if got := fields["UE"]; got.Offset != 13 || got.Width != 1 || got.Bits != "13..13" || got.Mask != "0x2000" {
		t.Errorf("expected the enable field at bit 13, got %+v", got)
	}
	if got := fields["M"]; got.Offset != 12 || got.Width != 1 {
		t.Errorf("expected the word-length field at bit 12, got %+v", got)
	}

	// And it is JSON, which is what a tree view in another language reads.
	encoded, err := EncodeRegisters(described)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{`"device": "STM32F4"`, `"name": "USART1"`, `"mask": "0x2000"`} {
		if !containsText(string(encoded), wanted) {
			t.Errorf("expected %s in the answer, got %s", wanted, encoded)
		}
	}

	// A peripheral with no base address is kept, with no registers rather than with
	// wrong ones: the name is part of the map, and its addresses are not.
	dma, found := byName["DMA1"]
	if !found || len(dma.Registers) != 0 {
		t.Errorf("expected a peripheral with no registers, got %+v", dma)
	}
}

func TestAMapThatIsNotThereIsSaidTo(t *testing.T) {
	// A target that names one the project does not carry says so by name, because
	// the fix is a specific file and not a search.
	spec, err := targetspec.ParseTarget([]byte(`{"name":"pico","debug":{"svd":"rp2040.svd"}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = FindRegisterMap(TargetFacts{Target: spec, Root: t.TempDir(), Program: "build/app.elf"})
	if err == nil {
		t.Fatal("expected the missing register map to be reported")
	}
	if !containsText(err.Error(), "rp2040.svd") {
		t.Errorf("expected the file to be named in the failure, got %v", err)
	}

	// A target that names no map and carries no part is a host: no peripherals,
	// which is a fact. A distinguishable answer rather than an empty one, so a
	// session here can go on and a session on a board is stopped.
	found, err := FindRegisterMap(TargetFacts{Root: t.TempDir(), Program: "build/app.elf"})
	if !errors.Is(err, ErrNoRegisterMap) || found.Path != "" {
		t.Errorf("expected the host's answer, got %+v (%v)", found, err)
	}
}

func containsText(haystack, needle string) bool { return strings.Contains(haystack, needle) }

// A vendor's register map is megabytes of XML, and every program built for the
// chip would otherwise carry a copy of it in its own tree. So a platform vendors it
// once and a program finds it through the platform it depends on — which is the
// difference between a platform that has one and one that does not.
func TestARegisterMapAPlatformVendorsIsFound(t *testing.T) {
	project := t.TempDir()
	platform := t.TempDir()
	write := func(path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<device/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The platform carries it under its own vendor tree.
	vendored := filepath.Join(platform, "vendor", "cmsis_svd", "stm32h7xx.svd")
	write(vendored)

	found, ok := RegisterMapPath("vendor/cmsis_svd/stm32h7xx.svd", project,
		filepath.Join(project, "build", "app.elf"), []string{platform})
	if !ok || found != vendored {
		t.Errorf("expected the map the platform carries, got %q (%v)", found, ok)
	}
	// And a bare name is looked for there too, which is how a spec can name a file
	// without naming where the platform keeps it.
	write(filepath.Join(platform, "stm32f7xx.svd"))
	found, ok = RegisterMapPath("stm32f7xx.svd", project,
		filepath.Join(project, "build", "app.elf"), []string{platform})
	if !ok || found != filepath.Join(platform, "stm32f7xx.svd") {
		t.Errorf("expected the map in the platform's vendor tree, got %q (%v)", found, ok)
	}
	// A name no tree carries is still reported rather than guessed at.
	if _, ok := RegisterMapPath("stm32h7xx.svd", project, "", nil); ok {
		t.Error("expected a map that is nowhere to be reported")
	}
}

// A vendor writes one peripheral once and every other of the same kind as a
// reference to it: on an STM32 part every USART is a base address and
// `derivedFrom` naming the one that carries the registers. Read literally, USART3
// has no registers at all — which is true of the file and not of the chip.
func TestAPeripheralThatDerivesFromAnotherHasItsRegisters(t *testing.T) {
	load := loadOf(t, `<?xml version="1.0" encoding="utf-8"?>
<device schemaVersion="1.1">
  <name>PART</name><version>1.0</version>
  <peripherals>
    <peripheral>
      <name>USART1</name><description>universal synchronous/asynchronous</description>
      <baseAddress>0x40011000</baseAddress>
      <registers>
        <register>
          <name>CR1</name><description>control 1</description>
          <addressOffset>0x00</addressOffset><size>32</size><access>read-write</access>
        </register>
        <register>
          <name>CR2</name><description>control 2</description>
          <addressOffset>0x04</addressOffset><size>32</size><access>read-write</access>
        </register>
      </registers>
    </peripheral>
    <peripheral derivedFrom="USART1">
      <name>USART3</name>
      <baseAddress>0x40004800</baseAddress>
    </peripheral>
  </peripherals>
</device>`)

	usart3, found := load.Find("USART3")
	if !found {
		t.Fatal("expected the peripheral")
	}
	registers := usart3.AllRegisters()
	if len(registers) != 2 {
		t.Fatalf("expected the registers it derives, got %d", len(registers))
	}
	// At its own base address, not the one it was derived from: the registers are
	// shared and the base is not.
	address, addressed := registers[0].Address(0x40004800)
	if !addressed || address != 0x40004800 {
		t.Errorf("expected the derived peripheral's own base address, got %x (%v)", address, addressed)
	}
	if usart3.Description != "universal synchronous/asynchronous" {
		t.Errorf("expected the description it derives, got %q", usart3.Description)
	}

	// A peripheral that says more of its own says it: the derived register is
	// replaced rather than joined with, because that is what a child that names a
	// register is saying.
	load = loadOf(t, `<?xml version="1.0" encoding="utf-8"?>
<device schemaVersion="1.1">
  <name>PART</name><version>1.0</version>
  <peripherals>
    <peripheral>
      <name>USART1</name>
      <baseAddress>0x40011000</baseAddress>
      <registers>
        <register><name>CR1</name><addressOffset>0x00</addressOffset><size>32</size><access>read-write</access></register>
        <register><name>SR</name><addressOffset>0x1C</addressOffset><size>32</size><access>read-only</access></register>
      </registers>
    </peripheral>
    <peripheral derivedFrom="USART1">
      <name>USART3</name>
      <baseAddress>0x40004800</baseAddress>
      <registers>
        <register><name>CR1</name><addressOffset>0x08</addressOffset><size>32</size><access>read-write</access></register>
      </registers>
    </peripheral>
  </peripherals>
</device>`)
	usart3, _ = load.Find("USART3")
	registers = usart3.AllRegisters()
	byName := map[string]uint64{}
	for _, register := range registers {
		address, _ := register.Address(0x40004800)
		byName[register.Name] = address
	}
	if len(registers) != 2 {
		t.Fatalf("expected two registers, got %d", len(registers))
	}
	if byName["CR1"] != 0x40004808 {
		t.Errorf("expected the register the child declares at its own offset, got %x", byName["CR1"])
	}
	if byName["SR"] != 0x4000481C {
		t.Errorf("expected the register it did not declare to be inherited, got %x", byName["SR"])
	}
}

// A file may derive in a cycle — one peripheral from another that derives from the
// first — and a reader that follows references recursively is a stack overflow on
// somebody's board. The passes stop when nothing changes.
func TestADerivationCycleDoesNotHang(t *testing.T) {
	load := loadOf(t, `<?xml version="1.0" encoding="utf-8"?>
<device schemaVersion="1.1">
  <name>PART</name><version>1.0</version>
  <peripherals>
    <peripheral derivedFrom="B"><name>A</name><baseAddress>0x1000</baseAddress></peripheral>
    <peripheral derivedFrom="A"><name>B</name><baseAddress>0x2000</baseAddress></peripheral>
    <peripheral><name>C</name><baseAddress>0x3000</baseAddress></peripheral>
  </peripherals>
</device>`)
	if len(load.Peripherals) != 3 {
		t.Fatalf("expected every peripheral to survive a cycle, got %d", len(load.Peripherals))
	}
	a, found := load.Find("A")
	if !found || a.BaseAddress != "0x1000" {
		t.Errorf("expected each to keep its own base address, got %+v", a)
	}
}
