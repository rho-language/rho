//
// @file engines.go
// @brief Which debugger a session runs on, and where the answer comes from.
//
// Three answers, in this order:
//
//   1. the launch configuration, when it names one — a person who wrote
//      `engine: lldb` meant it
//   2. a plugin that offers an engine for this board — the extension point, and
//      the only one that adds a debugger to this component without changing it
//   3. the GDB engine, which is here and needs nothing enabled
//
// The order matters in the last direction more than the first: a plugin that
// offers an engine for a board must beat the built-in one for that board, or
// enabling it would change nothing and there would be no way to tell whether that
// was a bug or a plugin that was not doing what its manifest said.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"rho/src/plugins"
)

// The engine this component ships with, and the name it is chosen by.
const (
	BuiltinGDB   = "gdb"
	builtinLabel = "GDB/MI"
)

// Engines are the debuggers available to a session, and where each came from.
type Engines struct {
	// Builtin is the GDB engine, which needs no plugin.
	Builtin bool
	// FromPlugins are the engines the enabled plugins offer.
	FromPlugins []EngineSpec
	// Sources are the plugins that were asked, for a diagnostic.
	Sources []string
}

// Discover is the debuggers a machine has, from the plugins that are enabled.
//
// A plugin that fails is reported and left out: the debuggers this component
// carries are still there, which is what makes one broken extension cost a person
// a board rather than a session.
func Discover(source plugins.Source) Engines {
	answer := Engines{Builtin: true}
	found := source.Discover(ComponentName)
	if len(found) == 0 {
		return answer
	}
	for _, plugin := range found {
		if !plugin.Answers(HookEngine) {
			continue
		}
		answer.Sources = append(answer.Sources, plugin.Name)
		result := plugins.RunHooks([]plugins.Plugin{plugin}, ComponentName, HookEngine,
			EngineRequest{Target: "", Builtin: []string{BuiltinGDB}})
		for _, payload := range result.Payload {
			specs, err := pluginEngines(payload)
			if err != nil {
				continue
			}
			for _, spec := range specs {
				if err := spec.Validate(); err != nil {
					continue
				}
				answer.FromPlugins = append(answer.FromPlugins, spec)
			}
		}
	}
	sort.Slice(answer.FromPlugins, func(i, j int) bool {
		return answer.FromPlugins[i].Name < answer.FromPlugins[j].Name
	})
	return answer
}

// FromPlugins2Names are the plugin engines by name, for a listing that does not
// care where they came from.
func (e Engines) FromPlugins2Names() []string {
	out := make([]string, 0, len(e.FromPlugins))
	for _, spec := range e.FromPlugins {
		out = append(out, spec.Name)
	}
	return out
}

// Names are the engines available, as a listing shows them.
func (e Engines) Names() []string {
	out := []string{BuiltinGDB}
	for _, spec := range e.FromPlugins {
		out = append(out, spec.Name)
	}
	return out
}

// Choose is the engine for a session.
//
// `wanted` is what the launch configuration named, if anything. An engine named
// that is not there is reported rather than ignored: a person who wrote `engine:
// lldb` and got a GDB session would spend a long time wondering why the feature
// they expected was missing.
func (e Engines) Choose(target string, wanted string) (EngineSpec, error) {
	if wanted != "" {
		if wanted == BuiltinGDB {
			return EngineSpec{Name: BuiltinGDB, Kind: KindDriver, Comment: builtinLabel}, nil
		}
		for _, spec := range e.FromPlugins {
			if spec.Name == wanted {
				return spec, nil
			}
		}
		return EngineSpec{}, fmt.Errorf("the session asks for the engine %q, and this machine has %s",
			wanted, strings.Join(e.Names(), ", "))
	}

	// A plugin that offers an engine for this board wins. A fallback does not,
	// because it would then beat the built-in one for every board, and enabling a
	// plugin would silently change what every session does.
	best := EngineSpec{}
	for _, spec := range e.FromPlugins {
		if !spec.Serves(target) {
			continue
		}
		if best.Name == "" {
			best = spec
			continue
		}
		// Two engines for one board is a decision, not an accident, and it is
		// resolved here rather than left to the order the plugins happened to load
		// in: by name, so that the same two plugins always pick the same one.
		if spec.Name < best.Name {
			best = spec
		}
	}
	if best.Name != "" {
		return best, nil
	}
	return EngineSpec{Name: BuiltinGDB, Kind: KindDriver, Comment: builtinLabel}, nil
}

/* -------------------------------------------------------------------------- */
/*  The register map from a plugin                                             */
/* -------------------------------------------------------------------------- */

// RegisterMapFromPlugins asks the plugins where a chip's registers are described.
//
// Asked of plugins rather than only read from a file, because where a vendor keeps
// its register map is a fact about the vendor that this component cannot hold: a
// project that vendors one has it, and a project whose HAL knows where to fetch it
// does not vendor it. Both are right, and a plugin is how the second is said.
func RegisterMapFromPlugins(source plugins.Source, target string, declared string) (string, string) {
	found := source.Discover(ComponentName)
	if len(found) == 0 {
		return "", ""
	}
	var (
		chosen  string
		comment string
	)
	for _, plugin := range found {
		if !plugin.Answers(HookRegisterMap) {
			continue
		}
		result := plugins.RunHooks([]plugins.Plugin{plugin}, ComponentName, HookRegisterMap,
			RegisterMapRequest{Target: target, SVD: declared})
		for _, payload := range result.Payload {
			answer := struct {
				SVD     string `json:"svd"`
				Comment string `json:"comment"`
			}{}
			if err := json.Unmarshal(payload, &answer); err != nil {
				continue
			}
			if strings.TrimSpace(answer.SVD) == "" || chosen != "" {
				continue
			}
			chosen = answer.SVD
			comment = answer.Comment
		}
	}
	return chosen, comment
}
