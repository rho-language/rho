//
// @file i18n_test.go
// @brief Tests for locale catalog parsing and generation.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDescriptorLocales(t *testing.T) {
	source := `version: 1
targets:
  - id: go-server
    emitter: go
    kind: server
    default_locale: en
    locales:
      - locale: en
        file: locales/en.json
      - locale: ro
        file: locales/ro.json
    files:
      - path: main.go
        role: entrypoint
`
	descriptor, err := ParseDescriptor([]byte(source))
	if err != nil {
		t.Fatalf("ParseDescriptor failed: %v", err)
	}
	target := descriptor.Targets[0]
	if target.DefaultLocale != "en" {
		t.Errorf("default locale = %q", target.DefaultLocale)
	}
	if len(target.Locales) != 2 || target.Locales[1].Code != "ro" {
		t.Errorf("locales = %#v", target.Locales)
	}
}

func TestParseDescriptorLocaleErrors(t *testing.T) {
	cases := []string{
		`version: 1
targets:
  - id: go-server
    emitter: go
    kind: server
    default_locale: ro
    locales:
      - locale: en
        file: locales/en.json
    files:
      - path: main.go
        role: entrypoint
`,
		`version: 1
targets:
  - id: go-server
    emitter: go
    kind: server
    locales:
      - locale: e
        file: locales/e.json
    files:
      - path: main.go
        role: entrypoint
`,
		`version: 1
targets:
  - id: go-server
    emitter: go
    kind: server
    locales:
      - locale: en
        file: /etc/en.json
    files:
      - path: main.go
        role: entrypoint
`,
	}
	for index, source := range cases {
		if _, err := ParseDescriptor([]byte(source)); err == nil {
			t.Errorf("case %d: expected an error", index)
		}
	}
}

func TestGenerateLocalesEmbedsAndOverrides(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "locales"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locales", "en.json"),
		[]byte(`{"greet":"Hello","base":"app"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locales", "module.json"),
		[]byte(`{"base":"module","extra":"x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locales", "ro.json"),
		[]byte(`{"greet":"Salut"}`), 0644); err != nil {
		t.Fatal(err)
	}
	target := Target{
		DefaultLocale: "en",
		Locales: []Locale{
			{Code: "en", File: "locales/en.json"},
			{Code: "en", File: "locales/module.json"},
			{Code: "ro", File: "locales/ro.json"},
		},
		Templates: []Template{{Name: "page", File: "page.html"}},
	}
	ctx := Context{ProjectRoot: root, OutputDir: out}
	if err := generateLocales(ctx, target); err != nil {
		t.Fatalf("generateLocales failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "i18n.go"))
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(data)
	for _, want := range []string{
		`"base": "module"`,
		`"extra": "x"`,
		`"greet": "Salut"`,
		`const i18nDefaultLocale = "en"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("generated catalogs missing %q:\n%s", want, rendered)
		}
	}
}

func TestGenerateLocalesRejectsNestedValues(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "locales"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "locales", "en.json"),
		[]byte(`{"nested":{"value":"x"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	target := Target{DefaultLocale: "en", Locales: []Locale{{Code: "en", File: "locales/en.json"}}}
	err := generateLocales(Context{ProjectRoot: root, OutputDir: out}, target)
	if err == nil || !strings.Contains(err.Error(), "not a string") {
		t.Fatalf("expected a nested-value error, got %v", err)
	}
}
