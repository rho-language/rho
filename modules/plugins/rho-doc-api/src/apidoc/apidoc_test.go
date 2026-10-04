//
// @file apidoc_test.go
// @brief Tests for the API reference plugin.
//
// The plugin is a protocol boundary: it reads a JSON request from one process
// and writes a JSON answer for another. What is tested here is the two shapes
// it has to get right — the envelope it decodes, and the page it answers with —
// and the two cases that decide whether it is quiet or noisy: a project with no
// routes, and a project whose sources cannot be read.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package apidoc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// answer runs the plugin over a request and returns what it wrote.
func answer(t *testing.T, request Request) Response {
	t.Helper()
	content, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(bytes.NewReader(content), &out); err != nil {
		t.Fatalf("the plugin failed: %v", err)
	}
	response := Response{}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("the plugin did not answer with the protocol: %v: %s", err, out.String())
	}
	return response
}

// siteRequest wraps a site in the envelope a whole-site hook is answered with.
func siteRequest(t *testing.T, payload SitePayload) Request {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		Version: ProtocolVersion, Hook: HookIndex,
		Command: "api-reference", Component: "rho-doc", Payload: encoded,
	}
}

// service writes a project with routes and returns its directory.
func service(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "module.yml"), `module:
  name: catalog
  type: rho
  version: 1.0.0

main: src/main.rho
`)
	write(t, filepath.Join(root, "src", "main.rho"), `module catalog

import std.web.http

// Show one item, read by its identifier.
route GET "/items/{id}" => show_item

fn show_item(req: Request) Response {
    let id: u32 = path_u32(req, "id")
    if !bind_ok(req) {
        return json(400, bind_error(req))
    }
    return json(200, "ok")
}
`)
	return root
}

// library writes a project with no routes and returns its directory.
func library(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "module.yml"), "module:\n  name: strings\n  type: rho\n  version: 1.0.0\n")
	write(t, filepath.Join(root, "src", "lib.rho"), "module strings\n\n// join concatenates two strings.\nfn join(a: string, b: string) string {\n    return a + b\n}\n")
	return root
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAProjectWithRoutesGetsAPage(t *testing.T) {
	response := answer(t, siteRequest(t, SitePayload{Site: "Catalogue", Root: service(t)}))
	if len(response.Pages) != 1 {
		t.Fatalf("expected one page, got %+v", response.Pages)
	}
	page := response.Pages[0]
	if page.Target != PageTarget || page.Title != PageTitle {
		t.Errorf("unexpected page: %+v", page)
	}
	// The section is what puts the page in the same sidebar as the module
	// reference, which is the whole reason this is a page and not a document.
	if page.Section != PageSection {
		t.Errorf("the page is listed under %q, want %q", page.Section, PageSection)
	}
	for _, want := range []string{"/items/{id}", "show_item", "Parameters"} {
		if !strings.Contains(page.HTML, want) {
			t.Errorf("the page does not mention %q", want)
		}
	}
}

func TestAProjectWithNoRoutesGetsNothing(t *testing.T) {
	// Most modules are libraries. A page saying "this module serves no HTTP
	// routes" would be noise on every one of them, so a module with no routes
	// gets no page and says nothing about it.
	response := answer(t, siteRequest(t, SitePayload{Site: "Strings", Root: library(t)}))
	if len(response.Pages) != 0 {
		t.Errorf("a library was given an API page: %+v", response.Pages)
	}
	if len(response.Diagnostics) != 0 {
		t.Errorf("a library was reported as a problem: %+v", response.Diagnostics)
	}
}

func TestABuildWithNoProjectGetsNothing(t *testing.T) {
	// A build of pages that came from somewhere other than a project directory
	// has no routes to read, and this plugin says nothing about it.
	response := answer(t, siteRequest(t, SitePayload{Site: "Pages"}))
	if len(response.Pages) != 0 || len(response.Diagnostics) != 0 {
		t.Errorf("a build with no project was answered with %+v", response)
	}
}

func TestAProjectThatCannotBeReadIsReported(t *testing.T) {
	// A missing page is worth a reason in the log: an API reference that is
	// silently absent is a reference nobody knows is missing.
	response := answer(t, siteRequest(t, SitePayload{
		Site: "Broken", Root: filepath.Join(t.TempDir(), "nowhere"),
	}))
	if len(response.Pages) != 0 {
		t.Errorf("a project that could not be read was described anyway: %+v", response.Pages)
	}
	if len(response.Diagnostics) != 1 {
		t.Fatalf("expected the failure to be reported, got %+v", response.Diagnostics)
	}
	if !strings.Contains(response.Diagnostics[0].Message, "could not be read") {
		t.Errorf("the diagnostic does not say what went wrong: %q", response.Diagnostics[0].Message)
	}
}

func TestARequestInAnotherProtocolVersionIsReported(t *testing.T) {
	request := siteRequest(t, SitePayload{Site: "Catalogue", Root: service(t)})
	request.Version = ProtocolVersion + 1

	response := answer(t, request)
	if len(response.Diagnostics) != 1 {
		t.Fatalf("the version mismatch was not reported: %+v", response)
	}
	if !strings.Contains(response.Diagnostics[0].Message, "speaks protocol") {
		t.Errorf("the reason is not named: %q", response.Diagnostics[0].Message)
	}
	if len(response.Pages) != 0 {
		t.Errorf("a request it cannot read was answered with pages: %+v", response.Pages)
	}
}

func TestTheSiteIsInThePayloadNotTheEnvelope(t *testing.T) {
	// The envelope is the toolchain's and says which hook is being answered; the
	// payload is the component's and says what it is about. A plugin that read
	// the site off the envelope's top level would find nothing, and that is a
	// bug the shape of the request alone would not reveal.
	content, err := json.Marshal(siteRequest(t, SitePayload{
		Site: "Catalogue", Root: service(t),
	}))
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(content, &envelope); err != nil {
		t.Fatal(err)
	}
	if _, flat := envelope["root"]; flat {
		t.Error("the site is on the envelope's top level rather than in the payload")
	}
	payload, ok := envelope["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("there is no payload: %s", content)
	}
	if payload["root"] == "" {
		t.Error("the payload does not carry the project directory")
	}
}

func TestThePlainTextListsTheEndpoints(t *testing.T) {
	// The plain text is what a search index reads, so a page a plugin added is
	// searchable like any other rather than a page nobody finds.
	response := answer(t, siteRequest(t, SitePayload{Site: "Catalogue", Root: service(t)}))
	if len(response.Pages) != 1 {
		t.Fatalf("expected one page, got %+v", response.Pages)
	}
	plain := response.Pages[0].Plain
	if !strings.Contains(plain, "GET /items/{id}") {
		t.Errorf("the plain text does not list the endpoint:\n%s", plain)
	}
	if !strings.Contains(plain, "Show one item") {
		t.Errorf("the plain text does not carry the summary:\n%s", plain)
	}
}

func TestThePageTargetDoesNotCollideWithTheModuleReference(t *testing.T) {
	// rho-doc writes its own api.html for the module reference. A plugin that
	// chose the same target would silently overwrite it, which is the one way a
	// plugin can cost a reader a page rather than add one.
	if PageTarget == "api" {
		t.Error("the API page targets the module reference's own address")
	}
}
