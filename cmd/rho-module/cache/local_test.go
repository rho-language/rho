package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: ListLocalModules                                                     */
/* -------------------------------------------------------------------------- */

func TestListLocalModules(t *testing.T) {
    tmp := t.TempDir()
    base := LocalCacheDir(tmp)

    os.MkdirAll(filepath.Join(base, "math@1.0.0"), 0755)
    os.MkdirAll(filepath.Join(base, "math@1.1.0"), 0755)

    modules, err := ListLocalModules(tmp)
    if err != nil {
        t.Fatalf("ListLocalModules failed: %v", err)
    }

    if len(modules) < 2 {
        t.Fatalf("expected at least 2 modules")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: LocalModuleExists                                                    */
/* -------------------------------------------------------------------------- */

func TestLocalModuleExists(t *testing.T) {
    tmp := t.TempDir()
    base := LocalCacheDir(tmp)

    os.MkdirAll(filepath.Join(base, "io@1.0.0"), 0755)

    if !LocalModuleExists(tmp, "io", "1.0.0") {
        t.Fatalf("expected io@1.0.0 to exist locally")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: LocalModulePath                                                      */
/* -------------------------------------------------------------------------- */

func TestLocalModulePath(t *testing.T) {
    tmp := t.TempDir()
    p := LocalModulePath(tmp, "crypto", "2.0.0")

    if !strings.Contains(p, "crypto@2.0.0") {
        t.Fatalf("wrong local module path")
    }
}
