//
// @file baseline.go
// @brief What the last run looked like, and what has changed since.
//
// A benchmark's number is only interesting next to another number. Which other one
// is the question, and the answer is the last time this was measured — on the base
// branch, or the commit before this one.
//
// So a run records what it found, and a later run compares. The record is a file in
// the repository rather than something only a log holds, because a diff on it is a
// diff anybody can read while reviewing the change that caused it.
//
// What is recorded is the whole of each benchmark's comparable figures, not a total
// and not a verdict. A total cannot be attributed: "4% slower" is a question, and
// the answer is which benchmark, which metric, and which direction.
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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BaselineVersion is the shape of a recorded baseline.
const BaselineVersion = 1

// Baseline is what a previous run measured.
type Baseline struct {
	Version int `json:"version"`
	// Recorded is when it was measured, which is what makes a stale one visible: a
	// baseline from a release ago is a comparison against the wrong thing, and it
	// looks exactly like a real one otherwise.
	Recorded string `json:"recorded"`
	// Target is what it ran on. A host's cycles and a board's cycles are different
	// units, so a comparison across targets is a number with no meaning — and is
	// refused rather than computed.
	Target string `json:"target,omitempty"`
	// Results is every benchmark's figures, by name.
	Results map[string]Metrics `json:"results"`
	// Source is the program it came from, so a reader can tell which binary the
	// numbers describe.
	Source string `json:"source,omitempty"`
}

// Recorded builds a baseline from a run's results.
func Recorded(results []Result, target, source string) *Baseline {
	out := &Baseline{
		Version:  BaselineVersion,
		Recorded: time.Now().UTC().Format(time.RFC3339),
		Target:   target,
		Source:   source,
		Results:  map[string]Metrics{},
	}
	for _, result := range results {
		out.Results[result.Benchmark.Name] = MetricsOf(result)
	}
	return out
}

// Metrics is a benchmark's recorded figures, and whether there are any.
func (b *Baseline) MetricsFor(name string) (Metrics, bool) {
	if b == nil {
		return Metrics{}, false
	}
	metrics, ok := b.Results[name]
	return metrics, ok
}

// ReadBaseline reads a recorded baseline.
//
// A file that is not there is reported as not there rather than as an error that
// stops the run: no baseline yet is the ordinary state of a project on its first
// day, and refusing to measure because there is nothing to compare against would
// mean the comparison could not be adopted before the first measurement.
func ReadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no baseline at %s; record one with `--write-baseline`, which is what the first run of a project does", path)
		}
		return nil, err
	}
	out := &Baseline{}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("read the baseline at %s: %w", path, err)
	}
	if out.Version != BaselineVersion {
		return nil, fmt.Errorf("the baseline at %s is version %d and this component writes version %d",
			path, out.Version, BaselineVersion)
	}
	return out, nil
}

// WriteBaseline records a baseline into a project's tree.
func WriteBaseline(path string, baseline *Baseline) error {
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// BaselineFromGit reads the baseline as a git ref carries it.
//
// The other way to have a baseline, and the one a pull request wants: the file as
// committed on the base branch, rather than one written here and committed. A ref
// that cannot be read is reported with what git said, because "no baseline" and "the
// branch is called something else" are different problems and a pipeline should be
// able to tell them apart.
func BaselineFromGit(root, ref, path string) (*Baseline, error) {
	command := exec.Command("git", "show", ref+":"+filepath.ToSlash(path))
	command.Dir = root
	data, err := command.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			said := strings.TrimSpace(string(exit.Stderr))
			if index := strings.IndexByte(said, '\n'); index >= 0 {
				said = said[:index]
			}
			if said != "" {
				return nil, fmt.Errorf("read %s from %s: %s", path, ref, said)
			}
		}
		return nil, fmt.Errorf("read %s from %s: %w", path, ref, err)
	}
	out := &Baseline{}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("read the baseline at %s from %s: %w", path, ref, err)
	}
	if out.Version != BaselineVersion {
		return nil, fmt.Errorf("the baseline at %s in %s is version %d and this component writes version %d",
			path, ref, out.Version, BaselineVersion)
	}
	return out, nil
}

// Change is one metric against a baseline.
type Change struct {
	// Name is the benchmark.
	Name string
	// Metric is what changed: `cycles`, `time`, `stack` or `heap`.
	Metric string
	// Before and After are the two figures.
	Before uint64
	After  uint64
	// Delta is the signed change in the metric's own unit — "30 cycles" is a fact,
	// and "2.4%" is an interpretation of it.
	Delta int64
	// Percent is the signed change against the baseline.
	Percent float64
	// Verdict is which way it reads: a lower number is an improvement for all four
	// metrics here, which is the whole reason they can share one direction.
	Verdict Verdict
}

// Changed is every metric of every benchmark that moved, largest relative change
// first.
//
// One comparison per metric rather than one per benchmark: a change that cut cycles
// and grew the heap has done one thing well and one thing badly, and averaging them
// into a single verdict would be a number describing neither.
func Changed(results []Result, baseline *Baseline) []Change {
	out := []Change{}
	for _, result := range results {
		before, had := baseline.MetricsFor(result.Benchmark.Name)
		if !had {
			out = append(out, Change{
				Name: result.Benchmark.Name, Metric: "all",
				After: result.Cycles.Median, Verdict: VerdictNew,
			})
			continue
		}
		after := MetricsOf(result)
		out = append(out, compare(result.Benchmark.Name, "cycles", before.CyclesMedian, after.CyclesMedian))
		out = append(out, compare(result.Benchmark.Name, "time", before.NanosecondsMedian, after.NanosecondsMedian))
		if before.StackBytes >= 0 && after.StackBytes >= 0 {
			out = append(out, compare(result.Benchmark.Name, "stack",
				uint64(before.StackBytes), uint64(after.StackBytes)))
		}
		out = append(out, compare(result.Benchmark.Name, "heap", before.HeapPeak, after.HeapPeak))
	}
	sortByMagnitude(out)
	return out
}

// compare is one metric before and after.
func compare(name, metric string, before, after uint64) Change {
	change := Change{
		Name: name, Metric: metric,
		Before: before, After: after,
		Delta:   int64(after) - int64(before),
		Verdict: Improvement(before, after),
	}
	if before > 0 {
		change.Percent = RelativeChange(before, after)
	}
	return change
}

// sortByMagnitude puts the biggest relative change first, which is the one a reader
// is looking for.
//
// By magnitude rather than signed, because a 50% improvement is as much news as a
// 50% regression and hiding it below the noise would be losing half of what the
// comparison found.
func sortByMagnitude(changes []Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		return abs(changes[i].Percent) > abs(changes[j].Percent)
	})
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

// Regressions are the changes that are worse by more than the limit.
//
// A limit rather than any change, because benchmarks move: a rebuild shifts
// alignment, a scheduler shift changes a median by a percent, and a gate on *any*
// change would be switched off within a month. The limit is what makes it a gate
// rather than a report.
func Regressions(changes []Change, budgets Budgets) []Change {
	if !budgets.FailOnRegressionDeclared {
		return nil
	}
	out := []Change{}
	for _, change := range changes {
		if change.Verdict != VerdictWorse {
			continue
		}
		if change.Percent > budgets.FailOnRegressionPercent {
			out = append(out, change)
		}
	}
	return out
}
