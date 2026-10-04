//
// @file report.go
// @brief What a run prints.
//
// Three things, in the order somebody asks for them: how much of each region is
// used, what is in it, and whether that is allowed. The bar is drawn to the
// region's own capacity rather than to the largest figure in the report, because
// the question a bar answers is how much room is left rather than which number is
// biggest.
//
// Every figure is labelled with where it came from. A total read from a linker map
// and a total inferred from section names in an ELF are both useful and they are
// not the same claim, and a report that printed them identically would be asking
// the reader to know which one they had.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"fmt"
	"io"
	"strings"
)

// Options is what a report is asked for.
type Options struct {
	// Top is how many symbols the attribution lists. Zero means none.
	Top int
	// Width is how wide the usage bars are drawn.
	Width int
	// Title names what is being reported, which is the board when it is known.
	Title string
	// Bytes prints exact byte counts beside the readable ones.
	Bytes bool
}

// WriteReport prints an analysis and its verdicts.
func WriteReport(out io.Writer, analysis *Analysis, check *Check, options Options) {
	width := options.Width
	if width < 8 {
		width = 25
	}

	title := options.Title
	if title == "" {
		title = analysis.Target
	}
	if title != "" {
		fmt.Fprintf(out, "Memory Region Breakdown (%s)\n", title)
	} else {
		fmt.Fprintln(out, "Memory Region Breakdown")
	}
	fmt.Fprintln(out, strings.Repeat("─", 72))

	if len(analysis.Usage) == 0 {
		fmt.Fprintln(out, "  no memory region could be established from the artifacts read")
	} else {
		nameWidth := 0
		for _, verdict := range check.Verdicts {
			if len(verdict.Name) > nameWidth {
				nameWidth = len(verdict.Name)
			}
		}
		for _, verdict := range check.Verdicts {
			writeVerdict(out, verdict, nameWidth, width, options)
		}
	}

	if options.Top > 0 {
		writeSymbols(out, analysis, options.Top)
	}

	writeProvenance(out, analysis)
	for _, diagnostic := range analysis.Diagnostics {
		fmt.Fprintf(out, "\nnote: %s\n", diagnostic)
	}
	if check != nil && len(check.Failed) > 0 {
		fmt.Fprintln(out)
		for _, failure := range check.Failed {
			fmt.Fprintf(out, "FAIL: %s\n", failure)
		}
	}
}

// writeVerdict is one region's line: a bar, two figures, a state and a reason.
//
// The capacity shown is the project's budget where it declared one and the
// linker script's own figure where it did not — labelled, because the two are
// different claims. A budget is somebody's decision; a region length is what the
// board has. Reporting them in the same column without saying which would make an
// unbudgeted firmware look like it had been reviewed.
func writeVerdict(out io.Writer, verdict Verdict, nameWidth, width int, options Options) {
	capacity := verdict.Usage.Length
	used := verdict.Usage.Used

	marker := map[string]string{
		StateOK:         "OK",
		StateWarn:       "WARN",
		StateOver:       "OVER",
		StateUnbudgeted: "----",
	}[verdict.State]

	if verdict.Usage.Length > 0 && verdict.State == StateUnbudgeted {
		fmt.Fprintf(out, "%-*s %s %10s / %-10s %5s  [%s]  no budget declared\n",
			nameWidth, verdict.Name, bar(used, capacity, width),
			humanBytes(used), humanBytes(capacity), humanPercent(float64(used)/float64(capacity)*100), marker)
		if options.Bytes {
			fmt.Fprintf(out, "%-*s   %d of %d bytes, the linker script's own region size\n",
				nameWidth, "", used, capacity)
		}
		return
	}

	if verdict.Percent < 0 {
		fmt.Fprintf(out, "%-*s %s %10s  %s  [%s]\n",
			nameWidth, verdict.Name, bar(used, capacity, width),
			humanBytes(used), "capacity unknown", marker)
		return
	}

	fmt.Fprintf(out, "%-*s %s %10s / %-10s %5s  [%s]\n",
		nameWidth, verdict.Name, bar(used, capacity, width),
		humanBytes(used), humanBytes(capacity), humanPercent(verdict.Percent), marker)

	if verdict.Note != "" {
		fmt.Fprintf(out, "%-*s   %s\n", nameWidth, "", verdict.Note)
	}
	if options.Bytes {
		fmt.Fprintf(out, "%-*s   %d of %d bytes\n", nameWidth, "", used, capacity)
	}
}

// writeSymbols is the footprint attribution: the largest things in the image, and
// which object each came from.
//
// Per region rather than one list, because a symbol's region is the fact that
// makes it useful — a 12 kB function in flash and a 12 kB buffer in RAM are
// different problems, and one merged list would hide which is which behind a
// single "top 15".
func writeSymbols(out io.Writer, analysis *Analysis, top int) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Top Symbol Footprint Attribution:")

	shown := 0
	for _, usage := range analysis.Usage {
		if len(usage.Symbols) == 0 {
			continue
		}
		limit := top
		if limit > len(usage.Symbols) {
			limit = len(usage.Symbols)
		}
		// One list per region, capped: a region with four hundred symbols and a
		// request for fifteen gets fifteen, not four hundred, and the other
		// regions get theirs too rather than being crowded out.
		if shown == 0 {
			fmt.Fprintf(out, "  %s:\n", usage.Name)
		} else {
			fmt.Fprintf(out, "  %s:\n", usage.Name)
		}
		for index := 0; index < limit; index++ {
			symbol := usage.Symbols[index]
			object := shortenObject(symbol.Object)
			if object == "" {
				object = "the linker script"
			}
			fmt.Fprintf(out, "  %3d. %-38s %9s  (%s)\n",
				index+1, symbol.Name, humanBytes(symbol.Size), object)
		}
		if limit < len(usage.Symbols) {
			fmt.Fprintf(out, "       ... and %d more in %s\n", len(usage.Symbols)-limit, usage.Name)
		}
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(out, "  (no symbols with a size were found in the artifacts read)")
	}
}

// writeProvenance says what was read, which is what makes a number checkable.
func writeProvenance(out io.Writer, analysis *Analysis) {
	fmt.Fprintln(out)
	switch analysis.Source {
	case "map+elf":
		fmt.Fprintf(out, "read: linker map %s and program %s\n",
			relativeOrAbsolute(analysis.Artifacts.MapFile), relativeOrAbsolute(analysis.Artifacts.ELFFile))
	case "map":
		fmt.Fprintf(out, "read: linker map %s\n", relativeOrAbsolute(analysis.Artifacts.MapFile))
	case "elf":
		fmt.Fprintf(out, "read: program %s (no linker map; regions inferred from section names)\n",
			relativeOrAbsolute(analysis.Artifacts.ELFFile))
	default:
		fmt.Fprintln(out, "read: nothing")
	}
}

func relativeOrAbsolute(path string) string { return path }

// WriteGrowth prints the change against a baseline.
//
// Largest growth first, and a shrink is printed as a shrink: a pull request that
// gives back 8 kB is worth knowing about for the same reason one that takes 8 kB
// is, and a report that only mentioned the direction that costs money would be a
// report of what went wrong rather than of what changed.
func WriteGrowth(out io.Writer, check *Check) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Change against the baseline:")
	for _, growth := range check.Growth {
		marker := ""
		if growth.OverLimit {
			marker = "  OVER LIMIT"
		}
		sign := "+"
		if growth.Delta < 0 {
			sign = ""
		}
		percent := ""
		if growth.Percent >= 0 {
			percent = fmt.Sprintf(" (%+.1f%%)", growth.Percent)
		}
		fmt.Fprintf(out, "  %-10s %s%s%s%s\n",
			growth.Name, sign, humanBytes(uint64(abs64(growth.Delta))), percent, marker)
	}
	if len(check.Growth) > 0 && check.HasBaseline {
		fmt.Fprintln(out, "  (a negative change is a smaller image than the baseline)")
	}
}
