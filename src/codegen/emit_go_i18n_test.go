//
// @file emit_go_i18n_test.go
// @brief Tests for the embedded locale catalogs.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"
)

func TestEmitGoI18n(t *testing.T) {
	rendered := EmitGoI18n("en", map[string]map[string]string{
		"ro": {"greet": `Salut "lume"`},
		"en": {"greet": "Hello"},
	})
	for _, want := range []string{
		`const i18nDefaultLocale = "en"`,
		`"en": map[string]string{`,
		`"greet": "Hello"`,
		`"ro": map[string]string{`,
		`"greet": "Salut \"lume\""`,
		"func i18nText(locale string, key string) (string, bool)",
		"func i18nHasLocale(locale string) bool",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the emitted catalogs do not contain %q", want)
		}
	}
}

func TestEmitGoI18nEmpty(t *testing.T) {
	rendered := EmitGoI18n("", nil)
	if !strings.Contains(rendered, `const i18nDefaultLocale = "en"`) {
		t.Error("an empty catalog set should still name the default locale")
	}
	if !strings.Contains(rendered, "var i18nCatalogs = map[string]map[string]string{") {
		t.Error("an empty catalog set should still declare the map")
	}
}
