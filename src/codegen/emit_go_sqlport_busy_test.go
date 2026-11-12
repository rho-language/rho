package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A database the generated program opens waits for another connection's write
// instead of failing the read at once: SQLite answers "database is locked"
// unless it is told to wait, and a service has several connections to one file.
func TestTheSqlPortOpensSqliteWithABusyTimeout(t *testing.T) {
	if !strings.Contains(sqlPortRuntime, "sql.Open(rhoSqlDriverName(), rhoSqlSource(rhoSqlDriverName(), path))") {
		t.Fatal("the sql port opens a database without its connection string")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	start := strings.Index(sqlPortRuntime, "func rhoSqlSource(")
	end := strings.Index(sqlPortRuntime[start:], "\n}\n")
	program := "package main\n\nimport (\n\t\"fmt\"\n\t\"strings\"\n)\n\n" + sqlPortRuntime[start:start+end+3] + `
func main() {
	fmt.Println(rhoSqlSource("sqlite", "/data/app.db"))
	fmt.Println(rhoSqlSource("sqlite", "/data/app.db?mode=ro"))
	fmt.Println(rhoSqlSource("postgres", "postgres://db"))
}
`
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(program), 0o644)
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module busy\n\ngo 1.21\n"), 0o644)
	run := exec.Command("go", "run", ".")
	run.Dir = dir
	run.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	want := "/data/app.db?_pragma=busy_timeout(5000)\n/data/app.db?mode=ro&_pragma=busy_timeout(5000)\npostgres://db\n"
	if string(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
