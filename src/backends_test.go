//
// @file backends_test.go
// @brief A program is refused, by name, for a backend that cannot supply what it uses.
//
// Each case is a program built for one target that cannot have what it imports
// or calls. The build has to fail before anything is generated, and the message
// has to name the module or the function and the backend it is for — not a C
// link error, not a Go "undefined".
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package rho

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateFor writes a project built for one target and generates it.
func generateFor(t *testing.T, target, language, source string, extra map[string]string) error {
	t.Helper()
	root := t.TempDir()
	entry := filepath.Join(root, "src", "main.rho")
	if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: refused\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nbuild:\n  language: " +
		language + "\n  targets:\n    - " + target + "\n"
	files := map[string]string{filepath.Join(root, "module.yml"): manifest, entry: source}
	for name, content := range extra {
		files[filepath.Join(root, name)] = content
	}
	for name, content := range files {
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := GenerateAllTargets(entry, "", nil, nil, nil)
	return err
}

func TestAModuleOrBuiltinIsRefusedByNameOnABackendThatCannotSupplyIt(t *testing.T) {
	for _, c := range []struct {
		name, target, language, source string
		want                           []string
	}{
		{
			name: "std.embedded on Go", target: "go", language: "go",
			source: "module app\n\nimport std.embedded.gpio\n\nfn main() void {\n    write(13, true)\n}\n",
			want:   []string{"the go target", "std.embedded.gpio is for the c backend", "main.rho:3:1: error: "},
		},
		{
			name: "std.embedded through another module on Go", target: "go", language: "go",
			source: "module app\n\nimport std.embedded.modbus\n\nfn main() void {\n    print(\"x\")\n}\n",
			want:   []string{"std.embedded.modbus is for the c backend", "std.embedded.rs485 is for the c backend", "std.embedded.uart is for the c backend"},
		},
		{
			name: "std.web.http on C", target: "c", language: "c",
			source: "module app\n\nimport std.web.http\n\nfn main() void {\n    print(int_to_str(status_ok()))\n}\n",
			want:   []string{"the c target", "std.web.http is for the go-server backend"},
		},
		{
			name: "std.web.http on a Go command-line target", target: "go", language: "go",
			source: "module app\n\nimport std.web.http\n\nfn main() void {\n    print(int_to_str(status_ok()))\n}\n",
			want:   []string{"the go target", "std.web.http is for the go-server backend"},
		},
		{
			name: "std.web.template on C", target: "c", language: "c",
			source: "module app\n\nimport std.web.template\n\nfn main() void {\n    print(escape(\"<\"))\n}\n",
			want:   []string{"std.web.template is for the go-server backend"},
		},
		{
			name: "std.sql on C", target: "c", language: "c",
			source: "module app\n\nimport std.sql\n\nfn main() void {\n    print(\"x\")\n}\n",
			want:   []string{"the c target", "std.sql is for the go backend"},
		},
		{
			name: "std.sql on Go without the sql port", target: "go", language: "go",
			source: "module app\n\nimport std.sql\n\nfn main() void {\n    print(\"x\")\n}\n",
			want:   []string{"std.sql needs the sql port, which this target does not render"},
		},
		{
			name: "a Go runtime built-in on C", target: "c", language: "c",
			source: "module app\n\nfn main() void {\n    print(get_args())\n}\n",
			want:   []string{"the c target", "get_args is for the go backend", "main.rho:4:11: error: "},
		},
		{
			name: "a runtime function declared extern on C", target: "c", language: "c",
			source: "module app\n\nextern fn rho_path_exists(path: string) bool\n\nfn main() void {\n    if rho_path_exists(\"/\") {\n        print(\"y\")\n    }\n}\n",
			want:   []string{"rho_path_exists is for the go backend"},
		},
		{
			name: "a service built-in on a Go command-line target", target: "go", language: "go",
			source: "module app\n\nfn main() void {\n    print(escape_html(\"<\"))\n}\n",
			want:   []string{"escape_html is for the go-server backend"},
		},
		{
			name: "a port's built-in on a target without the port", target: "go-server", language: "go",
			source: "module app\n\nfn main() void {\n    print(store_list(\"\", \"\", false))\n}\n",
			want:   []string{"the go-server target", "store_list needs the fsstore port"},
		},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			err := generateFor(t, c.target, c.language, c.source, nil)
			if err == nil {
				t.Fatal("the build was not refused")
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the refusal does not say %q:\n%v", want, err)
				}
			}
		})
	}
}

// What a target can supply is not refused: the same imports and calls build where
// they belong.
func TestWhatATargetSuppliesIsNotRefused(t *testing.T) {
	for _, c := range []struct {
		name, target, language, source string
		extra                          map[string]string
	}{
		{
			name: "std.embedded on C", target: "c", language: "c",
			source: "module app\n\nimport std.embedded.gpio\n\nfn main() void {\n    write(13, true)\n}\n",
		},
		{
			name: "std.web on the Go server", target: "go-server", language: "go",
			source: "module app\n\nimport std.web.http\nimport std.web.template\n\nfn main() void {\n    print(escape(\"<\") + int_to_str(status_ok()))\n}\n",
		},
		{
			name: "a Go runtime built-in on Go", target: "go", language: "go",
			source: "module app\n\nfn main() void {\n    print(get_args())\n}\n",
		},
		{
			name: "a function of the program's own that shares a built-in's name", target: "c", language: "c",
			source: "module app\n\nfn get_args() string {\n    return \"mine\"\n}\n\nfn main() void {\n    print(get_args())\n}\n",
		},
		{
			name: "std.sql on Go with the sql port", target: "go", language: "go",
			source: "module app\n\nimport std.sql\n\nfn main() void {\n    print(int_to_str(u32(kind_text())))\n}\n",
			extra:  map[string]string{"project.yml": sqlStdDescriptor},
		},
	} {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if err := generateFor(t, c.target, c.language, c.source, c.extra); err != nil {
				t.Fatalf("refused: %v", err)
			}
		})
	}
}

// A std module's header is where it says where it works, and every name it uses
// is one the build knows.
func TestEveryStdBackendTagNamesAKnownBackend(t *testing.T) {
	known := map[string]bool{"c": true, "go": true, "go-server": true}
	tagged := map[string]string{}
	err := filepath.Walk(filepath.Join("..", "std"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".rho" {
			return err
		}
		tag, ok := readStdBackendTag(path)
		if !ok {
			t.Errorf("%s has no std module line", path)
			return nil
		}
		for _, backend := range tag.Backends {
			if !known[backend] {
				t.Errorf("%s: unknown backend %q in @backends", path, backend)
			}
		}
		if len(tag.Backends) > 0 {
			tagged[tag.Module] = strings.Join(tag.Backends, ",") + "/" + tag.Port
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for module, want := range map[string]string{
		"std.sql": "go/sql", "std.web.http": "go-server/", "std.web.template": "go-server/",
		"std.embedded.gpio": "c/", "std.embedded.modbus": "c/",
	} {
		if tagged[module] != want {
			t.Errorf("%s is tagged %q, want %q", module, tagged[module], want)
		}
	}
}

func TestTheBackendHeaderIsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.rho")
	content := "//\n// @file m.rho\n// @backends: c, go-server\n// @port fsstore\n// @backendsx ignored\n//\n\nmodule std.example\n\n// @backends go\nfn f() void {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	tag, ok := readStdBackendTag(path)
	if !ok {
		t.Fatal("not read as a std module")
	}
	if tag.Module != "std.example" || strings.Join(tag.Backends, ",") != "c,go-server" || tag.Port != "fsstore" {
		t.Fatalf("read %+v", tag)
	}
	other := filepath.Join(t.TempDir(), "app.rho")
	if err := os.WriteFile(other, []byte("// @backends c\nmodule app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := readStdBackendTag(other); ok {
		t.Fatal("a program's own module was read as a std module")
	}
}
