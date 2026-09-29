//
// @file module.go
// @brief One module in full: what it is, what it documents, and what it is set
// to.
//
// The list a person scans and the page a person reads are different questions.
// The list answers "which of these is it"; this answers everything about one —
// what it says it is, what it needs, what it documents, and what it is
// currently configured to do.
//
// The settings are the same contract the component declares in its own manifest
// and keeps in the file that travels with it. Reading them is therefore the same
// reading the component itself does, and writing one is writing that file — so
// a change made here is a change the component makes the next time it runs, with
// no separate copy of its configuration anywhere.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package keeper

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/yaml"
)

// ModuleDetail is one module, in full.
type ModuleDetail struct {
	// Module is what the list showed, so a person who opened a row sees the row
	// they clicked and more of it, rather than a different thing.
	Module
	// Settings is what the module declares it can be configured with, and what
	// each is set to. Empty is a module that takes no configuration, which is
	// most of them and is why the page says so rather than showing a form with
	// nothing in it.
	Settings []Setting `json:"settings,omitempty"`
	// ConfigFile is where the values are, which is the file a person edits by
	// hand and the one a change here writes to.
	ConfigFile string `json:"configFile,omitempty"`
	// HasDocs is whether the module carries documentation worth offering, so a
	// page does not offer a link that opens nothing.
	HasDocs bool `json:"hasDocs"`
	// DocsPath is the file its documentation is, which is a README or a docs
	// index beside its manifest.
	DocsPath string `json:"docsPath,omitempty"`
	// Every version installed, so a person can see that a machine holds more
	// than one without leaving the page.
	Versions []string `json:"versions,omitempty"`
}

// Setting is one value a module can be configured with.
type Setting struct {
	// Name is how it is written in the file and how it is set here.
	Name string `json:"name"`
	// Description is what a person is shown beside it, and is why the contract
	// exists: a name alone does not say what a value is for.
	Description string `json:"description,omitempty"`
	// Value is what it is set to now, which is the file's value when it has one
	// and the declared default when it does not.
	Value string `json:"value"`
	// Default is what it falls back to, so a page can show a person that has
	// changed something away from what it was.
	Default string `json:"default,omitempty"`
	// Choices are the values it takes, when it takes a fixed set, which is what
	// makes a control a list rather than a free text box.
	Choices []string `json:"choices,omitempty"`
	// Changed is whether the value is not the default, which is what a person
	// needs to know before deciding whether to put it back.
	Changed bool `json:"changed"`
	// Command is the command that would set it, so that a change made here is
	// repeatable in a terminal like every other change this page makes.
	Command string `json:"command,omitempty"`
}

// Detail reads one module in full, or nil when there is no such module.
func Detail(name string) *ModuleDetail {
	row, kind, installed := describeByName(name)
	if installed == "" {
		return nil
	}
	detail := &ModuleDetail{
		Module:     row,
		Versions:   deps.InstalledVersions(kind, name),
		HasDocs:    HasDocs(name),
		DocsPath:   ModuleDocFile(name),
		ConfigFile: "",
	}

	root := installedRoot(name)
	if root == "" {
		return detail
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Settings == nil {
		return detail
	}
	if manifest.Settings.File != "" {
		detail.ConfigFile = filepath.Join(root, filepath.FromSlash(manifest.Settings.File))
	}
	// The values in the file are what the module runs on; the declared defaults
	// are what it falls back to. Both are shown, because a person deciding
	// whether to change something needs to know what it would go back to.
	values := map[string]string{}
	if detail.ConfigFile != "" {
		if data, readErr := os.ReadFile(detail.ConfigFile); readErr == nil {
			if document, parseErr := yaml.Load(string(data)); parseErr == nil {
				for key, value := range document.Map {
					if value.Kind != yaml.KindMap && value.Kind != yaml.KindList {
						values[key] = value.Str
					}
				}
			}
		}
	}
	for _, declared := range manifest.Settings.Settings {
		value, present := values[declared.Name]
		if !present {
			value = declared.Default
		}
		detail.Settings = append(detail.Settings, Setting{
			Name:        declared.Name,
			Description: declared.Description,
			Value:       value,
			Default:     declared.Default,
			Choices:     declared.Choices,
			Changed:     present && value != declared.Default,
			Command:     command("module enable", name),
		})
	}
	// A module whose configuration is a file with no declared settings still has
	// settings: they are the keys it actually holds. A page that showed nothing
	// for it would be wrong, because a thing it can be set to exists.
	if len(detail.Settings) == 0 && len(values) > 0 {
		for key, value := range values {
			detail.Settings = append(detail.Settings, Setting{Name: key, Value: value, Command: command("module enable", name)})
		}
		sort.Slice(detail.Settings, func(i, j int) bool { return detail.Settings[i].Name < detail.Settings[j].Name })
	}
	return detail
}

// describeByName is the row the list shows for a name, wherever it is installed.
func describeByName(name string) (Module, string, string) {
	for _, kind := range []string{deps.KindComponent, deps.KindPlugin} {
		versions := deps.InstalledVersions(kind, name)
		if len(versions) == 0 {
			continue
		}
		// The version a run would use is the one the row shows, and the reading
		// is the same one the list uses, so a detail page cannot disagree with
		// the row it was opened from.
		row := readOne(kind, name)
		return row, kind, name
	}
	return Module{}, "", ""
}

// SetSetting changes one value in a module's configuration file.
//
// It writes the module's own file, which travels with the module, because a
// value held somewhere else would be a second configuration: a file the module
// does not read, changed by a page the module never sees. Writing that one is
// what makes a change made here the same change the module would make itself.
//
// A value is refused when the setting takes a fixed set of choices and the value
// is not one of them. A configuration file nobody validates is a file that will
// be wrong the next time it is read.
func SetSetting(name, key, value string) Action {
	act := command("keeper setting", name+" "+key+"="+value)
	detail := Detail(name)
	if detail == nil {
		return Action{Command: act, Error: fmt.Sprintf("%s is not installed", name)}
	}
	if detail.ConfigFile == "" {
		return Action{Command: act, Error: fmt.Sprintf("%s carries no configuration file", name)}
	}
	declared := false
	for _, setting := range detail.Settings {
		if setting.Name != key {
			continue
		}
		declared = true
		if len(setting.Choices) > 0 {
			allowed := false
			for _, choice := range setting.Choices {
				if choice == value {
					allowed = true
					break
				}
			}
			if !allowed {
				return Action{
					Command: act,
					Error: fmt.Sprintf("%q is not one of the values %s takes (%s)",
						value, key, strings.Join(setting.Choices, ", ")),
				}
			}
		}
	}
	if !declared {
		return Action{Command: act, Error: fmt.Sprintf("%s does not declare a setting called %q", name, key)}
	}

	values := map[string]string{}
	if data, err := os.ReadFile(detail.ConfigFile); err == nil {
		if document, parseErr := yaml.Load(string(data)); parseErr == nil {
			for k, v := range document.Map {
				if v.Kind != yaml.KindMap && v.Kind != yaml.KindList {
					values[k] = v.Str
				}
			}
		}
	}
	values[key] = value

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out strings.Builder
	out.WriteString("# Written by the rho keeper dashboard. " +
		"A hand edit is kept: the values here are the module's own.\n")
	for _, k := range keys {
		fmt.Fprintf(&out, "%s: %s\n", k, values[k])
	}
	if err := os.WriteFile(detail.ConfigFile, []byte(out.String()), 0o644); err != nil {
		return Action{Command: act, Error: err.Error()}
	}
	return Action{
		OK:      true,
		Command: act,
		Message: fmt.Sprintf("%s now has %s = %s", name, key, value),
	}
}
