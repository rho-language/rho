//
// @file budget.go
// @brief What a project allows, and whether the image fits.
//
// Two questions, and they are different questions. "Does it fit" is about the
// board: does this image fit in this flash. "Did it grow" is about the change: is
// this pull request bigger than the last one, even if both fit.
//
// Only the first can be answered from the board alone, and only the second is
// worth a CI gate. A budget checked only at the limit is a budget that is
// exceeded one release at a time, with nothing in between saying so: a firmware
// that grows 300 bytes a week is 10 kB over in a year, and every one of those
// builds passed. So a project may declare a growth limit as well as a capacity,
// and the growth limit is the one that catches a slow problem.
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
	"sort"
	"strings"
)

// Budget is what one region is allowed to use.
type Budget struct {
	// Name is the region, as the linker script names it.
	Name string
	// MaxBytes is the capacity. Zero means the project declared none, and the
	// region is reported without being checked — a capacity invented from the
	// linker script is the number that was true before the datasheet was read.
	MaxBytes uint64
	// WarnAt is the percentage of the capacity at which the region is reported as
	// worth attention while still inside its budget.
	WarnAt float64
	// WarnAtDeclared says whether the project wrote a threshold, so that the
	// default and an explicit one can be told apart in the report.
	WarnAtDeclared bool
}

// Enforcement is what a check is allowed to fail on.
type Enforcement struct {
	// FailOnOverflow fails a region that is over its capacity.
	FailOnOverflow bool
	// FailOnGrowthBytes fails a region that grew by more than this against the
	// baseline.
	FailOnGrowthBytes uint64
	// FailOnGrowthDeclared says whether the project wrote a growth limit.
	FailOnGrowthDeclared bool
}

// Verdict is one region's outcome.
type Verdict struct {
	Name  string
	Usage Usage
	// Percent is the used fraction of the capacity, or -1 when there is none.
	Percent float64
	// State is `ok`, `warn`, `over` or `unbudgeted`.
	State string
	// Note is why, in a sentence, for anything that is not `ok`.
	Note string
}

// The states a region can be in.
//
// Four rather than two, because "inside its budget" and "inside its budget and
// fine" are different facts and the second is the one worth acting on early: a
// region at 95% is a region whose next feature does not fit, and nothing about
// that shows up until a build fails.
const (
	// StateOK is inside the budget and below the threshold.
	StateOK = "ok"
	// StateWarn is inside the budget and above the threshold.
	StateWarn = "warn"
	// StateOver is outside the budget.
	StateOver = "over"
	// StateUnbudgeted is a region the project declared no budget for.
	StateUnbudgeted = "unbudgeted"
)

// Check is the outcome of a whole run.
type Check struct {
	// Verdicts is one per region, in the order the report prints.
	Verdicts []Verdict
	// Growth is per-region change against the baseline, where there is one.
	Growth []Growth
	// Failed is what failed: a capacity exceeded, or a growth limit passed.
	Failed []string
	// OverBudget is whether any region is over its capacity.
	OverBudget bool
	// OverGrowth is whether any region grew past its limit.
	OverGrowth bool
	// HasBaseline says whether growth could be compared at all.
	HasBaseline bool
}

// Growth is one region's change against a baseline.
type Growth struct {
	Name string
	// Delta is the signed change in bytes. Negative is a smaller image, which is
	// reported rather than hidden: a pull request that gives back 8 kB is worth
	// knowing about for the same reason one that takes 8 kB is.
	Delta int64
	// Percent is the change against the baseline's usage, or -1 when the baseline
	// was zero and the change has no percentage.
	Percent float64
	// Limit is the growth limit that applied, or 0 when none did.
	Limit uint64
	// OverLimit is whether this region's growth is past its limit.
	OverLimit bool
}

// Evaluate applies a project's budgets to an analysis.
//
// A region the project declared no budget for is reported and not failed: a
// project that has budgets for flash and not for the backup SRAM has decided that
// the backup SRAM is not a thing to be worried about, and a check that failed it
// anyway would be failing a build over a decision somebody made deliberately.
func Evaluate(analysis *Analysis, budgets map[string]Budget, enforce Enforcement, baseline *Baseline) Check {
	out := Check{HasBaseline: baseline != nil}

	for _, usage := range analysis.Usage {
		budget, hasBudget := budgetFor(budgets, usage.Name)
		verdict := Verdict{Name: usage.Name, Usage: usage, Percent: -1, State: StateUnbudgeted}

		if !hasBudget || budget.MaxBytes == 0 {
			verdict.Note = "no budget declared"
			if usage.Ambiguous {
				verdict.Note += "; this region's addresses are also covered by another"
			}
			out.Verdicts = append(out.Verdicts, verdict)
			continue
		}

		verdict.Percent = float64(usage.Used) / float64(budget.MaxBytes) * 100
		threshold := budget.WarnAt
		if !budget.WarnAtDeclared || threshold <= 0 {
			threshold = DefaultWarnPercentValue
		}

		switch {
		case usage.Used > budget.MaxBytes:
			verdict.State = StateOver
			verdict.Note = fmt.Sprintf("%s over by %s",
				humanBytes(usage.Used-budget.MaxBytes), humanBytes(budget.MaxBytes))
			out.OverBudget = true
			out.Failed = append(out.Failed, fmt.Sprintf("%s is over its budget: %s of %s",
				usage.Name, humanBytes(usage.Used), humanBytes(budget.MaxBytes)))
			if enforce.FailOnOverflow {
				verdict.Note += " (failing: enforce.fail_on_overflow)"
			}
		case verdict.Percent >= threshold:
			verdict.State = StateWarn
			verdict.Note = fmt.Sprintf("%.0f%% of the way in, at the %s threshold",
				verdict.Percent, humanPercent(threshold))
		default:
			verdict.State = StateOK
			verdict.Note = fmt.Sprintf("%s to spare", humanBytes(budget.MaxBytes-usage.Used))
		}
		if usage.Ambiguous {
			verdict.Note += "; shares addresses with another region"
		}
		out.Verdicts = append(out.Verdicts, verdict)
	}

	// A region the project budgeted but the build did not use is worth saying: a
	// budget for a region nothing is placed in is either a typo in the region name
	// or a feature that was switched off, and both are silent otherwise.
	for name, budget := range budgets {
		if budget.MaxBytes == 0 {
			continue
		}
		if _, present := analysis.Region(name); present {
			continue
		}
		out.Verdicts = append(out.Verdicts, Verdict{
			Name:    name,
			Percent: 0,
			State:   StateUnbudgeted,
			Note:    fmt.Sprintf("budgeted at %s, but the build placed nothing in a region of that name", humanBytes(budget.MaxBytes)),
		})
	}

	if baseline != nil {
		out.Growth = growthAgainst(analysis, baseline, enforce)
		for _, one := range out.Growth {
			if one.OverLimit {
				out.OverGrowth = true
				out.Failed = append(out.Failed, fmt.Sprintf("%s grew by %s, over the %s limit",
					one.Name, humanBytes(uint64(abs64(one.Delta))), humanBytes(one.Limit)))
			}
		}
	}
	return out
}

// budgetFor is a project's budget for a region, matched without regard to case.
//
// Without regard to case because the two sides of this comparison are written by
// different hands: the linker script says `SRAM1` and a manifest may say `sram1`,
// and a budget that silently did not apply because of a capital letter would be
// reported as no budget at all — which is the one answer a reader cannot tell from
// a budget nobody wrote.
func budgetFor(budgets map[string]Budget, name string) (Budget, bool) {
	if budget, ok := budgets[name]; ok {
		return budget, true
	}
	for candidate, budget := range budgets {
		if strings.EqualFold(candidate, name) {
			return budget, true
		}
	}
	return Budget{}, false
}

// growthAgainst is the per-region change since a baseline.
func growthAgainst(analysis *Analysis, baseline *Baseline, enforce Enforcement) []Growth {
	out := []Growth{}
	for _, usage := range analysis.Usage {
		before, had := baseline.Region(usage.Name)
		if !had {
			continue
		}
		delta := int64(usage.Used) - int64(before.Used)
		growth := Growth{Name: usage.Name, Delta: delta, Percent: -1}
		if before.Used > 0 {
			growth.Percent = float64(delta) / float64(before.Used) * 100
		}
		if enforce.FailOnGrowthDeclared {
			growth.Limit = enforce.FailOnGrowthBytes
			growth.OverLimit = delta > int64(enforce.FailOnGrowthBytes)
		}
		out = append(out, growth)
	}
	// Largest growth first, which is the one a reader is looking for.
	sort.Slice(out, func(i, j int) bool { return out[i].Delta > out[j].Delta })
	return out
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

// WithoutWarnings is a set of budgets with the thresholds dropped, which is what
// `--no-warn` asks for.
//
// The capacities are kept: turning off the threshold is a request to stop being
// told a region is nearly full, not a request to stop checking whether it fits.
func WithoutWarnings(budgets map[string]Budget) map[string]Budget {
	out := map[string]Budget{}
	for name, budget := range budgets {
		budget.WarnAt = 0
		budget.WarnAtDeclared = false
		out[name] = budget
	}
	return out
}
