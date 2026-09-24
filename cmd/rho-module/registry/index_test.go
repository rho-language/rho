package registry

import "testing"

/* -------------------------------------------------------------------------- */
/*  Test: BuildIndex                                                           */
/* -------------------------------------------------------------------------- */

func TestBuildIndex(t *testing.T) {
    index, err := BuildIndex()
    if err != nil {
        t.Fatalf("BuildIndex failed: %v", err)
    }

    if len(index) == 0 {
        t.Fatalf("expected non-empty index")
    }
}
