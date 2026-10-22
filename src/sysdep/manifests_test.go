//
// @file manifests_test.go
// @brief Every prebuilt archive a module in this tree offers names its digest.
//
// A toolchain that is fetched is fetched by a program that refuses a recipe with no
// digest — so a manifest that forgot one is not a risk but a feature that does not
// work, and the way it would be found is a person trying to install a debugger.
// This finds it first.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package sysdep

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"rho/src/yaml"
)

var sixtyFourHex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Each `fetch` block in the components and plugins this repository carries has a
// well-formed digest.
func TestEveryShippedFetchRecipeNamesItsDigest(t *testing.T) {
	manifests, err := filepath.Glob("../../modules/*/*/module.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Skip("no module manifests found from here")
	}
	var recipes int
	for _, path := range manifests {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		document, err := yaml.Load(string(data))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		deps := Parse(yaml.Value{Kind: yaml.KindMap, Map: document.Map})
		if deps == nil {
			continue
		}
		for _, program := range deps.Executables {
			if program.Fetch == nil {
				continue
			}
			recipes++
			if !sixtyFourHex.MatchString(program.Fetch.SHA256) {
				t.Errorf("%s: the fetch recipe for %s has no valid sha256 (%q)",
					path, program.Name, program.Fetch.SHA256)
			}
		}
	}
	// A check that found nothing to check passes whatever the manifests say. rho-dbg
	// carries one recipe, so none found means the walk is looking in the wrong place.
	if recipes == 0 {
		t.Fatal("no fetch recipe was found in any manifest; the check is not looking where they are")
	}
	t.Logf("%d fetch recipes checked", recipes)
}
