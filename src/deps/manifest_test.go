package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil { t.Fatal(err) }
	if _, err := LoadManifest(tmp); err == nil { t.Fatal("expected tag/branch conflict") }
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
