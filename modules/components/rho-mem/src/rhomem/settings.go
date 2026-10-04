//
// @file settings.go
// @brief What this component is set to, read from its own manifest and its own file.
//
// The contract is declared in the component's manifest and the values are in the
// file that manifest names, so what a person reads to find out what can be
// configured is the same thing the component runs on. Read when a command runs,
// not once when this was compiled, because a person changes the file and runs
// again.
//
// The values are the same reading rho-lint and its siblings do, because two
// components reading their configuration two ways is one of them reading it wrong.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rho/src/deps"
	"rho/src/yaml"
)

// Settings is what a run may be configured with.
type Settings struct {
	// Top is how many symbols the attribution lists when the command names none.
	Top int
	// WarnPercent is the share of a region at which it is reported as worth
	// attention.
	WarnPercent float64
	// Width is how wide the usage bars are drawn.
	Width int
}

// The defaults, which are also what the manifest declares.
//
// Written here as well as in the manifest because the file is optional: a
// component installed without its configuration directory should still run, and
// should run with the values the manifest calls the defaults rather than with a
// zero width and no symbols.
var (
	// DefaultTop is how many symbols are listed.
	DefaultTop = 15
	// DefaultWarnPercent is the share of a region at which it is reported.
	// The same constant the budget check uses, so a run that falls back to the
	// default and a check that falls back to it agree.
	DefaultWarnPercent = DefaultWarnPercentValue
	// DefaultWidth is the bar width in characters.
	DefaultWidth = 25
)

// ReadSettings is this component's manifest and the file it names.
//
// A manifest that cannot be read has declared nothing, and the defaults are
// better than refusing a run over a file that was never going to be read.
func ReadSettings() Settings {
	out := Settings{
		Top:         DefaultTop,
		WarnPercent: DefaultWarnPercent,
		Width:       DefaultWidth,
	}

	root := ModuleRoot()
	if root == "" {
		return out
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Settings == nil {
		return out
	}

	applyDeclared(&out, manifest)
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

// applyDeclared is what the manifest's own declaration says the defaults are.
func applyDeclared(out *Settings, manifest *deps.Manifest) {
	for _, setting := range manifest.Settings.Settings {
		if setting.Default == "" {
			continue
		}
		applyValue(out, setting.Name, setting.Default)
	}
}

// applyValue reads one setting by the name it is declared under.
func applyValue(out *Settings, name, raw string) {
	raw = strings.TrimSpace(raw)
	switch name {
	case "top":
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			out.Top = value
		}
	case "warn_threshold":
		if value, ok := parsePercentString(raw); ok {
			out.WarnPercent = value
		}
	case "width":
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			out.Width = value
		}
	}
}

// parsePercentString is a percentage written as `85%` or as a plain number, which
// is a fraction between 0 and 1 — so `0.85` and `85%` mean the same thing and
// neither is read as the other.
func parsePercentString(raw string) (float64, bool) {
	if strings.HasSuffix(raw, "%") {
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, "%"), 64)
		return value, err == nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	if value > 0 && value <= 1 {
		return value * 100, true
	}
	return value, true
}

// ModuleRoot is where this component's own files are.
//
// From the environment when a harness set it, and from the executable's own
// directory otherwise: a component installed from a registry is a directory, and
// its manifest and its settings file travel with it.
func ModuleRoot() string {
	if root := strings.TrimSpace(os.Getenv("RHO_MODULE_ROOT")); root != "" {
		return root
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable)
	}
	return ""
}
