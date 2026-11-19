package rhoreq

import (
	"os"
	"path/filepath"
	"testing"
)

// A suite's command may be a list of arguments, as gateway and keeper write it;
// a script it names is still the suite's, so a @verifies inside it is that suite's.
func TestASuiteCommandWrittenAsAListRunsItsScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "tests", "tests.yml"), []byte("name: x\nsuites:\n  - name: smoke\n    kind: command\n    command: [python3, tools/smoke.py]\n"), 0o644)
	suites := readSuites(dir)
	if len(suites) != 1 || !suites[0].runs("tools/smoke.py") {
		t.Fatalf("expected the suite to run tools/smoke.py, got %+v", suites)
	}
}
