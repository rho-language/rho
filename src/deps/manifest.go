package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	Deps       []DepSpec
}

// LoadManifest loads a rho.mod manifest file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
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
		path = filepath.Join(path, "rho.mod")
	}
	return LoadManifest(path)
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
