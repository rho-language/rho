package core

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

/* -------------------------------------------------------------------------- */
/*  Test: Valid Module                                                         */
/* -------------------------------------------------------------------------- */

func TestValidateModule(t *testing.T) {
    tmp := t.TempDir()

    // module.yml
    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello
  type: module
  version: 0.1.0

main: src/lib.rho
`), 0644)

    // src/lib.rho
    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "lib.rho"), []byte("module hello"), 0644)

    if err := ValidateModule(tmp); err != nil {
        t.Fatalf("ValidateModule failed: %v", err)
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing main                                                         */
/* -------------------------------------------------------------------------- */

func TestValidateMissingMain(t *testing.T) {
    tmp := t.TempDir()

    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: hello
  type: module
  version: 0.1.0

main: src/lib.rho
`), 0644)

    os.Mkdir(filepath.Join(tmp, "src"), 0755)

    if err := ValidateModule(tmp); err == nil {
        t.Fatalf("expected error for missing main file")
    }
}

/* -------------------------------------------------------------------------- */
/*  Test: Plugin Validation                                                    */
/* -------------------------------------------------------------------------- */

func TestValidatePlugin(t *testing.T) {
    tmp := t.TempDir()

    os.WriteFile(filepath.Join(tmp, "module.yml"), []byte(`
module:
  name: plug
  type: plugin
  version: 0.1.0

main: src/plugin.rho
`), 0644)

    os.Mkdir(filepath.Join(tmp, "src"), 0755)
    os.WriteFile(filepath.Join(tmp, "src", "plugin.rho"), []byte("module plug"), 0644)

    writeComponent(t, filepath.Dir(tmp), "host", []string{"init"})
    writeAttachedPlugin(t, tmp, "testplugin", "host", []string{"init"}, [][2]string{{"init", "init"}})

    if err := ValidateModule(tmp); err != nil {
        t.Fatalf("plugin validation failed: %v", err)
    }
}

// writeComponent writes a component that offers a set of hooks, in the
// component tree of a repository, which is where a resolver looks for one.
func writeComponent(t *testing.T, root, name string, hooks []string) {
	t.Helper()
	dir := filepath.Join(root, "modules", "components", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n" +
		"  name: " + name + "\n" +
		"  type: component\n" +
		"  version: 0.1.0\n" +
		"  command: " + name + "\n" +
		"\n" +
		"main: " + name + "\n" +
		"\n" +
		"provides:\n" +
		"  hooks:\n"
	for _, hook := range hooks {
		manifest += "    - " + hook + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
}

// writeAttachedPlugin writes a plugin that declares the component it plugs into
// and the hooks of that component it answers.
func writeAttachedPlugin(t *testing.T, dir, name, component string, hooks []string, commands [][2]string) {
	t.Helper()
	manifest := "module:\n" +
		"  name: " + name + "\n" +
		"  type: plugin\n" +
		"  version: 0.1.0\n" +
		"\n" +
		"main: " + name + "\n" +
		"\n" +
		"extends: " + component + "\n" +
		"\n" +
		"hooks:\n"
	for _, hook := range hooks {
		manifest += "  - " + hook + "\n"
	}
	if len(commands) > 0 {
		manifest += "\ncommands:\n"
		for _, command := range commands {
			manifest += "  - name: " + command[0] + "\n" +
				"    run: " + name + "\n" +
				"    hook: " + command[1] + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Schema Validation Gates                                             */
/* -------------------------------------------------------------------------- */

// writeModule writes a module.yml plus its main file into dir.
func writeModule(t *testing.T, dir, manifest string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "main.rho"), []byte("module app"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsUnknownModuleType(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: bogus\n  version: 0.1.0\n\nmain: src/main.rho\n")

	err := ValidateModule(tmp)
	if err == nil {
		t.Fatal("expected error for unknown module type")
	}
	if !contains(err.Error(), "module.type") {
		t.Fatalf("expected module.type in error, got: %v", err)
	}
}

func TestValidateRejectsMissingModuleSection(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "main: src/main.rho\n")

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for missing module section")
	}
}

func TestValidateRejectsNonRhoMain(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte("module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: build/app\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := ValidateModule(tmp)
	if err == nil {
		t.Fatal("expected error for non-.rho main in rho module")
	}
	if !contains(err.Error(), "main") {
		t.Fatalf("expected main in error, got: %v", err)
	}
}

func TestValidateRejectsMalformedHooks(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "module.yml"), []byte("module:\n  name: plug\n  type: plugin\n  version: 0.1.0\n\nmain: src/plugin.rho\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "src", "plugin.rho"), []byte("module plug"), 0644); err != nil {
		t.Fatal(err)
	}
	// The plugin names a component but claims no hook of it, so nothing it
	// writes would ever be called.
	writeComponent(t, tmp, "host", []string{"init"})
	writeAttachedPlugin(t, tmp, "testplugin", "host", nil, nil)

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for a plugin that declares no hooks")
	}
}

func TestValidateRejectsAPluginThatExtendsNothing(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: orphan\n  type: plugin\n  version: 0.1.0\n\nmain: src/main.rho\n")

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for a plugin that names no component")
	}
}

func TestValidateRejectsAPluginWhoseHookIsNotOffered(t *testing.T) {
	tmp := t.TempDir()
	writeComponent(t, tmp, "host", []string{"init"})
	writeAttachedPlugin(t, tmp, "testplugin", "host", []string{"shutdown"}, [][2]string{{"shutdown", "shutdown"}})

	err := ValidateModule(tmp)
	if err == nil {
		t.Fatal("expected error for a hook the component does not offer")
	}
	if !strings.Contains(err.Error(), "not offered") {
		t.Fatalf("expected the reason to be named, got %v", err)
	}
}

func TestValidateRejectsAPluginWhoseComponentIsMissing(t *testing.T) {
	tmp := t.TempDir()
	writeAttachedPlugin(t, tmp, "testplugin", "absent", []string{"init"}, [][2]string{{"init", "init"}})

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for a component that cannot be resolved")
	}
}

func TestValidateRejectsACommandForAnUndeclaredHook(t *testing.T) {
	tmp := t.TempDir()
	writeComponent(t, tmp, "host", []string{"init"})
	writeAttachedPlugin(t, tmp, "testplugin", "host", []string{"init"}, [][2]string{{"shutdown", "shutdown"}})

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for a command answering a hook the plugin does not declare")
	}
}

func TestValidateAcceptsDependsList(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\ndepends:\n  - name: common\n    git: https://example.com/common.git\n    tag: v1.0.0\n")

	if err := ValidateModule(tmp); err != nil {
		t.Fatalf("expected depends list to validate, got: %v", err)
	}
}

func TestValidateRejectsBadSemver(t *testing.T) {
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: rho\n  version: not-a-version\n\nmain: src/main.rho\n")

	if err := ValidateModule(tmp); err == nil {
		t.Fatal("expected error for invalid semver")
	}
}

func TestValidateRejectsAPluginThatExtendsAPlugin(t *testing.T) {
	// A plugin plugs into a component, which is the thing that runs a tool. A
	// plugin that extended another plugin would be a module nothing calls,
	// which is exactly what validation is for.
	// The other plugin has to be installed for it to resolve at all, which is
	// the case this check is for: a plugin that another plugin names.
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	installed := filepath.Join(home, "modules", "installed", "plugin", "host@0.1.0")
	if err := os.MkdirAll(installed, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: host\n  type: plugin\n  version: 0.1.0\n\nmain: host\n\nextends: rho-doc\n\nhooks:\n  - index\n\ncommands:\n  - name: index\n    run: host\n    hook: index\n"
	if err := os.WriteFile(filepath.Join(installed, "module.yml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	writeAttachedPlugin(t, tmp, "testplugin", "host", []string{"index"}, [][2]string{{"index", "index"}})

	err := ValidateModule(tmp)
	if err == nil || !strings.Contains(err.Error(), "a plugin plugs into a component") {
		t.Fatalf("expected a plugin extending a plugin to be rejected, got %v", err)
	}
}

func TestValidateRejectsWhatANonComponentOffers(t *testing.T) {
	// Only a component runs a tool, so only a component has something to offer
	// a plugin.
	tmp := t.TempDir()
	writeModule(t, tmp, "module:\n  name: app\n  type: rho\n  version: 0.1.0\n\nmain: src/main.rho\n\nprovides:\n  hooks:\n    - index\n")

	err := ValidateModule(tmp)
	if err == nil || !strings.Contains(err.Error(), "cannot offer hooks") {
		t.Fatalf("expected a non-component offering hooks to be rejected, got %v", err)
	}
}
