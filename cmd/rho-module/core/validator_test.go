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
