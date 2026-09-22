package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	Name    string
	Type    DepType
	GitURL  string
	Version VersionRange
	EnvVar  string
	Path    string
	Include string
	Src     string
}

type Manifest struct {
	ModuleName string
	Version    string
	Main       string
	Features   []string
	Deps       []DepSpec
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

	lines := strings.Split(string(data), "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
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
	m := &Manifest{}
	module := doc.Map["module"]
	if module.Kind == yaml.KindMap {
		m.ModuleName = yamlString(module, "name")
		m.Version = yamlString(module, "version")
	}
	m.Main = yamlString(doc, "main")
	m.Features = yamlStrings(doc.Map["features"])
	for _, item := range doc.Map["depends"].List {
		if item.Kind != yaml.KindMap {
			return nil, fmt.Errorf("depends entries must be maps")
		}
		dep := DepSpec{
			Name: yamlString(item, "name"), Type: DepType(yamlString(item, "type")),
			GitURL: yamlString(item, "git"), EnvVar: yamlString(item, "env"),
			Path: yamlString(item, "path"), Include: yamlString(item, "include"), Src: yamlString(item, "src"),
		}
		dep.Version.Raw = yamlString(item, "version")
		cs, parseErr := ParseVersionRange(dep.Version.Raw)
		if parseErr != nil {
			return nil, fmt.Errorf("parse version range for %s: %w", dep.Name, parseErr)
		}
		dep.Version.Constraints = cs
		m.Deps = append(m.Deps, dep)
	}
	return m, nil
}

func yamlString(value yaml.Value, key string) string {
	if item, ok := value.Map[key]; ok {
		return strings.Trim(item.Str, "\"")
	}
	return ""
}

func yamlStrings(value yaml.Value) []string {
	values := []string{}
	for _, item := range value.List {
		if item.Kind == yaml.KindString {
			values = append(values, strings.Trim(item.Str, "\""))
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
