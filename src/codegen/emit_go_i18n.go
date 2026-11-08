//
// @file emit_go_i18n.go
// @brief Embedded locale catalogs for the Go backend.
//
// A server target carries flat JSON catalogs. They are read at build time and
// emitted as Go maps, so a page never reads a file and a device needs no JSON
// parser at runtime.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// I18nFileName is the generated file holding the locale catalogs.
const I18nFileName = "i18n.go"

// EmitGoI18n renders the embedded catalogs and the lookup the template
// renderer uses. An empty catalog set still emits the file, because a project
// with templates carries the renderer and the renderer names the lookup.
func EmitGoI18n(defaultLocale string, catalogs map[string]map[string]string) string {
	if defaultLocale == "" {
		defaultLocale = "en"
	}

	var b strings.Builder
	b.WriteString(goBanner(irModuleForBanner(), I18nFileName, "Generated Rho locale catalogs."))
	b.WriteString("package main\n\n")
	fmt.Fprintf(&b, "const i18nDefaultLocale = %s\n\n", strconv.Quote(defaultLocale))
	b.WriteString("// The flat catalogs, keyed by language tag and then by message key.\n")
	b.WriteString("var i18nCatalogs = map[string]map[string]string{\n")
	locales := make([]string, 0, len(catalogs))
	for locale := range catalogs {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	for _, locale := range locales {
		fmt.Fprintf(&b, "\t%s: map[string]string{\n", strconv.Quote(locale))
		keys := make([]string, 0, len(catalogs[locale]))
		for key := range catalogs[locale] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "\t\t%s: %s,\n", strconv.Quote(key), strconv.Quote(catalogs[locale][key]))
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString(goI18nHelpers())
	return b.String()
}

// goI18nHelpers is the lookup the generated template renderer calls. A key
// missing from the active language falls back to the default language; a key
// missing from both is reported as absent, and the renderer prints the key.
func goI18nHelpers() string {
	return `// i18nText returns a translated string, falling back to the default locale.
func i18nText(locale string, key string) (string, bool) {
	if catalog, ok := i18nCatalogs[locale]; ok {
		if value, ok := catalog[key]; ok {
			return value, true
		}
	}
	if locale != i18nDefaultLocale {
		if catalog, ok := i18nCatalogs[i18nDefaultLocale]; ok {
			if value, ok := catalog[key]; ok {
				return value, true
			}
		}
	}
	return "", false
}

// i18nHasLocale reports whether a language has a catalog.
func i18nHasLocale(locale string) bool {
	_, ok := i18nCatalogs[locale]
	return ok
}

`
}
