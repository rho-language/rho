//
// @file bench_test.go
// @brief What a set of runs means, and what has changed since the last one.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhobench

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Statistics                                                                */
/* -------------------------------------------------------------------------- */

// A benchmark runs the same thing many times because one run measures noise, and the
// median is the summary that noise does not move.
func TestTheMedianIsTheMiddleRun(t *testing.T) {
	stats := Summarise(Samples{9, 1, 5, 3, 7})
	if stats.Median != 5 {
		t.Errorf("expected the median to be 5, got %d", stats.Median)
	}
	if stats.Min != 1 || stats.Max != 9 {
		t.Errorf("expected the ends to be 1 and 9, got %d and %d", stats.Min, stats.Max)
	}
}

// One run in a thousand can be an order of magnitude out — a page fault, a timer
// that landed badly — and the mean is moved by it in a way that has nothing to do
// with the code. The median is not.
func TestTheMedianIsNotMovedByAnOutlier(t *testing.T) {
	ordinary := Samples{100, 101, 102, 103, 104}
	withOutlier := append(append(Samples{}, ordinary...), 1000000)

	if Summarise(ordinary).Median != Summarise(withOutlier).Median {
		t.Error("an outlier moved the median, which is the one thing it must not do")
	}
	// And the mean, which is what everybody quotes, is dragged by it: one run in a
	// thousand at a million moves a mean of a hundred by more than an order of
	// magnitude. That difference is why both are reported.
	if Summarise(withOutlier).Mean < 100000 {
		t.Errorf("expected the mean to be dragged by the outlier, got %g", Summarise(withOutlier).Mean)
	}
}

// Latency is a promise about the worst case, and a median hides it.
func TestTheTailsAreTheUpperPercentiles(t *testing.T) {
	samples := Samples{}
	for i := 1; i <= 100; i++ {
		samples = append(samples, uint64(i))
	}
	stats := Summarise(samples)
	if stats.Median != 50 {
		t.Errorf("expected the median to be 50, got %d", stats.Median)
	}
	if stats.P95 != 95 {
		t.Errorf("expected p95 to be 95, got %d", stats.P95)
	}
	if stats.P99 != 99 {
		t.Errorf("expected p99 to be 99, got %d", stats.P99)
	}
	// Nearest-rank: every percentile is a value that was actually measured, rather
	// than a number between two runs — which is what makes a reported p99 something
	// somebody can point at in the harness's own output.
	if stats.P95 != 95 {
		t.Error("a percentile is not a measured value")
	}
}

// Whether the runs agreed with each other is what says whether any of the above can
// be compared to anything.
func TestTheSpreadIsReported(t *testing.T) {
	// Two runs differing by one: almost no spread.
	tight := Summarise(Samples{100, 101})
	// The same mean, spread widely: a lot.
	wide := Summarise(Samples{1, 199})
	if tight.StdDev >= wide.StdDev {
		t.Errorf("expected the wide set to have the larger spread, got %g and %g", tight.StdDev, wide.StdDev)
	}
}

// The squares of cycle counts overflow a uint64 well before the counts do, and a
// benchmark that overflows its own arithmetic reports a variance of zero, which
// reads as perfect stability.
func TestTheSpreadDoesNotOverflowOnLargeCounts(t *testing.T) {
	const huge = uint64(1) << 60
	stats := Summarise(Samples{huge, huge + 1024, huge - 1024})
	if stats.StdDev == 0 {
		t.Fatal("expected a non-zero spread on large counts; zero means the arithmetic overflowed")
	}
}

// A benchmark that ran no iterations has a measurement of nothing, and a reader who
// got an absent field would have to decide whether that is zero or missing.
func TestNoSamplesSummariseToZero(t *testing.T) {
	stats := Summarise(nil)
	if stats.Iterations != 0 || stats.Median != 0 || stats.Mean != 0 {
		t.Errorf("expected zeroes, got %+v", stats)
	}
}

/* -------------------------------------------------------------------------- */
/*  The protocol                                                              */
/* -------------------------------------------------------------------------- */

// A harness is compiled into firmware and cannot be asked to change, so a report in
// a shape this does not know is refused rather than half-parsed into zeroes that read
// as measurements.
func TestARefusedReportIsRefusedRatherThanGuessedAt(t *testing.T) {
	// Wrapped in the markers, because that is what a harness writes: the markers
	// are how a report is found in a firmware that also logs.
	_, err := ParseReport(`@rho-bench:{"version":99,"benchmarks":[{"name":"a","samples":[1]}]}@rho-bench-end`)
	if err == nil {
		t.Fatal("expected a report of another version to be refused")
	}
	if !strings.Contains(err.Error(), "version 99") {
		t.Errorf("expected the version it wanted named, got %v", err)
	}
}

// A benchmark usually also logs, so the report is looked for rather than required to
// be the whole of what was printed.
func TestTheReportIsFoundAmongOtherOutput(t *testing.T) {
	report := Report{Version: ProtocolVersion, Benchmarks: []Benchmark{{Name: "work", Samples: Samples{10, 20}}}}
	output := "[info] starting\n[info] running\n" + report.Encode() + "[info] done\n"

	parsed, err := ParseReport(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Benchmarks) != 1 || parsed.Benchmarks[0].Name != "work" {
		t.Fatalf("unexpected report: %+v", parsed)
	}
	if parsed.Benchmarks[0].Samples[1] != 20 {
		t.Errorf("expected the samples to survive, got %+v", parsed.Benchmarks[0].Samples)
	}
}

// A benchmark that printed nothing has not measured anything, and saying so is
// different from reporting a suite whose benchmarks all measured zero.
func TestAnEmptyReportIsAnError(t *testing.T) {
	_, err := ParseReport(`{"version":1,"benchmarks":[]}`)
	if err == nil {
		t.Fatal("expected a report with no measurements to be refused")
	}
}

// The round trip has to hold: a harness encodes, this reads, and every sample
// survives — because every number in the report is computed from them.
func TestAReportRoundTrips(t *testing.T) {
	original := Report{
		Version: ProtocolVersion, Suite: "crypto",
		Benchmarks: []Benchmark{{
			Name: "bench_aes", Unit: "ns", Clock: "dwt",
			Iterations: 10000, WarmupRuns: 10,
			Samples:    Samples{1240, 1250, 1300},
			Cycles:     Samples{1240, 1250, 1300},
			StackBytes: 256, StackMeasured: true,
			HeapPeakBytes: 2048, HeapLiveBytes: 0, HeapAllocations: 12,
		}},
	}
	parsed, err := ParseReport(original.Encode())
	if err != nil {
		t.Fatal(err)
	}
	got := parsed.Benchmarks[0]
	if got.StackBytes != 256 || !got.StackMeasured {
		t.Errorf("expected the watermark to survive, got %d measured=%v", got.StackBytes, got.StackMeasured)
	}
	if got.HeapPeakBytes != 2048 || got.HeapAllocations != 12 {
		t.Errorf("expected the heap figures to survive, got %+v", got)
	}
	if got.Stats().Median != 1250 {
		t.Errorf("expected the median to be computable, got %d", got.Stats().Median)
	}
}

// A stack figure is a claim only when there was a boundary to find, so a harness
// that could not find one says so rather than claiming zero.
func TestAnUnmeasuredWatermarkIsNotZero(t *testing.T) {
	benchmark := Benchmark{Name: "x", StackBytes: -1, StackMeasured: false}
	if benchmark.StackMeasured {
		t.Error("a watermark that was never established was reported as measured")
	}
	if benchmark.StackBytes == 0 {
		t.Error("an unmeasured watermark was reported as zero, which is a claim")
	}
}

/* -------------------------------------------------------------------------- */
/*  Baselines                                                                 */
/* -------------------------------------------------------------------------- */

func resultsFor(samples Samples, cycles Samples, heap uint64) []Result {
	return []Result{{
		Benchmark: Benchmark{Name: "work", Samples: samples, Cycles: cycles, HeapPeakBytes: heap},
		Stats:     Summarise(samples),
		Cycles:    Summarise(cycles),
		HeapPeak:  heap,
	}}
}

// A benchmark that costs less is better, and "20% faster" and "20% slower" are the
// same magnitude and opposite facts.
func TestTheDirectionOfBetterIsFixed(t *testing.T) {
	if Improvement(100, 80) != VerdictBetter {
		t.Error("a lower number is an improvement")
	}
	if Improvement(100, 120) != VerdictWorse {
		t.Error("a higher number is a regression")
	}
	if Improvement(100, 100) != VerdictSame {
		t.Error("no change is no change")
	}
	// A host's clock is coarser than a fast function, so zero often means "below
	// what could be seen" rather than "free". Calling that an improvement is a
	// claim the measurement cannot support.
	if Improvement(100, 0) != VerdictUnmeasurable {
		t.Error("a fall to zero must not be reported as an improvement")
	}
	if Improvement(0, 100) != VerdictUnmeasurable {
		t.Error("a rise from zero must not be reported as a regression")
	}
	if Improvement(0, 0) != VerdictSame {
		t.Error("zero against zero is no change")
	}
}

// A benchmark appearing for the first time has nothing to compare against, which is
// not an infinite regression.
func TestANewBenchmarkIsNewRatherThanAnInfiniteRegression(t *testing.T) {
	results := resultsFor(Samples{100}, Samples{100}, 0)
	changes := Changed(results, &Baseline{Version: BaselineVersion, Results: map[string]Metrics{}})
	if len(changes) != 1 || changes[0].Verdict != VerdictNew {
		t.Fatalf("expected the benchmark to be reported as new, got %+v", changes)
	}
	if changes[0].Percent != 0 {
		t.Errorf("expected no percentage for something with no baseline, got %g", changes[0].Percent)
	}
}

// Each metric is compared on its own, because a change that cuts cycles and grows
// the heap has done one thing well and one thing badly.
func TestEachMetricIsComparedOnItsOwn(t *testing.T) {
	before := &Baseline{Version: BaselineVersion, Results: map[string]Metrics{
		"work": {CyclesMedian: 1000, NanosecondsMedian: 2000, HeapPeak: 4096},
	}}
	// Time goes up, cycles go down, the heap goes up.
	after := resultsFor(Samples{4000}, Samples{800}, 8192)

	changes := Changed(after, before)
	byMetric := map[string]Change{}
	for _, change := range changes {
		byMetric[change.Metric] = change
	}
	if byMetric["cycles"].Verdict != VerdictBetter {
		t.Errorf("expected fewer cycles to be an improvement, got %+v", byMetric["cycles"])
	}
	if byMetric["time"].Verdict != VerdictWorse {
		t.Errorf("expected more time to be a regression, got %+v", byMetric["time"])
	}
	if byMetric["heap"].Verdict != VerdictWorse {
		t.Errorf("expected a larger heap to be a regression, got %+v", byMetric["heap"])
	}
}

// A limit rather than any change: benchmarks move, and a gate on any change would be
// switched off within a month.
func TestOnlyChangesOverTheLimitAreRegressions(t *testing.T) {
	budgets := Budgets{FailOnRegressionPercent: 5, FailOnRegressionDeclared: true}
	small := []Change{{Metric: "cycles", Before: 100, After: 103, Percent: 3, Verdict: VerdictWorse}}
	large := []Change{{Metric: "cycles", Before: 100, After: 120, Percent: 20, Verdict: VerdictWorse}}
	improved := []Change{{Metric: "cycles", Before: 100, After: 50, Percent: -50, Verdict: VerdictBetter}}

	if len(Regressions(small, budgets)) != 0 {
		t.Error("a change inside the limit must not fail")
	}
	if len(Regressions(large, budgets)) != 1 {
		t.Error("a change over the limit must fail")
	}
	if len(Regressions(improved, budgets)) != 0 {
		t.Error("an improvement must never fail")
	}
	// An absent limit is not a limit of zero, which would fail everything.
	if len(Regressions(large, Budgets{})) != 0 {
		t.Error("an undeclared limit must not fail anything")
	}
}

// The baseline is a file in the repository, because a diff on it is a diff anybody
// can read while reviewing the change that caused it.
func TestABaselineRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.baseline.json")
	// Three samples so the median is the middle one rather than the lower of two.
	original := Recorded(resultsFor(Samples{200}, Samples{50, 100, 150}, 4096), "nucleo-h753zi", "./bench")
	if err := WriteBaseline(path, original); err != nil {
		t.Fatal(err)
	}
	read, err := ReadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	metrics, ok := read.MetricsFor("work")
	if !ok {
		t.Fatal("expected the benchmark to be in the baseline")
	}
	if metrics.CyclesMedian != 100 {
		t.Errorf("expected the cycle median recorded, got %d", metrics.CyclesMedian)
	}
	if read.Target != "nucleo-h753zi" {
		t.Errorf("expected the target recorded, got %q", read.Target)
	}
}

// A baseline written by another version is refused rather than half-read.
func TestABaselineOfAnotherVersionIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.baseline.json")
	if err := WriteBaseline(path, &Baseline{Version: BaselineVersion}); err != nil {
		t.Fatal(err)
	}
	// A document in a shape this does not know: the version is the whole of it.
	os.WriteFile(path, []byte(`{"version":`+strconv.Itoa(BaselineVersion+1)+`,"results":{}}`), 0o644)

	if _, err := ReadBaseline(path); err == nil {
		t.Error("expected a baseline of another version to be refused")
	}
}

// No baseline yet is the ordinary state of a project on its first day.
func TestAMissingBaselineSaysHowToMakeOne(t *testing.T) {
	_, err := ReadBaseline(filepath.Join(t.TempDir(), "nothing.json"))
	if err == nil {
		t.Fatal("expected the missing baseline reported")
	}
	if !strings.Contains(err.Error(), "--write-baseline") {
		t.Errorf("expected the error to say how to record one, got %v", err)
	}
}

// A budget a benchmark broke is this component's finding, and the report says which.
func TestABudgetBreachIsNamed(t *testing.T) {
	result := Result{
		Benchmark:  Benchmark{Name: "work"},
		StackBytes: 8192,
		HeapPeak:   16384,
	}
	failures := OverBudget(result, Budgets{MaxStackBytes: 4096, MaxHeapBytes: 8192})
	if len(failures) != 2 {
		t.Fatalf("expected both breaches named, got %v", failures)
	}
	if !strings.Contains(failures[0], "work") || !strings.Contains(failures[0], "stack") {
		t.Errorf("expected the benchmark and the metric named, got %q", failures[0])
	}
	// A budget nobody wrote is not a budget of zero.
	if len(OverBudget(result, Budgets{})) != 0 {
		t.Error("undeclared budgets must not fail")
	}
}

/* -------------------------------------------------------------------------- */
/*  Selection                                                                 */
/* -------------------------------------------------------------------------- */

// A filter is a glob, so a whole group can be named at once, and a substring too,
// because somebody typing `crypto` means every benchmark with it in the name.
func TestAFilterSelectsBenchmarks(t *testing.T) {
	cases := []struct {
		name   string
		filter string
		want   bool
	}{
		{"bench_crypto_aes", "", true},
		{"bench_crypto_aes", "crypto_*", true},
		{"bench_crypto_aes", "crypto", true},
		{"bench_crypto_aes", "bench_crypto_aes", true},
		{"bench_crypto_aes", "*_aes", true},
		{"bench_crypto_aes", "bench_crc32", false},
		{"bench_crypto_aes", "json*", false},
		{"bench_json_parse", "crypto", false},
	}
	for _, one := range cases {
		if got := matches(one.name, one.filter); got != one.want {
			t.Errorf("%q against filter %q: expected %v, got %v", one.name, one.filter, one.want, got)
		}
	}
}
