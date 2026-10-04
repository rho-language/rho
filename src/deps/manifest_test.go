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

dep sensor-lib {
    type = "c"
    git = "https://github.com/aurel/sensor-lib.git"
	tag = "v1.0.0"
    version = ">=1.0.0 <2.0.0"
    env = "SENSOR_LIB_PATH"
    path = "../external/sensor-lib"
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
	if d.Name != "sensor-lib" || d.Type != DepTypeC || d.GitURL == "" || d.GitTag != "v1.0.0" || d.GitBranch != "" || d.Version.Raw != ">=1.0.0 <2.0.0" || d.EnvVar != "SENSOR_LIB_PATH" || d.Path != "../external/sensor-lib" || d.Include != "include" || d.Src != "src" {
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
  - name: sensor-lib
    type: c
    version: 1.0.0
    path: /opt/sensor-lib
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
	if len(m.Features) != 1 || len(m.Deps) != 1 || m.Deps[0].Path != "/opt/sensor-lib" {
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
	if !m.Extends.Answers("rho-doc", "index") || m.Extends.Answers("rho-doc", "shutdown") {
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

func TestLoadYAMLManifestReadsAPluginSpanningComponents(t *testing.T) {
	path := writeManifest(t, `module:
  name: rho-zenith
  type: plugin
  version: 0.1.0
main: rho-zenith
extends:
  - component: rho-target
    implements: [platform]
    hooks: [spec]
  - component: rho-kconfig
    implements: [platform]
    hooks: [render]
serves: [zenith]
commands:
  - name: target-spec
    run: rho-zenith
    hook: spec
    component: rho-target
`)

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Extends == nil {
		t.Fatal("expected the relation a spanning plugin plugs into to be read")
	}
	targets := m.Extends.All()
	if len(targets) != 2 {
		t.Fatalf("expected one relation per component, got %+v", targets)
	}
	if targets[0].Component != "rho-target" || targets[1].Component != "rho-kconfig" {
		t.Fatalf("expected the components it extends, got %+v", targets)
	}
	if len(targets[0].Implements) != 1 || targets[0].Implements[0] != "platform" {
		t.Fatalf("expected the contract each relation fills, got %+v", targets)
	}
	if len(m.Extends.Platforms) != 1 || m.Extends.Platforms[0] != "zenith" {
		t.Fatalf("expected the platform it serves, got %+v", m.Extends.Platforms)
	}
	if m.Extends.Commands[0].Component != "rho-target" {
		t.Fatalf("expected the command to name its component, got %+v", m.Extends.Commands[0])
	}
	if !m.Extends.Answers("rho-target", "spec") {
		t.Error("expected the target hook to be answered for rho-target")
	}
	if m.Extends.Answers("rho-target", "render") {
		t.Error("a hook of another component was answered for rho-target")
	}
	if !m.Extends.Answers("rho-kconfig", "render") {
		t.Error("expected the configuration hook to be answered for rho-kconfig")
	}
}

func TestLoadYAMLManifestReadsAPlatformIdentity(t *testing.T) {
	path := writeManifest(t, "module:\n  name: board-lib\n  type: platform\n  version: 0.1.0\nplatform: zenith\n")

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Platform != "zenith" {
		t.Fatalf("expected the platform identity to be read, got %q", m.Platform)
	}
}

func TestPlatformIsRecognizedFromTheModuleGraph(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "app")
	common := filepath.Join(root, "sensor-lib")
	platform := filepath.Join(root, "board-lib")
	for _, dir := range []string{app, common, platform} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(app, "app", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndepends:\n  - name: sensor-lib\n    type: c\n    path: ../sensor-lib\n")
	// The interfaces module enumerates the platform that implements it, which
	// is how a project written against the interfaces alone is still recognized.
	write(common, "sensor-lib", "module:\n  name: sensor-lib\n  type: interfaces\n  version: 1.0.0\nplatforms:\n  - name: board-lib\n    path: ../board-lib\n")
	write(platform, "board-lib", "module:\n  name: board-lib\n  type: platform\n  version: 0.1.0\nplatform: zenith\n")

	if got := Platform(app); got != "zenith" {
		t.Fatalf("expected the platform to be recognized through the graph, got %q", got)
	}
	if got := Platform(common); got != "zenith" {
		t.Fatalf("expected the platform to be recognized from the interfaces module, got %q", got)
	}
	if got := Platform(t.TempDir()); got != "" {
		t.Fatalf("expected no platform for a project that names none, got %q", got)
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

// A project's `sql:` block is read at build time as well as at generation time:
// the build has to know a project has a database, because the generated code
// calls into a port and a build that did not know would produce a project whose
// functions nothing defines.
func TestLoadYAMLManifestSQL(t *testing.T) {
	dir := t.TempDir()
	manifest := `module:
  name: myapp
  type: rho
  version: 0.1.0
main: src/main.rho
sql:
  driver: rho-sql-sqlite
  schema: db/schema.sql
  queries: db/queries.sql
  out: src/db
  package: billing.db
`
	path := filepath.Join(dir, "module.yml")
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if !m.SQL.Declared() {
		t.Fatal("a project that declared SQL was read as having none")
	}
	if m.SQL.Driver != "rho-sql-sqlite" {
		t.Errorf("the driver is %q, want the one the project named", m.SQL.Driver)
	}
	if got := m.SQL.SchemaPath(dir); got != filepath.Join(dir, "db", "schema.sql") {
		t.Errorf("the schema resolves to %q", got)
	}
	if got := m.SQL.QueriesPath(dir); got != filepath.Join(dir, "db", "queries.sql") {
		t.Errorf("the statements resolve to %q", got)
	}
	if got := m.SQL.OutPath(dir); got != filepath.Join(dir, "src", "db") {
		t.Errorf("the generated Rho goes in %q", got)
	}
	if m.SQL.Package != "billing.db" {
		t.Errorf("the generated file is packaged as %q", m.SQL.Package)
	}
}

// With no `out`, the generated file belongs beside the statements it came from,
// so a reader looking for it looks in the one place it is.
func TestLoadYAMLManifestSQLDefaultsOutToTheStatements(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yml")
	if err := os.WriteFile(path, []byte(
		"module:\n  name: myapp\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nsql:\n  driver: rho-sql-sqlite\n  schema: db/schema.sql\n  queries: db/queries.sql\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.SQL.OutPath(dir); got != filepath.Join(dir, "db") {
		t.Errorf("with no `out` the generated Rho goes in %q, want the statements' own directory", got)
	}
}

// A project with no `sql:` block has no database, which is a fact rather than
// something to refuse: most modules are libraries.
func TestLoadYAMLManifestWithoutSQL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module.yml")
	if err := os.WriteFile(path, []byte("module:\n  name: myapp\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.SQL != nil {
		t.Error("a project that declared no database was read as having one")
	}
}
