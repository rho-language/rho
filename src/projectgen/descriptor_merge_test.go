package projectgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/ir"
)

// A project's project.yml writes what is its own; the built-in target supplies
// the rest, and a target with an id of its own stands on its own.
func TestAProjectDescriptorIsReadOverTheBuiltInOne(t *testing.T) {
	descriptor, err := ParseDescriptorOverDefault([]byte(`targets:
  - id: go-server
    templates:
      - dir: templates
    middleware:
      - name: cors
`))
	if err != nil {
		t.Fatal(err)
	}
	target, found := descriptor.Target("go-server")
	if !found {
		t.Fatal("go-server is gone")
	}
	if target.Emitter != "go" || target.TargetKind() != "server" || len(target.Files) == 0 {
		t.Errorf("the built-in settings were not kept: %+v", target)
	}
	if len(target.Templates) != 1 || target.Templates[0].Dir != "templates" || len(target.Middleware) != 1 {
		t.Errorf("the project's settings were not read: %+v", target)
	}
	if _, found := descriptor.Target("c"); !found {
		t.Error("the targets the project did not write are gone")
	}
	if descriptor.OutputDir != DefaultOutputDir {
		t.Errorf("output dir = %q", descriptor.OutputDir)
	}
	if _, err := ParseDescriptorOverDefault([]byte("targets:\n  - id: mine\n    emitter: go\n")); err == nil || !strings.Contains(err.Error(), "files") {
		t.Errorf("a target of its own without files: %v", err)
	}
}

// A templates dir carries every file under it by its file name; two files
// with one name are refused, and a template named outright wins over a file.
func TestATemplatesDirCarriesItsFilesByName(t *testing.T) {
	root := t.TempDir()
	write := func(path string) {
		full := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(path), 0o644)
	}
	write("templates/pages/home.html")
	write("templates/parts/row.html")
	write("special/home.html")
	templates, err := expandTemplates(root, []Template{{Dir: "templates"}, {Name: "home", File: "special/home.html"}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, template := range templates {
		got[template.Name] = template.File
	}
	if got["home"] != "special/home.html" || got["row"] != "templates/parts/row.html" || len(got) != 2 {
		t.Errorf("templates = %v", got)
	}
	write("templates/cards/row.html")
	if _, err := expandTemplates(root, []Template{{Dir: "templates"}}); err == nil || !strings.Contains(err.Error(), `two templates are named "row"`) {
		t.Errorf("two files with one name: %v", err)
	}
}

// A template the program asks for by name has to be carried.
func TestATemplateTheProgramAsksForMustBeCarried(t *testing.T) {
	program := ir.ModuleIR{Funcs: []ir.FuncIR{{Name: "page", Body: []ir.Instr{
		{Op: ir.OpIf, Condition: "true", Then: []ir.Instr{{Op: ir.OpReturn, Args: []string{`template_fill(template_get("missing"), "")`}}}},
		{Op: ir.OpReturn, Args: []string{`template_get("home")`}},
	}}}}
	err := checkTemplateNames(program, []Template{{Name: "home", File: "home.html"}})
	if err == nil || !strings.Contains(err.Error(), `template "missing", asked for in page`) {
		t.Errorf("error = %v", err)
	}
	if err := checkTemplateNames(program, []Template{{Name: "home"}, {Name: "missing"}}); err != nil {
		t.Errorf("every template carried: %v", err)
	}
}
