package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/yaml"
)

func TestLoadManifest(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "rho.mod")
	content := `module testmod
version 1.0.0
main = "src/main.rho"
feature = "math.control"

dep robohub-common {
    type = "c"
    git = "https://github.com/aurel/robohub-common.git"
	tag = "v1.0.0"
    version = ">=1.0.0 <2.0.0"
    env = "ROBOHUB_COMMON_PATH"
    path = "../external/robohub-common"
    include = "include"
    src = "src"
}
`
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(tmp)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.ModuleName != "testmod" || m.Version != "1.0.0" || m.Main != "src/main.rho" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if len(m.Features) != 1 || m.Features[0] != "math.control" || len(m.Deps) != 1 {
		t.Fatalf("unexpected manifest contents: %+v", m)
	}
	d := m.Deps[0]
	if d.Name != "robohub-common" || d.Type != DepTypeC || d.GitURL == "" || d.GitTag != "v1.0.0" || d.GitBranch != "" || d.Version.Raw != ">=1.0.0 <2.0.0" || d.EnvVar != "ROBOHUB_COMMON_PATH" || d.Path != "../external/robohub-common" || d.Include != "include" || d.Src != "src" {
		t.Fatalf("unexpected dependency: %+v", d)
	}
}

func TestLoadYAMLManifest(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "module.yml")
	content := `module:
  name: yaml-app
  type: rho
  version: 1.0.0
main: src/main.rho
features:
  - math.control
depends:
  - name: robohub-common
    type: c
    version: 1.0.0
    path: /opt/robohub-common
`
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(tmp)
	if err != nil {
		t.Fatalf("LoadManifest YAML error: %v", err)
	}
	if m.Command != "" {
		t.Fatalf("expected no published command, got %q", m.Command)
	}
	if m.ModuleName != "yaml-app" || m.Version != "1.0.0" || m.Main != "src/main.rho" {
		t.Fatalf("unexpected YAML manifest: %+v", m)
	}
	if len(m.Features) != 1 || len(m.Deps) != 1 || m.Deps[0].Path != "/opt/robohub-common" {
		t.Fatalf("unexpected YAML manifest contents: %+v", m)
	}
}

func TestLoadYAMLManifestRejectsTagAndBranch(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "module.yml")
	content := "module:\n  name: app\n  version: 1.0.0\ndepends:\n  - name: dep\n    git: https://example.com/dep.git\n    tag: v1.0.0\n    branch: main\n"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(tmp); err == nil {
		t.Fatal("expected tag/branch conflict")
	}
}

/* -------------------------------------------------------------------------- */
/*  Schema validation gates manifest loading                                  */
/* -------------------------------------------------------------------------- */

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "module.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A depends entry with no source at all cannot be resolved, so it must be
// rejected before any build work starts.
func TestLoadYAMLManifestRejectsOrphanDependency(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\nmain: src/main.rho\ndepends:\n  - name: orphan\n    version: 1.0.0\n")

	_, err := LoadManifest(path)
	if err == nil {
		t.Fatal("expected error for dependency without git or path")
	}
	if !strings.Contains(err.Error(), "depends[0]") {
		t.Fatalf("expected depends[0] path in error, got: %v", err)
	}
}

func TestLoadYAMLManifestRejectsUnknownType(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: bogus\n  version: 1.0.0\nmain: src/main.rho\n")

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error for unknown module type")
	}
}

// A dependency may omit type: the resolver infers c/rho from the
// dependency's own manifest.
func TestLoadYAMLManifestAllowsOmittedDepType(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\nmain: src/main.rho\ndepends:\n  - name: common\n    path: /opt/common\n    version: 1.0.0\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("expected manifest to load, got: %v", err)
	}
	if len(m.Deps) != 1 || m.Deps[0].Name != "common" {
		t.Fatalf("unexpected deps: %+v", m.Deps)
	}
}

// Dual C/Go generation is driven by the build section.
func TestLoadYAMLManifestBuildSection(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\nmain: src/main.rho\nbuild:\n  language: cgo\n  include_dirs:\n    - include\n  source_dirs:\n    - src\n  sources:\n    - src/app.c\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("expected build section to load, got: %v", err)
	}
	if m.BuildLanguage != "cgo" {
		t.Fatalf("unexpected build language %q", m.BuildLanguage)
	}
	if len(m.BuildIncludeDirs) != 1 || m.BuildIncludeDirs[0] != "include" {
		t.Fatalf("unexpected include dirs: %+v", m.BuildIncludeDirs)
	}
	if len(m.BuildSourceDirs) != 1 || m.BuildSourceDirs[0] != "src" {
		t.Fatalf("unexpected source dirs: %+v", m.BuildSourceDirs)
	}
	if len(m.BuildSources) != 1 || m.BuildSources[0] != "src/app.c" {
		t.Fatalf("unexpected sources: %+v", m.BuildSources)
	}
}

func TestLoadYAMLManifestRejectsUnknownBuildLanguage(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\nmain: src/main.rho\nbuild:\n  language: rust\n")

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error for unsupported build language")
	}
}

func TestLoadYAMLManifestRejectsBuildWithoutLanguage(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\nmain: src/main.rho\nbuild:\n  include_dirs:\n    - include\n")

	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error for build section without language")
	}
}

func TestLoadYAMLManifestReadsDocAsCode(t *testing.T) {
	path := writeManifest(t, `module:
  name: registry
  type: rho
  version: 0.1.0
main: src/main.rho
docs:
  title: Module registry
  section: applications
  order: "20"
  index: docs/index.md
  sources:
    - docs/index.md
    - docs/deployment.md
`)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Docs == nil {
		t.Fatal("expected the doc-as-code section to be read")
	}
	if m.Docs.Title != "Module registry" || m.Docs.Section != "applications" || m.Docs.Order != "20" || m.Docs.Index != "docs/index.md" {
		t.Fatalf("unexpected docs section: %+v", m.Docs)
	}
	if len(m.Docs.Sources) != 2 || m.Docs.Sources[0] != "docs/index.md" || m.Docs.Sources[1] != "docs/deployment.md" {
		t.Fatalf("unexpected doc sources: %+v", m.Docs.Sources)
	}
}

func TestLoadYAMLManifestWithoutDocsHasNoSection(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\nmain: src/main.rho\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Docs != nil {
		t.Fatalf("expected no docs section, got %+v", m.Docs)
	}
	if m.Extends != nil {
		t.Fatalf("expected nothing extended, got %+v", m.Extends)
	}
	if m.Provides != nil {
		t.Fatalf("expected nothing offered, got %+v", m.Provides)
	}
}

func TestLoadYAMLManifestReadsThePluginRelation(t *testing.T) {
	path := writeManifest(t, `module:
  name: rho-doc-toc
  type: plugin
  version: 0.1.0
main: rho-doc-toc
extends: rho-doc
hooks:
  - index
  - page
commands:
  - name: table-of-contents
    run: rho-doc-toc
    hook: index
`)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Extends == nil {
		t.Fatal("expected the relation a plugin plugs into to be read")
	}
	if m.Extends.Component != "rho-doc" {
		t.Fatalf("expected the component it extends, got %q", m.Extends.Component)
	}
	if len(m.Extends.Hooks) != 2 || m.Extends.Hooks[0] != "index" || m.Extends.Hooks[1] != "page" {
		t.Fatalf("expected the hooks it answers, got %+v", m.Extends.Hooks)
	}
	if !m.Extends.Answers("index") || m.Extends.Answers("shutdown") {
		t.Fatalf("expected only the declared hooks to be answered, got %+v", m.Extends.Hooks)
	}
	if len(m.Extends.Commands) != 1 {
		t.Fatalf("expected one command, got %+v", m.Extends.Commands)
	}
	command := m.Extends.Commands[0]
	if command.Name != "table-of-contents" || command.Run != "rho-doc-toc" || command.Stage() != "index" {
		t.Fatalf("unexpected command: %+v", command)
	}
}

func TestLoadYAMLManifestReadsWhatAComponentOffers(t *testing.T) {
	path := writeManifest(t, "module:\n  name: rho-doc\n  type: component\n  version: 0.1.0\n  command: doc\n\nmain: rho-doc\n\nprovides:\n  hooks:\n    - index\n    - page\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Command != "doc" {
		t.Fatalf("expected the published command, got %q", m.Command)
	}
	if m.Provides == nil || len(m.Provides.Hooks) != 2 {
		t.Fatalf("expected the hooks it offers, got %+v", m.Provides)
	}
	if m.Extends != nil {
		t.Fatalf("a component extends nothing, got %+v", m.Extends)
	}
}

func TestLoadYAMLManifestWithoutAProvidesSectionOffersNothing(t *testing.T) {
	path := writeManifest(t, "module:\n  name: app\n  type: rho\n  version: 1.0.0\n\nmain: src/main.rho\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Provides != nil {
		t.Fatalf("expected nothing offered, got %+v", m.Provides)
	}
}

func TestLoadYAMLManifestReadsTheConfigBridge(t *testing.T) {
	// The bridge a project declares is what the generator copies and the
	// component resolves, so all four parts of it are read.
	path := writeManifest(t, `module:
  name: sensorhub
  type: rho
  version: 0.1.0
main: src/main.rho
config:
  dialect: espidf
  kconfig_fragments:
    - kconfig/board.Kconfig
  values:
    CONFIG_UART_BAUD: "57600"
    CONFIG_SPI_MASTER: "y"
  module: config.rho
`)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Config == nil {
		t.Fatal("expected the configuration bridge to be read")
	}
	if m.Config.Dialect != "espidf" {
		t.Errorf("expected the dialect, got %q", m.Config.Dialect)
	}
	if len(m.Config.Fragments) != 1 || m.Config.Fragments[0] != "kconfig/board.Kconfig" {
		t.Errorf("expected the fragments, got %+v", m.Config.Fragments)
	}
	if len(m.Config.Values) != 2 || m.Config.Values["CONFIG_UART_BAUD"] != "57600" {
		t.Errorf("expected the values, got %+v", m.Config.Values)
	}
	if m.Config.Module != "config.rho" {
		t.Errorf("expected the module of constants, got %q", m.Config.Module)
	}
}

func TestLoadYAMLManifestWithoutAConfigSectionHasNoBridge(t *testing.T) {
	// Most projects declare no configuration, and that is a decision: a section
	// that says nothing is no section rather than one that configures nothing.
	path := writeManifest(t, "module:\n  name: plain\n  type: rho\n  version: 1.0.0\n\nmain: src/main.rho\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Config != nil {
		t.Fatalf("expected no bridge, got %+v", m.Config)
	}
}

func TestAConfigSectionThatDeclaresNothingIsNoSection(t *testing.T) {
	// A section that names nothing is no section, rather than one that
	// configures nothing: a build with no bridge and a build with an empty
	// bridge should say the same thing, because they do the same thing.
	// (The YAML in use cannot write an empty section, so this is asked of the
	// reader directly.)
	empty := map[string]yaml.Value{}
	if config := parseConfigManifest(yaml.Value{Kind: yaml.KindMap, Map: empty}); config != nil {
		t.Fatalf("expected no bridge, got %+v", config)
	}
	onlyAValue := map[string]yaml.Value{"dialect": {Kind: yaml.KindString, Str: "linux"}}
	config := parseConfigManifest(yaml.Value{Kind: yaml.KindMap, Map: onlyAValue})
	if config == nil {
		t.Fatal("expected a section that names a dialect to be a section")
	}
	if config.Dialect != "linux" {
		t.Fatalf("expected the dialect to be read, got %+v", config)
	}
}
