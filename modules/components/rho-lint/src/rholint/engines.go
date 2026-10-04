//
// @file engines.go
// @brief Which analyzers a machine has, and which of them a run uses.
//
// Three answers, in this order:
//
//  1. the command line, when it names one — a person who wrote
//     `--engine clang-tidy` meant it
//  2. the two engines this component carries, when they read what the project has
//  3. the engines the enabled plugins offer
//
// Which is not rho-dbg's order, and the difference is deliberate. A debugging
// session is driven by exactly one debugger, so rho-dbg's `engine` contract is
// `one: true` and the built-in loses to any plugin that names the board. A lint run
// is *several* analyzers over one database — that is the whole reason the output
// needs normalizing — so every enabled plugin joins the run and the built-ins are
// simply the two that are always there.
//
// What is shared is the discipline at the edges. A plugin that fails is reported and
// left out, so one broken extension costs a project an analyzer rather than the run.
// A spec that does not validate is reported and left out, so a typo in a manifest is
// a diagnostic rather than a crash. And the order everything is reported in is fixed
// rather than depending on the order plugins happened to load in, so two runs of the
// same tree on two machines produce the same report.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rholint

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
)

// Engines is the analyzers available to a run, and where each came from.
type Engines struct {
	// Builtin are the two engines this component carries.
	Builtin []Spec
	// FromPlugins are the engines the enabled plugins offer.
	FromPlugins []Spec
	// Sources are the plugins that were asked, for a diagnostic.
	Sources []string
	// Problems are the specs that were offered and could not be used, said rather
	// than dropped: a plugin whose analyzer is not installed is a fact about the
	// machine, and a run that silently omitted it would be a run whose silence
	// looked like a clean tree.
	Problems []string
}

// PluginSource is where this component looks for the modules that extend it.
//
// The Rho home and the project's own tree together, because a plugin is either
// installed in the registry or is a module in the project being linted — and a
// project whose own `modules/plugins` declares an analyzer should get it without
// publishing anything. This is the same source every other component builds, so a
// plugin is found by the same rules here as by `rho module search`.
func PluginSource(root string) plugins.Source {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if absolute, err := filepath.Abs(root); err == nil {
		root = absolute
	}
	return plugins.Source{Tree: root, RhoHome: deps.RhoHome()}
}

// Discover is the analyzers a machine has, from the plugins that are enabled.
//
// The same rule as everywhere else in this toolchain: an extension that fails is
// reported and left out, because the analyzers this component carries are still
// there and one broken plugin must not cost a project the whole run.
func Discover(source plugins.Source) Engines {
	answer := Engines{Builtin: Builtin()}

	found := source.Discover(ComponentName)
	for _, plugin := range found {
		if !plugin.Answers(HookEngine) {
			continue
		}
		answer.Sources = append(answer.Sources, plugin.Name)
		result := plugins.RunHooks([]plugins.Plugin{plugin}, ComponentName, HookEngine,
			EngineRequest{Target: "", Builtin: answer.Names()})
		for _, payload := range result.Payload {
			specs, err := pluginSpecs(payload)
			if err != nil {
				answer.Problems = append(answer.Problems, fmt.Sprintf(
					"plugin %s answered with something that is not a list of engines: %v", plugin.Name, err))
				continue
			}
			for _, spec := range specs {
				spec.Root = plugin.Root
				if err := spec.Validate(); err != nil {
					answer.Problems = append(answer.Problems, err.Error())
					continue
				}
				answer.FromPlugins = append(answer.FromPlugins, spec)
			}
		}
		for _, diagnostic := range result.Diagnostics {
			answer.Problems = append(answer.Problems, diagnostic.String())
		}
	}
	sort.Slice(answer.FromPlugins, func(i, j int) bool {
		return answer.FromPlugins[i].Name < answer.FromPlugins[j].Name
	})
	sort.Strings(answer.Problems)
	return answer
}

// Names are every engine available, in the order a report shows them.
//
// The two carried engines first and then the plugins by name. Fixed rather than
// sorted, because `rho` before `c` means something: a finding in the source a person
// wrote should come before the same finding in the translation of it.
func (e Engines) Names() []string {
	out := make([]string, 0, len(e.Builtin)+len(e.FromPlugins))
	for _, spec := range e.Builtin {
		out = append(out, spec.Name)
	}
	for _, spec := range e.FromPlugins {
		out = append(out, spec.Name)
	}
	return out
}

// Spec is the engine a name chooses.
//
// An engine named that is not there is reported rather than ignored. A person who
// wrote `--engine cppcheck` and got a run with clang-tidy in it would spend a long
// time working out why the analyzer they asked for said nothing — and the reason
// would be that this command quietly substituted a different one, which is the
// behaviour this refuses.
func (e Engines) Spec(name string, board string) (Spec, error) {
	for _, spec := range e.Builtin {
		if spec.Name == name {
			return spec, nil
		}
	}
	for _, spec := range e.FromPlugins {
		if spec.Name == name && spec.Serves(board) {
			return spec, nil
		}
	}
	return Spec{}, fmt.Errorf("this run asks for the engine %q, and this machine has %s",
		name, describeOrNone(e.Names()))
}

// Has reports whether an engine is available, for a caller that is not asking for
// it and only needs to know.
func (e Engines) Has(name string) bool {
	for _, known := range e.Names() {
		if known == name {
			return true
		}
	}
	return false
}

// Choose are the engines a run uses, and the names it could not find.
//
// `wanted` is what the command line named and `configured` is what the settings say,
// and an empty `wanted` means the settings decide. An empty `configured` means every
// engine available, which is what a machine with one project on it wants and what an
// empty `default_engines` means.
//
// The three cases are kept apart rather than merged, because they answer different
// questions and merging them is how "I turned an engine off and it still ran"
// happens: a name on the command line is a decision, the settings are a
// configuration, and the absence of both is a machine with nothing to say.
//
// A name that is not here is *not* an error as long as another one is. This is the
// difference that decides whether a machine part-way through being set up gets any
// analysis at all: the shipped default names `clang-tidy` and `cppcheck`, neither of
// which is an engine until a plugin provides one, and a run that refused on the
// missing one would find nothing on every machine that has not installed them. So
// they come back as the second value, named in the run's output — and only a request
// that resolved to nothing at all is refused, because that is a person who asked for
// an analyzer and there is none.
func (e Engines) Choose(wanted []string, configured []string, board string) ([]Spec, []string, error) {
	names := wanted
	if len(names) == 0 {
		names = configured
	}
	if len(names) == 0 {
		specs := make([]Spec, 0, len(e.Builtin)+len(e.FromPlugins))
		specs = append(specs, e.Builtin...)
		for _, spec := range e.FromPlugins {
			if spec.Serves(board) {
				specs = append(specs, spec)
			}
		}
		return specs, nil, nil
	}

	specs := make([]Spec, 0, len(names))
	missing := []string{}
	for _, name := range names {
		spec, err := e.Spec(trimEngineName(name), board)
		if err != nil {
			missing = append(missing, strings.TrimSpace(name))
			continue
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil, missing, fmt.Errorf("this run asks for %s, and this machine has %s",
			strings.Join(missing, ", "), describeOrNone(e.Names()))
	}
	return specs, missing, nil
}

// trimEngineName is an engine name as a person writes it.
//
// `clang-tidy`, `clang_tidy` and `clang-tidy@1` all reach the same engine, because a
// settings file and a command line are written by hand and a rule about the spelling
// is a rule that will be broken once and be reported as a missing engine.
func trimEngineName(name string) string {
	trimmed := strings.TrimSpace(name)
	if version := strings.LastIndex(trimmed, "@"); version > 0 {
		trimmed = trimmed[:version]
	}
	return strings.ReplaceAll(trimmed, "_", "-")
}

func describeOrNone(names []string) string {
	if len(names) == 0 {
		return "none installed or provided"
	}
	return strings.Join(names, ", ")
}

// Reporters are the renderers a machine has: the two this component carries, and
// whatever the enabled plugins add.
//
// Discovered through the same mechanism as the engines and for the same reason —
// the set of formats a run can produce is a fact about the machine — but not
// filtered by anything: a reporter does not read sources, so there is no language it
// could be the wrong one for.
func Reporters(source plugins.Source) (built []ReportSpec, provided []ReportSpec, sources []string) {
	built = []ReportSpec{
		{Name: KindText, Comment: "one line per finding, in the spelling every toolchain prints"},
		{Name: KindSARIF, Comment: "SARIF 2.1.0, for a code scanning service or an editor"},
	}
	found := source.Discover(ComponentName)
	for _, plugin := range found {
		if !plugin.Answers(HookReport) {
			continue
		}
		sources = append(sources, plugin.Name)
		result := plugins.RunHooks([]plugins.Plugin{plugin}, ComponentName, HookReport,
			ReportRequest{Format: ""})
		for _, payload := range result.Payload {
			answer := struct {
				Reports []ReportSpec `json:"reports"`
			}{}
			if err := json.Unmarshal(payload, &answer); err != nil {
				continue
			}
			for _, one := range answer.Reports {
				if strings.TrimSpace(one.Name) == "" {
					continue
				}
				provided = append(provided, one)
			}
		}
	}
	sort.Slice(provided, func(i, j int) bool { return provided[i].Name < provided[j].Name })
	return built, provided, sources
}

// Formats are every format `--format` accepts, which is what a run has to choose
// between before it starts.
func FormatsAvailable(source plugins.Source) []string {
	built, provided, _ := Reporters(source)
	out := make([]string, 0, len(built)+len(provided))
	for _, one := range built {
		out = append(out, one.Name)
	}
	for _, one := range provided {
		out = append(out, one.Name)
	}
	return out
}

// EngineRequest is what a plugin is asked: which analyzers this machine already has.
//
// `Builtin` is sent rather than the plugin having to know it, for the same reason
// rho-dbg sends its own: a plugin that offers the analyzer this component already
// carries has done nothing, and it can tell that from the request instead of from
// having read this component's manifest.
type EngineRequest struct {
	// Target is the board, as its spec names it. Empty when the run is on a project
	// that targets this machine.
	Target string `json:"target"`
	// Languages are the source languages this machine's sources are in, which is
	// what a plugin offering a C++ analyzer needs to know before it offers it.
	Languages []string `json:"languages,omitempty"`
	// Builtin are the engines this component already has.
	Builtin []string `json:"builtin,omitempty"`
}
