//
// @file stats.go
// @brief What a set of runs means.
//
// A benchmark runs the same thing many times because one run measures noise. The
// clock interrupts, the cache is cold or warm, the scheduler moved the thread, the
// neighbour on the bus woke up. A single measurement is a draw from that, and a
// comparison against another single measurement is a comparison of two draws — which
// is how a benchmark reports a 20% improvement caused by nothing at all.
//
// So the runs are summarised rather than sampled from. Five numbers carry most of
// it: the middle one, the spread either side of it, and how far apart the fastest
// and slowest were.
//
//   - **Median** is the middle run. Not the mean: one run in a thousand is an order
//     of magnitude out — a page fault, a timer that landed badly — and the mean is
//     moved by it in a way that has nothing to do with the code.
//   - **p95 and p99** are the tails, because latency is a promise about the worst
//     case and a median hides it. A firmware that is 50 µs most of the time and
//     4 ms occasionally has a problem the median does not have.
//   - **Standard deviation** is whether the runs agreed with each other, which is
//     what says whether any of the above can be compared to anything.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhobench

import (
	"math"
	"sort"
)

// Samples is a set of measurements of one thing.
type Samples []uint64

// Stats is what a set of samples means.
type Stats struct {
	// Iterations is how many samples there were.
	Iterations int `json:"iterations"`
	// Min, Max and Median are the middle and the ends.
	Min    uint64 `json:"min"`
	Max    uint64 `json:"max"`
	Median uint64 `json:"median"`
	// Mean is the arithmetic average, reported because it is what everybody
	// quotes and comparing against it costs nothing.
	Mean float64 `json:"mean"`
	// StdDev is how far apart the runs were. Zero for one run, which is not a
	// measurement of anything and is said rather than shown as certainty.
	StdDev float64 `json:"stddev"`
	// P95 and P99 are the tails.
	P95 uint64 `json:"p95"`
	P99 uint64 `json:"p99"`
}

// Summarise is what a set of samples means.
//
// An empty set summarises to zeroes rather than to nothing: a benchmark that ran no
// iterations has a measurement of nothing, and a reader that got an absent field
// would have to decide whether that is zero or missing. It is zero.
func Summarise(samples Samples) Stats {
	if len(samples) == 0 {
		return Stats{}
	}
	sorted := append(Samples{}, samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	stats := Stats{
		Iterations: len(sorted),
		Min:        sorted[0],
		Max:        sorted[len(sorted)-1],
		Median:     percentile(sorted, 50),
		P95:        percentile(sorted, 95),
		P99:        percentile(sorted, 99),
	}
	var total float64
	for _, value := range sorted {
		total += float64(value)
	}
	stats.Mean = total / float64(len(sorted))

	// Standard deviation about the mean, with the sum of squares taken in float64
	// because the squares of cycle counts overflow a uint64 well before the counts
	// do — and a benchmark that overflows its own arithmetic is a benchmark that
	// reports a variance of zero, which reads as perfect stability.
	var sumSquares float64
	for _, value := range sorted {
		delta := float64(value) - stats.Mean
		sumSquares += delta * delta
	}
	stats.StdDev = math.Sqrt(sumSquares / float64(len(sorted)))
	return stats
}

// percentile is the value at a percentage of a sorted set.
//
// The nearest-rank definition: the smallest value at or above which that percentage
// of the samples fall. Chosen over interpolating because it returns a value that was
// actually measured — every percentile of a benchmark is then a real run's number
// rather than a number between two of them — and because it is the definition
// `p95 latency` means to anybody reading a latency report.
func percentile(sorted Samples, percent float64) uint64 {
	if len(sorted) == 0 {
		return 0
	}
	if percent <= 0 {
		return sorted[0]
	}
	if percent >= 100 {
		return sorted[len(sorted)-1]
	}
	rank := int(math.Ceil(percent / 100 * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// RelativeChange is how much `from` changed to `to`, as a percentage, and whether
// it went up or down.
//
// Signed, because "20% faster" and "20% slower" are the same magnitude and opposite
// facts, and a report that showed both as `20` would be asking a reader to remember
// which way round it was.
func RelativeChange(from, to uint64) float64 {
	if from == 0 {
		// No baseline to compare with. Zero rather than infinity: the caller
		// reports this as "new" rather than as an infinite regression, which is
		// what a benchmark appearing for the first time is.
		return 0
	}
	return (float64(to) - float64(from)) / float64(from) * 100
}

// Improvement is how much a metric moved, described in words a reader can act on.
//
// A benchmark is about direction: whether something got cheaper. So a movement
// that lowers a number is an improvement and one that raises it is a regression,
// which is the opposite of what "percent change" would imply for time.
//
// A movement to or from zero is neither, because zero usually means the change
// fell below what the clock could see rather than that nothing was left.
func Improvement(before, after uint64) Verdict {
	if before == after {
		return VerdictSame
	}
	if (before == 0) != (after == 0) {
		// Zero is usually "below what the clock could see" rather than "nothing
		// left": a host's `clock_gettime` resolves to about 100 ns, so a 70-cycle
		// function reads as `0` whether it just got faster or always was. Reporting
		// that as an improvement is a claim the measurement cannot support.
		return VerdictUnmeasurable
	}
	if after < before {
		return VerdictBetter
	}
	return VerdictWorse
}

// Verdict is how a change reads.
type Verdict string

// The three ways a change reads.
const (
	// VerdictBetter means a number went down, which for time and memory is good.
	VerdictBetter Verdict = "improved"
	// VerdictWorse means a number went up.
	VerdictWorse Verdict = "regressed"
	// VerdictSame means nothing moved.
	VerdictSame Verdict = "unchanged"
	// VerdictNew means there was nothing to compare against.
	VerdictNew Verdict = "new"
	// VerdictUnmeasurable means one side fell below what the clock could resolve,
	// so the direction is not something the measurement can say.
	VerdictUnmeasurable Verdict = "below the clock's resolution"
)
