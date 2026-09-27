package cli

import (
    "os"
    "path/filepath"
    "strings"
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

    if _, err := os.Stat(filepath.Join(name, "src", "plugin.rho")); err != nil {
        t.Fatalf("src/plugin.rho missing")
    }

    // A plugin has to say what it plugs into, so a scaffold that omits the
    // declaration produces a module nothing will ever call.
    written, err := os.ReadFile(filepath.Join(name, "module.yml"))
    if err != nil {
        t.Fatalf("cannot read module.yml: %v", err)
    }
    for _, want := range []string{"extends:", "hooks:", "commands:"} {
        if !strings.Contains(string(written), want) {
            t.Fatalf("module.yml is missing %s:\n%s", want, written)
        }
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
