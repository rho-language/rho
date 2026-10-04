//
// @file report.go
// @brief What a run prints.
//
// Three things, in the order somebody asks for them: what was measured, what it
// weighs, and how that compares with the last time. The table is the middle one and
// it is a table because a benchmark is read by comparing numbers across rows, which
// is what a list of lines makes harder than it needs to be.
//
// The direction of "better" is fixed across every column: a lower cycle count, a
// lower time, a lower stack figure and a lower heap figure are all improvements.
// That is not an accident of presentation — it is what lets one threshold
// (`fail_on_regression_percent`) mean the same thing for all of them.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhobench

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ANSI colours, as the eight the terminal has had since forever.
const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[90m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiBold   = "\x1b[1m"
)

// Options is what a report is asked for.
type Options struct {
	// Target is what was measured, for the heading.
	Target string
	// Width is how wide the name column is. Zero means as wide as the names.
	Width int
	// Color writes escape sequences. Off when the output is not a terminal, because
	// a log file full of them is a file `grep` cannot read.
	Color bool
	// Baseline is what to compare against, when there is one.
	Baseline *Baseline
}

// Summary is everything a run produced, which is what a report is written from.
type Summary struct {
	// Results are the benchmarks, in the order they ran.
	Results []Result
	// Changes are what moved against the baseline, if there was one.
	Changes []Change
	// Regressions are the changes that broke the limit.
	Regressions []Change
	// BudgetFailures are the measurements that broke a declared budget.
	BudgetFailures []string
	// Footprint is the static memory figure, when `rho-mem` could supply one.
	Footprint *Footprint
	// Baseline is what the changes were computed against, kept for the report's
	// own heading — a change without the baseline it came from is a number.
	Baseline *Baseline
	// Diagnostics is what could not be established.
	Diagnostics []string
	// Target is what was measured.
	Target string
	// Duration is how long the whole run took.
	Duration int64
}

// WriteReport prints a run.
func WriteReport(out io.Writer, run Summary, options Options) {
	width := options.Width
	if width < 20 {
		for _, one := range run.Results {
			if len(one.Benchmark.Name) > width {
				width = len(one.Benchmark.Name)
			}
		}
	}
	if width < 20 {
		width = 20
	}
	// Padded for the columns that follow, so the table lines up.
	width += 2

	title := options.Target
	if title == "" {
		title = run.Target
	}
	if title == "" {
		title = "host"
	}
	fmt.Fprintf(out, "%sBenchmark Results: %s\n", paint(options.Color, ansiBold), title)
	if run.Duration > 0 {
		fmt.Fprintf(out, "%swhole run: %.1fs%s\n",
			paint(options.Color, ansiDim), float64(run.Duration)/1e9, reset(options.Color))
	}
	fmt.Fprintln(out, strings.Repeat("─", 92))

	if len(run.Results) == 0 {
		fmt.Fprintln(out, "  no benchmark reported any measurements")
		return
	}

	header := fmt.Sprintf("%-*s %10s %8s %8s %14s %12s %10s", width,
		"Benchmark", "Iterations", "Cycles", "StdDev", "Time", "Stack", "Heap")
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, strings.Repeat("─", 92))

	for _, one := range run.Results {
		writeRow(out, one, width, options)
	}

	if run.Footprint != nil {
		writeFootprint(out, run.Footprint, options)
	}
	if len(run.Changes) > 0 {
		writeChanges(out, run.Changes, run.Baseline, options)
	}
	for _, failure := range run.BudgetFailures {
		fmt.Fprintf(out, "%sFAIL%s %s\n", paint(options.Color, ansiRed), reset(options.Color), failure)
	}
	for _, diagnostic := range run.Diagnostics {
		fmt.Fprintf(out, "\n%snote:%s %s\n", paint(options.Color, ansiYellow), reset(options.Color), diagnostic)
	}
}

// writeRow is one benchmark's line.
func writeRow(out io.Writer, one Result, width int, options Options) {
	bench := one.Benchmark
	fmt.Fprintf(out, "%-*s %10s %8s %8s %14s %12s %10s\n", width,
		truncate(bench.Name, width),
		count(uint64(bench.Iterations)),
		count(one.Cycles.Median),
		deviation(one.Cycles),
		duration(one),
		stack(one.StackBytes),
		bytes(one.HeapPeak))
}

// count is a number in thousands, which is how iteration counts are read.
func count(value uint64) string {
	if value == 0 {
		return "-"
	}
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}
	if value < 1000000 {
		return fmt.Sprintf("%.1fk", float64(value)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(value)/1000000)
}

// deviation is a standard deviation beside its median, or as a fraction.
//
// A median with no spread beside it is a number with nothing to say how repeatable
// it is, which is the difference between a benchmark and a single measurement.
func deviation(stats Stats) string {
	if stats.Iterations < 2 || stats.Median == 0 {
		return "-"
	}
	// As a multiple of the median rather than a percentage, because a standard
	// deviation is not robust: one iteration the scheduler stole turns it into
	// thousands of percent, which reads as a broken number rather than as what it
	// is — a run with an outlier. The p99 and the max beside it say that properly.
	spread := stats.StdDev / float64(stats.Median)
	switch {
	case spread >= 100:
		return ">100x"
	case spread >= 10:
		return fmt.Sprintf("%.0fx", spread)
	default:
		return fmt.Sprintf("%.1fx", spread)
	}
}

// duration is a benchmark's median iteration, in the unit a person would say it in.
func duration(one Result) string {
	if one.Stats.Median == 0 {
		return "-"
	}
	nanoseconds := float64(one.Stats.Median)
	switch {
	case nanoseconds < 1000:
		return fmt.Sprintf("%d ns", uint64(nanoseconds))
	case nanoseconds < 1000*1000:
		return fmt.Sprintf("%.2f us", nanoseconds/1000)
	case nanoseconds < 1000*1000*1000:
		return fmt.Sprintf("%.2f ms", nanoseconds/1e6)
	default:
		return fmt.Sprintf("%.2f s", nanoseconds/1e9)
	}
}

func stack(value int64) string {
	if value < 0 {
		return "-"
	}
	return bytes(uint64(value))
}

// bytes is a size in the unit a person would say it in.
func bytes(size uint64) string {
	switch {
	case size == 0:
		return "-"
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
}

// writeChanges is the comparison against a baseline.
func writeChanges(out io.Writer, changes []Change, baseline *Baseline, options Options) {
	label := "the previous run"
	if baseline != nil && baseline.Recorded != "" {
		label = baseline.Recorded
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%sChange against the baseline (%s):%s\n", paint(options.Color, ansiBold), label, reset(options.Color))
	fmt.Fprintln(out, strings.Repeat("─", 92))

	// The widest name in the list, so that every metric lands in one column.
	nameWidth := 0
	for _, change := range changes {
		if len(change.Name) > nameWidth {
			nameWidth = len(change.Name)
		}
	}
	if nameWidth > 34 {
		nameWidth = 34
	}
	for _, change := range changes {
		marker := " "
		colour := ""
		switch change.Verdict {
		case VerdictBetter:
			marker, colour = "+", ansiGreen
		case VerdictWorse:
			marker, colour = "!", ansiRed
		case VerdictNew:
			marker, colour = "?", ansiYellow
		case VerdictUnmeasurable:
			// Not a colour: there is nothing to feel good or bad about, and
			// painting it would imply one.
			marker, colour = "~", ""
		}
		before, after := bytes(change.Before), bytes(change.After)
		if change.Metric == "cycles" || change.Metric == "time" {
			before, after = count(change.Before), count(change.After)
		}
		// The name and the metric are in columns of their own. With one space
		// between them `bench_crc32 cycles` reads as a benchmark called
		// `bench_crc32 cycles`, and somebody hunting for a name in a list of
		// changes would find the wrong one.
		suffix := ""
		if change.Verdict == VerdictUnmeasurable {
			suffix = "  (below what the clock resolves; not a measurement of faster)"
		} else {
			suffix = fmt.Sprintf(" (%s%s%s)", paint(options.Color, colour),
				fmt.Sprintf("%+.1f%%", change.Percent), reset(options.Color))
		}
		fmt.Fprintf(out, "%s %-*s %-7s %10s -> %-10s%s\n",
			marker, nameWidth, truncate(change.Name, nameWidth), change.Metric,
			before, after, suffix)
	}
}

// paint is a colour, when colour is wanted at all.
func paint(enabled bool, colour string) string {
	if !enabled {
		return ""
	}
	return colour
}

// reset is the closing sequence, when colour was opened at all.
func reset(enabled bool) string {
	if !enabled {
		return ""
	}
	return ansiReset
}

// truncate is a name cut to fit, with an ellipsis saying it was cut — a silently
// shortened name would be a benchmark nobody could find in the list.
func truncate(name string, width int) string {
	if len(name) <= width {
		return name
	}
	if width <= 3 {
		return name[:max(0, width)]
	}
	return name[:width-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// WriteJSON writes a run as a document, for another program to read.
//
// The same reasoning as the terminal report: this is what a pipeline archives and
// what a dashboard reads, and neither can read a picture of a terminal.
func WriteJSON(out io.Writer, run Summary) error {
	type document struct {
		Target      string     `json:"target"`
		DurationNs  int64      `json:"duration_ns"`
		Results     []Result   `json:"results"`
		Changes     []Change   `json:"changes,omitempty"`
		Regressions []Change   `json:"regressions,omitempty"`
		Footprint   *Footprint `json:"footprint,omitempty"`
		Failed      []string   `json:"failed,omitempty"`
		Diagnostics []string   `json:"diagnostics,omitempty"`
	}
	encoded, err := json.MarshalIndent(document{
		Target: run.Target, DurationNs: run.Duration,
		Results: run.Results, Changes: run.Changes, Regressions: run.Regressions,
		Footprint: run.Footprint, Failed: run.BudgetFailures, Diagnostics: run.Diagnostics,
	}, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(encoded))
	return err
}

// SortedNames is the benchmarks in a run, by name, for anything that wants them
// ordered rather than in the order they ran.
func SortedNames(results []Result) []string {
	names := make([]string, 0, len(results))
	for _, one := range results {
		names = append(names, one.Benchmark.Name)
	}
	sort.Strings(names)
	return names
}
