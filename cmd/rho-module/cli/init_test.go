package cli

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Create module                                                        */
/* -------------------------------------------------------------------------- */

func TestInitModule(t *testing.T) {
    tmp := t.TempDir()
    name := filepath.Join(tmp, "hello")

    if err := runInit(name, "module"); err != nil {
        t.Fatalf("runInit failed: %v", err)
    }

    if _, err := os.Stat(filepath.Join(name, "module.yml")); err != nil {
        t.Fatalf("module.yml missing")
    }

    if _, err := os.Stat(filepath.Join(name, "src", "lib.rho")); err != nil {
        t.Fatalf("src/lib.rho missing")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Create plugin                                                        */
/* -------------------------------------------------------------------------- */

func TestInitPlugin(t *testing.T) {
    tmp := t.TempDir()
    name := filepath.Join(tmp, "plug")

    if err := runInit(name, "plugin"); err != nil {
        t.Fatalf("runInit failed: %v", err)
    }

    if _, err := os.Stat(filepath.Join(name, "plugin", "hooks.yml")); err != nil {
        t.Fatalf("hooks.yml missing")
    }

    if _, err := os.Stat(filepath.Join(name, "src", "plugin.rho")); err != nil {
        t.Fatalf("src/plugin.rho missing")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Invalid type                                                         */
/* -------------------------------------------------------------------------- */

func TestInitInvalidType(t *testing.T) {
    tmp := t.TempDir()
    name := filepath.Join(tmp, "bad")

    if err := runInit(name, "invalid"); err == nil {
        t.Fatalf("expected error for invalid type")
    }
}
