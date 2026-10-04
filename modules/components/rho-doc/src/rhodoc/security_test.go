//
// @file security_test.go
// @brief The bounds a documentation build runs inside.
//
// A build reads a tree it was pointed at and writes a directory it was given.
// Both of those are promises worth keeping, because the tree may have been
// written by someone the reader does not know: a page address cannot be allowed
// to write outside the output directory, and a link in a source cannot be
// allowed to carry a scheme that runs in a reader's browser.
//
// These are the tests for those two promises, plus the one about running the
// tree's own plugins.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhodoc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Writing                                                                   */
/* -------------------------------------------------------------------------- */

func TestAPageAddressCannotLeaveTheOutputDirectory(t *testing.T) {
	isolate(t)
	// A page's address comes from what the tree declares, and the tree may have
	// been written by someone the reader does not know, so an address that
	// climbs out of the output directory is refused rather than followed.
	root := t.TempDir()
	out := filepath.Join(root, "site")
	site := &Site{Pages: []*SitePage{{
		Target: "../../escape",
		Href:   "../../escape.html",
		Title:  "Escape",
	}}}

	err := site.Write(out)
	if err == nil {
		t.Fatal("expected an address outside the output directory to be refused")
	}
	if !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected the reason to be named, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "escape.html")); statErr == nil {
		t.Fatal("a page was written outside the output directory")
	}
}

func TestAPageAddressThatIsANestedNameIsFine(t *testing.T) {
	isolate(t)
	// The check refuses an escape, not a nested address: a module's pages live
	// in a directory of the module's name, inside the output directory.
	root := t.TempDir()
	writeFile(t, root, "app/module.yml", "module:\n  name: app\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\n")
	writeFile(t, root, "app/docs/index.md", "# Index\n")

	modules, _ := DiscoverModules(root)
	site := BuildSite(modules, ApiSet{}, SiteOptions{})
	out := filepath.Join(root, "site")
	if err := site.Write(out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "app", "index.html")); err != nil {
		t.Fatalf("expected the page inside the output directory: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Links                                                                     */
/* -------------------------------------------------------------------------- */

func TestALinkSchemeThatWouldRunIsNotWritten(t *testing.T) {
	isolate(t)
	// A generated site is served to readers, so a source cannot decide that a
	// link runs a script in the browser of whoever clicks it.
	document := Parse("p.md", "# T\n\n[click](javascript:alert(1))\n\n![img](data:text/html;base64,PHNjcmlwdD4=)\n\n[ok](https://example.com)\n\n[rel](./other.md)\n\n[anchor](#a-heading)\n")

	page := RenderPage(document)
	if strings.Contains(page.Body, "javascript:") {
		t.Fatalf("a javascript destination reached the page:\n%s", page.Body)
	}
	if strings.Contains(page.Body, "data:text/html") {
		t.Fatalf("a data destination reached the page:\n%s", page.Body)
	}
	for _, want := range []string{"https://example.com", "./other.md", "#a-heading"} {
		if !strings.Contains(page.Body, want) {
			t.Fatalf("expected %s to survive, got:\n%s", want, page.Body)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Plugins                                                                   */
/* -------------------------------------------------------------------------- */

func TestTheTreesOwnPluginsAreNotRunByDefault(t *testing.T) {
	isolate(t)
	// A plugin is a program. An installed plugin was published by someone the
	// user chose to install; a plugin in the tree being documented was supplied
	// by whoever wrote that tree.
	root := t.TempDir()
	writePlugin(t, root, "toc", ComponentName, "  - "+HookIndex+"\n",
		"\ncommands:\n"+aCommand(t, "contents", HookIndex, "contents"))

	if plugins := (Source{Tree: root}).Discover(ComponentName); len(plugins) != 0 {
		t.Fatalf("expected a build to leave the tree's own plugins alone, got %+v", plugins)
	}
}
