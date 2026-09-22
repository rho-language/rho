package deps

import (
	"os"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	tmp := "test_rho.mod"
	content := `
module testmod
version 1.0.0
main = "src/main.rho"

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
		t.Fatalf("write: %v", err)
	}
	defer os.Remove(tmp)

	m, err := LoadManifest(tmp)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}

	if m.ModuleName != "testmod" {
		t.Fatalf("ModuleName = %s", m.ModuleName)
	}
	if m.Version != "1.0.0" {
		t.Fatalf("Version = %s", m.Version)
	}
	if m.Main != "src/main.rho" {
		t.Fatalf("Main = %s", m.Main)
	}
	if len(m.Deps) != 1 {
		t.Fatalf("Deps count = %d", len(m.Deps))
	}

	d := m.Deps[0]
	if d.Name != "robohub-common" {
		t.Fatalf("Dep name = %s", d.Name)
	}
	if d.Type != DepTypeC {
		t.Fatalf("Dep type = %s", d.Type)
	}
	if d.GitURL == "" {
		t.Fatalf("GitURL empty")
	}
	if d.Version.Raw != ">=1.0.0 <2.0.0" {
		t.Fatalf("VersionRange = %s", d.Version.Raw)
	}
	if d.EnvVar != "ROBOHUB_COMMON_PATH" {
		t.Fatalf("EnvVar = %s", d.EnvVar)
	}
	if d.Path != "../external/robohub-common" {
		t.Fatalf("Path = %s", d.Path)
	}
	if d.Include != "include" {
		t.Fatalf("Include = %s", d.Include)
	}
	if d.Src != "src" {
		t.Fatalf("Src = %s", d.Src)
	}
}
