//
// @file config.go
// @brief What a project declared about measuring itself, and the config a run works from.
//
// The `bench:` block is a project's own declaration, read through the manifest model
// every component shares — so what `rho bench` runs is the same block a person reads
// in `module.yml`, parsed by the parser that validates it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhobench

import (
	"path/filepath"
	"strings"
	"time"

	"rho/src/deps"
)

// Budgets are the limits a check enforces.
type Budgets struct {
	// MaxStackBytes is the most stack a benchmark may use.
	MaxStackBytes uint64
	// MaxHeapBytes is the most heap a benchmark may hold live.
	MaxHeapBytes uint64
	// MaxCyclesPerIteration is the most cycles one timed iteration may take.
	MaxCyclesPerIteration uint64
	// FailOnRegressionPercent is how much slower a benchmark may get before a check
	// fails, and how much its stack or heap may grow.
	FailOnRegressionPercent float64
	// FailOnRegressionDeclared says whether one was written, so an absent limit is
	// not read as a limit of zero — which would fail everything.
	FailOnRegressionDeclared bool
}

// Config is what a run works from.
type Config struct {
	// Iterations is how many timed runs each benchmark makes.
	Iterations int
	// Warmup is how many are discarded first.
	Warmup int
	// StackWatermark asks for the stack high-water mark to be measured.
	StackWatermark bool
	// Budgets are the limits a check enforces.
	Budgets Budgets
	// Targets are the boards or hosts this project is measured on, in preference
	// order.
	Targets []string
	// Baseline is where a previous run's figures were recorded, relative to the
	// project.
	Baseline string
}

// DefaultBaselineFile is where a project's figures are recorded.
//
// Beside the manifest rather than inside `build/`, because it is a thing to be
// committed and reviewed: a diff on it is a diff anybody can read while reviewing
// the change that caused it, and a number that only exists in a build directory
// cannot be diffed at all.
const DefaultBaselineFile = "bench.baseline.json"

// ReadConfig is a project's benchmark declaration, as this component runs on it.
//
// Everything the block does not say keeps its default, because a benchmark that
// needed its configuration written out before it would run would be a benchmark
// nobody wrote while deciding whether they should.
func ReadConfig(projectRoot string) Config {
	out := Config{
		Iterations:     1000,
		Warmup:         10,
		StackWatermark: true,
		Targets:        []string{"host"},
	}
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil || manifest.Bench == nil {
		return out
	}
	declared := manifest.Bench
	if declared.DefaultIterations > 0 {
		out.Iterations = declared.DefaultIterations
	}
	if declared.WarmupRuns >= 0 {
		out.Warmup = declared.WarmupRuns
	}
	out.StackWatermark = declared.StackWatermark
	if len(declared.Targets) > 0 {
		out.Targets = append([]string{}, declared.Targets...)
	}
	out.Budgets = Budgets{
		MaxStackBytes:            declared.Budgets.MaxStackBytes,
		MaxHeapBytes:             declared.Budgets.MaxHeapBytes,
		MaxCyclesPerIteration:    declared.Budgets.MaxCyclesPerIteration,
		FailOnRegressionPercent:  declared.Budgets.FailOnRegressionPercent,
		FailOnRegressionDeclared: declared.Budgets.FailOnRegressionDeclared,
	}
	return out
}

// BaselinePath is where a project's figures are recorded.
func (c Config) BaselinePath(projectRoot string) string {
	name := strings.TrimSpace(c.Baseline)
	if name == "" {
		name = DefaultBaselineFile
	}
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(projectRoot, filepath.FromSlash(name))
}

// Metrics are the numbers that are compared against a baseline.
//
// Named rather than positional because a baseline file is a thing a person reads in
// a diff, and which column a number was in is the first question anybody asks of it.
type Metrics struct {
	// CyclesMedian is the middle cycle count per timed iteration, which is what a
	// cycle budget is written against.
	CyclesMedian uint64
	// NanosecondsMedian is the middle wall-clock time per timed iteration.
	NanosecondsMedian uint64
	// StackBytes is the most stack one iteration used, or negative when not
	// measured.
	StackBytes int64
	// HeapPeak is the most heap live at once.
	HeapPeak uint64
}

// MetricsOf is the comparable figures of one benchmark.
func MetricsOf(result Result) Metrics {
	return Metrics{
		CyclesMedian:      result.Cycles.Median,
		NanosecondsMedian: result.Stats.Median,
		StackBytes:        result.StackBytes,
		HeapPeak:          result.HeapPeak,
	}
}

// Duration is how long a benchmark runs, which is what a report says rather than
// the cycle count when the clock is a host's and the frequency is unknown.
func Duration(result Result) time.Duration {
	if result.Stats.Median == 0 {
		return 0
	}
	return time.Duration(result.Stats.Median) * time.Nanosecond
}
