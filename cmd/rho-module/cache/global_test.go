package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: ListGlobalModules                                                    */
/* -------------------------------------------------------------------------- */

func TestListGlobalModules(t *testing.T) {
    base := GlobalCacheDir()
    os.MkdirAll(filepath.Join(base, "math@1.0.0"), 0755)
    os.MkdirAll(filepath.Join(base, "math@1.1.0"), 0755)

    modules, err := ListGlobalModules()
    if err != nil {
        t.Fatalf("ListGlobalModules failed: %v", err)
    }

    if len(modules) < 2 {
        t.Fatalf("expected at least 2 modules")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: GlobalModuleExists                                                   */
/* -------------------------------------------------------------------------- */

func TestGlobalModuleExists(t *testing.T) {
    base := GlobalCacheDir()
    os.MkdirAll(filepath.Join(base, "io@1.0.0"), 0755)

    if !GlobalModuleExists("io", "1.0.0") {
        t.Fatalf("expected io@1.0.0 to exist")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: GlobalModulePath                                                     */
/* -------------------------------------------------------------------------- */

func TestGlobalModulePath(t *testing.T) {
    p := GlobalModulePath("crypto", "2.0.0")
    if !strings.Contains(p, "crypto@2.0.0") {
        t.Fatalf("wrong global module path")
    }
}
