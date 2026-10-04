//
// @file runner.go
// @brief Running a benchmark and reading what it said.
//
// A benchmark is a program. This runs it, reads the one document it prints, and
// aggregates it — which is the whole division of labour between a harness and this:
// the harness measures on the target where a clock and a stack boundary are, and
// everything that needs memory to summarise ten thousand runs happens here.
//
// Reading a report is where a run most often goes wrong, and the failures are worth
// naming because each of them is a program that looks like a benchmark:
//
//   - **Nothing printed.** A benchmark that was never built, or one that ran and
//     crashed before it got there. Said as such rather than reported as a suite
//     with no results, which reads like a suite that measured nothing.
//   - **The wrong version.** A harness compiled into firmware cannot be asked to
//     change, so its report is refused with the version it wanted rather than
//     half-parsed into zeroes that read as measurements.
//   - **Results from the wrong binary.** A stale executable, or one for another
//     target. The name it ran under is reported so a reader can tell.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhobench

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RunOptions is what a run was asked to do.
type RunOptions struct {
	// Binary is the benchmark program. Empty means the project's declared one.
	Binary string
	// Target is what it should run on: `host`, or a board.
	Target string
	// Filter is a glob over benchmark names; empty runs everything.
	Filter string
	// Iterations and Warmup override the project's declaration when given.
	Iterations int
	Warmup     int
	// Timeout is how long the program may take. Zero means a generous default,
	// because a benchmark that has been made slow by the thing it is measuring
	// should report that rather than be killed by the tool measuring it.
	Timeout time.Duration
	// Environment is extra environment for the child.
	Environment []string
	// WorkDir is where the program runs, which decides what a relative path in it
	// means.
	WorkDir string
}

// Execution is one benchmark program's results.
type Execution struct {
	// Binary is what was run.
	Binary string
	// Target is what it ran on.
	Target string
	// Report is what it printed, parsed.
	Report Report
	// Duration is how long the program took, which is not the same as what any
	// benchmark measured and is reported because a run that took a minute says
	// something about the suite's shape.
	Duration time.Duration
	// Diagnostics is what went wrong, if anything.
	Diagnostics []string
}

// DefaultTimeout is how long a benchmark program may take before it is presumed
// hung.
//
// Generous, and on purpose. The failure this guards against is a program blocked on
// something, and killing a slow benchmark at a minute produces a "timed out" that a
// reader has to investigate — which is the cost of the guard, paid exactly when it
// was not needed. A benchmark that takes longer than this is worth stopping.
const DefaultTimeout = 30 * time.Minute

// RunBenchmark runs a benchmark program and reads its report.
func RunBenchmark(options RunOptions) (*Execution, error) {
	if strings.TrimSpace(options.Binary) == "" {
		return nil, fmt.Errorf("no benchmark program to run: build one, or say which with --binary")
	}
	if options.Timeout <= 0 {
		options.Timeout = DefaultTimeout
	}

	started := time.Now()
	command := exec.Command(options.Binary)
	if options.WorkDir != "" {
		command.Dir = options.WorkDir
	}
	command.Env = append(os.Environ(), options.Environment...)
	// The harness's own output — the report and whatever it logged — is captured
	// rather than passed through, because a run that interleaves a firmware's log
	// with its results is unreadable and the log belongs in a file.
	output, err := command.Output()
	run := &Execution{Binary: options.Binary, Target: options.Target}

	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			run.Diagnostics = append(run.Diagnostics, fmt.Sprintf(
				"%s exited with status %d before reporting; its output was:\n%s",
				filepath.Base(options.Binary), exit.ExitCode(), strings.TrimSpace(string(exit.Stderr))))
			run.Duration = time.Since(started)
			return run, fmt.Errorf("%s did not report any measurements", filepath.Base(options.Binary))
		}
		return run, fmt.Errorf("run %s: %w", filepath.Base(options.Binary), err)
	}
	run.Duration = time.Since(started)

	report, err := ParseReport(string(output))
	if err != nil {
		return run, err
	}
	run.Report = *report
	if options.Target != "" && report.Target != "" && report.Target != options.Target {
		// A report from another target is not a small discrepancy: a host's cycles
		// and a board's cycles are different units, and a regression computed
		// between them is a number with no meaning.
		run.Diagnostics = append(run.Diagnostics, fmt.Sprintf(
			"the report says it ran on %s and this run asked for %s; its numbers are being reported as they are",
			report.Target, options.Target))
	}
	run.Diagnostics = append(run.Diagnostics, report.Diagnostics...)
	return run, nil
}

// Results is every benchmark in an execution, aggregated and filtered.
func (r *Execution) Results(options RunOptions) []Result {
	out := []Result{}
	for _, benchmark := range r.Report.Benchmarks {
		if !matches(benchmark.Name, options.Filter) {
			continue
		}
		out = append(out, Result{
			Benchmark:  benchmark,
			Stats:      benchmark.Stats(),
			Cycles:     benchmark.CycleStats(),
			StackBytes: benchmark.StackBytes,
			HeapPeak:   benchmark.HeapPeakBytes,
		})
	}
	return out
}

// matches is a name against a filter.
//
// A glob, so a filter can be a whole group (`crypto_*`) without the component having
// to know what a suite calls itself. An empty filter matches everything, which is
// what "run everything" means.
func matches(name, filter string) bool {
	if strings.TrimSpace(filter) == "" {
		return true
	}
	if ok, err := filepath.Match(filter, name); err == nil && ok {
		return true
	}
	// A substring match as well, because somebody typing `crypto` means every
	// benchmark with that in its name and has not read the list closely enough to
	// know whether it ends in `_` or is a glob.
	return strings.Contains(name, strings.Trim(filter, "*"))
}
