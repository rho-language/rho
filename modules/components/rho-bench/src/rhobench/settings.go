//
// @file settings.go
// @brief What this component is set to, read from its own manifest and its own file.
//
// The contract is declared in the component's manifest and the values are in the file
// that manifest names, so what a person reads to find out what can be configured is
// the same thing the component runs on.
//
// The order matters: the manifest's declaration is read first, because it is what a
// person reads to find out what the defaults are, and the file second, because a file
// is somebody's decision about this installation. A component installed without its
// configuration directory still runs, with the values the manifest calls the
// defaults.
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
	"time"

	"rho/src/deps"
	"rho/src/yaml"
)

// Settings is what a run may be configured with.
type Settings struct {
	Iterations int
	Warmup     int
	Width      int
	Timeout    time.Duration
}

// The defaults, which are also what the manifest declares.
var (
	// DefaultIterations is how many timed runs each benchmark makes.
	DefaultIterations = 1000
	// DefaultWarmup is how many are discarded first.
	DefaultWarmup = 10
	// DefaultWidth is how wide the benchmark name column is.
	DefaultWidth = 24
)

// ReadSettings is this component's own configuration, over a project's.
//
// The component's file first, because it is about this installation; the project's
// second, because it is about this firmware and therefore closer to the thing being
// measured. Where they disagree the project wins, which is the same precedence every
// other component uses.
func ReadSettings(projectRoot string) Config {
	out := ReadOwnSettings()

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

// ReadOwnSettings is this component's manifest and the file it names.
func ReadOwnSettings() Config {
	out := Config{Iterations: DefaultIterations, Warmup: DefaultWarmup, StackWatermark: true}
	root := ModuleRoot()
	if root == "" {
		return out
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Settings == nil {
		return out
	}
	for _, setting := range manifest.Settings.Settings {
		applyValue(&out, setting.Name, setting.Default)
	}
	if manifest.Settings.File == "" {
		return out
	}
	data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifest.Settings.File)))
	if readErr != nil {
		// A file that is not there is a component nobody has changed, which is not
		// a failure, and a run is never blocked by a configuration file.
		return out
	}
	document, parseErr := yaml.Load(string(data))
	if parseErr != nil {
		return out
	}
	for name, value := range document.Map {
		applyValue(&out, name, value.Str)
	}
	return out
}

// applyValue reads one setting by the name it is declared under.
func applyValue(out *Config, name, raw string) {
	raw = strings.TrimSpace(raw)
	switch name {
	case "iterations", "default_iterations":
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			out.Iterations = value
		}
	case "warmup_runs":
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			out.Warmup = value
		}
	}
}

// ModuleRoot is where this component's own files are.
func ModuleRoot() string {
	if root := strings.TrimSpace(os.Getenv("RHO_MODULE_ROOT")); root != "" {
		return root
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable)
	}
	return ""
}
