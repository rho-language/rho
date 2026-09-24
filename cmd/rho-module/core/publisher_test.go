package core

import (
    "os"
    "path/filepath"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Publish Module                                                       */
/* -------------------------------------------------------------------------- */

func TestPublishModule(t *testing.T) {
    tmp := t.TempDir()

    // module.yml
    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello
  type: module
  version: 0.1.0

main: src/lib.rho
`), 0644)

    // README.md
    os.WriteFile(filepath.Join(tmp, "README.md"), []byte("Hello"), 0644)

    // src/lib.rho
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "lib.rho"), []byte("module hello"), 0644)

    if err := PublishModule(tmp); err != nil {
        t.Fatalf("PublishModule failed: %v", err)
    }
}
