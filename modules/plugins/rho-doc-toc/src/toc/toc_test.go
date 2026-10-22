//
// @file toc_test.go
// @brief Tests for the documentation plugin.
//
// The plugin is the worked example of the extension protocol, so these tests
// cover the protocol rather than the markup: what a request produces, what a
// version the plugin does not speak produces, and what an empty request does.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package toc

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// answer runs the plugin over a request and returns what it wrote.
func answer(t *testing.T, request Request) Response {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(bytes.NewReader(payload), &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	response := Response{}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("the plugin did not answer with the protocol: %v: %s", err, out.String())
	}
	return response
}

// indexRequest wraps a site in the envelope a whole-site hook is answered with.
func indexRequest(site Site) Request {
	payload, err := json.Marshal(site)
	if err != nil {
		panic(err)
	}
	return Request{Version: ProtocolVersion, Hook: HookIndex, Component: "rho-doc", Payload: payload}
}

// pageRequest wraps one page in the envelope a per-page hook is answered with.
func pageRequest(site string, page Page) Request {
	payload, err := json.Marshal(Site{Site: site, Page: &page})
	if err != nil {
		panic(err)
	}
	return Request{Version: ProtocolVersion, Hook: HookPage, Component: "rho-doc", Payload: payload}
}

// testRequest is what a build sends: the envelope, with the site in its
// payload. Building the envelope rather than the payload alone is the point of
// the test — a plugin that read the site off the envelope's top level would
// find nothing here, which is exactly the bug this shape exists to catch.
func testRequest() Request {
	payload, err := json.Marshal(Site{
		Site: "Rho",
		Pages: []Page{
			{Target: "index", Title: "Index", Href: "index.html", Section: "guides"},
			{Target: "guide", Title: "Guide", Href: "guides/app/guide.html", Section: "guides"},
			{Target: "api", Title: "API reference", Href: "api.html", Section: "api"},
		},
	})
	if err != nil {
		panic(err)
	}
	return Request{
		Version:   ProtocolVersion,
		Hook:      HookIndex,
		Component: "rho-doc",
		Payload:   payload,
	}
}

func TestTheIndexHookBuildsAContentsPage(t *testing.T) {
	response := answer(t, testRequest())
	if len(response.Pages) != 1 {
		t.Fatalf("expected one page, got %+v", response.Pages)
	}
	page := response.Pages[0]
	if page.Target != "contents" || page.Title != "Contents" {
		t.Fatalf("unexpected page: %+v", page)
	}
	// Every page in the request is linked, and the links are the addresses the
	// build gave, not paths the plugin invented.
	for _, want := range []string{"index.html", "guides/app/guide.html", "api.html", "Index", "API reference"} {
		if !strings.Contains(page.HTML, want) {
			t.Errorf("expected %q in the contents, got:\n%s", want, page.HTML)
		}
	}
}

func TestTheContentsPageSitsWithTheMostPages(t *testing.T) {
	response := answer(t, testRequest())
	if response.Pages[0].Section != "" {
		t.Fatalf("expected the contents beside the pages, got %q", response.Pages[0].Section)
	}
}

func TestThePageHookAddsAGenerationNotice(t *testing.T) {
	page := Page{Target: "api-std-core-option", Title: "std.core.option", HTML: "<h1>std.core.option</h1>"}
	request := pageRequest("Rho", page)

	response := answer(t, request)
	if len(response.Replacements) != 1 {
		t.Fatalf("expected a replacement, got %+v", response.Replacements)
	}
	body := response.Replacements["api-std-core-option"]
	if !strings.Contains(body, "generated") {
		t.Fatalf("expected a generation notice, got:\n%s", body)
	}
	// The page's own content survives: the notice is added, not substituted.
	if !strings.Contains(body, "<h1>std.core.option</h1>") {
		t.Fatalf("expected the page to be kept, got:\n%s", body)
	}
}

func TestARequestInAnotherProtocolVersionIsReported(t *testing.T) {
	request := testRequest()
	request.Version = ProtocolVersion + 1

	response := answer(t, request)
	if len(response.Diagnostics) != 1 {
		t.Fatalf("expected the version mismatch to be reported, got %+v", response)
	}
	if !strings.Contains(response.Diagnostics[0].Message, "speaks protocol") {
		t.Fatalf("expected the reason to be named, got %q", response.Diagnostics[0].Message)
	}
	if len(response.Pages) != 0 {
		t.Fatalf("expected no pages from a request it cannot read, got %+v", response.Pages)
	}
}

func TestAnEmptyRequestIsAnswered(t *testing.T) {
	// A build that found nothing still asks its plugins, and a plugin that
	// cannot build a contents page has to say so rather than fail.
	response := answer(t, indexRequest(Site{Site: "Rho"}))
	if len(response.Pages) != 1 {
		t.Fatalf("expected a contents page even with nothing in it, got %+v", response.Pages)
	}
	if !strings.Contains(response.Pages[0].HTML, "All pages") {
		t.Fatalf("expected a heading, got:\n%s", response.Pages[0].HTML)
	}
}

func TestMarkupInATitleIsEscaped(t *testing.T) {
	// Titles come from sources, and a source is text: it is escaped rather
	// than trusted, or a title could carry markup into every page that lists it.
	request := testRequest()
	site := Site{}
	if err := json.Unmarshal(request.Payload, &site); err != nil {
		t.Fatal(err)
	}
	site.Pages[0].Title = `<script>alert(1)</script>`
	payload, err := json.Marshal(site)
	if err != nil {
		t.Fatal(err)
	}
	request.Payload = payload

	response := answer(t, request)
	if strings.Contains(response.Pages[0].HTML, "<script>") {
		t.Fatalf("expected the title escaped, got:\n%s", response.Pages[0].HTML)
	}
}
