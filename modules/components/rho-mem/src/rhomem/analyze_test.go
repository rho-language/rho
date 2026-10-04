//
// @file analyze_test.go
// @brief Attributing a byte to the region it costs.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"bytes"
	"strings"
	"testing"
)

// analyseMap is the analysis of a map written in one line of test.
func analyseMap(t *testing.T, body string) *Analysis {
	t.Helper()
	path := writeMap(t, body)
	analysis, err := Analyze(path, "", "test-board")
	if err != nil {
		t.Fatal(err)
	}
	return analysis
}

// The regions are the ones the linker script declared, in the order it declared
// them: a report that reordered itself between two runs of the same binary is one
// nobody can read twice.
func TestRegionsAreReportedInDeclarationOrder(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
SRAM1            0x0000000030000000 0x0000000000048000 rw
FLASH            0x0000000008000000 0x0000000000200000 xr
DTCM             0x0000000020000000 0x0000000000020000 rw

Linker script and memory map

.text           0x0000000008000000      0x100
`)
	names := []string{}
	for _, usage := range analysis.Usage {
		names = append(names, usage.Name)
	}
	want := []string{"SRAM1", "FLASH", "DTCM"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("expected %v, got %v", want, names)
	}
}

// A `.data` section occupies two regions at once: its initializer image is in
// flash and its variables are in RAM, and both are paid for. Counting it once
// counted the wrong region once, which is the difference between a number and a
// number that happens to be right.
func TestDataCostsBothItsLoadAndItsRunRegion(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000200000 xr
RAM              0x0000000020000000 0x0000000000020000 rw

Linker script and memory map

.text           0x0000000008000000      0x400
.data           0x0000000020000000       0x88 load address 0x0000000008000400
.bss            0x0000000020000088      0x100 load address 0x0000000020000400
`)
	flash, _ := analysis.Region("FLASH")
	ram, _ := analysis.Region("RAM")

	// Flash holds the code and the 0x88 bytes of initializer image; the 0x100
	// bytes of `.bss` store nothing, so its load address costs flash nothing.
	if want := uint64(0x400 + 0x88); flash.Used != want {
		t.Errorf("FLASH: expected %d bytes, got %d", want, flash.Used)
	}
	// RAM holds the 0x88 of variables and the 0x100 of zero-initialised storage.
	if want := uint64(0x88 + 0x100); ram.Used != want {
		t.Errorf("RAM: expected %d bytes, got %d", want, ram.Used)
	}
}

// The gap between two sections is in the image. A link pads to align, those bytes
// are programmed, and a report that summed section sizes would leave them out —
// which is how a firmware that fills its flash exactly passes every size check
// and then fails to be written.
func TestPaddingBetweenSectionsIsCounted(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000200000 xr

Linker script and memory map

.isr_vector     0x0000000008000000      0x298
.text           0x00000000080002c0     0x74a0
`)
	flash, _ := analysis.Region("FLASH")
	// 0x298 of vectors, then 0x28 of padding, then the code.
	want := uint64(0x2c0 + 0x74a0)
	if flash.Used != want {
		t.Fatalf("expected the span %d bytes, got %d", want, flash.Used)
	}
	if flash.Used <= 0x298+0x74a0 {
		t.Error("the padding between the sections was not counted")
	}
}

// Two regions that cover the same addresses make "the region containing this" a
// question with more than one answer. The more specific one wins, and the overlap
// is said rather than left for a reader to work out from the numbers.
func TestOverlappingRegionsGoToTheMoreSpecificOne(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
RAM              0x0000000020000000 0x0000000000020000 rw
DTCM             0x0000000020000000 0x0000000000002000 rw

Linker script and memory map

.bss            0x0000000020000000      0x100
`)
	// DTCM is 8 kB inside RAM's 128 kB, so it is the more specific answer.
	dtcm, _ := analysis.Region("DTCM")
	if dtcm.Used != 0x100 {
		t.Errorf("expected the usage charged to DTCM, got %d", dtcm.Used)
	}
	found := false
	for _, diagnostic := range analysis.Diagnostics {
		if strings.Contains(diagnostic, "cover the same addresses") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the overlap to be reported, got %v", analysis.Diagnostics)
	}
}

// An address in no region is still an address somebody spent, and dropping it is
// how an image outgrows a budget without any budget noticing.
func TestAnAddressInNoRegionIsReported(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000200000 xr

Linker script and memory map

.text           0x0000000090000000      0x100
`)
	if _, ok := analysis.Region("UNMAPPED"); !ok {
		t.Fatalf("expected an unmapped region, got %+v", analysis.Usage)
	}
}

// A symbol is placed in the region its address is in, which is what makes a list
// of symbols a list of things in a region rather than a list of numbers.
func TestSymbolsArePlacedInRegions(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000200000 xr
RAM              0x0000000020000000 0x0000000000020000 rw

Linker script and memory map

.text           0x0000000008000000      0x400
 .text.big
                0x0000000008000000      0x200 main-dep0/libfoo.a(foo.c.obj)
                0x0000000008000000              big

.bss            0x0000000020000000      0x200
 .bss.buffer
                0x0000000020000000      0x100 CMakeFiles/main.dir/app.c.obj
                0x0000000020000000              buffer
`)
	flash, _ := analysis.Region("FLASH")
	ram, _ := analysis.Region("RAM")
	if len(flash.Symbols) != 1 || flash.Symbols[0].Name != "big" {
		t.Fatalf("expected `big` in flash, got %+v", flash.Symbols)
	}
	if len(ram.Symbols) != 1 || ram.Symbols[0].Name != "buffer" {
		t.Fatalf("expected `buffer` in RAM, got %+v", ram.Symbols)
	}
	// Largest first, which is the order the report prints them in.
	if flash.Symbols[0].Size != 0x200 {
		t.Errorf("unexpected size: %d", flash.Symbols[0].Size)
	}
}

// The same symbols from the map and from the program are one set of symbols.
// Counted twice they would double the image and change nothing about it.
func TestTheProgramAndTheMapDoNotDoubleCount(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000200000 xr

Linker script and memory map

.text           0x0000000008000000      0x100
 .text.big
                0x0000000008000000       0x40 main-dep0/libfoo.a(foo.c.obj)
                0x0000000008000000               big
`)
	flash, _ := analysis.Region("FLASH")
	if len(flash.Symbols) != 1 {
		t.Fatalf("expected one symbol, got %d: %+v", len(flash.Symbols), flash.Symbols)
	}
}

/* -------------------------------------------------------------------------- */
/*  Budgets                                                                   */
/* -------------------------------------------------------------------------- */

// A region inside its budget is ok, and the report says how much is left — which
// is the number somebody wants when they are deciding whether to add something.
func TestARegionInsideItsBudgetIsOk(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x100
`)
	verdict := Evaluate(analysis, map[string]Budget{
		"FLASH": {Name: "FLASH", MaxBytes: 0x10000, WarnAt: 85, WarnAtDeclared: true},
	}, Enforcement{}, nil)
	if len(verdict.Verdicts) != 1 || verdict.Verdicts[0].State != StateOK {
		t.Fatalf("expected ok, got %+v", verdict.Verdicts)
	}
	if verdict.OverBudget {
		t.Error("a region inside its budget was reported as over")
	}
}

// Being inside the budget and being fine are different facts. A region at 95% is
// a region whose next feature does not fit, and nothing shows that until a build
// fails.
func TestARegionNearItsLimitWarns(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x3f0
`)
	verdict := Evaluate(analysis, map[string]Budget{
		"FLASH": {Name: "FLASH", MaxBytes: 0x400, WarnAt: 85, WarnAtDeclared: true},
	}, Enforcement{}, nil)
	if verdict.Verdicts[0].State != StateWarn {
		t.Fatalf("expected a warning below the threshold's region, got %+v", verdict.Verdicts[0])
	}
	if verdict.OverBudget {
		t.Error("a warning must not also be an overflow")
	}
}

// A region over its budget is the failure this component exists to catch, and
// only when the project asked for it.
func TestARegionOverItsBudgetIsReported(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x500
`)
	budgets := map[string]Budget{"FLASH": {Name: "FLASH", MaxBytes: 0x400}}

	// Reported without enforcement: the report is not a check.
	reported := Evaluate(analysis, budgets, Enforcement{}, nil)
	if !reported.OverBudget {
		t.Error("expected the overflow to be reported")
	}
	if len(reported.Failed) != 1 || !strings.Contains(reported.Failed[0], "over its budget") {
		t.Errorf("expected the overflow named, got %v", reported.Failed)
	}
	if reported.Verdicts[0].State != StateOver {
		t.Errorf("expected the state to be over, got %+v", reported.Verdicts[0].State)
	}
}

// A project with no budget for a region gets it reported and not failed: it
// decided the region is not a thing to worry about.
func TestAnUnbudgetedRegionIsReportedAndNotFailed(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x500
`)
	verdict := Evaluate(analysis, nil, Enforcement{FailOnOverflow: true}, nil)
	if verdict.OverBudget {
		t.Error("an unbudgeted region must not fail a check")
	}
	if verdict.Verdicts[0].State != StateUnbudgeted {
		t.Errorf("expected the state to say so, got %+v", verdict.Verdicts[0].State)
	}
}

// A budget written for a region the build placed nothing in is either a typo in
// the name or a feature that was switched off, and both are silent otherwise.
func TestABudgetForAnUnusedRegionIsSaid(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x100
`)
	verdict := Evaluate(analysis, map[string]Budget{
		"DTCM": {Name: "DTCM", MaxBytes: 0x20000},
	}, Enforcement{}, nil)
	found := false
	for _, one := range verdict.Verdicts {
		if one.Name == "DTCM" && strings.Contains(one.Note, "placed nothing") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the unused budget reported, got %+v", verdict.Verdicts)
	}
}

// A budget is written against the region's name, and the two sides of that
// comparison are written by different hands.
func TestABudgetMatchesItsRegionWithoutRegardToCase(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
SRAM1            0x0000000030000000 0x0000000000010000 rw

Linker script and memory map

.bss            0x0000000030000000      0x100
`)
	verdict := Evaluate(analysis, map[string]Budget{
		"sram1": {Name: "sram1", MaxBytes: 0x1000, WarnAt: 85, WarnAtDeclared: true},
	}, Enforcement{}, nil)
	if verdict.Verdicts[0].State == StateUnbudgeted {
		t.Error("a budget differing only in case was not applied")
	}
}

/* -------------------------------------------------------------------------- */
/*  Growth                                                                    */
/* -------------------------------------------------------------------------- */

// A growth limit is the only thing that catches a firmware that grows a little
// every release and passes every check on the way to being too big.
func TestGrowthPastTheLimitIsReported(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x600
`)
	baseline := &Baseline{Version: BaselineVersion, Regions: []BaselineRegion{
		{Name: "FLASH", Used: 0x400},
	}}
	verdict := Evaluate(analysis, map[string]Budget{
		"FLASH": {Name: "FLASH", MaxBytes: 0x1000},
	}, Enforcement{FailOnGrowthBytes: 0x100, FailOnGrowthDeclared: true}, baseline)

	if !verdict.OverGrowth {
		t.Fatal("expected the growth to be over the limit")
	}
	if verdict.Growth[0].Delta != 0x200 {
		t.Errorf("expected a delta of 0x200, got %d", verdict.Growth[0].Delta)
	}
}

// A pull request that gives memory back is worth knowing about for the same
// reason one that takes it is.
func TestAShrinkIsReportedAsItIs(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x300
`)
	baseline := &Baseline{Version: BaselineVersion, Regions: []BaselineRegion{
		{Name: "FLASH", Used: 0x800},
	}}
	verdict := Evaluate(analysis, nil, Enforcement{}, baseline)
	if verdict.Growth[0].Delta != -0x500 {
		t.Errorf("expected a delta of -0x500, got %d", verdict.Growth[0].Delta)
	}
	if verdict.OverGrowth {
		t.Error("a smaller image must not fail a growth check")
	}
}

/* -------------------------------------------------------------------------- */
/*  Baselines                                                                 */
/* -------------------------------------------------------------------------- */

// A baseline is a file in the repository, which is what makes a change to it
// reviewable: a number that only ever appears in a log is a number nobody notices
// changing.
func TestABaselineRoundTrips(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr
RAM              0x0000000020000000 0x0000000000010000 rw

Linker script and memory map

.text           0x0000000008000000      0x400
 .text.big
                0x0000000008000000      0x200 main-dep0/libfoo.a(foo.c.obj)
                0x0000000008000000              big

.bss            0x0000000020000000      0x100
`)
	path := t.TempDir() + "/mem.baseline.json"
	if err := WriteBaseline(path, Record(analysis, 5)); err != nil {
		t.Fatal(err)
	}
	read, err := ReadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	flash, ok := read.Region("FLASH")
	if !ok || flash.Used != 0x400 {
		t.Errorf("expected FLASH at 0x400, got %+v (present=%v)", flash, ok)
	}
	if len(read.Symbols) != 1 || read.Symbols[0].Name != "big" {
		t.Errorf("expected the largest symbol recorded, got %+v", read.Symbols)
	}
	if read.Target != "test-board" {
		t.Errorf("expected the target recorded, got %q", read.Target)
	}
}

// No baseline yet is the ordinary state of a project on its first day, and is
// said in those words rather than being an error that stops the run.
func TestAMissingBaselineSaysHowToMakeOne(t *testing.T) {
	_, err := ReadBaseline(t.TempDir() + "/nothing.json")
	if err == nil {
		t.Fatal("expected an error naming the missing baseline")
	}
	if !strings.Contains(err.Error(), "--write-baseline") {
		t.Errorf("expected the error to say how to record one, got %v", err)
	}
}

// A file written by another version is refused rather than half-read.
func TestABaselineOfAnotherVersionIsRefused(t *testing.T) {
	path := t.TempDir() + "/mem.baseline.json"
	if err := WriteBaseline(path, &Baseline{Version: BaselineVersion}); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir() + "/mem.baseline.json"
	if err := WriteBaseline(other, &Baseline{Version: BaselineVersion + 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBaseline(other); err == nil {
		t.Error("expected a baseline of another version to be refused")
	}
}

/* -------------------------------------------------------------------------- */
/*  The report                                                                */
/* -------------------------------------------------------------------------- */

// The report is a person reading a terminal, so what it has to do is be read: a
// bar per region, the largest things in the image, and what it read to say so.
func TestTheReportShowsBarsSymbolsAndItsSources(t *testing.T) {
	analysis := analyseMap(t, `Memory Configuration

Name             Origin             Length             Attributes
FLASH            0x0000000008000000 0x0000000000010000 xr

Linker script and memory map

.text           0x0000000008000000      0x400
 .text.big
                0x0000000008000000      0x200 main-dep0/libfoo.a(foo.c.obj)
                0x0000000008000000              big
`)
	verdict := Evaluate(analysis, map[string]Budget{
		"FLASH": {Name: "FLASH", MaxBytes: 0x1000, WarnAt: 85, WarnAtDeclared: true},
	}, Enforcement{}, nil)

	out := &bytes.Buffer{}
	WriteReport(out, analysis, &verdict, Options{Top: 5, Width: 20, Title: "test-board"})
	report := out.String()

	for _, want := range []string{"test-board", "FLASH", "[", "big", "foo.c.obj", "read:"} {
		if !strings.Contains(report, want) {
			t.Errorf("expected the report to mention %q:\n%s", want, report)
		}
	}
}

// A bar is drawn to the region's own capacity, because the question a bar answers
// is how much room is left rather than which number is biggest.
func TestABarIsDrawnToTheRegionsOwnCapacity(t *testing.T) {
	if got := bar(500, 1000, 10); got != "[#####.....]" {
		t.Errorf("expected half full, got %q", got)
	}
	if got := bar(0, 1000, 10); got != "[..........]" {
		t.Errorf("expected empty, got %q", got)
	}
	// An image over its budget draws full rather than running past its own width.
	if got := bar(2000, 1000, 10); got != "[##########]" {
		t.Errorf("expected full, got %q", got)
	}
	// Something in a region with no capacity drawn is still visible.
	if got := bar(1, 0, 10); got != "[..........]" {
		t.Errorf("expected a bar for a capacity-less region, got %q", got)
	}
}

// A firmware's flash is 2 MB, which is 2097152, and a budget written against
// 2097152 is a budget nobody can hold in their head while reading a build log.
func TestSizesAreWrittenInTheUnitAPersonWouldSay(t *testing.T) {
	cases := map[uint64]string{
		512:     "512 B",
		1024:    "1.0 KB",
		2097152: "2.0 MB",
		30704:   "30.0 KB",
	}
	for size, want := range cases {
		if got := humanBytes(size); got != want {
			t.Errorf("%d: expected %q, got %q", size, want, got)
		}
	}
}

// A vendor's naming is not derivable from a part number without guessing, so the
// linker script's own region names are what a budget is written against.
func TestAnObjectPathIsShortenedToWhatIdentifiesIt(t *testing.T) {
	long := "CMakeFiles/main.dir/home/user/project/vendor/stm32h7xx_hal_driver/Src/stm32h7xx_hal_uart.c.obj"
	if got := shortenObject(long); got != "Src/stm32h7xx_hal_uart.c.obj" {
		t.Errorf("unexpected shortening: %q", got)
	}
	// A name with no directory in it is already as short as it can be.
	if got := shortenObject("main-dep0/libfoo.a(app.c.obj)"); got != "main-dep0/libfoo.a(app.c.obj)" {
		t.Errorf("unexpected shortening: %q", got)
	}
}
