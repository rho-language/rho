//
// @file emit_go_template_test.go
// @brief Tests for the template renderer the Go backend embeds.
//
// The renderer is emitted as Go source, so these tests build the emitted file
// with a test that drives it. A renderer that reads correctly but does not
// compile, or that nests blocks incorrectly, is caught here rather than in a
// generated page.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// templateTests is the test file placed beside the generated renderer.
const templateTests = `package main

import (
	"strings"
	"testing"
)

func render(t *testing.T, name string, kv ...string) string {
	t.Helper()
	pairs := ""
	for i := 0; i+1 < len(kv); i += 2 {
		pairs += templateSep + kv[i] + templateSep + kv[i+1]
	}
	return templateFill(templateGet(name), pairs)
}

func TestPlaceholders(t *testing.T) {
	got := render(t, "placeholders", "value", "<b>&</b>")
	want := "&lt;b&gt;&amp;&lt;/b&gt;|<b>&</b>"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFallbacks(t *testing.T) {
	got := render(t, "fallbacks", "present", "set", "empty", "")
	want := "fb|default|set"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestComments(t *testing.T) {
	got := render(t, "comments", "value", "v")
	want := "abv"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConditionals(t *testing.T) {
	cases := []struct {
		flag string
		want string
	}{
		{"true", "yes|some|B"},
		{"", "no|none|B"},
	}
	for _, one := range cases {
		got := render(t, "conditionals", "flag", one.flag)
		if got != one.want {
			t.Errorf("flag %q: got %q, want %q", one.flag, got, one.want)
		}
	}
}

func TestNestedBlocksOfTheSameKind(t *testing.T) {
	got := render(t, "nested", "outer", "true", "inner", "true")
	if got != "ABD" {
		t.Fatalf("outer and inner true: got %q, want %q", got, "ABD")
	}
	got = render(t, "nested", "outer", "true", "inner", "")
	if got != "ACD" {
		t.Fatalf("outer true, inner false: got %q, want %q", got, "ACD")
	}
	got = render(t, "nested", "outer", "", "inner", "true")
	if got != "E" {
		t.Fatalf("outer false: got %q, want %q", got, "E")
	}
}

func TestEachAndItsHelpers(t *testing.T) {
	got := render(t, "each", "items", "a\nb")
	want := "[0:a:true::2][1:b::true:2]"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	got = render(t, "each", "items", "")
	if got != "empty" {
		t.Fatalf("empty loop: got %q, want %q", got, "empty")
	}
}

func TestEachExposesTabSeparatedFields(t *testing.T) {
	got := render(t, "eachfields", "rows", "a\t1\nb\t2")
	if got != "a=1;b=2;" {
		t.Fatalf("got %q, want %q", got, "a=1;b=2;")
	}
}

func TestNestedEachShadowsTheOuterItem(t *testing.T) {
	got := render(t, "nestedeach", "outer", "p1\np2", "inner", "i1\ni2")
	if got != "i1i2i1i2" {
		t.Fatalf("got %q, want %q", got, "i1i2i1i2")
	}
}

func TestWithScopesLookups(t *testing.T) {
	got := render(t, "with", "user.name", "Ann", "user", "U", "name", "outer")
	if got != "Ann|U" {
		t.Fatalf("prefixed scope: got %q, want %q", got, "Ann|U")
	}
	got = render(t, "with", "user", "plain", "name", "outer")
	if got != "outer|plain" {
		t.Fatalf("plain value: got %q, want %q", got, "outer|plain")
	}
	got = render(t, "with", "name", "outer")
	if got != "none" {
		t.Fatalf("absent scope: got %q, want %q", got, "none")
	}
}

func TestWhitespaceTrimming(t *testing.T) {
	got := render(t, "trim", "value", "X")
	if got != "AXB" {
		t.Fatalf("placeholders: got %q, want %q", got, "AXB")
	}
	got = render(t, "trimnewline", "value", "X")
	if got != "AX\nB" {
		t.Fatalf("newline: got %q, want %q", got, "AX\nB")
	}
	got = render(t, "trimcomment")
	if got != "AB" {
		t.Fatalf("comment: got %q, want %q", got, "AB")
	}
	got = render(t, "trimblock", "flag", "true")
	if got != "AyesB" {
		t.Fatalf("block: got %q, want %q", got, "AyesB")
	}
}

func TestTranslation(t *testing.T) {
	got := render(t, "translate", "__locale", "ro")
	if got != "Salut|missing" {
		t.Fatalf("ro translation: got %q", got)
	}
	got = render(t, "translate", "__locale", "en")
	if got != "Hello|missing" {
		t.Fatalf("en translation: got %q", got)
	}
	got = render(t, "translatevars", "__locale", "ro", "value", "24.5")
	if got != "Temperatură: 24.5°C" {
		t.Fatalf("variable translation: got %q", got)
	}
	got = render(t, "translatevars", "__locale", "ro", "value", "<b>")
	if !strings.Contains(got, "&lt;b&gt;") {
		t.Fatalf("translation did not escape the value: %q", got)
	}
	got = render(t, "fallback", "__locale", "ro")
	if got != "English only" {
		t.Fatalf("default-language fallback: got %q", got)
	}
}

func TestPartialsShareTheValues(t *testing.T) {
	got := render(t, "partial", "title", "T")
	if got != "before T after" {
		t.Fatalf("got %q", got)
	}
}
`

// TestGeneratedTemplateRenderer builds the emitted renderer and runs its tests,
// so the source a project receives is the source that is checked.
func TestGeneratedTemplateRenderer(t *testing.T) {
	requireGoToolchain(t)
	dir := t.TempDir()
	contents := map[string]string{
		"placeholders":  `{{ value }}|{{{ value }}}`,
		"fallbacks":     `{{ missing ?? "fb" }}|{{ empty | "default" }}|{{ present ?? "x" }}`,
		"comments":      `a{# ignored #}b{# another {{ bad }} #}{{ value }}`,
		"conditionals":  `{{#if flag}}yes{{else}}no{{/if}}|{{#unless flag}}none{{else}}some{{/unless}}|{{#if missing}}A{{else}}B{{/if}}`,
		"nested":        `{{#if outer}}A{# note #}{{#if inner}}B{{else}}C{{/if}}D{{else}}E{{/if}}`,
		"each":          `{{#each items}}[{{@index}}:{{this}}:{{@first}}:{{@last}}:{{@count}}]{{else}}empty{{/each}}`,
		"eachfields":    `{{#each rows}}{{@0}}={{@1}};{{/each}}`,
		"nestedeach":    `{{#each outer}}{{#each inner}}{{this}}{{/each}}{{/each}}`,
		"with":          `{{#with user}}{{name}}|{{this}}{{else}}none{{/with}}`,
		"trim":          `A {{- value -}} B`,
		"trimnewline":   "A\n  {{- value }}\nB",
		"trimcomment":   `A {#- note -#} B`,
		"trimblock":     `A {{- #if flag -}} yes {{- /if -}} B`,
		"partial":       `before {{> part }} after`,
		"part":          `{{ title }}`,
		"translate":     `{{t "greet"}}|{{t "missing"}}`,
		"translatevars": `{{t "temp" val=value}}`,
		"fallback":      `{{t "english_only"}}`,
	}
	rendered := EmitGoTemplates(contents)
	if !strings.Contains(rendered, "func templateFill") {
		t.Fatal("the emitted source does not carry the renderer")
	}
	files := map[string]string{
		"templates.go":      rendered,
		"templates_test.go": templateTests,
		"i18n.go": EmitGoI18n("en", map[string]map[string]string{
			"en": {
				"greet":        "Hello",
				"temp":         "Temp: {{val}}°C",
				"english_only": "English only",
			},
			"ro": {
				"greet": "Salut",
				"temp":  "Temperatură: {{val}}°C",
			},
		}),
		"go.mod": "module templates\n\ngo 1.21\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the generated renderer failed its tests: %v\n%s", err, out)
	}
}
