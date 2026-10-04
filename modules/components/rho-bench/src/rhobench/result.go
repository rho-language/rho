//
// @file result.go
// @brief What a benchmark reports, and the protocol it reports it in.
//
// A benchmark is a program that runs on a target and prints numbers. Who aggregates
// them is a choice, and it is made here rather than in the harness for one reason:
// the harness is compiled into somebody's firmware, and every line of it is code
// that runs on a board with 128 kB of RAM and a 115200 baud console. The less of it
// there is, the better. So the harness measures and prints; everything else — the
// statistics, the comparison, the report, the verdict — happens here, on a machine
// with memory.
//
// The protocol is one JSON document on standard output, written by the harness and
// read by this. It is a document rather than a format invented here: a benchmark
// written in any language that can print one runs under `rho bench` without
// linking anything from this project.
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
	"strings"
)

// ProtocolVersion is the shape of a benchmark's report.
//
// Versioned because a harness is compiled into firmware and cannot be asked to
// change: a report in a shape this does not know is refused with its version
// named, rather than half-parsed into zeroes that read as measurements.
const ProtocolVersion = 1

// Report is what a benchmark printed.
type Report struct {
	// Version is the protocol this report is written in.
	Version int `json:"version"`
	// Target is what it ran on: a board name, or `host`.
	Target string `json:"target,omitempty"`
	// Suite is the group it belongs to, so a project can have several.
	Suite string `json:"suite,omitempty"`
	// Benchmarks are what it measured.
	Benchmarks []Benchmark `json:"benchmarks"`
	// Diagnostics is anything the harness itself wanted to say — a clock it could
	// not read, a watermark it could not establish.
	Diagnostics []string `json:"diagnostics,omitempty"`
}

// Benchmark is one measured function.
type Benchmark struct {
	// Name is the function, as the harness registered it.
	Name string `json:"name"`
	// Unit is what the numbers are in: `cycles`, `ns`, `us`, `ms`. Carried in the
	// report rather than assumed, because a bare number with no unit is a number
	// nobody can check.
	Unit string `json:"unit,omitempty"`
	// Iterations is how many timed runs there were.
	Iterations int `json:"iterations,omitempty"`
	// WarmupRuns is how many were discarded before measuring.
	WarmupRuns int `json:"warmup_runs,omitempty"`
	// Samples are the timed runs, and the whole of what the statistics are
	// computed from.
	//
	// Every one is reported rather than a summary, because the summary is
	// computed here: the harness is the thing on the target and this is the thing
	// that knows what the median means, and a harness that also summarised would
	// have two implementations of it.
	Samples Samples `json:"samples,omitempty"`
	// CyclesPerIteration is the same runs read from the target's cycle counter,
	// when it has one. Empty on a host, where the clock is the host's.
	Cycles Samples `json:"cycles,omitempty"`
	// StackBytes is the most stack one iteration used, when the harness measured
	// it. Negative means "not measured", which is different from zero.
	StackBytes int64 `json:"stack_bytes,omitempty"`
	// StackMeasured says whether the watermark was established at all. A harness
	// that filled the stack but could not find the boundary reports no watermark
	// rather than zero, because zero is a claim.
	StackMeasured bool `json:"stack_measured,omitempty"`
	// HeapPeakBytes is the most heap live at once, when the harness tracked it.
	HeapPeakBytes uint64 `json:"heap_peak_bytes,omitempty"`
	// HeapLiveBytes is what was still live at the end, which is what a leak looks
	// like: a peak that is stable and a live count that is not.
	HeapLiveBytes uint64 `json:"heap_live_bytes,omitempty"`
	// HeapAllocations is how many allocations the whole run made.
	HeapAllocations uint64 `json:"heap_allocations,omitempty"`
	// Clock is what the samples are from: `dwt`, `rdtsc`, `chrono`. Reported so a
	// number can be traced to where it came from.
	Clock string `json:"clock,omitempty"`
}

// ParseReport is a harness's output, and whether it was one.
//
// A benchmark that printed something else has not failed — a program that also logs
// prints other things — so the document is looked for rather than required to be
// the whole output. The marker is a line the harness writes around it, which is
// what makes finding it reliable when the firmware logs to the same console.
func ParseReport(output string) (*Report, error) {
	const begin = "@rho-bench:"
	const end = "@rho-bench-end"

	start := strings.Index(output, begin)
	if start < 0 {
		return nil, fmt.Errorf("no benchmark report on standard output; looked for a line starting %q", begin)
	}
	body := output[start+len(begin):]
	if finish := strings.Index(body, end); finish >= 0 {
		body = body[:finish]
	}
	report := &Report{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), report); err != nil {
		return nil, fmt.Errorf("the benchmark report could not be read: %w", err)
	}
	if report.Version != ProtocolVersion {
		return nil, fmt.Errorf("the benchmark reports version %d and this component reads version %d",
			report.Version, ProtocolVersion)
	}
	if len(report.Benchmarks) == 0 {
		return nil, fmt.Errorf("the benchmark reported no measurements")
	}
	return report, nil
}

// Encode is a report as the harness writes it: the marker, the document, and the
// end marker, on lines of their own.
//
// The markers are what let a benchmark be run in a firmware that also prints:
// without them a log line that happens to look like JSON would be read as a result.
func (r Report) Encode() string {
	encoded, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return "@rho-bench:" + string(encoded) + "@rho-bench-end\n"
}

// Stats is what one benchmark's samples mean, in the unit they are in.
func (b Benchmark) Stats() Stats { return Summarise(b.Samples) }

// CycleStats is what one benchmark's cycle counts mean.
//
// Empty when the harness had no cycle counter — a host, or a target that was not
// asked for one — and a caller asking for these is asking about a target it should
// have named.
func (b Benchmark) CycleStats() Stats { return Summarise(b.Cycles) }

// Result is one benchmark measured, aggregated, with what it was compared against.
type Result struct {
	Benchmark Benchmark `json:"benchmark"`
	// Stats is the runtime summary, in the benchmark's own unit.
	Stats Stats `json:"stats"`
	// Cycles is the cycle summary, empty on a host.
	Cycles Stats `json:"cycles"`
	// StackBytes and HeapPeak are the dynamic figures, when measured.
	StackBytes int64  `json:"stack_bytes,omitempty"`
	HeapPeak   uint64 `json:"heap_peak_bytes,omitempty"`
	// Against is what the baseline said, when there was one.
	Against *Comparison `json:"against,omitempty"`
	// Failed is what failed about this benchmark, which is the report's own
	// findings rather than this component's opinion of them.
	Failed []string `json:"failed,omitempty"`
}

// Comparison is one measurement against a baseline.
type Comparison struct {
	// Before and After are the numbers, in the metric's own unit.
	Before uint64 `json:"before"`
	After  uint64 `json:"after"`
	// Delta is the signed change in the metric's own unit, because "30 cycles" is
	// a fact and "2.4%" is an interpretation of it.
	Delta int64 `json:"delta"`
	// Percent is the signed change as a percentage of the baseline.
	Percent float64 `json:"percent"`
	// Verdict is which way it reads.
	Verdict Verdict `json:"verdict"`
}

// OverBudget is whether a measurement broke a limit the project declared.
func OverBudget(result Result, budgets Budgets) []string {
	failures := []string{}
	if budgets.MaxStackBytes > 0 && result.StackBytes > int64(budgets.MaxStackBytes) {
		failures = append(failures, fmt.Sprintf("%s used %d B of stack, over the %d B budget",
			result.Benchmark.Name, result.StackBytes, budgets.MaxStackBytes))
	}
	if budgets.MaxHeapBytes > 0 && result.HeapPeak > budgets.MaxHeapBytes {
		failures = append(failures, fmt.Sprintf("%s peaked at %d B of heap, over the %d B budget",
			result.Benchmark.Name, result.HeapPeak, budgets.MaxHeapBytes))
	}
	if budgets.MaxCyclesPerIteration > 0 && result.Cycles.Iterations > 0 {
		if median := result.Cycles.Median; median > budgets.MaxCyclesPerIteration {
			failures = append(failures, fmt.Sprintf("%s took %d cycles per iteration, over the %d budget",
				result.Benchmark.Name, median, budgets.MaxCyclesPerIteration))
		}
	}
	return failures
}
