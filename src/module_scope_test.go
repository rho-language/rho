package rho

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Two imported modules that declare the same name each keep their own: a
// module's call to its own init is its own, and a qualified call reaches the
// module it names. Before, the first declaration won across the program, so
// module b's init() ran module a's.
func TestImportedModulesKeepTheirOwnNames(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("module.yml", "module:\n  name: scope\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n")
	write("src/app/a.rho", "module app.a\nfn init() string { return \"a\" }\nfn hello_a() string { return init() }\n")
	write("src/app/b.rho", "module app.b\nfn init() string { return \"b\" }\nfn hello_b() string { return init() }\n")
	write("src/main.rho", "module main\nimport app.a\nimport app.b\nfn main() void {\n    print(hello_a() + hello_b() + \"|\" + a.hello_a() + b.init())\n}\n")

	dirs, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "scope")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "ab|ab" {
		t.Errorf("output = %q, want %q", out, "ab|ab")
	}
}

// A qualified call names an imported module and a function it declares; anything
// else is refused with what is wrong.
func TestAQualifiedCallMustNameAModuleAndItsFunction(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "src", "app"), 0o755)
	os.WriteFile(filepath.Join(root, "module.yml"), []byte("module:\n  name: scope\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n"), 0o644)
	os.WriteFile(filepath.Join(root, "src", "app", "a.rho"), []byte("module app.a\nfn hello() string { return \"a\" }\n"), 0o644)
	for call, want := range map[string]string{
		"a.nothing()": `module app.a has no function "nothing"`,
		"c.hello()":   `"c" is not a value in scope or an imported module`,
	} {
		os.WriteFile(filepath.Join(root, "src", "main.rho"), []byte("module main\nimport app.a\nfn main() void {\n    print("+call+")\n}\n"), 0o644)
		_, err := GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: error = %v, want it to say %s", call, err, want)
		}
	}
}

// A module says in its manifest what it is imported as and what it exports. An
// application that declares it imports it by that name and uses what it exports;
// anything else in the module is its own, so a private helper neither leaks nor
// collides with the application's function of the same name. An application
// that does not declare the module cannot import it.
func TestAnImportIsTheInterfaceADeclaredDependencyExports(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	workspace := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("acme/.git/HEAD", "ref: refs/heads/main\n")
	write("acme/modules/greet/module.yml", "module:\n  name: acme-greet\n  type: module\n  version: 0.1.0\nmain: src/greet.rho\nassets:\n  - greeting.txt\nprovides:\n  import: acme.greet\n  exports:\n    - hello\n")
	// The module reads its own asset through a runtime function it declares:
	// a foreign function in the runtime's namespace is the runtime's, whichever
	// module declared it.
	write("acme/modules/greet/greeting.txt", "hello from greet")
	write("acme/modules/greet/src/greet.rho", "module greet\nextern fn rho_asset(module: string, path: string) string\nfn helper() string { return rho_asset(\"acme-greet\", \"greeting.txt\") }\nfn hello() string { return helper() }\n")
	write("app/.git/HEAD", "ref: refs/heads/main\n")
	manifest := "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndeps:\n  acme:\n    acme-greet: \">=0.1.0\"\nbuild:\n  language: go\n  targets:\n    - go\n"
	write("app/module.yml", manifest)
	write("app/src/main.rho", "module main\nimport acme.greet\nfn helper() string { return \"app\" }\nfn main() void {\n    print(greet.hello() + \"|\" + hello() + \"|\" + helper())\n}\n")
	t.Setenv("RHO_WORKSPACE", workspace)

	entry := filepath.Join(workspace, "app", "src", "main.rho")
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "app")
	// The .git folders above only mark repository roots for the resolver.
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "hello from greet|hello from greet|app" {
		t.Errorf("output = %q", out)
	}

	// A private name is refused, with what the module does export.
	write("app/src/main.rho", "module main\nimport acme.greet\nfn main() void {\n    print(greet.helper())\n}\n")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "helper is not exported by acme.greet (it exports hello)") {
		t.Errorf("private call: error = %v", err)
	}

	// An undeclared module is not importable.
	write("app/module.yml", strings.Replace(manifest, "deps:\n  acme:\n    acme-greet: \">=0.1.0\"\n", "", 1))
	write("app/src/main.rho", "module main\nimport acme.greet\nfn main() void {\n    print(greet.hello())\n}\n")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err == nil {
		t.Error("an undeclared module was imported")
	}
}

// A module's types and constants are its own, as its functions are: two
// imported modules that each declare a Config and a LIMIT have two of each, an
// application's own Config is its own, and `a.Config` names module a's.
func TestImportedModulesKeepTheirOwnTypesAndConstants(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	root := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(root, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("module.yml", "module:\n  name: scope\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n")
	write("src/app/a.rho", "module app.a\nstruct Config { x: u32 }\nconst LIMIT: u32 = 1\nfn make() Config { return Config{ LIMIT } }\nfn show(c: Config) u32 { return c.x }\n")
	write("src/app/b.rho", "module app.b\nstruct Config { y: string }\nconst LIMIT: u32 = 2\nfn make() Config { return Config{ \"b\" } }\nfn limit() u32 { return LIMIT }\n")
	write("src/main.rho", "module main\nimport app.a\nimport app.b\nstruct Config { name: string }\n"+
		"fn main() void {\n    let mine: Config = Config{ \"m\" }\n    let c: a.Config = a.Config{ a.LIMIT + 4 }\n"+
		"    print(mine.name + int_to_str(a.show(c)) + int_to_str(a.show(a.make())) + b.make().y + int_to_str(b.limit() + b.LIMIT))\n}\n")

	entry := filepath.Join(root, "src", "main.rho")
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "scope")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "m51b4" {
		t.Errorf("output = %q, want %q", out, "m51b4")
	}

	// A name two imported modules declare, and the importer does not, has to say
	// which one it means.
	write("src/main.rho", "module main\nimport app.a\nimport app.b\nfn main() void {\n    let c: Config = a.make()\n    print(int_to_str(LIMIT))\n}\n")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "write a.Config or b.Config") {
		t.Errorf("ambiguous type: error = %v", err)
	}
	write("src/main.rho", "module main\nimport app.a\nimport app.b\nfn main() void {\n    print(int_to_str(LIMIT))\n}\n")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "write a.LIMIT or b.LIMIT") {
		t.Errorf("ambiguous constant: error = %v", err)
	}
}

// A module that publishes exports lets another module name only the types and
// constants it lists.
func TestAModuleExportsItsTypesAndConstants(t *testing.T) {
	workspace := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("acme/.git/HEAD", "ref: refs/heads/main\n")
	write("acme/modules/shape/module.yml", "module:\n  name: acme-shape\n  type: module\n  version: 0.1.0\nmain: src/shape.rho\nprovides:\n  import: acme.shape\n  exports:\n    - Point\n    - ORIGIN_X\n    - origin\n")
	write("acme/modules/shape/src/shape.rho", "module shape\nconst ORIGIN_X: u32 = 0\nconst SECRET: u32 = 7\nstruct Point { x: u32 }\nstruct Hidden { y: u32 }\nfn origin() Point { return Point{ ORIGIN_X + SECRET - SECRET } }\n")
	write("app/.git/HEAD", "ref: refs/heads/main\n")
	write("app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\ndeps:\n  acme:\n    acme-shape: \">=0.1.0\"\nbuild:\n  language: go\n  targets:\n    - go\n")
	t.Setenv("RHO_WORKSPACE", workspace)
	entry := filepath.Join(workspace, "app", "src", "main.rho")

	write("app/src/main.rho", "module main\nimport acme.shape\nfn main() void {\n    let p: shape.Point = shape.Point{ shape.ORIGIN_X }\n    let q: Point = shape.origin()\n    print(int_to_str(p.x + q.x))\n}\n")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err != nil {
		t.Fatalf("exported names: %v", err)
	}
	for _, private := range []string{"let h: shape.Hidden = shape.Hidden{ 1 }", "let s: u32 = shape.SECRET", "let s: u32 = SECRET"} {
		write("app/src/main.rho", "module main\nimport acme.shape\nfn main() void {\n    "+private+"\n}\n")
		_, err := GenerateAllTargets(entry, "", nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "is not exported by acme.shape (it exports Point, ORIGIN_X, origin)") {
			t.Errorf("%s: error = %v", private, err)
		}
	}
}

// An interface declares what its providers implement; an application binds it to
// providers and configures them. The calls to the bound providers are generated
// into the interface, so its own code decides how to use them (here, every
// enabled provider receives an alert). A provider that does not implement the
// interface as declared, or that the application does not declare, is refused.
func TestAnInterfaceIsBoundToProviders(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	workspace := t.TempDir()
	write := func(path, text string) {
		full := filepath.Join(workspace, filepath.FromSlash(path))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("acme/.git/HEAD", "ref: refs/heads/main\n")
	write("acme/modules/alert/module.yml", `module:
  name: acme-alert
  type: module
  version: 1.0.0
main: src/alert.rho
provides:
  import: acme.alert
  exports: [raise]
interface:
  functions:
    send: "(level: string, message: string) u32"
`)
	write("acme/modules/alert/src/alert.rho", `module alert
fn raise(level: string, message: string) u32 {
    var sent: u32 = 0
    var rest: string = bound_providers()
    while rest != "" {
        let at: i32 = str_index_of(rest, "\n")
        let provider: string = str_slice(rest, 0, u32(at))
        rest = str_slice(rest, u32(at) + 1, strlen(rest) - u32(at) - 1)
        if bind_config(provider, "enabled") != "false" {
            sent = sent + send_to(provider, level, message)
        }
    }
    return sent
}
`)
	provider := func(name, importName, body string) {
		write("acme/providers/"+name+"/module.yml", "module:\n  name: acme-"+name+"\n  type: provider\n  version: 0.1.0\nmain: src/provider.rho\nimplements: [acme.alert]\nprovides:\n  import: "+importName+"\n  exports: [send]\n  settings: [prefix]\n")
		write("acme/providers/"+name+"/src/provider.rho", "module "+name+"\nextern fn rho_bind_config(provider: string, key: string) string\n"+body)
	}
	provider("log", "acme.alert_log", `fn send(level: string, message: string) u32 {
    print(rho_bind_config("acme-log", "prefix") + level + ":" + message + ";")
    return 1
}
`)
	provider("chat", "acme.alert_chat", `fn send(level: string, message: string) u32 {
    print("chat " + message + ";")
    return 1
}
`)
	write("app/.git/HEAD", "ref: refs/heads/main\n")
	manifest := `module:
  name: app
  type: rho
  version: 0.1.0
main: src/main.rho
deps:
  acme:
    acme-alert: ">=1.0.0"
    acme-log: ">=0.1.0"
    acme-chat: ">=0.1.0"
bind:
  acme.alert:
    providers: [acme-log, acme-chat]
    config:
      acme-log:
        prefix: "LOG "
      acme-chat:
        enabled: "false"
build:
  language: go
  targets:
    - go
`
	write("app/module.yml", manifest)
	write("app/src/main.rho", "module main\nimport acme.alert\nfn main() void {\n    print(int_to_str(alert.raise(\"ERROR\", \"pump stopped\")))\n}\n")
	t.Setenv("RHO_WORKSPACE", workspace)
	t.Setenv("RHO_APP_WORKSPACE", "")

	entry := filepath.Join(workspace, "app", "src", "main.rho")
	dirs, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	binary := filepath.Join(t.TempDir(), "app")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(binary).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "LOG ERROR:pump stopped;1" {
		t.Errorf("output = %q", out)
	}

	// The application's workspace overrides a setting without a rebuild.
	appWorkspace := t.TempDir()
	os.WriteFile(filepath.Join(appWorkspace, "bind.conf"), []byte("acme-chat.enabled=true\n"), 0o600)
	run := exec.Command(binary)
	run.Env = append(os.Environ(), "RHO_APP_WORKSPACE="+appWorkspace)
	if out, _ := run.Output(); string(out) != "LOG ERROR:pump stopped;chat pump stopped;2" {
		t.Errorf("with the override: output = %q", out)
	}

	// An interface imported twice — by the application and by a file it
	// includes — is one interface with one binding.
	write("app/src/extra.rho", "module extra\nimport acme.alert\nfn warn_once() u32 { return alert.raise(\"WARN\", \"low\") }\n")
	write("app/src/main.rho", "module main\ninclude \"extra.rho\"\nimport acme.alert\nfn main() void {\n    print(int_to_str(alert.raise(\"ERROR\", \"pump stopped\") + warn_once()))\n}\n")
	twice, err := GenerateAllTargets(entry, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("an interface imported twice: %v", err)
	}
	build = exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	build.Dir = twice[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build, imported twice: %v\n%s", err, out)
	}
	if out, _ := exec.Command(binary).Output(); string(out) != "LOG ERROR:pump stopped;LOG WARN:low;2" {
		t.Errorf("imported twice: output = %q", out)
	}

	// A provider whose function does not match the interface is refused.
	write("acme/providers/chat/src/provider.rho", "module chat\nfn send(level: string) u32 { return 1 }\n")
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "acme-chat does not implement acme.alert.send as declared") {
		t.Errorf("mismatched provider: error = %v", err)
	}

	// A provider the application binds but does not declare is refused.
	write("app/module.yml", strings.Replace(manifest, "    acme-chat: \">=0.1.0\"\n", "", 1))
	if _, err := GenerateAllTargets(entry, "", nil, nil, nil); err == nil || !strings.Contains(err.Error(), "acme.alert binds acme-chat, which app does not declare") {
		t.Errorf("undeclared provider: error = %v", err)
	}
}
