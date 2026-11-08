//
// @file locale_report.go
// @brief Coverage of a locale catalog against the default language.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import "sort"

// LocaleReport is what a language is missing from the default catalog and what
// it carries that the default does not.
type LocaleReport struct {
	Missing []string
	Unused  []string
}

// localeReports compares every non-default catalog with the default one. The
// result is keyed by language and sorted, so a build warning is stable.
func localeReports(defaultLocale string, catalogs map[string]map[string]string) map[string]LocaleReport {
	reports := map[string]LocaleReport{}
	defaultKeys := catalogs[defaultLocale]
	if defaultKeys == nil {
		return reports
	}
	for locale, catalog := range catalogs {
		if locale == defaultLocale {
			continue
		}
		report := LocaleReport{}
		for key := range defaultKeys {
			if _, ok := catalog[key]; !ok {
				report.Missing = append(report.Missing, key)
			}
		}
		for key := range catalog {
			if _, ok := defaultKeys[key]; !ok {
				report.Unused = append(report.Unused, key)
			}
		}
		sort.Strings(report.Missing)
		sort.Strings(report.Unused)
		reports[locale] = report
	}
	return reports
}
