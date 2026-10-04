package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/schema"
	"rho/src/sysdep"
	"rho/src/yaml"
)

type DepType string

const (
	DepTypeC   DepType = "c"
	DepTypeRho DepType = "rho"
)

type VersionRange struct {
	Raw         string
	Constraints []Constraint
}

type DepSpec struct {
	Name      string
	Type      DepType
	GitURL    string
	GitTag    string
	GitBranch string
	Version   VersionRange
	EnvVar    string
	Path      string
	Include   string
	Src       string
}

// CMakeExports is what a module exports to CMake, declared by the module itself.
//
// The target is the primary export: linking it carries the include directories,
// the definitions and any further libraries transitively, so a consumer names
// nothing but the target. The variables exist for a build file that predates the
// target and asks for a path by name — declared here rather than guessed by
// whoever reads them, because a guessed variable name is a name that will be
// wrong for the next dependency.
type CMakeExports struct {
	// Target is the imported target a consumer links, such as
	// `rho::sensor-lib`.
	Target string
	// IncludeDirs are relative to the exporting module's root.
	IncludeDirs []string
	// Defines are compiled into every consumer, such as `SENSOR_LIB_ENABLE_IRQ=1`.
	Defines []string
	// LinkLibraries are further targets a consumer of this one also links.
	LinkLibraries []string
	// Vars are CMake variables to set, such as a header path a legacy build file
	// asks for. A value may contain `${PACKAGE_DIR}`, which stands for the
	// exporting module's own directory.
	Vars map[string]string
}

// PackageDirToken is what a declared CMake variable may use to mean "where the
// module that declared me is". It is written rather than produced by the module,
// so a value can be stated once and resolved wherever the module is checked out.
const PackageDirToken = "${PACKAGE_DIR}"

// HardwareManifest is where a platform keeps the declarations of the hardware it
// supports. Each path is relative to the module's own root, which is what makes
// them a contract rather than a discovery: nothing has to sit at a conventional
// place for a build to find it.
type HardwareManifest struct {
	// Boards is the directory of this module's boards. Each is read through its
	// own manifest rather than enumerated here, because a board may have several
	// presets and a list here would cap it at one.
	Boards string
	// Presets is the directory of build presets: where a family's sources and a
	// board's overrides are declared.
	Presets string
	// Targets is the directory of target specs: the memory map, the toolchain,
	// and how the board is reached. A spec for a part rather than a board.
	Targets string
	// Target is one board's own spec. A spec that names a board describes that
	// board, so it belongs to the board rather than to the platform that
	// implements every board of the family at once.
	Target string
	// Kconfig is the directory of configuration fragments.
	Kconfig string
}

// Declared reports whether the module declared any hardware at all.
func (h *HardwareManifest) Declared() bool {
	return h != nil && (h.Presets != "" || h.Targets != "" || h.Kconfig != "")
}

// DocManifest is the doc-as-code a module carries: where its documentation is
// written, and where the pages belong in a site.
//
// The generator is the rho-doc component, so nothing here knows how a page is
// rendered. A module says what it has; the generator says what to do with it.
type DocManifest struct {
	// Title overrides the page title taken from the source.
	Title string
	// Section is the sidebar group the pages are listed under.
	Section string
	// Order sorts this module against its peers in a section.
	Order string
	// Sources are the doc-as-code files, relative to the module root. A glob
	// names a whole tree without listing it.
	Sources []string
	// Index is the page a section leads with.
	Index string
}

// Provides is what a component offers a plugin: the hooks a run of the tool
// goes through. A plugin is checked against this list, so a plugin cannot claim
// a stage the tool never runs.
type Provides struct {
	// Hooks are the names a plugin may answer, in the order a run goes through
	// them.
	Hooks []string
	// Contracts are what this component needs a plugin to be, which is a
	// different question from which hooks it answers.
	//
	// Hooks say how a plugin is called; they do not say what it is for. A
	// component that offers `index` and `page` may be extended by a plugin that
	// writes a table of contents and by one that writes an API reference, and
	// both answer `index`. A contract names which of those a plugin is, so a
	// person enabling one can see what they are enabling and a component can
	// say how many of them it takes.
	Contracts []Contract
}

// DebugMapping is what a project says about debugging the chip it is built for.
//
// Declared by the application and authoritative: a project that names its chip's
// register map has said which one is right, and everything else — the target spec,
// the vendor's own naming — is what a project falls back to when nobody has said.
type DebugMapping struct {
	// SVD is the register map for each board this project is built for, by the name
	// the board's target spec answers to.
	//
	// A map and not one path, because a project written for several boards has
	// several chips: one path would be one board's registers standing in for all of
	// them, and on every board it is wrong for it would be wrong silently. A board
	// not named here has no claim from the project, and the search continues with
	// what the board's spec and the vendor's naming say.
	SVD map[string]string
}

// PathFor is the file the project named for a board, or "" when it named none.
func (d *DebugMapping) PathFor(target string) string {
	if d == nil || len(d.SVD) == 0 {
		return ""
	}
	return strings.TrimSpace(d.SVD[target])
}

// Declared reports whether the project has said anything about debugging at all.
func (d *DebugMapping) Declared() bool {
	if d == nil {
		return false
	}
	for _, path := range d.SVD {
		if strings.TrimSpace(path) != "" {
			return true
		}
	}
	return false
}

// ModuleSettings is what a module can be configured with, and the file its
// configured values live in.
//
// The declaration is in the manifest rather than only in the values file because
// the manifest is what a person reads to find out what a module can be asked to
// do: the values file says what they are set to, and the manifest says what may
// be set, what it is called and what it is for.
type ModuleSettings struct {
	// File is the YAML file the values live in, relative to the module root.
	// It is part of the package, so a module installed from a registry carries
	// its own configuration with it and reads the same file wherever it lands.
	File string
	// Settings are the values a person may change, each with what it is called
	// and what it is for. The default is what the file itself carries, and is
	// repeated here so that a module with no values file yet still says what it
	// takes.
	Settings []Setting
}

// Setting is one value a module can be asked for.
type Setting struct {
	// Name is how the value is written in the file and how a person sets it.
	Name string
	// Description is what a person reading a form is shown, and is the reason
	// the declaration exists: a name alone does not say what a value is for.
	Description string
	// Default is what the value is when nothing has set it.
	Default string
	// Choices are the values a setting takes, when it takes a fixed set. A
	// setting with choices is one a person can be shown a list for rather than
	// asked to know the spelling of.
	Choices []string
}

// Contract is one thing a component needs a plugin to be.
//
// The name is the contract, and One says how many plugins may fill it: a
// component that runs statements needs exactly one engine, while one that
// assembles a set of pages takes every content plugin that is enabled.
type Contract struct {
	// Name is what a plugin writes in `implements` to provide this.
	Name string
	// Description is what a person reading a list of plugins is shown, and is
	// the answer to "what is this for" that a list of hooks cannot give.
	Description string
	// One is whether exactly one enabled plugin may fill this. False is a
	// pipeline: every enabled plugin runs.
	One bool
	// Hooks are the hooks a plugin filling this answers. They are the same set
	// the component offers; naming them here is what makes a contract
	// checkable rather than a label.
	Hooks []string
}

// ExtendsTarget is one component a plugin plugs into, and the contracts and
// hooks of that component the plugin answers there.
//
// A plugin that extends one component has exactly one of these. A platform
// plugin that spans the toolchain — the target it builds for, the configuration
// it renders, the C it maps, the board it flashes — has one per component, so
// "what this plugin is" is a set of relations rather than a single one.
type ExtendsTarget struct {
	// Component is the name of the component this plugin extends.
	Component string
	// Implements are the contracts of that component this plugin fills.
	//
	// A plugin says what it is here rather than leaving a reader to infer it
	// from the hooks it answers: two plugins may answer the same hook and be
	// entirely different things, and this is what tells them apart when someone
	// decides whether to run one.
	Implements []string
	// Hooks are the hooks of that component this plugin answers. Every one of
	// them has to be offered by the component.
	Hooks []string
}

// Extends is how a plugin says which components it plugs into.
//
// The relation is declared rather than assumed: a tool loads only the plugins
// that name it, and a plugin that names a tool nothing offers is a plugin no
// run ever calls.
//
// A plugin may extend several components, which is what a platform mapping
// needs: the same platform expresses itself through rho-target, rho-kconfig,
// rho-cimport and rho-flash at once, and a plugin that could name only one of
// them could not be one plugin.
type Extends struct {
	// Targets are the components this plugin extends, one entry per component.
	Targets []ExtendsTarget
	// Platforms are the platform identities this plugin serves, as a platform
	// module declares them (`platform: zenith`). Empty means the plugin is
	// platform-agnostic and is offered whatever a project builds for.
	Platforms []string
	// Commands are the executables that answer a hook.
	Commands []Command

	// Component, Implements and Hooks are the single-target form.
	//
	// A plugin that extends one component writes `extends: rho-doc` and these
	// hold it directly. A plugin that extends several writes a list and Targets
	// holds the set; these then mirror the first, so a reader that only wants
	// to name "the component" still can. All answers come from All, which
	// prefers Targets.
	Component  string
	Implements []string
	Hooks      []string
}

// All is every component this plugin extends, whichever form the manifest used.
func (e *Extends) All() []ExtendsTarget {
	if e == nil {
		return nil
	}
	if len(e.Targets) > 0 {
		return e.Targets
	}
	if e.Component == "" {
		return nil
	}
	return []ExtendsTarget{{Component: e.Component, Implements: e.Implements, Hooks: e.Hooks}}
}

// Components are the names of the components this plugin extends.
func (e *Extends) Components() []string {
	out := []string{}
	for _, target := range e.All() {
		out = append(out, target.Component)
	}
	return out
}

// Answers reports whether the plugin answers a hook for a component.
//
// A component is named because a hook name is not unique across the toolchain:
// two components may both offer `render`, and a plugin that answers one of them
// has not answered the other. An empty component asks the single-target
// question, which is what a caller with one relation is asking.
func (e *Extends) Answers(component, hook string) bool {
	for _, target := range e.All() {
		if component != "" && target.Component != component {
			continue
		}
		for _, offered := range target.Hooks {
			if offered == hook {
				return true
			}
		}
	}
	return false
}

// AnswersHook reports whether the plugin answers a hook for any component.
func (e *Extends) AnswersHook(hook string) bool {
	return e.Answers("", hook)
}

// Command is one executable a plugin answers a hook with.
type Command struct {
	// Name identifies the command in a build log and in a protocol request.
	Name string
	// Run is the executable: a path, or a name found on PATH.
	Run string
	// Hook is the stage this command answers. An empty hook means the command
	// is named for it, which is what a one-hook plugin can rely on.
	Hook string
	// Component is the component this command answers, when a plugin extends
	// more than one. Empty means the command answers whichever component is
	// asking, which is what a single-target plugin means and is why existing
	// plugins need no change.
	Component string
}

// Stage is the hook a command answers.
func (c Command) Stage() string {
	if c.Hook != "" {
		return c.Hook
	}
	return c.Name
}

type Manifest struct {
	ModuleName string
	ModuleType string
	Version    string
	// Description is the one-line summary a module states about itself. A
	// page generated from the manifest shows it, so a module documents itself
	// in the place it already describes itself.
	Description string
	Main        string
	// Command is the name a module publishes to be dispatched to. Empty means
	// the module is not something a person types, which is what a library and a
	// plugin are.
	Command string
	// ImportPath is the dotted name Rho code uses to import this component's
	// Rho sources. Empty means the component exports no Rho library.
	ImportPath    string
	Features      []string
	Deps          []DepSpec
	BuildLanguage string
	BuildTargets  []string
	// BuildDir is where this project keeps everything a build generates. Empty
	// means `build`, which is the convention rather than a rule.
	BuildDir string
	// BuildOutputDir is where built binaries are written, inside BuildDir.
	BuildOutputDir string
	// BuildToolchain names the target a project builds for, as a rho-target
	// spec: an architecture, an MCU or a board. Empty means the project builds
	// for the machine it is built on, which is the target named `native`.
	//
	// It is parsed here rather than in whichever manager uses it because it is a
	// statement about the project, not about a dependency: it says what hardware
	// this is for, and every build of it is for that hardware whatever resolves
	// its dependencies.
	BuildToolchain string
	// BuildPresets are the boards this program is written for, when it is not
	// written for one.
	//
	// Read because a program written for several boards says so here, and
	// everything downstream needs the same list: a build picks one of them, a
	// generated editor configuration offers one entry per board so the build and
	// the session cannot be for different ones, and a flash command names one.
	// A list here is the answer rather than one preset, because a program that is
	// written for one board says so with `toolchain` and does not need this.
	BuildPresets []string
	BuildDefines []string
	// BuildGuardedSources are sources compiled only when a symbol is set.
	BuildGuardedSources []GuardedSource
	BuildIncludeDirs    []string
	BuildSourceDirs     []string
	BuildSources        []string
	// Connection is the native library a project links, and the headers it may
	// call. Nil means the project generates nothing that needs one.
	Connection *CConnection
	// Config is the Kconfig bridge a project declares: the fragments that
	// define its symbols, the values it sets, and which dialect the generated
	// headers are written in.
	Config *ConfigManifest
	// Requires are the runtime packages a built project links against, which is
	// a different thing from Deps: a dependency is resolved into the project,
	// and a requirement is a package the generated code calls into and that the
	// project's own manifest names for anyone who builds it later.
	//
	// They are read rather than carried because nothing here resolves a Rho
	// package. What the build does with them is refuse to stay silent about a
	// project that generated code calling into a package it never named, which
	// is a build that works on the machine that generated it and nowhere else.
	Requires []string
	// Hardware is what a platform declares about the parts it supports and
	// where those declarations are, or nil when it declared none.
	//
	// It is a contract rather than a convention. Reading a directory called
	// `targets` beside a dependency is a guess that reads as a promise; this is
	// the module saying which of its own directories a build should read.
	Hardware *HardwareManifest
	// Exports is what this module exports to a build system, or nil when it
	// declares none. A C dependency that declares it is telling a consumer where
	// its headers are instead of leaving a consumer to work it out.
	Exports *ExportsManifest
	// VendorDir is where this module keeps its vendor sources, or empty when it
	// declares none.
	VendorDir string
	// VendorRepos are the vendor repositories this module builds against, each
	// with the folder inside VendorDir it lives in.
	VendorRepos []VendorRef
	// Platform is the platform identity this module implements, such as
	// `zenith`, or empty when it names none.
	//
	// A platform module carries it so that the platform can be recognized
	// rather than assumed from a name or a repository: a project whose module
	// graph contains a module declaring `platform: zenith` is a project being
	// built for that platform, and the plugins that serve that identity are the
	// ones a run of the toolchain chooses between. A module that declares
	// nothing is not a platform, and a project with none of them is built the
	// standard way.
	Platform string
	// Platforms are the modules that implement this module's interfaces, or nil
	// when it declares none. A build looks at what the project declared first and
	// falls back to here, which is how a program written against the interfaces
	// alone finds a platform without being told which one to use.
	Platforms []PlatformRef
	// Testing is when this module's tests are eligible to be built, or nil when
	// it declared nothing. A module that declares no tests has none built, which
	// is why a build never has to guess whether a target it finds is meant to run.
	Testing *TestManifest
	// SQL is the database a project declares, or nil when it declared none.
	//
	// It is read here rather than in the component that generates from it,
	// because the build has to know a project has a database: the generated
	// statements call into a port, and a build that did not know would produce
	// a project whose code refers to functions nothing defines.
	SQL *SQLManifest
	// Docs is the doc-as-code this module carries. Nil means the module says
	// nothing about its documentation.
	Docs *DocManifest
	// Settings is what a person may configure this module with, and the file the
	// configured values live in. Nil means the module takes no configuration,
	// which is what a library is.
	//
	// It is not the Kconfig bridge, which is a project's build configuration and
	// is read by the generator: that is Config, and this is a person changing
	// how an installed tool behaves.
	Settings *ModuleSettings
	// Debug is what this project says about debugging the chip it is built for, and
	// it is the answer: a name here is used as it is written.
	Debug *DebugMapping
	// SystemDeps is what this module needs from the machine it runs on. A different
	// kind of requirement from `deps`, which this toolchain resolves: everything in
	// `deps` is another module of this toolchain and `rho-deps` fetches it, while
	// everything in here is a program or a permission somebody installs.
	//
	// Read and reported by `rho-doctor`, acted for by `rho-keeper`, and resolved by
	// neither: a report that installed things would be a report that changed the
	// machine it was reporting on.
	SystemDeps *sysdep.SystemDeps
	// Provides is what this module offers a plugin. Only a component has
	// something to offer.
	Provides *Provides
	// Extends is what this module plugs into. Only a plugin extends something.
	Extends *Extends
}

// DefaultBuildDir is where a project keeps generated files when its manifest does
// not say otherwise. It is a default rather than a rule: `build.dir` declares
// otherwise, because a project may want its build somewhere this toolchain has
// no opinion about.
const DefaultBuildDir = "build"

// DefaultOutputDir is where built binaries go when a project does not say.
const DefaultOutputDir = "output"

// OutputDirectory returns where this project's built binaries are written.
//
// Always inside the build directory: a binary is a build output, and a project
// that has several targets keeps them side by side rather than overwriting each
// other.
func (m *Manifest) OutputDirectory() string {
	directory := DefaultOutputDir
	if m != nil && m.BuildOutputDir != "" {
		directory = m.BuildOutputDir
	}
	// Inside the build directory, not beside it. A binary is a build output, and
	// the two are the same kind of thing: a project that kept its binaries beside
	// the folder it generates them into would have to clean two folders to start
	// again, and a `clean` that removed one would leave the other looking current.
	return filepath.Join(m.BuildDirectory(), directory)
}

// BuildDirectory returns where this project's generated files live.
//
// A declared path is absolute, or relative to the project. A relative one that
// climbs out was refused when the manifest was read, so what reaches here is
// either the project's own folder or a path of its own choosing.
func (m *Manifest) BuildDirectory() string {
	if m == nil || m.BuildDir == "" {
		return DefaultBuildDir
	}
	return m.BuildDir
}

func ManifestPath(projectRoot string) string {
	path := filepath.Join(projectRoot, "module.yml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return filepath.Join(projectRoot, "rho.mod")
}

// LoadManifest loads a rho.mod manifest file.
func LoadManifest(path string) (*Manifest, error) {
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		path = filepath.Join(path, "module.yml")
	} else if filepath.Base(path) == "rho.mod" {
		candidate := filepath.Join(filepath.Dir(path), "module.yml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			path = candidate
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if filepath.Ext(path) == ".yml" || filepath.Ext(path) == ".yaml" {
		return loadYAMLManifest(data)
	}

	m := &Manifest{}
	var currentDep *DepSpec
	// inDepsBlock tracks a `deps:` map so its entries are not mistaken for
	// other top-level keys.
	var inDepsBlock bool

	lines := strings.Split(string(data), "\n")
	for _, raw := range lines {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}

		// The manifest form for dependencies is a map, which is what the schema
		// declares and what modules actually write:
		//
		//	deps:
		//	  math: ">=1.0.0"
		//
		// Detecting the block needs the original indentation, so it is read from
		// the raw line before the text is trimmed.
		indented := len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t')
		if inDepsBlock && indented {
			if name, constraint, found := strings.Cut(ln, ":"); found && strings.TrimSpace(name) != "" {
				m.Deps = append(m.Deps, DepSpec{
					Name:    strings.TrimSpace(name),
					Version: VersionRange{Raw: unquote(strings.TrimSpace(constraint))},
				})
				continue
			}
		}
		if ln == "}" || !indented {
			inDepsBlock = false
		}
		if !inDepsBlock && (ln == "deps:" || ln == "deps") {
			inDepsBlock = true
			continue
		}

		if currentDep == nil && strings.HasPrefix(ln, "module ") {
			m.ModuleName = parseValue(strings.TrimPrefix(ln, "module"))
			continue
		}

		if currentDep == nil && strings.HasPrefix(ln, "version ") {
			m.Version = parseValue(strings.TrimPrefix(ln, "version"))
			continue
		}

		if currentDep == nil && (strings.HasPrefix(ln, "main ") || strings.HasPrefix(ln, "main:")) {
			value := strings.TrimPrefix(ln, "main")
			value = strings.TrimSpace(strings.TrimPrefix(value, ":"))
			m.Main = unquote(strings.TrimPrefix(strings.TrimSpace(value), "="))
			continue
		}

		if currentDep == nil && strings.HasPrefix(ln, "feature ") {
			feature := parseValue(strings.TrimPrefix(ln, "feature"))
			if feature != "" {
				m.Features = append(m.Features, feature)
			}
			continue
		}

		if strings.HasPrefix(ln, "dep ") {
			name := strings.TrimSpace(strings.TrimPrefix(ln, "dep"))
			name = strings.TrimSuffix(name, "{}")
			name = strings.TrimSuffix(name, "{")
			name = strings.TrimSpace(name)
			m.Deps = append(m.Deps, DepSpec{Name: name})
			currentDep = &m.Deps[len(m.Deps)-1]
			continue
		}

		if strings.TrimSpace(ln) == "}" {
			currentDep = nil
			continue
		}

		if currentDep != nil {
			parseDepField(currentDep, ln)
		}
	}

	// Parse version ranges now so Constraints are populated.
	for i := range m.Deps {
		if m.Deps[i].GitTag != "" && m.Deps[i].GitBranch != "" {
			return nil, fmt.Errorf("dependency %s cannot specify both tag and branch", m.Deps[i].Name)
		}
		cs, err := ParseVersionRange(m.Deps[i].Version.Raw)
		if err != nil {
			return nil, fmt.Errorf("parse version range for %s: %w", m.Deps[i].Name, err)
		}
		m.Deps[i].Version.Constraints = cs
	}

	return m, nil
}

// LoadDepManifest loads a manifest from a dependency's directory.
func LoadDepManifest(path string) (*Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat dependency: %w", err)
	}
	if info.IsDir() {
		path = filepath.Join(path, "module.yml")
		if _, statErr := os.Stat(path); statErr != nil {
			path = filepath.Join(filepath.Dir(path), "rho.mod")
		}
	}
	return LoadManifest(path)
}

func loadYAMLManifest(data []byte) (*Manifest, error) {
	doc, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse module.yml: %w", err)
	}

	// Schema validation
	moduleType := yamlString(doc.Map["module"], "type")
	scResult := schema.ValidateModuleManifest(doc, moduleType)
	if scResult.HasErrors() {
		return nil, fmt.Errorf("schema validation failed:\n%s", scResult.ErrorString())
	}

	m := &Manifest{}
	module := doc.Map["module"]
	if module.Kind == yaml.KindMap {
		m.ModuleName = yamlString(module, "name")
		m.ModuleType = yamlString(module, "type")
		m.Version = yamlString(module, "version")
		m.Command = yamlString(module, "command")
		m.ImportPath = yamlString(module, "import_path")
		m.Description = yamlString(module, "description")
	}
	build := doc.Map["build"]
	if build.Kind == yaml.KindMap {
		m.BuildLanguage = yamlString(build, "language")
		m.BuildDir = yamlString(build, "dir")
		m.BuildOutputDir = yamlString(build, "output_dir")
		if err := escape("build.dir", m.BuildDir); err != nil {
			return nil, fmt.Errorf("%s: %w", m.ModuleName, err)
		}
		m.BuildTargets = yamlStrings(build.Map["targets"])
		m.BuildToolchain = yamlString(build, "toolchain")
		m.BuildPresets = yamlStrings(build.Map["presets"])
		m.BuildIncludeDirs = yamlStrings(build.Map["include_dirs"])
		m.BuildSourceDirs = yamlStrings(build.Map["source_dirs"])
		m.BuildSources = yamlStrings(build.Map["sources"])
		m.BuildDefines = yamlStrings(build.Map["defines"])
		m.BuildGuardedSources = parseGuardedSources(build.Map["guarded_sources"])
	}

	m.Exports = parseExports(doc.Map["exports"])
	m.Testing = parseTesting(doc.Map["testing"])
	m.Platforms = parsePlatforms(doc.Map["platforms"])
	m.Platform = yamlString(doc, "platform")

	if vendor, ok := doc.Map["vendor"]; ok && vendor.Kind == yaml.KindMap {
		m.VendorDir = yamlString(vendor, "dir")
		m.VendorRepos = parseVendorRepos(vendor.Map["repos"])
	}

	// Where a platform keeps its hardware declarations, if it declares any.
	if hardware, ok := doc.Map["hardware"]; ok && hardware.Kind == yaml.KindMap {
		declared := &HardwareManifest{
			Boards:  yamlString(hardware, "boards"),
			Presets: yamlString(hardware, "presets"),
			Target:  yamlString(hardware, "target"),
			Targets: yamlString(hardware, "targets"),
			Kconfig: yamlString(hardware, "kconfig"),
		}
		for _, one := range []struct {
			kind string
			path string
		}{
			{"hardware.presets", declared.Presets},
			{"hardware.targets", declared.Targets},
			{"hardware.kconfig", declared.Kconfig},
		} {
			if err := escape(one.kind, one.path); err != nil {
				return nil, fmt.Errorf("%s: %w", m.ModuleName, err)
			}
		}
		m.Hardware = declared
	}
	// The configuration bridge is a top-level section, so it is read here
	// rather than with the build fields: a project that declares a build and one
	// that only declares a configuration are both possible, and a section read
	// inside another would be read only when that other is there.
	m.Connection = parseCConnection(doc.Map["c_connection"])
	if m.Connection != nil {
		// A library the project links is the platform's, and the platform says
		// where it put it. A project reaching out of itself to say so is naming
		// this checkout rather than the platform.
		if err := escape("c_connection.path", m.Connection.Path); err != nil {
			return nil, fmt.Errorf("%s: %w", m.ModuleName, err)
		}
		for _, dir := range m.Connection.IncludeDirs {
			if err := escape("c_connection.include_dirs entry", dir); err != nil {
				return nil, fmt.Errorf("%s: %w", m.ModuleName, err)
			}
		}
	}
	m.Requires = parseRequires(doc.Map["requires"])
	m.Settings = parseSettingsManifest(doc.Map["settings"])
	m.Debug = parseDebugManifest(doc.Map["debug"])
	// A block read out of the document's own map rather than from the document: the
	// parser wants the block, and this is where the manifest's keys live.
	m.SystemDeps = sysdep.Parse(yaml.Value{Kind: yaml.KindMap, Map: doc.Map})
	m.Config = parseConfigManifest(doc.Map["config"])
	m.SQL = parseSQLManifest(doc.Map["sql"])
	m.Main = yamlString(doc, "main")
	m.Features = yamlStrings(doc.Map["features"])

	// `deps` is a map of name to version constraint, which is what the schema
	// declares:
	//
	//	deps:
	//	  math: ">=1.0.0"
	//
	// The name is kept whole and the constraint becomes the version range, so
	// the dependency system and the import resolution see the same thing.
	for name, constraint := range doc.Map["deps"].Map {
		spec := DepSpec{Name: name}
		spec.Version.Raw = constraint.Str
		m.Deps = append(m.Deps, spec)
	}
	sort.Slice(m.Deps, func(i, j int) bool { return m.Deps[i].Name < m.Deps[j].Name })

	for _, item := range doc.Map["depends"].List {
		if item.Kind != yaml.KindMap {
			return nil, fmt.Errorf("depends entries must be maps")
		}
		dep := DepSpec{
			Name: yamlString(item, "name"), Type: DepType(yamlString(item, "type")),
			GitURL: yamlString(item, "git"), EnvVar: yamlString(item, "env"),
			GitTag: yamlString(item, "tag"), GitBranch: yamlString(item, "branch"),
			Path: yamlString(item, "path"), Include: yamlString(item, "include"), Src: yamlString(item, "src"),
		}
		dep.Version.Raw = yamlString(item, "version")
		if dep.GitTag != "" && dep.GitBranch != "" {
			return nil, fmt.Errorf("dependency %s cannot specify both tag and branch", dep.Name)
		}
		cs, parseErr := ParseVersionRange(dep.Version.Raw)
		if parseErr != nil {
			return nil, fmt.Errorf("parse version range for %s: %w", dep.Name, parseErr)
		}
		dep.Version.Constraints = cs
		m.Deps = append(m.Deps, dep)
	}
	m.Docs = parseDocManifest(doc.Map["docs"])
	m.Provides = parseProvides(doc.Map["provides"])
	m.Extends = parseExtends(doc)
	return m, nil
}

// escape reports whether a declared path reaches outside the module that
// declared it, and says so in terms a reader can act on.
//
// A path a module names about its own tree is absolute or a child of where the
// module is. `../../` is not: it is a statement about where the module happened
// to be put on one machine, and it stops being true the moment the module moves,
// is fetched, or is published. A path like that reads as a declaration of what a
// dependency is when it is a statement about this checkout, which is how an
// application ends up naming `../../../platform/build/lib.a` and breaking on
// everyone else's machine. What belongs to a dependency is declared by that
// dependency; what belongs to a project is inside the project.
func escape(kind, path string) error {
	if path == "" || filepath.IsAbs(path) {
		return nil
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return fmt.Errorf("%s %q reaches outside the module that declared it; name an absolute path, or a folder inside the module, or let the dependency that owns it declare it", kind, path)
		}
	}
	return nil
}

// CConnection is the link between a Rho project and a native library.
//
// A project that calls C says which library it links, which headers it may
// call, and which vendor library implements each build target: a target is what
// a person names (`--target stm32`) and the library is what a build links
// (`vendor_stm32`).
type CConnection struct {
	// Library is the CMake target every build of this project links.
	Library string
	// Path is a compiled library to link on a host run, where there is no CMake
	// to resolve a target name. Empty for a project that is only ever built by
	// CMake, which is the normal case.
	Path string
	// Headers are the C headers the foreign interface is derived from.
	Headers []string
	// IncludeDirs are passed to the compiler and used to read a header by path.
	IncludeDirs []string
	// Defines are the macros a build always has.
	Defines []string
	// Variants are the build targets that change what is linked, keyed by the
	// target name a person types.
	Variants map[string]CVariant
}

// CVariant is one build target of a connected project: the vendor libraries it
// links, and the macros that select the implementation at compile time.
type CVariant struct {
	// Target is the vendor CMake target, such as `vendor_stm32`.
	Target string
	// Libraries are the vendor libraries, when there is more than one.
	Libraries []string
	// Defines select the implementation in the sources.
	Defines []string
	// IncludeDirs are the headers the vendor adds.
	IncludeDirs []string
}

// LinkLibraries are the libraries a variant links: its own target first, then
// anything else it names. A build that links nothing extra links the target.
func (v CVariant) LinkLibraries() []string {
	libs := []string{}
	if v.Target != "" {
		libs = append(libs, v.Target)
	}
	libs = append(libs, v.Libraries...)
	return libs
}

// Variant returns the build target a name names, and whether there is one.
func (c *CConnection) Variant(name string) (CVariant, bool) {
	if c == nil {
		return CVariant{}, false
	}
	variant, found := c.Variants[name]
	return variant, found
}

// ConfigManifest is the Kconfig bridge a project declares.
//
// The fragments are Kconfig files: they define the symbols a project may set and
// the defaults it starts from. The values are the symbols this project sets, and
// a module of Rho constants may set more. The dialect says which headers the
// generated build writes, because the same symbols are spelled differently by the
// tools that read them.
//
// It is declared at the top level of a manifest rather than under build: what a
// project configures is a property of the project, not of one target, and a
// project with a C target and a Go target is configured the same way for both.
type ConfigManifest struct {
	// Fragments are the Kconfig files that define the symbols, relative to the
	// project root.
	Fragments []string
	// ValuesFile is the file this project writes the symbols it sets in,
	// relative to the project root.
	ValuesFile string
	// Values are the symbols this project sets, as text: a bool is true or
	// false, an int or hex is its digits, a string is the string.
	Values map[string]string
	// Dialect is which headers the build writes: linux, espidf or freertos.
	Dialect string
	// Module is a Rho module of typed constants that sets symbols, relative to
	// the project root. It is read with the compiler's own front end, so a
	// value written in Rho is a value the compiler would accept.
	Module string
}

// parseRequires reads the runtime packages a project links against.
//
// A requirement is a name, and the version beside it is what the project was
// written against. Neither is resolved here: they are what the module states
// about itself, and the name is what a warning is about when it is absent.
func parseRequires(node yaml.Value) []string {
	if node.Kind != yaml.KindList {
		return nil
	}
	out := make([]string, 0, len(node.List))
	for _, entry := range node.List {
		if entry.Kind != yaml.KindMap {
			// A requirement written as a bare name is still a requirement, and
			// refusing it would be refusing a manifest the schema accepts.
			if name := strings.TrimSpace(entry.Str); name != "" {
				out = append(out, name)
			}
			continue
		}
		if name := strings.TrimSpace(entry.Map["name"].Str); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// SQLManifest is a project's declaration that it has a database.
//
// Everything here is a location or a name. None of it is a setting: the driver
// decides what its own settings are, so a project does not have to know that a
// database is a file on one engine and a connection string on another.
type SQLManifest struct {
	// Driver is the engine plugin the generated code runs against, as it is
	// named in the manifest.
	Driver string
	// Schema and Queries are the files the SQL is written in, relative to the
	// project root. Both are needed: the schema says what the tables are, which
	// is what a result is typed from, and the queries say which of them run.
	Schema  string
	Queries string
	// Out is the directory the generated Rho is written to, relative to the
	// project root. Empty means the queries file's own directory.
	Out string
	// Package is the Rho module the generated file belongs to. Empty means the
	// project's own name.
	Package string
}

// Declared reports whether a project said it has a database.
func (s *SQLManifest) Declared() bool {
	return s != nil && (s.Driver != "" || s.Schema != "" || s.Queries != "")
}

// SchemaPath is the schema file, resolved against a project root.
func (s *SQLManifest) SchemaPath(projectRoot string) string {
	return projectRootPath(projectRoot, s.Schema)
}

// QueriesPath is the queries file, resolved against a project root.
func (s *SQLManifest) QueriesPath(projectRoot string) string {
	return projectRootPath(projectRoot, s.Queries)
}

// OutPath is the directory the generated Rho goes in, resolved against a
// project root. With no `out`, it is the queries file's own directory, so a
// project that keeps its SQL in one place gets its generated code beside it.
func (s *SQLManifest) OutPath(projectRoot string) string {
	if s == nil {
		return ""
	}
	if s.Out != "" {
		return projectRootPath(projectRoot, s.Out)
	}
	// With no `out`, the generated code is written beside the files it was
	// generated from. The statements are preferred when there are any, and the
	// schema otherwise, because a project that declared tables and no statements
	// still has somewhere its generated code belongs.
	if s.Queries != "" {
		return filepath.Dir(projectRootPath(projectRoot, s.Queries))
	}
	if s.Schema != "" {
		return filepath.Dir(projectRootPath(projectRoot, s.Schema))
	}
	return ""
}

// projectRootPath resolves a manifest-relative path against a project root.
func projectRootPath(projectRoot, relative string) string {
	if relative == "" {
		return ""
	}
	if filepath.IsAbs(relative) {
		return filepath.Clean(relative)
	}
	return filepath.Clean(filepath.Join(projectRoot, filepath.FromSlash(relative)))
}

// parseSQLManifest reads the database a project declares.
func parseSQLManifest(node yaml.Value) *SQLManifest {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil
	}
	sql := &SQLManifest{
		Driver:  yamlString(node, "driver"),
		Schema:  yamlString(node, "schema"),
		Queries: yamlString(node, "queries"),
		Out:     yamlString(node, "out"),
		Package: yamlString(node, "package"),
	}
	if !sql.Declared() {
		// A section that names nothing is no section, rather than one that
		// generates from files that were never given.
		return nil
	}
	return sql
}

// parseCConnection reads the native library a project links.
func parseCConnection(node yaml.Value) *CConnection {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil
	}
	connection := &CConnection{
		Library:     yamlString(node, "library"),
		Path:        yamlString(node, "path"),
		Headers:     yamlStrings(node.Map["headers"]),
		IncludeDirs: yamlStrings(node.Map["include_dirs"]),
		Defines:     yamlStrings(node.Map["defines"]),
		Variants:    map[string]CVariant{},
	}
	if variants, ok := node.Map["variants"]; ok && variants.Kind == yaml.KindMap {
		for name, value := range variants.Map {
			if value.Kind != yaml.KindMap {
				continue
			}
			connection.Variants[name] = CVariant{
				Target:      yamlString(value, "target"),
				Libraries:   yamlStrings(value.Map["libraries"]),
				Defines:     yamlStrings(value.Map["defines"]),
				IncludeDirs: yamlStrings(value.Map["include_dirs"]),
			}
		}
	}
	return connection
}

// parseConfigManifest reads the Kconfig bridge from a manifest.
func parseConfigManifest(node yaml.Value) *ConfigManifest {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil
	}
	config := &ConfigManifest{
		Fragments:  yamlStrings(node.Map["kconfig_fragments"]),
		ValuesFile: yamlString(node, "values_file"),
		Dialect:    yamlString(node, "dialect"),
		Module:     yamlString(node, "module"),
	}
	if values, ok := node.Map["values"]; ok && values.Kind == yaml.KindMap {
		config.Values = map[string]string{}
		for key, value := range values.Map {
			config.Values[key] = value.Str
		}
	}
	if len(config.Fragments) == 0 && len(config.Values) == 0 && config.Module == "" && config.Dialect == "" {
		// A section that declares nothing is no section, rather than one that
		// silently configures nothing. A section that names only a dialect is
		// read, because a manifest said something: a project with no fragments
		// and a dialect is a bridge the merge will report on, not a bridge to
		// be dropped here.
		return nil
	}
	return config
}

// parseDebugManifest reads what a project says about debugging its chip.
func parseDebugManifest(node yaml.Value) *DebugMapping {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil
	}
	// A map of board to file. Nested rather than flat, because it is nested: which
	// file is for which board is the whole content of it.
	mapping := &DebugMapping{}
	if svd := node.Map["svd"]; svd.Kind == yaml.KindMap {
		for board, path := range svd.Map {
			if declared := strings.TrimSpace(yamlUnquote(path.Str)); declared != "" && board != "" {
				if mapping.SVD == nil {
					mapping.SVD = map[string]string{}
				}
				mapping.SVD[board] = declared
			}
		}
	}
	if !mapping.Declared() {
		return nil
	}
	return mapping
}

// parseSettingsManifest reads what a module can be configured with.
//
// A file with no settings is still a file: it is where the values go, and a
// module that takes configuration it has not described yet is one a person
// cannot be shown a form for.
func parseSettingsManifest(node yaml.Value) *ModuleSettings {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil
	}
	settings := &ModuleSettings{File: yamlUnquote(node.Map["file"].Str)}
	for _, entry := range node.Map["settings"].List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		setting := Setting{
			Name:        yamlUnquote(entry.Map["name"].Str),
			Description: yamlUnquote(entry.Map["description"].Str),
			Default:     yamlUnquote(entry.Map["default"].Str),
		}
		for _, choice := range entry.Map["choices"].List {
			if choice.Str != "" {
				setting.Choices = append(setting.Choices, choice.Str)
			}
		}
		if setting.Name == "" {
			continue
		}
		settings.Settings = append(settings.Settings, setting)
	}
	if settings.File == "" && len(settings.Settings) == 0 {
		return nil
	}
	return settings
}

// parseProvides reads what a component offers a plugin.
func parseProvides(node yaml.Value) *Provides {
	if node.Kind != yaml.KindMap {
		return nil
	}
	provides := &Provides{
		Hooks:     yamlStrings(node.Map["hooks"]),
		Contracts: parseContracts(node.Map["contracts"]),
	}
	if len(provides.Hooks) == 0 && len(provides.Contracts) == 0 {
		// A section with neither says nothing, which is how a module says it
		// offers no place for a plugin to stand.
		return nil
	}
	return provides
}

// parseContracts reads what a component needs a plugin to be.
func parseContracts(node yaml.Value) []Contract {
	if node.Kind != yaml.KindList {
		return nil
	}
	out := make([]Contract, 0, len(node.List))
	for _, entry := range node.List {
		if entry.Kind != yaml.KindMap {
			continue
		}
		contract := Contract{
			Name:        yamlUnquote(entry.Map["name"].Str),
			Description: yamlUnquote(entry.Map["description"].Str),
			// `one: false` is written to say a contract is a pipeline, and it
			// means the same as leaving it out. A contract that says nothing
			// takes every plugin that offers it, because a component that
			// assembles something is finished by contributions rather than
			// chosen between.
			One:   yamlUnquote(entry.Map["one"].Str) == "true",
			Hooks: yamlStrings(entry.Map["hooks"]),
		}
		if contract.Name == "" {
			continue
		}
		out = append(out, contract)
	}
	return out
}

// parseExtends reads what a plugin plugs into.
//
// Two forms are accepted. The one a single-component plugin writes names a
// component directly:
//
//	extends: rho-doc
//	implements: [contents]
//	hooks: [index, page]
//
// The one a plugin that spans the toolchain writes is a list, because there is
// no single component to name:
//
//	extends:
//	  - component: rho-target
//	    implements: [platform]
//	    hooks: [spec]
//	  - component: rho-kconfig
//	    implements: [platform]
//	    hooks: [render]
//
// Both are read into the same shape; the first is the second with one entry.
func parseExtends(doc yaml.Value) *Extends {
	node := doc.Map["extends"]
	if node.Kind == yaml.KindNull {
		return nil
	}
	extension := &Extends{Platforms: yamlStrings(doc.Map["serves"])}

	switch node.Kind {
	case yaml.KindString:
		component := yamlUnquote(node.Str)
		if component == "" {
			return nil
		}
		extension.Component = component
		extension.Implements = yamlStrings(doc.Map["implements"])
		extension.Hooks = yamlStrings(doc.Map["hooks"])
	case yaml.KindList:
		for _, entry := range node.List {
			if entry.Kind != yaml.KindMap {
				continue
			}
			target := ExtendsTarget{
				Component:  yamlString(entry, "component"),
				Implements: yamlStrings(entry.Map["implements"]),
				Hooks:      yamlStrings(entry.Map["hooks"]),
			}
			if target.Component == "" {
				continue
			}
			extension.Targets = append(extension.Targets, target)
		}
		if len(extension.Targets) == 0 {
			return nil
		}
		// Mirror the first target so a reader that only asks for "the
		// component" has an answer rather than none.
		extension.Component = extension.Targets[0].Component
		extension.Implements = extension.Targets[0].Implements
		extension.Hooks = extension.Targets[0].Hooks
	default:
		return nil
	}

	for _, item := range doc.Map["commands"].List {
		if item.Kind != yaml.KindMap {
			continue
		}
		extension.Commands = append(extension.Commands, Command{
			Name:      yamlString(item, "name"),
			Run:       yamlString(item, "run"),
			Hook:      yamlString(item, "hook"),
			Component: yamlString(item, "component"),
		})
	}
	return extension
}

// parseDocManifest reads the doc-as-code a module carries. A missing or empty
// section yields nil, which is how a module says it carries no documentation.
func parseDocManifest(node yaml.Value) *DocManifest {
	if node.Kind != yaml.KindMap || len(node.Map) == 0 {
		return nil
	}
	return &DocManifest{
		Title:   yamlString(node, "title"),
		Section: yamlString(node, "section"),
		Order:   yamlString(node, "order"),
		Sources: yamlStrings(node.Map["sources"]),
		Index:   yamlString(node, "index"),
	}
}

func yamlString(value yaml.Value, key string) string {
	if item, ok := value.Map[key]; ok {
		return yamlUnquote(item.Str)
	}
	return ""
}

// yamlUnquote removes the quotes of a quoted value, using the one rule the
// loader uses: a value whose first and last characters are quotes is quoted
// only when no quote of its own sits between them, which is what keeps
// `"prog" --flag "arg"` a command rather than one word.
func yamlUnquote(value string) string {
	return yaml.Unquote(value)
}

func yamlStrings(value yaml.Value) []string {
	values := []string{}
	for _, item := range value.List {
		if item.Kind == yaml.KindString {
			values = append(values, yamlUnquote(item.Str))
		}
	}
	return values
}

func parseValue(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "="))
	return unquote(s)
}

func parseDepField(dep *DepSpec, ln string) {
	switch {
	case strings.HasPrefix(ln, "type "):
		dep.Type = DepType(unquote(strings.TrimPrefix(ln, "type =")))
	case strings.HasPrefix(ln, "git "):
		dep.GitURL = unquote(strings.TrimPrefix(ln, "git ="))
	case strings.HasPrefix(ln, "tag "):
		dep.GitTag = unquote(strings.TrimPrefix(ln, "tag ="))
	case strings.HasPrefix(ln, "branch "):
		dep.GitBranch = unquote(strings.TrimPrefix(ln, "branch ="))
	case strings.HasPrefix(ln, "version "):
		dep.Version.Raw = unquote(strings.TrimPrefix(ln, "version ="))
	case strings.HasPrefix(ln, "env "):
		dep.EnvVar = unquote(strings.TrimPrefix(ln, "env ="))
	case strings.HasPrefix(ln, "path "):
		dep.Path = unquote(strings.TrimPrefix(ln, "path ="))
	case strings.HasPrefix(ln, "include "):
		dep.Include = unquote(strings.TrimPrefix(ln, "include ="))
	case strings.HasPrefix(ln, "src "):
		dep.Src = unquote(strings.TrimPrefix(ln, "src ="))
	}
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), "\"")
}

// TestManifest says when this module's tests are eligible to be built.
//
// The two kinds are separate because they are different programs against
// different runtimes: a host test links a hosted C library and runs on the
// machine doing the building, and an on-device test links the bare-metal one and
// runs on the hardware. A module that has both has to be able to say that a build
// for one target gets one of them and not the other.
type TestManifest struct {
	// HostTests run on the machine doing the building.
	HostTests TestKind
	// TargetTests run on the device the build is for.
	TargetTests TestKind
}

// TestKind is one kind of test and the target it is for.
type TestKind struct {
	// Enabled says the module declares this kind at all. A kind that is not
	// declared is not built, whatever the target.
	Enabled bool
	// TargetConstraint names the target the tests are for, as the name a target
	// is asked for by. Empty means the kind applies to any target.
	TargetConstraint string
}

// AppliesTo reports whether this kind of test is built for a named target.
//
// A constraint names one target rather than describing a class of them, so that
// "the host" is `native` because that is what the host is called, and a module
// that wants its tests on two targets says so with two entries rather than
// inventing a category a build would then have to know how to interpret.
func (k TestKind) AppliesTo(targetName string) bool {
	if !k.Enabled {
		return false
	}
	if k.TargetConstraint == "" {
		return true
	}
	return k.TargetConstraint == targetName
}

// Declared reports whether this module declared anything about its tests.
func (t *TestManifest) Declared() bool {
	return t != nil && (t.HostTests.Enabled || t.TargetTests.Enabled)
}

// ExportsManifest is what a module exports, by build system.
type ExportsManifest struct {
	Cmake *CMakeExports
}

// parseExports reads a module's export declarations.
func parseExports(node yaml.Value) *ExportsManifest {
	if node.Kind != yaml.KindMap {
		return nil
	}
	exports := &ExportsManifest{}
	if cmake, ok := node.Map["cmake"]; ok && cmake.Kind == yaml.KindMap {
		declared := &CMakeExports{
			Target:        yamlString(cmake, "target"),
			IncludeDirs:   yamlStrings(cmake.Map["include_dirs"]),
			Defines:       yamlStrings(cmake.Map["defines"]),
			LinkLibraries: yamlStrings(cmake.Map["link_libraries"]),
			Vars:          map[string]string{},
		}
		if vars, ok := cmake.Map["vars"]; ok && vars.Kind == yaml.KindMap {
			for name, value := range vars.Map {
				if value.Kind == yaml.KindString {
					declared.Vars[name] = value.Str
				}
			}
		}
		if declared.Target != "" || len(declared.IncludeDirs) > 0 || len(declared.Defines) > 0 ||
			len(declared.LinkLibraries) > 0 || len(declared.Vars) > 0 {
			exports.Cmake = declared
		}
	}
	if exports.Cmake == nil {
		return nil
	}
	return exports
}

// parseTesting reads what a module declared about its tests.
func parseTesting(node yaml.Value) *TestManifest {
	if node.Kind != yaml.KindMap {
		return nil
	}
	testing := &TestManifest{
		HostTests:   parseTestKind(node.Map["host_tests"]),
		TargetTests: parseTestKind(node.Map["target_tests"]),
	}
	if !testing.Declared() {
		return nil
	}
	return testing
}

// parseTestKind reads one kind of test declaration.
func parseTestKind(node yaml.Value) TestKind {
	if node.Kind != yaml.KindMap {
		return TestKind{}
	}
	return TestKind{
		Enabled:          node.Map["enabled"].Str == "true",
		TargetConstraint: yamlString(node, "target_constraint"),
	}
}

// PlatformRef is a module that implements a set of interfaces.
type PlatformRef struct {
	// Name is the module's name, which is what a report calls it.
	Name string
	// Path is where it is checked out, relative to the module that names it.
	Path string
	// Git and Branch are where it is fetched from when it is not checked out.
	Git    string
	Branch string
}

// parsePlatforms reads the platforms a module enumerates.
func parsePlatforms(node yaml.Value) []PlatformRef {
	if node.Kind != yaml.KindList {
		return nil
	}
	platforms := []PlatformRef{}
	for _, item := range node.List {
		if item.Kind != yaml.KindMap {
			continue
		}
		platforms = append(platforms, PlatformRef{
			Name:   yamlString(item, "name"),
			Path:   yamlString(item, "path"),
			Git:    yamlString(item, "git"),
			Branch: yamlString(item, "branch"),
		})
	}
	return platforms
}

// VendorRef is one vendor repository a module builds against, and where it is.
type VendorRef struct {
	// Name is what a preset's dependency calls this repository. The two are
	// joined by this name rather than by a path either of them wrote, so that
	// moving a checkout does not change the contract.
	Name string
	// Path is the folder inside the vendor directory, relative to it.
	Path string
	// Git and Branch are where it is fetched from when it is not already there.
	Git    string
	Branch string
}

// RepositoryPath returns where a named repository is, in this module.
func (h *VendorManifest) RepositoryPath(name string) (string, bool) {
	for _, repo := range h.Repos {
		if repo.Name == name {
			return repo.Path, true
		}
	}
	return "", false
}

// VendorManifest is where a module keeps its vendor sources.
type VendorManifest struct {
	// Dir is where they are kept, relative to the module.
	Dir string
	// Repos are the individual repositories, each with its own folder in Dir.
	Repos []VendorRef
}

// Declared reports whether this module says anything about its vendor sources.
func (h *VendorManifest) Declared() bool {
	return h != nil && (h.Dir != "" || len(h.Repos) > 0)
}

// parseVendorRepos reads the repositories a module declares.
func parseVendorRepos(node yaml.Value) []VendorRef {
	if node.Kind != yaml.KindList {
		return nil
	}
	repos := []VendorRef{}
	for _, item := range node.List {
		if item.Kind != yaml.KindMap {
			continue
		}
		repos = append(repos, VendorRef{
			Name:   yamlString(item, "name"),
			Path:   yamlString(item, "path"),
			Git:    yamlString(item, "git"),
			Branch: yamlString(item, "branch"),
		})
	}
	return repos
}

// GuardedSource is a set of sources compiled only when a symbol is set.
type GuardedSource struct {
	// Config is the symbol, without the CONFIG_ prefix.
	Config string
	// Sources are the files, by name, that the symbol governs.
	Sources []string
}

// parseGuardedSources reads the sources a module guards on a symbol.
func parseGuardedSources(node yaml.Value) []GuardedSource {
	if node.Kind != yaml.KindList {
		return nil
	}
	guarded := []GuardedSource{}
	for _, item := range node.List {
		if item.Kind != yaml.KindMap {
			continue
		}
		symbol := strings.TrimPrefix(readScalar(item.Map["config"]), "CONFIG_")
		if symbol == "" {
			continue
		}
		guarded = append(guarded, GuardedSource{
			Config:  symbol,
			Sources: yamlStrings(item.Map["sources"]),
		})
	}
	return guarded
}

// readScalar is a mapping value's text, or "" when it is not a scalar.
func readScalar(value yaml.Value) string {
	if value.Kind != yaml.KindString {
		return ""
	}
	return value.Str
}

// VendorRoots are the directories a project's dependencies keep their vendor code
// in, for the project at root.
//
// A file a platform vendors lives beside that platform, not beside the program
// that uses it: a vendor's register map is megabytes of XML that every program
// built for the chip would otherwise carry a copy of. So a path a project declares
// is looked for in the project itself and then in each of the platforms it depends
// on — which is the difference between a platform that vendors a register map once
// and a platform that cannot.
//
// Only C dependencies are looked in: those are the ones a program is linked
// against, and therefore the ones whose vendor trees a build already reads. A module
// dependency has no code of its own to look in.
func (m *Manifest) VendorRoots(projectRoot string) []string {
	if m == nil {
		return nil
	}
	roots := []string{}
	for _, dependency := range m.Deps {
		if dependency.Type != DepTypeC || dependency.Path == "" {
			continue
		}
		root := dependency.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(projectRoot, root)
		}
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	return roots
}
