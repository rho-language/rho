//
// @file plan.go
// @brief Turning what a project declared into what a run will do.
//
// Between the manifest and the run there is a gap: a manifest states that a
// suite exists, and a run needs a suite it can execute. This is what closes it —
// selection, expansion, and resolution.
//
//   - selection answers "which suites", by name, by kind or by tag
//   - expansion answers "one per cell", for a suite declared over a matrix
//   - resolution answers "how", by handing a suite of a kind this runner does
//     not know to whichever plugin does
//
// The three are separate because they fail differently. A selection that named
// nothing is a filter that matched nothing, which is worth saying. An expansion
// that produced a suite nobody can run is a project with a test it has no runner
// for, which is an error. A resolution that produced a suite with no steps is a
// plugin that answered and said nothing, which is also an error — a plugin that
// silently returns nothing would be indistinguishable from a project with no
// tests.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhotest

import (
	"fmt"
	"sort"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  What to run                                                               */
/* -------------------------------------------------------------------------- */

// Selection is what a run was asked for.
//
// Empty means everything, so `rho test` with no arguments runs the project as it
// declares itself. That is the right default: a project's manifest is a
// deliberate statement about what its tests are, and a run that quietly ran a
// subset of it would be a run that quietly did less than it appeared to.
type Selection struct {
	// Names are suite names to run. A name matches a declared suite, or one cell
	// of its matrix, by the expanded name.
	Names []string
	// Kinds are suite kinds to run, such as "unit" or "smoke".
	Kinds []string
	// Tags are tags a suite must carry. A suite carrying any of them is run.
	Tags []string
	// Excluded are tags a suite must not carry, applied after Tags.
	Excluded []string
}

// Empty reports whether the selection asked for nothing in particular.
func (s Selection) Empty() bool {
	return len(s.Names) == 0 && len(s.Kinds) == 0 && len(s.Tags) == 0 && len(s.Excluded) == 0
}

// Describes renders the selection for a report line, so a run says what it was
// asked for rather than only what it did.
func (s Selection) Describes() string {
	parts := []string{}
	if len(s.Names) > 0 {
		parts = append(parts, "names="+strings.Join(s.Names, ","))
	}
	if len(s.Kinds) > 0 {
		parts = append(parts, "kind="+strings.Join(s.Kinds, ","))
	}
	if len(s.Tags) > 0 {
		parts = append(parts, "tag="+strings.Join(s.Tags, ","))
	}
	if len(s.Excluded) > 0 {
		parts = append(parts, "without="+strings.Join(s.Excluded, ","))
	}
	if len(parts) == 0 {
		return "everything"
	}
	return strings.Join(parts, " ")
}

/* -------------------------------------------------------------------------- */
/*  Matching                                                                  */
/* -------------------------------------------------------------------------- */

// Matches reports whether a declared suite is in the selection.
//
// Names are matched against the base name and every expanded cell, so
// `--suite hello_world` runs the whole matrix rather than nothing — which is what
// somebody naming an axis of a matrix means.
func (s Selection) Matches(suite Declared) bool {
	if len(s.Names) > 0 && !s.matchesName(suite) {
		return false
	}
	if len(s.Kinds) > 0 && !contains(s.Kinds, suite.Kind) {
		return false
	}
	if len(s.Tags) > 0 && !carriesAny(suite.Tags, s.Tags) {
		return false
	}
	for _, excluded := range s.Excluded {
		if contains(suite.Tags, excluded) {
			return false
		}
	}
	return true
}

func (s Selection) matchesName(suite Declared) bool {
	for _, name := range s.Names {
		if name == suite.Name {
			return true
		}
		if suite.Matrix != nil {
			for _, cell := range suite.Matrix.Expand(suite.Name) {
				if name == cell {
					return true
				}
			}
		}
	}
	return false
}

func contains(haystack []string, needle string) bool {
	for _, held := range haystack {
		if held == needle {
			return true
		}
	}
	return false
}

func carriesAny(held, wanted []string) bool {
	for _, want := range wanted {
		if contains(held, want) {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  Expanding                                                                 */
/* -------------------------------------------------------------------------- */

// Expand turns the manifest into one entry per suite a run will consider.
//
// A declared suite without a matrix is one entry. A declared suite with a matrix
// is one entry per cell, named after the axes it was expanded from, so a report
// says which board and which example rather than which index.
func Expand(manifest Manifest, selection Selection) []Declared {
	out := []Declared{}
	for _, suite := range manifest.Declared {
		if !selection.Matches(suite) {
			continue
		}
		if suite.Matrix == nil {
			expanded := suite
			out = append(out, expanded)
			continue
		}
		for _, cell := range suite.Matrix.Expand(suite.Name) {
			// Every axis is carried, not only the one that happened to be declared
			// last. A cell carries all of what it was expanded from: a runner that
			// could only see one axis would be able to name the directory to build and
			// not the board to build it for, which is the half that decides whether the
			// build can happen at all.
			axes := make([]string, 0, len(suite.Matrix.Axes))
			values := map[string][]string{}
			for _, axis := range suite.Matrix.Axes {
				axes = append(axes, axis)
				values[axis] = []string{AxisValue(cell, axis)}
			}
			out = append(out, Declared{
				Name:    cell,
				Kind:    suite.Kind,
				Tags:    suite.Tags,
				Skip:    suite.Skip,
				Options: suite.Options,
				Matrix:  &Matrix{Axes: axes, Values: values},
			})
		}
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Resolving                                                                 */
/* -------------------------------------------------------------------------- */

// Resolver turns a declared suite into a runnable one.
//
// It is an interface rather than a function so the built-in kinds and the plugins
// are the same kind of thing to the plan: a kind is resolved by whatever claims
// it, and the runner never needs to know which that was.
type Resolver interface {
	// Resolves reports whether this resolver knows a kind.
	Resolves(kind string) bool
	// Resolve returns the runnable suite, or a reason it could not be made.
	Resolve(declared Declared, root string) (Suite, error)
}

// ResolverFunc adapts a function to the interface, so a kind the runner knows
// itself is registered in one line rather than as a type.
type ResolverFunc struct {
	Kind string
	Run  func(declared Declared, root string) (Suite, error)
}

// Resolves reports whether this resolver was registered for a kind.
func (f ResolverFunc) Resolves(kind string) bool { return f.Kind == kind }

// Resolve runs the registered function.
func (f ResolverFunc) Resolve(declared Declared, root string) (Suite, error) {
	return f.Run(declared, root)
}

// ResolveAll turns declarations into runnable suites.
//
// A suite nobody can resolve is reported as skipped with the reason, rather than
// dropped: a manifest that declares a test the installation has no runner for is
// a fact about the run worth stating, and dropping it would let a run report
// success while a declared test went unrun.
func ResolveAll(declared []Declared, root string, resolvers ...Resolver) ([]Suite, []error) {
	suites := make([]Suite, 0, len(declared))
	problems := []error{}
	for _, entry := range declared {
		// A suite the manifest declared as not run stays as it is: whatever would
		// have resolved it is not consulted, because the answer was given already.
		if entry.Skip != "" {
			suites = append(suites, Suite{
				Name: entry.Name,
				Kind: entry.Kind,
				Tags: entry.Tags,
				Root: root,
				Skip: entry.Skip,
			})
			continue
		}
		resolver := resolverFor(entry.Kind, resolvers)
		if resolver == nil {
			suites = append(suites, Suite{
				Name: entry.Name,
				Kind: entry.Kind,
				Tags: entry.Tags,
				Root: root,
				Skip: fmt.Sprintf("no runner is installed for %s suites", entry.Kind),
			})
			problems = append(problems, fmt.Errorf("suite %q is a %s suite and no runner is installed for it", entry.Name, entry.Kind))
			continue
		}
		suite, err := resolver.Resolve(entry, root)
		if err != nil {
			suites = append(suites, Suite{
				Name: entry.Name,
				Kind: entry.Kind,
				Tags: entry.Tags,
				Root: root,
				Skip: err.Error(),
			})
			problems = append(problems, fmt.Errorf("suite %q: %w", entry.Name, err))
			continue
		}
		if suite.Name == "" {
			suite.Name = entry.Name
		}
		if suite.Kind == "" {
			suite.Kind = entry.Kind
		}
		if len(suite.Tags) == 0 {
			suite.Tags = entry.Tags
		}
		if suite.Root == "" {
			suite.Root = root
		}
		if len(suite.Steps) == 0 {
			suites = append(suites, Suite{
				Name: suite.Name, Kind: suite.Kind, Tags: suite.Tags, Root: suite.Root,
				Skip: "its runner produced no steps to run",
			})
			problems = append(problems, fmt.Errorf("suite %q was resolved to a suite with no steps", suite.Name))
			continue
		}
		suites = append(suites, suite)
	}
	return suites, problems
}

func resolverFor(kind string, resolvers []Resolver) Resolver {
	for _, resolver := range resolvers {
		if resolver.Resolves(kind) {
			return resolver
		}
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  Describing what a run will do                                              */
/* -------------------------------------------------------------------------- */

// DescribePlan renders the suites a run will consider, one per line, which is
// what a dry run prints: what the manifest declared, what the selection kept, and
// what each will do — without doing any of it.
func DescribePlan(manifest Manifest, selection Selection) []string {
	lines := []string{}
	lines = append(lines, fmt.Sprintf("project:  %s", nameOr(manifest.Name, "(unnamed)")))
	lines = append(lines, fmt.Sprintf("manifest: %s", sourceOr(manifest.Source, "(none)")))
	lines = append(lines, fmt.Sprintf("selected: %s", selection.Describes()))
	if len(manifest.Declared) == 0 {
		return append(lines, "no suites declared")
	}
	for _, suite := range Expand(manifest, selection) {
		labels := []string{}
		if len(suite.Tags) > 0 {
			labels = append(labels, strings.Join(SortedTags(suite.Tags), ","))
		}
		if suite.Skip != "" {
			lines = append(lines, fmt.Sprintf("  skip  %s  [%s]  %s", suite.Name, suite.Kind, suite.Skip))
			continue
		}
		if len(labels) == 0 {
			lines = append(lines, fmt.Sprintf("  run   %s  [%s]", suite.Name, suite.Kind))
			continue
		}
		lines = append(lines, fmt.Sprintf("  run   %s  [%s]  {%s}", suite.Name, suite.Kind, strings.Join(labels, ",")))
	}
	return lines
}

// SortedSuites returns suites in a stable order by name, so two runs of the same
// manifest report in the same order and a diff between two reports is about what
// changed rather than about iteration order.
func SortedSuites(suites []Suite) []Suite {
	out := append([]Suite{}, suites...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func nameOr(name, fallback string) string {
	if name == "" {
		return fallback
	}
	return name
}

func sourceOr(source, fallback string) string {
	if source == "" {
		return fallback
	}
	return source
}
