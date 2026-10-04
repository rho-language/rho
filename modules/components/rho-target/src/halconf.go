//
// @file halconf.go
// @brief Writing the header a vendor's HAL asks for by name.
//
// The STMicroelectronics HAL has an umbrella header that opens with
//
//	#include "stm32f4xx_hal_conf.h"
//
// — the bare name, not a path. Nothing in the vendor's tree satisfies that
// include: what the vendor ships is `stm32f4xx_hal_conf_template.h`, a template
// that a project is expected to copy, edit and keep. Every ST project in
// existence has one of these in it, written by hand, kept in git, and going stale
// silently: the template moves with a vendor release and the copy does not.
//
// So the toolchain writes it, during a build, from the vendor's own template and
// from two things the project already declares:
//
//   - which drivers are enabled, from the platform's own manifest, because that
//     is what the platform supports and a project does not choose;
//   - the clock values, from the target spec, because that is where a board's
//     crystal lives and two boards of one family need not have the same.
//
// Everything else is the template's, untouched. The template is the only
// authoritative statement of what those macros mean, and a hand-written
// reproduction of it is a second copy to keep in step with the vendor's.
//
// Written into the generated project rather than into `vendor/`, because it is a
// build output: it is derived from a build's inputs and changes when they change.
// A generated file kept in the source tree is generated once and then stale for
// ever, which is the thing this replaces.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rho/rho-target/src/rhtarget"
	"rho/src/deps"
)

// How a vendor names these three files, which differ only in their ending: the
// umbrella header includes `stm32f4xx_hal_conf.h`, the template is
// `stm32f4xx_hal_conf_template.h`, and this program writes the first from the
// second.
const (
	halHeaderSuffix       = "_hal.h"
	halConfSuffix         = "_conf.h"
	halConfTemplateSuffix = "_conf_template.h"
)

// halModulePattern is a `#define` of one of the vendor's driver enable macros.
//
// Matched whole so that a module this project does not enable can be switched
// off rather than left on: the template enables every one of them, and a board
// compiled with a driver it does not have produces a link error naming a symbol
// that was never meant to be there.
var halModulePattern = regexp.MustCompile(`^(\s*)#define\s+(HAL_[A-Z0-9_]+_MODULE_ENABLED)\s*$`)

// clockValuePattern is `#define HSE_VALUE 25000000`, which the target spec
// declares as `HSE_VALUE=25000000`.
var clockValuePattern = regexp.MustCompile(`^\s*#define\s+([A-Z0-9_]*VALUE[A-Z0-9_]*)\s+`)

// halConfName is the header name a family's HAL asks for, from the HAL header
// the spec declared: `stm32f4xx_hal.h` names `stm32f4xx`, and that is the family.
func halConfName(header string) (string, bool) {
	header = strings.Trim(strings.TrimSpace(header), `"`)
	if !strings.HasSuffix(header, halHeaderSuffix) {
		return "", false
	}
	// The family is the header's own stem: `stm32f4xx_hal.h` is the name of the
	// F4 family, and the configuration header is that stem with `_conf.h` where
	// `_hal.h` was. Deriving the whole name from the one the spec gave is what
	// keeps them in step — the vendor names them together and a build that
	// spelled the second differently would look for a file that was never going
	// to be fetched.
	stem := strings.TrimSuffix(header, ".h")
	if stem == header || stem == "" {
		return "", false
	}
	return stem + halConfSuffix, true
}

// findHalTemplate looks for the vendor's template for a family, in the platform's
// vendor tree.
//
// Only there, because that is where the HAL driver repository is fetched and the
// template is inside it. Walking the whole project instead would find a template
// in a directory nobody declared as a vendor tree — a vendored copy in a
// dependency, an editor's cache — and derive this platform's HAL from whichever
// came first.
func findHalTemplate(project string, name string) (string, bool) {
	wanted := name + halConfTemplateSuffix
	for _, root := range vendorTrees(project) {
		found := ""
		filepath.WalkDir(root, func(current string, entry os.DirEntry, err error) error {
			if err != nil || found != "" {
				return nil
			}
			if !entry.IsDir() && entry.Name() == wanted {
				found = current
			}
			return nil
		})
		if found != "" {
			return found, true
		}
	}
	return "", false
}

// vendorTrees are the vendor directories a project's dependencies declare.
func vendorTrees(project string) []string {
	trees := []string{}
	seen := map[string]bool{}
	for _, root := range cDependencyRoots(project) {
		module, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || module == nil || module.VendorDir == "" {
			continue
		}
		directory := module.VendorDir
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(root, directory)
		}
		if seen[directory] {
			continue
		}
		seen[directory] = true
		trees = append(trees, directory)
	}
	return trees
}

// cDependencyRoots are the roots of the C modules a project declares, which is
// where a platform and its vendor tree are.
func cDependencyRoots(project string) []string {
	roots := []string{}
	seen := map[string]bool{}
	var walk func(base string, depth int)
	walk = func(base string, depth int) {
		if depth > 8 {
			return
		}
		module, err := deps.LoadManifest(deps.ManifestPath(base))
		if err != nil || module == nil {
			return
		}
		for _, dependency := range module.Deps {
			if dependency.Type != deps.DepTypeC {
				continue
			}
			path := dependency.Path
			if path == "" {
				continue
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(base, path)
			}
			if seen[path] {
				continue
			}
			seen[path] = true
			roots = append(roots, path)
			walk(path, depth+1)
		}
	}
	walk(project, 0)
	return roots
}

// halModulesEnabled is the list of driver macros this platform turns on, from its
// own manifest.
//
// From the platform rather than the application, because what a platform supports
// is the platform's to say and an application is not in a position to know. An
// application that could switch drivers on would be able to ask for one the
// platform has no code behind, and the failure would be a link error several
// thousand lines away from the request.
func halModulesEnabled(roots []string) []string {
	enabled := []string{}
	seen := map[string]bool{}
	for _, root := range roots {
		module, err := deps.LoadManifest(deps.ManifestPath(root))
		if err != nil || module == nil {
			continue
		}
		for _, define := range module.BuildDefines {
			if !strings.HasPrefix(define, "HAL_") {
				continue
			}
			if !seen[define] {
				seen[define] = true
				enabled = append(enabled, define)
			}
		}
	}
	return enabled
}

// writeHalConf writes the configuration header a family's HAL includes by name.
//
// Written into the generated project, which is on the include path, so the bare
// `#include "stm32f4xx_hal_conf.h"` in the vendor's umbrella header finds it
// without anything having to be told where it is.
func writeHalConf(project string, spec rhtarget.Spec) error {
	name, ok := halConfName(spec.HALHeader)
	if !ok {
		// A build that names no vendor HAL has no configuration header to write.
		// Not an error: the host target, a Raspberry Pi and anything else whose
		// spec says nothing about a vendor HAL are exactly this.
		return nil
	}
	template, ok := findHalTemplate(project, strings.TrimSuffix(name, halConfSuffix))
	if !ok {
		// The vendor ships no template, so there is nothing to derive it from. Said
		// rather than left to fail later on a missing header, which names a file
		// and not the reason.
		return fmt.Errorf("this build is for %s, but no vendor tree holds the vendor's %s%s: "+
			"the HAL driver repository for this family has not been fetched",
			spec.Name, name, halConfTemplateSuffix)
	}

	// Where it goes, and whether it is already there.
	//
	// The platform's vendor tree, because that is where the vendor's own half of
	// this lives and a generated file about the vendor belongs beside it rather
	// than in the project that happened to need it: two projects of different
	// families would otherwise each write their own, and each would be the only
	// copy of a thing that is a property of the family.
	destination, ok := vendorIncludeFor(project, name)
	if !ok {
		return fmt.Errorf("this build is for %s, but no dependency declares a vendor tree to put %s in", spec.Name, name)
	}
	if _, err := os.Stat(destination); err == nil {
		// Already there. Left as it is rather than regenerated, because it is a
		// file a person may have looked at and a build that overwrites it every
		// time is a build nobody will ever edit one.
		return nil
	}

	source, err := os.ReadFile(template)
	if err != nil {
		return fmt.Errorf("read the vendor's HAL template: %w", err)
	}
	enabled := halModulesEnabled(cDependencyRoots(project))
	content := deriveHalConf(string(source), name, enabled, spec.Defines)

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// vendorIncludeFor is where a platform keeps the headers that answer the vendor's
// by name.
//
// The `includes` directory inside the platform's own vendor tree, because that is
// what it is for: the vendor's umbrella header includes a configuration header by
// a bare name, and a directory of headers that satisfy those names is what makes
// that work without every file having to spell out a path.
func vendorIncludeFor(project, name string) (string, bool) {
	trees := vendorTrees(project)
	if len(trees) == 0 {
		return "", false
	}
	return filepath.Join(trees[0], "includes", name), true
}

// deriveHalConf produces the header from the vendor's template.
//
// Three changes and no others: the include guard, which the template writes with
// `_TEMPLATE_` in it and which would collide with the other families' headers if
// they were all on one include path; the driver enables; and the clock values the
// target spec declared.
func deriveHalConf(template, name string, enabled, defines []string) string {
	family := strings.TrimSuffix(name, halConfSuffix)
	guard := strings.ToUpper(family) + "_CONF_H"
	text := strings.ReplaceAll(template, family+"_TEMPLATE_H", guard)
	text = strings.ReplaceAll(text, family+"_TEMPLATE_H_", guard+"_")

	var b strings.Builder
	b.WriteString("/*\n")
	b.WriteString(" * @file " + name + "\n")
	b.WriteString(" * @brief This family's HAL configuration, written by rho-target from the vendor's\n")
	b.WriteString(" *        own template during a build.\n")
	b.WriteString(" *\n")
	b.WriteString(" * Generated. Everything below the vendor's own notice is the vendor's, with the\n")
	b.WriteString(" * driver enables from this platform's manifest and the clock values from this\n")
	b.WriteString(" * build's target spec. Nothing here is written by hand, and editing it is\n")
	b.WriteString(" * pointless: the next build writes it again.\n")
	b.WriteString(" *\n")
	b.WriteString(" * SPDX-License-Identifier: Apache-2.0\n")
	b.WriteString(" */\n\n")

	clocks := clockValues(defines)
	for _, line := range strings.Split(text, "\n") {
		if match := halModulePattern.FindStringSubmatch(line); match != nil {
			indent, module := match[1], match[2]
			if !containsString(enabled, module) {
				line = indent + "/* #define " + module + " — not a driver this platform has code for */"
			}
			b.WriteString(line)
			b.WriteString("\n")
			continue
		}
		if match := clockValuePattern.FindStringSubmatch(line); match != nil {
			if value, ok := clocks[match[1]]; ok {
				b.WriteString(fmt.Sprintf("%s#define %s %s\n", match[0][:len(match[0])-len(strings.TrimLeft(match[0], " "))], match[1], value))
				continue
			}
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// clockValues are the `NAME=value` pairs a target spec declared.
func clockValues(defines []string) map[string]string {
	values := map[string]string{}
	for _, define := range defines {
		name, value, found := strings.Cut(define, "=")
		if !found {
			continue
		}
		name = strings.TrimSpace(name)
		if !strings.Contains(name, "VALUE") {
			continue
		}
		values[name] = value
	}
	return values
}

func containsString(held []string, needle string) bool {
	for _, one := range held {
		if one == needle {
			return true
		}
	}
	return false
}
