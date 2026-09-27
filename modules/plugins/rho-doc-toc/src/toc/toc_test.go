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
// SPDX-License-Identifier: MIT
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

func testRequest() Request {
	return Request{
		Version: ProtocolVersion,
		Hook:    HookIndex,
		Site:    "Rho",
		Pages: []Page{
			{Target: "index", Title: "Index", Href: "index.html", Section: "guides"},
			{Target: "guide", Title: "Guide", Href: "guides/app/guide.html", Section: "guides"},
			{Target: "api", Title: "API reference", Href: "api.html", Section: "api"},
		},
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
	if response.Pages[0].Section != "guides" {
		t.Fatalf("expected the contents beside the pages, got %q", response.Pages[0].Section)
	}
}

func TestThePageHookAddsAGenerationNotice(t *testing.T) {
	request := testRequest()
	request.Hook = HookPage
	page := Page{Target: "api-std-core-option", Title: "std.core.option", HTML: "<h1>std.core.option</h1>"}
	request.Page = &page

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
	response := answer(t, Request{Version: ProtocolVersion, Hook: HookIndex, Site: "Rho"})
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
	request.Pages[0].Title = `<script>alert(1)</script>`

	response := answer(t, request)
	if strings.Contains(response.Pages[0].HTML, "<script>") {
		t.Fatalf("expected the title escaped, got:\n%s", response.Pages[0].HTML)
	}
}
