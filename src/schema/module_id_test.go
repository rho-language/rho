package schema

import (
	"strings"
	"testing"

	"rho/src/yaml"
)

func TestCheckModuleID(t *testing.T) {
	for _, good := range []string{"robohub.driver.ds18b20", "acme.relay-board", "a.b_c.d9"} {
		if err := CheckModuleID(good); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
	for _, bad := range []string{"", "robohub", "Robohub.driver", "a..b", "a.b-", "a.-b", "a.b--c", "a b.c", "a/b.c", ".a.b"} {
		if CheckModuleID(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPreviousIDsAreChecked(t *testing.T) {
	load := func(text string) yaml.Value {
		v, err := yaml.Load(text)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	ok := load("module:\n  name: x\n  id: acme.relay\n  previous_ids:\n    - old.relay\n")
	if err := requireModuleID(ok, "module"); err != nil {
		t.Fatalf("a valid rename was refused: %v", err.Message)
	}
	same := load("module:\n  name: x\n  id: acme.relay\n  previous_ids:\n    - acme.relay\n")
	if err := requireModuleID(same, "module"); err == nil || !strings.Contains(err.Message, "current id") {
		t.Fatalf("the current id as a previous id was accepted: %v", err)
	}
	bad := load("module:\n  name: x\n  id: acme.relay\n  previous_ids:\n    - Old\n")
	if requireModuleID(bad, "module") == nil {
		t.Fatal("a malformed previous id was accepted")
	}
}

func TestCheckIDsFindsWhatCannotAllBeTrue(t *testing.T) {
	problems := CheckIDs([]IDEntry{
		{Path: "a/module.yml", ID: "acme.one"},
		{Path: "b/module.yml", ID: "acme.one"},
		{Path: "c/module.yml", ID: "acme.three", Previous: []string{"acme.two"}},
		{Path: "d/module.yml", ID: "acme.two"},
		{Path: "e/module.yml", ID: "acme.five", Previous: []string{"acme.old"}},
	})
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "both declare") || !strings.Contains(joined, "previous id") || len(problems) != 2 {
		t.Fatalf("problems = %v", problems)
	}
}
