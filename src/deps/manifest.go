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
}

// Extends is how a plugin says which component it plugs into.
//
// The relation is declared rather than assumed: a tool loads only the plugins
// that name it, and a plugin that names a tool nothing offers is a plugin no
// run ever calls.
type Extends struct {
	// Component is the name of the component this plugin extends.
	Component string
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

// parseProvides reads what a component offers a plugin.
func parseProvides(node yaml.Value) *Provides {
	if node.Kind != yaml.KindMap {
		return nil
	}
	provides := &Provides{Hooks: yamlStrings(node.Map["hooks"])}
	if len(provides.Hooks) == 0 {
		// A section with no hooks says nothing, which is how a module says it
		// offers no place for a plugin to stand.
		return nil
	}
	return provides
}

// parseExtends reads what a plugin plugs into.
func parseExtends(doc yaml.Value) *Extends {
	component := yamlUnquote(doc.Map["extends"].Str)
	if component == "" {
		return nil
	}
	extension := &Extends{
		Component: component,
		Hooks:     yamlStrings(doc.Map["hooks"]),
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
