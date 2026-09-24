package core

import "testing"

/* -------------------------------------------------------------------------- */
/*  Test: Search Registry                                                      */
/* -------------------------------------------------------------------------- */

func TestSearchRegistry(t *testing.T) {
    results, err := SearchRegistry("math")
    if err != nil {
        t.Fatalf("SearchRegistry failed: %v", err)
    }

    if len(results) == 0 {
        t.Fatalf("expected at least one result")
    }

    if results[0].Name != "math" {
        t.Fatalf("wrong module returned")
    }
}
