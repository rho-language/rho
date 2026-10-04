//
// @file builtin.go
// @brief The two engines this component carries.
//
// Everything else is a plugin, because everything else is a program that already
// exists. These two are here for one reason: so that `rho lint` says something on a
// machine with nothing installed rather than only reporting that nothing is.
//
// `rho` reads the Rho sources through the compiler's own parser — the same parser
// a build uses, so a file it rejects is a file the build rejects, and the two
// cannot disagree about what is a declaration. It is the only engine anywhere in
// this run that can say anything about a line that is not C.
//
// `c` reads the C the build actually compiles, which is the same set of files
// whether a person wrote them in C or the generator translated them from the Rho
// beside it. That is what makes it cover both kinds of project: the language of a
// source and the language a project was written in are different questions, and
// only the first decides which rules apply.
//
// What these rules are is a deliberate narrowness. They were chosen because each is
// decidable from what is in front of the reader without a compiler front end — so
// they cannot be wrong in the way a half-implemented type inference is, and every
// one of them has a test that names the C or the Rho it fires on. The checks that
// need a front end are clang-tidy's and cppcheck's job, which is what the plugins
// are for.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import "strings"

// The names the two engines carried here are chosen by.
const (
	// EngineRho is the engine over the Rho sources.
	EngineRho = "rho"
	// EngineC is the engine over the C a build compiles.
	EngineC = "c"
)

// Builtin are the engines this component carries, in the order a report shows them.
//
// A fixed order rather than sorted by name, because `rho` before `c` is the
// meaningful one: a finding in the source a person wrote comes before the same
// finding in the translation of it, and the run that reports both should have the
// real one first.
func Builtin() []Spec {
	return []Spec{
		{
			Name:      EngineRho,
			Kind:      KindDriver,
			Languages: []string{LanguageRho},
			Comment:   "the Rho sources, through the compiler's own parser",
		},
		{
			Name:      EngineC,
			Kind:      KindDriver,
			Languages: []string{LanguageC},
			Comment:   "the C this build compiles, whichever language it was written in",
		},
	}
}

// BuiltinSpec is one of the engines carried here, by name.
//
// Not every name is one: a run that names an engine which does not exist has to be
// able to say that it does not exist rather than quietly getting nothing.
func BuiltinSpec(name string) (Spec, bool) {
	for _, spec := range Builtin() {
		if spec.Name == name {
			return spec, true
		}
	}
	return Spec{}, false
}

/* -------------------------------------------------------------------------- */
/*  The engine                                                                 */
/* -------------------------------------------------------------------------- */

// driver is one of the engines carried here: a rule set over one language.
type driver struct {
	spec  Spec
	rules RuleSet
}

// RuleSet is what an engine checks.
//
// A list of functions over a source rather than an interface with a method per
// rule, so that a rule is one function with a name, a severity and a sentence —
// which is all three things a report needs and all a rule should have to supply.
type RuleSet struct {
	// Name is the engine, which every rule is attributed to.
	Name string
	// Language is what the rules read.
	Language string
	// Rules are the checks.
	Rules []Check
}

// Check is one rule: what it is called, how bad it is, and how it decides.
type Check struct {
	// ID is the rule's name as a report and a suppression show it. Already
	// qualified with the engine's name, because these two engines share one
	// namespace and `unused-import` in one of them is not a rule in the other.
	ID string
	// Severity is what a match is reported at.
	Severity Severity
	// Description is what the rule is for, which is what a report shows when
	// somebody asks why it fired.
	Description string
	// Apply reads a source and reports what it finds.
	Apply func(source Source) []Diagnostic
}

// Source is one file as the rules read it.
type Source struct {
	// Path is the file, absolute.
	Path string
	// Root is the project directory, which findings are made relative to.
	Root string
	// Text is the file's contents.
	Text string
	// Generated are the names of the modules this project's build generates.
	//
	// Given to the rules rather than looked up by them, because a rule that reads the
	// build directory is a rule whose answer depends on whether the project has been
	// built — and a run before a build would then report differently from a run after
	// one, for the same source.
	Generated map[string]bool
}

// Find is the rules of one engine by name.
//
// A lookup rather than a filter because a caller has a name and wants that rule or
// nothing, and a filter over a slice would hand back a slice to check.
func (r RuleSet) Find(name string) (Check, bool) {
	for _, rule := range r.Rules {
		if rule.ID == name {
			return rule, true
		}
	}
	return Check{}, false
}

// Rule writes a finding for a check against a source.
//
// The three things every rule would otherwise repeat, and one of them is not
// boilerplate: the rule is attributed to its engine, so a finding in the report can
// be traced back to a rule and an engine rather than to a string somebody typed.
func (r RuleSet) Report(rule Check, source Source, line int, column int, message string) Diagnostic {
	return Diagnostic{
		Rule:     rule.ID,
		Severity: rule.Severity,
		Message:  message,
		File:     Relative(source.Path, source.Root),
		Line:     line,
		Column:   column,
		Engine:   r.Name,
	}
}

// Describe is what this engine is, as a report shows it.
func (d *driver) Describe() Description {
	return Description{
		Name:      d.spec.Name,
		Kind:      KindDriver,
		Languages: []string{d.rules.Language},
		Source:    SourceBuiltin,
		Comment:   d.spec.Comment,
		Rules:     d.rules.Describe(),
	}
}

// Run analyses what the request names.
//
// An engine that is handed no file of its language is not run at all, and answers
// with nothing rather than with a complaint: `rho lint` on a project written in C
// has no Rho sources and that is not a failure of the Rho engine, it is that there
// was nothing for it to do. Which engines were pointed at what is in the run's own
// output.
func (d *driver) Run(run Request) ([]Diagnostic, error) {
	files := FilesOf(run.Units, []string{d.rules.Language})
	if len(files) == 0 {
		return nil, nil
	}

	wanted := wantedRules(run.Rules, d.rules)
	if len(wanted) == 0 {
		return nil, nil
	}

	generated := map[string]bool{}
	for _, name := range run.Generated {
		generated[name] = true
	}

	found := []Diagnostic{}
	for _, path := range files {
		source, err := ReadSource(path, run.Root, generated)
		if err != nil {
			// One unreadable file is not a failed run. A vendor header that was
			// removed between the database being written and now would otherwise
			// cost a project the analysis of everything else, and the report would
			// carry one line where it should carry a page.
			found = append(found, Diagnostic{
				Rule:     d.rules.Name + "-unreadable",
				Severity: SeverityWarning,
				Message:  err.Error(),
				File:     Relative(path, run.Root),
				Engine:   d.rules.Name,
			})
			continue
		}
		for _, rule := range wanted {
			for _, one := range rule.Apply(source) {
				found = append(found, one)
			}
		}
	}
	return found, nil
}

// wantedRules are the checks to run: the ones named, or all of them.
//
// A name that is not a rule of this engine is refused by the caller rather than
// matched to nothing, because `rho lint --rule ununsed-import` finding nothing and
// saying nothing is the same outcome as the rule not existing and it must not be.
func wantedRules(names []string, set RuleSet) []Check {
	if len(names) == 0 {
		return set.Rules
	}
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	out := make([]Check, 0, len(set.Rules))
	for _, rule := range set.Rules {
		if wanted[rule.ID] {
			out = append(out, rule)
		}
	}
	return out
}

// Describe is what these rules are, for a SARIF file's rule list.
func (r RuleSet) Describe() []Rule {
	out := make([]Rule, 0, len(r.Rules))
	for _, rule := range r.Rules {
		out = append(out, Rule{
			ID:          rule.ID,
			Description: rule.Description,
			Severity:    rule.Severity,
		})
	}
	return out
}

// NewDriver is one of the engines carried here, by name.
func NewDriver(name string) (Engine, bool) {
	for _, spec := range Builtin() {
		if spec.Name != name {
			continue
		}
		switch name {
		case EngineRho:
			return &driver{spec: spec, rules: RhoRules()}, true
		case EngineC:
			return &driver{spec: spec, rules: CRules()}, true
		}
	}
	return nil, false
}

// RulesFor is the rule set of one engine, by name.
func RulesFor(name string) (RuleSet, bool) {
	switch strings.TrimSpace(name) {
	case EngineRho:
		return RhoRules(), true
	case EngineC:
		return CRules(), true
	default:
		return RuleSet{}, false
	}
}
