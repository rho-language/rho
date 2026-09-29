package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/schema"
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

// Extends is how a plugin says which component it plugs into.
//
// The relation is declared rather than assumed: a tool loads only the plugins
// that name it, and a plugin that names a tool nothing offers is a plugin no
// run ever calls.
type Extends struct {
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
	// Commands are the executables that answer a hook.
	Commands []Command
}

// Answers reports whether the plugin answers a hook.
func (e *Extends) Answers(hook string) bool {
	for _, offered := range e.Hooks {
		if offered == hook {
			return true
		}
	}
	return false
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
	ImportPath       string
	Features         []string
	Deps             []DepSpec
	BuildLanguage    string
	BuildTargets     []string
	BuildIncludeDirs []string
	BuildSourceDirs  []string
	BuildSources     []string
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
	// Provides is what this module offers a plugin. Only a component has
	// something to offer.
	Provides *Provides
	// Extends is what this module plugs into. Only a plugin extends something.
	Extends *Extends
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
		m.BuildTargets = yamlStrings(build.Map["targets"])
		m.BuildIncludeDirs = yamlStrings(build.Map["include_dirs"])
		m.BuildSourceDirs = yamlStrings(build.Map["source_dirs"])
		m.BuildSources = yamlStrings(build.Map["sources"])
	}
	// The configuration bridge is a top-level section, so it is read here
	// rather than with the build fields: a project that declares a build and one
	// that only declares a configuration are both possible, and a section read
	// inside another would be read only when that other is there.
	m.Connection = parseCConnection(doc.Map["c_connection"])
	m.Requires = parseRequires(doc.Map["requires"])
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

// CConnection is the link between a Rho project and a native library.
//
// A project that calls C says which library it links, which headers it may
// call, and which vendor library implements each build target: a target is what
// a person names (`--target stm32`) and the library is what a build links
// (`robohub_stm32`).
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
	// Target is the vendor CMake target, such as robohub_stm32.
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
		Fragments: yamlStrings(node.Map["kconfig_fragments"]),
		Dialect:   yamlString(node, "dialect"),
		Module:    yamlString(node, "module"),
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
func parseExtends(doc yaml.Value) *Extends {
	component := yamlUnquote(doc.Map["extends"].Str)
	if component == "" {
		return nil
	}
	extension := &Extends{
		Component:  component,
		Implements: yamlStrings(doc.Map["implements"]),
		Hooks:      yamlStrings(doc.Map["hooks"]),
	}
	for _, item := range doc.Map["commands"].List {
		if item.Kind != yaml.KindMap {
			continue
		}
		extension.Commands = append(extension.Commands, Command{
			Name: yamlString(item, "name"),
			Run:  yamlString(item, "run"),
			Hook: yamlString(item, "hook"),
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
