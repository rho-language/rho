package deps

import (
	"os"
	"path/filepath"
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
	if d.Name != "robohub-common" || d.Type != DepTypeC || d.GitURL == "" || d.Version.Raw != ">=1.0.0 <2.0.0" || d.EnvVar != "ROBOHUB_COMMON_PATH" || d.Path != "../external/robohub-common" || d.Include != "include" || d.Src != "src" {
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
