//
// @file state.go
// @brief What the installation is, read from the installation.
//
// A dashboard is worth exactly as much as the reading behind it. This reads the
// toolchain's own state through the same functions the CLI uses, so what a page
// shows and what a command would do cannot disagree — a second reader, written
// separately, would be right until the first time one of them was changed and
// the other was not.
//
// Everything here is read. Nothing is computed from a guess: a version is enabled
// because the record says so, a module is installed because the directory is
// there, and a module that cannot be read is reported as unreadable rather than
// omitted, because a list that quietly leaves things out is a list nobody can
// trust to be complete.
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
	"rho/src/plugins"
)

// Module is one installed module, as a page shows it.
type Module struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Version string `json:"version"`
	// InstalledVersions is every version present, newest first. A machine may
	// hold several, and the page that shows one is hiding the rest.
	InstalledVersions []string `json:"installedVersions,omitempty"`
	// Enabled is whether this version is the one a run uses. Installing made it
	// available; using it is a separate decision, and this is it.
	Enabled bool `json:"enabled"`
	// Record is whether anything was recorded about it, which tells "you turned
	// this off" apart from "nothing was ever said about it".
	Record string `json:"record,omitempty"`
	// Description, Author, Main and Command come from the module's own
	// manifest: the thing that says what it is for is the thing that describes
	// it, and a dashboard that paraphrased it would be one more place to keep in
	// step.
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Main        string `json:"main,omitempty"`
	Command     string `json:"command,omitempty"`
	// ImportPath is the name Rho code imports this module by, which is not
	// always the name it is installed under.
	ImportPath string `json:"importPath,omitempty"`
	// Type and Version are what the manifest calls the module and itself, kept
	// apart from the directory name because a published component can be
	// installed under a name that is not its own.
	Type    string `json:"type,omitempty"`
	Extends string `json:"extends,omitempty"`
	// Provides and Implements are the contracts: what the component needs a
	// plugin to be, and what this plugin says it is. The second is the answer to
	// "what is this for", which is why it is worth reading.
	Provides   []string `json:"provides,omitempty"`
	Implements []string `json:"implements,omitempty"`
	// Readable is why a module could not be described, when it could not. A page
	// shows the reason rather than a blank row.
	Readable string `json:"unreadable,omitempty"`
	// HasDocs is whether the module carries documentation, so a list can offer
	// it. A badge that opens nothing is a badge that lies, so it is asked of the
	// file rather than assumed from the module having a name.
	HasDocs bool `json:"hasDocs"`
	// HasSettings is whether the module declares it can be configured, so a row
	// can say so before it is opened.
	HasSettings bool `json:"hasSettings"`
}

// Installation is everything the dashboard shows, read in one pass.
type Installation struct {
	// Keeper is this dashboard: its own version, which comes from its manifest
	// the same way every other version on the page does. A page that did not say
	// which build it was would be a page whose contents cannot be compared with
	// a later one.
	Keeper Keeper `json:"keeper"`
	// Home is the Rho home this describes, and Empty is whether there is one at
	// all: a dashboard with nothing to say should say so rather than render an
	// empty table that looks like an installation with nothing in it.
	Home  string `json:"home,omitempty"`
	Empty bool   `json:"empty"`
	// Toolchain is what `rho install` put in place.
	Toolchain Toolchain `json:"toolchain"`
	// Modules is every installed component, plugin and module, in a stable
	// order, so a page does not reshuffle itself between reads.
	Modules []Module `json:"modules"`
	// Contracts is every contract the installed components offer, so a page can
	// show what each one needs a plugin to be even before any plugin is
	// installed.
	Contracts []Contract `json:"contracts"`
	// Errors is what could not be read. A dashboard that hides its own failures
	// is a dashboard that cannot be trusted when it matters.
	Errors []string `json:"errors,omitempty"`
}

// Keeper is the dashboard itself, as its chrome shows it.
type Keeper struct {
	// Version is this module's own version, read from its manifest.
	Version string `json:"version"`
	Name    string `json:"name"`
	// Config is the file its settings are in, which is what a person looks for
	// when they want to change one.
	Config string `json:"config,omitempty"`
	// Addr is where this run is listening.
	Addr string `json:"addr,omitempty"`
}

// Toolchain is what `rho install` puts on the machine.
type Toolchain struct {
	Installed bool     `json:"installed"`
	Home      string   `json:"home,omitempty"`
	Binaries  []Binary `json:"binaries,omitempty"`
	// Missing names what a person would expect to find and is not there, which
	// is the question "is rho install actually installed?" really is.
	Missing []string `json:"missing,omitempty"`
}

// Binary is one program the toolchain installed.
type Binary struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
	Path    string `json:"path,omitempty"`
	Size    int64  `json:"size,omitempty"`
}

// Contract is what an installed component needs a plugin to be.
type Contract struct {
	Component   string   `json:"component"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	One         bool     `json:"one"`
	Hooks       []string `json:"hooks,omitempty"`
	// Filled is the plugins that fill it, which for a contract of one has to be
	// exactly one for the component to be able to run at all.
	Filled []string `json:"filled,omitempty"`
	// Status is whether the contract is satisfied, which is the question a page
	// is really asking: a component that needs an engine and has none cannot
	// run, and a person looking at a list of installed modules would not see it
	// from the list alone.
	//
	// It is worked out here rather than in the page, so that what the page says
	// and what a script reading the API gets are the same answer.
	Status string `json:"status"`
}

// Whether a contract is satisfied. A name rather than a number, because this is
// read by a person as well as by a page.
const (
	// ContractOK is a contract with what it needs: one plugin for a contract of
	// one, and at least one for a contract of many.
	ContractOK = "ok"
	// ContractMissing is a contract nothing fills. A component that needs one
	// cannot run.
	ContractMissing = "missing"
	// ContractConflict is a contract of one with more than one plugin filling
	// it, which is not a choice and so is not an answer either.
	ContractConflict = "conflict"
)

// Satisfied is the status of a contract, from what fills it.
func Satisfied(one bool, filled int) string {
	switch {
	case one && filled == 0, !one && filled == 0:
		return ContractMissing
	case one && filled > 1:
		return ContractConflict
	default:
		return ContractOK
	}
}

// readOne is the row the list shows for one module, read through the list's own
// reading so that a page opened from a row cannot disagree with the row it was
// opened from.
func readOne(kind, name string) Module {
	versions := deps.InstalledVersions(kind, name)
	if len(versions) == 0 {
		return Module{}
	}
	home := deps.RhoHome()
	if home == "" {
		return Module{}
	}
	// The version a run would use is the one the row shows: a machine may hold
	// several, and the one in use is the one a person is asking about.
	for _, version := range versions {
		root := filepath.Join(home, "modules", "installed", kind, name+"@"+version)
		if _, err := os.Stat(filepath.Join(root, "module.yml")); err != nil {
			continue
		}
		state, err := deps.LoadEnabled()
		if err != nil {
			state = nil
		}
		return describe(kind, deps.InstalledModule{
			Name: name, Version: version, Root: root,
			Enabled: state.IsEnabled(kind, name, version),
		}, state)
	}
	return Module{}
}

// Read reads the installation.
//
// It reads rather than infers throughout, and reports what it could not read
// instead of dropping it: a page that lists what it found and says nothing about
// what it did not is worse than one that admits there was more to see.
func Read() Installation {
	out := Installation{Empty: true}
	home := deps.RhoHome()
	if home == "" {
		out.Errors = append(out.Errors,
			"no Rho installation was found; run: rho install")
		return out
	}
	out.Home = home
	out.Toolchain = readToolchain(home)
	// The dashboard's own version comes from its manifest, the same way every
	// other version on the page does. A page that did not say which build it was
	// would be a page whose contents cannot be compared with a later one's.
	out.Keeper = readKeeper()

	state, err := deps.LoadEnabled()
	if err != nil {
		// The record exists and could not be read. Every module below would
		// then answer from a state nobody can see, so this is said once, loudly,
		// and the reading goes on: a partial answer that says it is partial is
		// more use than no answer.
		out.Errors = append(out.Errors, err.Error())
		state = nil
	}

	// A home with nothing installed is a real state — a machine that has run
	// `rho install` and installed no modules — and saying so is better than an
	// empty table that looks like a page that failed to load.
	byKind := map[string][]Module{}
	contracts := map[string]Contract{}
	for _, kind := range []string{deps.KindComponent, deps.KindPlugin} {
		for _, installed := range deps.ListInstalled(kind) {

			module := describe(kind, installed, state)
			if module.Name == "" {
				continue
			}
			byKind[kind] = append(byKind[kind], module)
			out.Empty = false
		}
	}
	if out.Empty {
		out.Errors = append(out.Errors,
			"nothing is installed under this Rho home; run: rho module search, then rho module install")
	}
	// One list rather than three, in a stable order, because a page that sorts
	// by name is a page a reader can scan.
	// One row per module rather than one per version: a page that listed a
	// module once per version installed would show the same thing several
	// times, which is not what a reader is counting.
	//
	// The row is the version that is used, because that is the one a build
	// links. Where nothing is enabled it is the newest installed, which is the
	// one a build would link if somebody turned it on now. The versions that
	// are on the machine but are not the one in use are on the row, so that
	// showing one does not imply there is only one.
	type rowKey struct{ kind, name string }
	chosen := map[rowKey]Module{}
	for _, kind := range []string{deps.KindComponent, deps.KindPlugin} {
		for _, module := range byKind[kind] {
			if module.Name == "" {
				continue
			}
			key := rowKey{kind, module.Name}
			previous, seen := chosen[key]
			switch {
			case !seen:
				chosen[key] = module
			case module.Enabled && !previous.Enabled:
				// The used version replaces one that was not used.
				chosen[key] = module
			}
		}
	}
	for _, kind := range []string{deps.KindComponent, deps.KindPlugin} {
		for _, module := range byKind[kind] {
			if picked, ok := chosen[rowKey{kind, module.Name}]; ok && picked.Version == module.Version {
				out.Modules = append(out.Modules, picked)
			}
		}
	}
	sort.SliceStable(out.Modules, func(i, j int) bool {
		if out.Modules[i].Kind != out.Modules[j].Kind {
			return out.Modules[i].Kind < out.Modules[j].Kind
		}
		return out.Modules[i].Name < out.Modules[j].Name
	})

	// Contracts come from the components that are actually installed, so the
	// page describes this machine rather than what the repository declares.
	for _, module := range out.Modules {
		if module.Kind != deps.KindComponent {
			continue
		}
		root := deps.InstalledComponentRoot(module.Name)
		if root == "" {
			continue
		}
		for _, contract := range readContracts(root, module.Name) {
			contracts[contract.Component+"."+contract.Name] = contract
		}
	}
	for _, contract := range contracts {
		contract.Filled = pluginsFilling(contract.Component, contract.Name)
		contract.Status = Satisfied(contract.One, len(contract.Filled))
		out.Contracts = append(out.Contracts, contract)
	}
	sort.Slice(out.Contracts, func(i, j int) bool {
		if out.Contracts[i].Component != out.Contracts[j].Component {
			return out.Contracts[i].Component < out.Contracts[j].Component
		}
		return out.Contracts[i].Name < out.Contracts[j].Name
	})
	return out
}

// describe is one installed module, read from its own manifest.
//
// A manifest that cannot be read is still listed, with the reason, because a
// module that is installed and undescribable is something a person needs to know
// about and not something to omit.
func describe(kind string, installed deps.InstalledModule, state *deps.Enabled) Module {
	module := Module{
		Name:              installed.Name,
		Kind:              kind,
		Version:           installed.Version,
		Enabled:           installed.Enabled,
		InstalledVersions: deps.InstalledVersions(kind, installed.Name),
	}

	if state != nil {
		if version, decided := state.Recorded(kind, installed.Name); decided {
			module.Record = version
		}
	}
	manifest, err := deps.LoadManifest(filepath.Join(installed.Root, "module.yml"))
	if err != nil || manifest == nil {
		module.Readable = "its manifest could not be read"
		return module
	}
	module.Description = manifest.Description
	module.Main = manifest.Main
	module.HasDocs = ModuleDocFile(installed.Name) != ""
	module.HasSettings = manifest.Settings != nil &&
		(manifest.Settings.File != "" || len(manifest.Settings.Settings) > 0)
	module.Type = manifest.ModuleType
	module.Version = manifest.Version
	module.Command = manifest.Command
	module.ImportPath = manifest.ImportPath
	if manifest.Provides != nil {
		for _, contract := range manifest.Provides.Contracts {
			module.Provides = append(module.Provides, contract.Name)
		}
	}
	if manifest.Extends != nil {
		module.Extends = manifest.Extends.Component
		module.Implements = manifest.Extends.Implements
	}
	return module
}

// readKeeper is this dashboard, as its own chrome shows it.
//
// It is read from the same manifest everything else on the page is read from,
// so the version a person sees beside the name is the version that is running.
func readKeeper() Keeper {
	out := Keeper{Name: "rho-keeper"}
	root := moduleRoot()
	if root == "" {
		return out
	}
	if manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml")); err == nil && manifest != nil {
		out.Name = manifest.ModuleName
		out.Version = manifest.Version
		if manifest.Settings != nil && manifest.Settings.File != "" {
			out.Config = filepath.Join(root, filepath.FromSlash(manifest.Settings.File))
		}
	}
	return out
}

// readToolchain is what `rho install` put in place.
//
// It answers "is this installed?" by looking, rather than by trusting a record
// that a failed install might have written. An installation that is half done
// should look half done.
func readToolchain(home string) Toolchain {
	out := Toolchain{Home: home}
	bin := filepath.Join(home, "bin")
	present := 0
	for _, name := range []string{"rho", "rho-deps"} {
		path := filepath.Join(bin, name)
		entry := Binary{Name: name, Path: path}
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			entry.Present, entry.Size = true, info.Size()
			present++
		} else {
			out.Missing = append(out.Missing, name)
		}
		out.Binaries = append(out.Binaries, entry)
	}
	out.Installed = present > 0 && len(out.Missing) == 0
	return out
}

// readContracts is what a component needs a plugin to be, from its own manifest.
func readContracts(root, component string) []Contract {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Provides == nil {
		return nil
	}
	out := make([]Contract, 0, len(manifest.Provides.Contracts))
	for _, contract := range manifest.Provides.Contracts {
		out = append(out, Contract{
			Component:   component,
			Name:        contract.Name,
			Description: contract.Description,
			One:         contract.One,
			Hooks:       contract.Hooks,
		})
	}
	return out
}

// pluginsFilling is the plugins that fill a contract, read through discovery
// rather than by scanning directories, so it is the same set a run would use.
func pluginsFilling(component, contract string) []string {
	home := deps.RhoHome()
	if home == "" {
		return nil
	}
	source := plugins.Source{RhoHome: home}
	out := []string{}
	for _, plugin := range source.Discover(component) {
		if plugin.Fills(contract) {
			out = append(out, plugin.Name)
		}
	}
	sort.Strings(out)
	return out
}

// Versions is the installed versions of a module, for a page that shows one
// module rather than all of them.
func Versions(kind, name string) []string {
	return deps.InstalledVersions(kind, name)
}

// Summary is the one-line-per-module form, for `rho keeper status` and for a
// header that says how much is installed without being the whole page.
//
// It is deliberately text rather than JSON: it goes on a terminal, where a person
// is reading it, and a machine reads the same facts through the API.
func Summary(state Installation) string {
	var b strings.Builder
	if state.Empty {
		b.WriteString("nothing is installed under this Rho home")
		if state.Home != "" {
			fmt.Fprintf(&b, " (%s)", state.Home)
		}
		b.WriteString("\n")
		return b.String()
	}
	current := map[string]string{}
	for _, module := range state.Modules {
		if module.Enabled {
			current[module.Name] = module.Version
		}
	}
	for _, module := range state.Modules {
		used := "not enabled"
		if module.Enabled {
			used = "enabled"
		}
		fmt.Fprintf(&b, "%-9s %s@%s  %s\n", module.Kind, module.Name, module.Version, used)
	}
	enabled := 0
	for range current {
		enabled++
	}
	fmt.Fprintf(&b, "\n%d installed, %d enabled\n", len(state.Modules), enabled)
	return b.String()
}
