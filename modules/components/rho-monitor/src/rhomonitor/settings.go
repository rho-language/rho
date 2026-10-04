//
// @file settings.go
// @brief What this component is set to, read from its own manifest and its own file.
//
// The contract is declared in the component's manifest and the values are in the
// file that manifest names, so what a person reads to find out what can be
// configured is the same thing the component runs on — and the same reading every
// other component makes, because two components reading their configuration two
// ways is one of them reading it wrong.
//
// The order is what makes the settings work: the manifest's declaration is read
// first, because it is what a person reads to find out what the defaults are, and
// the file second, because a file is somebody's decision about this installation.
// A component installed without its configuration directory still runs, with the
// values the manifest calls the defaults.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

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
	// Backend is the transport to read when the command line names none.
	Backend Kind
	// BaudRate is the line speed a serial console is opened at.
	BaudRate int
	// Timestamps prefixes each line with when it arrived.
	Timestamps bool
	// ColorLevels colours a line by the severity word in it.
	ColorLevels bool
	// LogFile is where a copy of everything is written.
	LogFile string
	// Duration is how long a run reads before it stops. Zero means until the
	// stream ends or somebody interrupts it.
	Duration time.Duration
}

// ReadSettings is this component's manifest and the file it names.
func ReadSettings() Settings {
	out := Settings{
		Backend:     Kind("auto"),
		BaudRate:    115200,
		Timestamps:  true,
		ColorLevels: true,
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
	applyFile(&out, document.Map)
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

// applyFile is what the values file says.
func applyFile(out *Settings, values map[string]yaml.Value) {
	for name, value := range values {
		applyValue(out, name, value.Str)
	}
}

// applyValue reads one setting by the name it is declared under.
func applyValue(out *Settings, name, raw string) {
	raw = strings.TrimSpace(raw)
	switch name {
	case "backend":
		switch strings.ToLower(raw) {
		case "rtt", "uart", "swo", "auto":
			out.Backend = Kind(strings.ToLower(raw))
		}
	case "baud_rate":
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			out.BaudRate = value
		}
	case "timestamps":
		if value, declared := readBool(raw); declared {
			out.Timestamps = value
		}
	case "color_levels":
		if value, declared := readBool(raw); declared {
			out.ColorLevels = value
		}
	case "log_file":
		out.LogFile = raw
	case "duration":
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			out.Duration = time.Duration(value) * time.Second
		}
	}
}

// readBool is a decision written the three ways a person writes one.
func readBool(raw string) (bool, bool) {
	switch strings.ToLower(raw) {
	case "true", "yes", "on", "1":
		return true, true
	case "false", "no", "off", "0":
		return false, true
	}
	return false, false
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
