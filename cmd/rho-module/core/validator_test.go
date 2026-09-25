package core

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Valid Module                                                         */
/* -------------------------------------------------------------------------- */

func TestValidateModule(t *testing.T) {
    tmp := t.TempDir()

    // module.yml
    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello
  type: module
  version: 0.1.0

main: src/lib.rho
`), 0644)

    // src/lib.rho
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "lib.rho"), []byte("module hello"), 0644)

    if err := ValidateModule(tmp); err != nil {
        t.Fatalf("ValidateModule failed: %v", err)
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing main                                                         */
/* -------------------------------------------------------------------------- */

func TestValidateMissingMain(t *testing.T) {
    tmp := t.TempDir()

    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello
  type: module
  version: 0.1.0

main: src/lib.rho
`), 0644)

    os.Mkdir(filepath.Join(tmp, "src"), 0755)

    if err := ValidateModule(tmp); err == nil {
        t.Fatalf("expected error for missing main file")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Plugin Validation                                                    */
/* -------------------------------------------------------------------------- */

func TestValidatePlugin(t *testing.T) {
    tmp := t.TempDir()

    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: plug
  type: plugin
  version: 0.1.0

main: src/plugin.rho
`), 0644)

    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "plugin.rho"), []byte("module plug"), 0644)

    os.Mkdir(filepath.Join(tmp, "plugin"), 0755)
    os.WriteFile(filepath.Join(tmp, "plugin", "hooks.yml"), []byte(`
hooks:
  - init
`), 0644)

    if err := ValidateModule(tmp); err != nil {
        t.Fatalf("plugin validation failed: %v", err)
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Schema Validation Gates                                             */
/* -------------------------------------------------------------------------- */

// writeModule writes a module.yml plus its main file into dir.
func writeModule(t *testing.T, dir, manifest string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.rho"), []byte("module app"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsUnknownModuleType(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: bogus\n  version: 0.1.0\n\nmain: src/main.rho\n")

	err := ValidateModule(tmp)
	if err == nil {
		t.Fatal("expected error for unknown module type")
	}
	if !contains(err.Error(), "module.type") {
		t.Fatalf("expected module.type in error, got: %v", err)
	}
}

func TestValidateRejectsMissingModuleSection(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "main: src/main.rho\n")

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for missing module section")
	}
}

func TestValidateRejectsNonRhoMain(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: build/app\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := ValidateModule(tmp)
	if err == nil {
		t.Fatal("expected error for non-.rho main in rho module")
	}
	if !contains(err.Error(), "main") {
		t.Fatalf("expected main in error, got: %v", err)
	}
}

func TestValidateRejectsMalformedHooks(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte("module:\n  name: plug\n  type: plugin\n  version: 0.1.0\n\nmain: src/plugin.rho\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "src", "plugin.rho"), []byte("module plug"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "plugin"), 0755); err != nil {
		t.Fatal(err)
	}
	// hooks is present but declares no value.
	if err := os.WriteFile(filepath.Join(tmp, "plugin", "hooks.yml"), []byte("hooks:\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for hooks.yml with no hooks")
	}
}

func TestValidateAcceptsDependsList(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\ndepends:\n  - name: common\n    git: https://example.com/common.git\n    tag: v1.0.0\n")

	if err := ValidateModule(tmp); err != nil {
		t.Fatalf("expected depends list to validate, got: %v", err)
	}
}

func TestValidateRejectsBadSemver(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: rho\n  version: not-a-version\n\nmain: src/main.rho\n")

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for invalid semver")
	}
}
