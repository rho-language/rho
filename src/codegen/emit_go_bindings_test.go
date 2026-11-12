package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A setting written through rho_bind_config_set is what rho_bind_config answers
// next, over the default the application's bind: gave, and the file holding it
// is readable by its owner only.
func TestABindingSettingIsWrittenAndReadBack(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	dir := t.TempDir()
	source := strings.Replace(EmitGoBindings(map[string]string{"acme-log.path": "/default"}), "package main", "package main\n", 1)
	if err := os.WriteFile(filepath.Join(dir, BindingsFileName), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	program := `package main

import "fmt"

func main() {
	fmt.Print(rhoBindConfig("acme-log", "path"), "|")
	fmt.Print(rhoBindConfigSet("acme-log", "path", "/elsewhere"), "|")
	fmt.Print(rhoBindConfigSet("acme-log", "enabled", "false"), "|")
	fmt.Print(rhoBindConfigSet("acme-log", "path", "/third"), "|")
	fmt.Print(rhoBindConfig("acme-log", "path"), "|", rhoBindConfig("acme-log", "enabled"))
}
`
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(program), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module bindtest\n\ngo 1.21\n"), 0o644)
	workspace := t.TempDir()
	run := exec.Command("go", "run", ".")
	run.Dir = dir
	run.Env = append(os.Environ(), "RHO_APP_WORKSPACE="+workspace, "GOFLAGS=-mod=mod", "GOWORK=off")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if string(out) != "/default|true|true|true|/third|false" {
		t.Errorf("output = %q", out)
	}
	info, err := os.Stat(filepath.Join(workspace, "bind.conf"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("bind.conf: %v, mode %v", err, info.Mode().Perm())
	}
	data, _ := os.ReadFile(filepath.Join(workspace, "bind.conf"))
	if strings.Count(string(data), "acme-log.path=") != 1 {
		t.Errorf("a setting written twice is on two lines:\n%s", data)
	}
}
