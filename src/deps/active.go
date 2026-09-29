//
// @file active.go
// @brief Which of the installed modules is the one that runs.
//
// A machine may hold several versions of a component or a plugin, installed at
// any time and for any reason. What runs is not the newest of them: it is the
// one that is *active*, and there is exactly one. A version installed as a
// dependency, or left behind by an upgrade, is there without being chosen, and a
// component whose contract has moved on should not be what a build silently
// picks up because it has the largest number.
//
// So the state is a small record of which version of each module is live. A
// module is **not** live until somebody says so: installing a component or a
// plugin makes it available, and using it is a separate decision the person who
// installed it has to make. That is the whole point of a record — a module
// nobody chose is not one a build should quietly reach for, because the program
// that results runs code its author did not ask for, on a machine whose state
// nobody wrote down.
//
// A module the record names with a version is enabled at that version, whatever
// else is installed. A module the record does not name, or names with nothing, is
// not enabled at any version.
//
// A run that needs a module which is not enabled says so and stops, and says how
// to make it so. Silently falling back to one that is not enabled is the failure
// this exists to prevent: a program that built on a machine where a component
// was enabled and failed on one where it was not is a program whose build is not
// reproducible, and the fix has to be visible rather than inferred.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/yaml"
)

// EnabledFile is what the active versions are recorded in, inside the Rho home.
//
// It is a record of a decision rather than a cache of one, so it is written by a
// person running a command and read by everything else. It sits beside the
// installed trees rather than inside them: it is about which of them to use, and
// belongs to no one of them.
const EnabledFile = "enabled.yml"

// Kinds of installed module. A name is enabled per kind, because a component and
// a plugin of the same name are different programs and are chosen separately.
const (
	KindComponent = "component"
	KindPlugin    = "plugin"
	KindModule    = "module"
)

// Enabled is which version of each module is the one that runs.
type Enabled struct {
	// versions maps a kind and a name to the version that is enabled. A module
	// the map does not mention is not enabled, and an entry whose version is
	// empty is one that was installed and then deliberately switched off — the
	// two are kept apart so that "never chosen" and "chosen and withdrawn" can
	// be told apart, which is the difference between a first run and a
	// deliberate one.
	versions map[string]string
}

// enabledKey is how a kind and a name are keyed together, since two modules of
// the same name in different kinds are different things.
func enabledKey(kind, name string) string { return kind + "/" + name }

// EnabledPath is the record of which versions are live, or "" when there is no
// Rho home to keep one in.
func EnabledPath() string {
	home := RhoHome()
	if home == "" {
		return ""
	}
	return filepath.Join(home, "modules", EnabledFile)
}

// LoadEnabled reads the record, or an empty one when there is none.
//
// A record that cannot be read is an error rather than an empty one: a record
// that exists and is unreadable would otherwise be read as "nothing is enabled",
// which is a statement about the installation that nobody made.
func LoadEnabled() (*Enabled, error) {
	out := &Enabled{versions: map[string]string{}}
	path := EnabledPath()
	if path == "" {
		return out, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("the record of which modules are enabled cannot be read: %w", err)
	}
	document, err := yaml.Load(string(data))
	if err != nil {
		return out, fmt.Errorf("the record of which modules are enabled cannot be read: %w", err)
	}
	for kind, byName := range document.Map {
		// A section is a kind of module and the versions of it that are enabled.
		// One that is neither is a record written by something that is not this
		// tool, and reading past it would read a machine's state as though it
		// were empty — which would then be written back over the state this
		// could not read.
		if byName.Kind != yaml.KindMap {
			return out, fmt.Errorf(
				"the record of which modules are enabled has a %q section that is not a list of names", kind)
		}
		for name, value := range byName.Map {
			if value.Kind != yaml.KindString {
				return out, fmt.Errorf(
					"the record of which modules are enabled has a version for %s in %q that is not text", name, kind)
			}
			out.versions[enabledKey(kind, name)] = value.Str
		}
	}
	return out, nil
}

// IsEnabled reports whether a version of a module is the one that runs.
//
// A version that is not the enabled one is reported as not enabled even when it is
// installed, which is the whole distinction: an installed version is available,
// an active one is used.
func (a *Enabled) IsEnabled(kind, name, version string) bool {
	if a == nil {
		return true
	}
	recorded, decided := a.versions[enabledKey(kind, name)]
	if !decided {
		// Nothing was said about this module, and nothing being said is not a
		// decision to use it. Installing made it available; using it is a choice
		// that has to be made.
		return false
	}
	return recorded != "" && recorded == version
}

// Recorded reports whether anything was said about a module, which is what tells
// a default apart from a decision.
func (a *Enabled) Recorded(kind, name string) (string, bool) {
	if a == nil {
		return "", false
	}
	version, found := a.versions[enabledKey(kind, name)]
	return version, found
}

// Disable records that a module is installed and not to be used.
//
// It is recorded rather than merely absent, so that a module somebody switched
// off is not quietly switched on again by the next install, and so a listing can
// say "you turned this off" rather than "nothing was ever said about it".
func (a *Enabled) Disable(kind, name string) {
	if a.versions == nil {
		a.versions = map[string]string{}
	}
	a.versions[enabledKey(kind, name)] = ""
}

// SetEnabled records the version of a module that runs, or that none does.
//
// An empty version is a module that is installed and not enabled, which is how a
// disabled module is recorded: one absence rather than a second list that could
// disagree with the first.
func (a *Enabled) SetEnabled(kind, name, version string) {
	if a.versions == nil {
		a.versions = map[string]string{}
	}
	a.versions[enabledKey(kind, name)] = version
}

// Save writes the record.
func (a *Enabled) Save() error {
	path := EnabledPath()
	if path == "" {
		return fmt.Errorf("no Rho installation was found; there is nowhere to record which modules are enabled")
	}
	document := yaml.Value{Kind: yaml.KindMap, Map: map[string]yaml.Value{}}
	// Written in a stable order, so a record that is rewritten for no change
	// is a record that is unchanged and the reader can tell.
	keys := make([]string, 0, len(a.versions))
	for key := range a.versions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		kind, name, _ := strings.Cut(key, "/")
		if document.Map[kind].Kind != yaml.KindMap {
			document.Map[kind] = yaml.Value{Kind: yaml.KindMap, Map: map[string]yaml.Value{}}
		}
		document.Map[kind].Map[name] = yaml.Value{Kind: yaml.KindString, Str: a.versions[key]}
	}
	var out strings.Builder
	writeYAML(&out, document, 0)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
}

// InstalledVersions is every version of a module that is installed, newest first.
//
// It is what a person is shown when choosing which one to activate: the choice
// is between what exists, and the versions left behind by earlier installs are
// among them on purpose.
func InstalledVersions(kind, name string) []string {
	home := RhoHome()
	if home == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, "modules", "installed", kind))
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		installed, version, cut := strings.Cut(entry.Name(), "@")
		if !cut || installed != name || version == "" {
			continue
		}
		out = append(out, version)
	}
	sort.Slice(out, func(i, j int) bool { return compareModuleVersions(out[i], out[j]) > 0 })
	return out
}

// InstalledModule is one installed version of a module, and whether it is the one
// that runs.
type InstalledModule struct {
	Name    string
	Version string
	Root    string
	// Enabled is whether this version is the one a run uses. Several versions
	// may be installed and only one of them may be; the rest are there and not
	// used, which is a distinction a listing exists to show.
	Enabled bool
}

// ListInstalled is every module of a kind that is installed, with the one that
// runs marked, so a reader can see both what is there and what is in use.
func ListInstalled(kind string) []InstalledModule {
	home := RhoHome()
	if home == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, "modules", "installed", kind))
	if err != nil {
		return nil
	}
	state, err := LoadEnabled()
	if err != nil {
		// A record that cannot be read is reported by whatever needs it; a
		// listing that skipped the state would show nothing as active and be
		// read as "nothing is installed".
		state = &Enabled{versions: map[string]string{}}
	}
	out := []InstalledModule{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, version, cut := strings.Cut(entry.Name(), "@")
		if !cut || name == "" || version == "" {
			continue
		}
		out = append(out, InstalledModule{
			Name:    name,
			Version: version,
			Root:    filepath.Join(home, "modules", "installed", kind, entry.Name()),
			Enabled: state.IsEnabled(kind, name, version),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return compareModuleVersions(out[i].Version, out[j].Version) > 0
	})
	return out
}

// NotEnabledError is what a run says when it needed a module that is installed
// and not enabled.
//
// It names the module and says how to make it active, because the reader is
// looking at a failure and the fix is one command away. It is a distinct type
// because a caller that needs a component can act on it — install it, activate
// it — where a generic error leaves it nothing to do but report.
type NotEnabledError struct {
	// Kind and Name are the module that was needed.
	Kind string
	Name string
	// Installed is every version of it that is installed, which is what the
	// message offers as the choice.
	Installed []string
}

func (e *NotEnabledError) Error() string {
	if len(e.Installed) == 0 {
		return fmt.Sprintf(
			"the %s %s is needed and is not installed; install it with: rho module publish %s",
			e.Kind, e.Name, filepath.Join("modules", e.Kind+"s", e.Name))
	}
	// The command is `rho module enable` whatever the kind is: a component and
	// a plugin are both modules, and a reader told a command that does not exist
	// has been told the wrong thing at the moment they most need the right one.
	return fmt.Sprintf(
		"the %s %s is installed but not enabled, and this is needed to run; "+
			"enable it with: rho module enable %s@%s",
		e.Kind, e.Name, e.Name, e.Installed[0])
}

// Enable records the highest installed version of a module as the one that runs,
// which is what installing something and then turning it on means.
func Enable(kind, name string) error {
	versions := InstalledVersions(kind, name)
	if len(versions) == 0 {
		return fmt.Errorf("%s %s is not installed", kind, name)
	}
	return SetVersionEnabled(kind, name, versions[0])
}

// Disable records that a module is installed and not to be used.
func Disable(kind, name string) error {
	if len(InstalledVersions(kind, name)) == 0 {
		return fmt.Errorf("%s %s is not installed", kind, name)
	}
	state, err := LoadEnabled()
	if err != nil {
		return err
	}
	state.Disable(kind, name)
	return state.Save()
}

// SetVersionEnabled records which version of a module runs.
//
// A version that is not installed is refused rather than recorded: a record
// naming a version that is not there would make every later run fail with a
// message about a module the reader believes they have.
func SetVersionEnabled(kind, name, version string) error {
	installed := false
	for _, candidate := range InstalledVersions(kind, name) {
		if candidate == version {
			installed = true
			break
		}
	}
	if !installed {
		if len(InstalledVersions(kind, name)) == 0 {
			return fmt.Errorf("%s %s is not installed", kind, name)
		}
		return fmt.Errorf("%s %s@%s is not installed; installed: %s",
			kind, name, version, strings.Join(InstalledVersions(kind, name), ", "))
	}
	state, err := LoadEnabled()
	if err != nil {
		return err
	}
	state.SetEnabled(kind, name, version)
	return state.Save()
}

// writeYAML writes a value the way a manifest is written, which is what a person
// reads when they open the record.
func writeYAML(out *strings.Builder, node yaml.Value, indent int) {
	pad := strings.Repeat("  ", indent)
	switch node.Kind {
	case yaml.KindMap:
		keys := make([]string, 0, len(node.Map))
		for key := range node.Map {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := node.Map[key]
			if value.Kind == yaml.KindMap {
				fmt.Fprintf(out, "%s%s:\n", pad, key)
				writeYAML(out, value, indent+1)
				continue
			}
			fmt.Fprintf(out, "%s%s: %s\n", pad, key, quoteYAML(value.Str))
		}
	case yaml.KindList:
		for _, value := range node.List {
			fmt.Fprintf(out, "%s- %s\n", pad, quoteYAML(value.Str))
		}
	}
}

// quoteYAML is a value written so that a reader sees the same string the record
// holds, whatever it contains.
func quoteYAML(value string) string {
	if value == "" {
		// An empty version means "installed and not enabled", which is not the
		// same as a version called "".
		return `""`
	}
	if strings.ContainsAny(value, ":#{}[]&*?|<>=!%@`\"'\n") || strings.HasPrefix(value, " ") {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value) + `"`
	}
	return value
}
