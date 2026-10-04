//
// @file reference_test.go
// @brief Tests for the API reference page.
//
// A page is read by a person, so what is tested here is what a person needs to
// find: every endpoint, every parameter, every status, and the credential the
// service asks for. The escaping is tested too, because a description is read
// out of source text and source text is not trusted to be inert.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhoapiref

import (
	"strings"
	"testing"

	"github.com/rho/rho-router/src/rhorouter"
)

// service is a spec covering what a page has to show.
func service() rhorouter.Spec {
	return rhorouter.Spec{
		Version:       rhorouter.SpecVersion,
		Module:        "catalog",
		Title:         "Catalogue",
		Description:   "A catalogue of things",
		ModuleVersion: "2.1.0",
		Security: &rhorouter.Security{
			Scheme:      rhorouter.SecurityBearer,
			Description: "A publish token, obtained from POST /auth/token.",
			PublicPaths: []string{"/healthz"},
		},
		Operations: []rhorouter.Operation{
			{
				ID: "show_item", Method: "GET", Path: "/items/{id}", Tag: "catalogue",
				Summary:     "Show one item.",
				Description: "The item is read by its identifier.\n\nA missing one is a 404.",
				Parameters: []rhorouter.Parameter{
					{Name: "id", In: rhorouter.InPath, Type: "u32", Required: true},
					{Name: "verbose", In: rhorouter.InQuery, Type: "bool"},
					{Name: "X-Tenant", In: rhorouter.InHeader, Type: "string"},
				},
				Responses: []rhorouter.Response{
					{Status: 200, Description: "OK", ContentType: "application/json"},
					{Status: 404, Description: "Not Found"},
				},
			},
			{
				ID: "make_item", Method: "POST", Path: "/items", Tag: "catalogue",
				Summary: "Store one item.",
				RequestBody: &rhorouter.Body{
					ContentType: "application/json",
					Fields: []rhorouter.Parameter{
						{Name: "name", In: rhorouter.InBody, Type: "string", Required: true},
					},
				},
				Responses: []rhorouter.Response{{Status: 201, Description: "Created"}},
			},
			{
				ID: "health", Method: "GET", Path: "/healthz",
				Summary: "A liveness probe.", Deprecated: true,
				Responses: []rhorouter.Response{{Status: 200, Description: "OK"}},
			},
		},
	}
}

func render() string { return Render(service(), ReferenceOptions{Server: "https://api.rho.dev"}) }

func TestEveryEndpointIsListed(t *testing.T) {
	page := render()
	for _, want := range []string{"/items/{id}", "/items", "/healthz"} {
		if !strings.Contains(page, want) {
			t.Errorf("the endpoints table does not list %q", want)
		}
	}
	for _, want := range []string{"show_item", "make_item", "health"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not mention the operation %q", want)
		}
	}
}

func TestEveryOperationIsWrittenInFull(t *testing.T) {
	page := render()
	// One section per operation, each anchored by the handler's name so a link
	// into the page is a name a person wrote rather than one derived from a
	// path.
	for _, want := range []string{`id="show_item"`, `id="make_item"`, `id="health"`} {
		if !strings.Contains(page, want) {
			t.Errorf("no section is anchored at %s", want)
		}
	}
}

func TestTheSummaryAndTheProseAreBothShown(t *testing.T) {
	page := render()
	if !strings.Contains(page, "Show one item.") {
		t.Error("the summary is missing")
	}
	// A description that runs to more than one line is a heading and the prose
	// under it, and a reader needs both.
	if !strings.Contains(page, "The item is read by its identifier.") {
		t.Error("the first paragraph of the description is missing")
	}
	if !strings.Contains(page, "A missing one is a 404.") {
		t.Error("the second paragraph of the description is missing")
	}
}

func TestParametersAreTabulatedWithWhereTheyComeFrom(t *testing.T) {
	page := render()
	// The path first, then the query, then the header: a reader looking for a
	// name scans in that order.
	order := []string{"id", "verbose", "X-Tenant"}
	at := -1
	for _, name := range order {
		found := strings.Index(page, "<code>"+name+"</code>")
		if found < 0 {
			t.Fatalf("the parameter %q is not tabulated", name)
		}
		if found < at {
			t.Errorf("the parameter %q is listed out of order", name)
		}
		at = found
	}
	// A path wildcard is required whatever the sources said, because the router
	// will not have matched without it.
	if !strings.Contains(page, "<td>path</td>") {
		t.Error("the table does not say where a value comes from")
	}
}

func TestARequestBodyIsDescribed(t *testing.T) {
	page := render()
	if !strings.Contains(page, "Request body") {
		t.Fatal("an operation with a body does not say so")
	}
	if !strings.Contains(page, "application/json") {
		t.Error("the body's content type is missing")
	}
	if !strings.Contains(page, "<code>name</code>") {
		t.Error("the body's fields are not tabulated")
	}
}

func TestStatusesAreListedWithTheirNames(t *testing.T) {
	page := render()
	for _, want := range []string{"200", "OK", "404", "Not Found", "201", "Created"} {
		if !strings.Contains(page, want) {
			t.Errorf("the responses do not mention %q", want)
		}
	}
}

func TestTheCredentialIsSaidOnce(t *testing.T) {
	page := render()
	if !strings.Contains(page, "Authentication") {
		t.Error("a service that asks for a credential has no authentication section")
	}
	if !strings.Contains(page, "A publish token, obtained from POST /auth/token.") {
		t.Error("the credential is not described")
	}
	// And what is served without one is said too, because that is the half a
	// reader otherwise has to find out by calling.
	if !strings.Contains(page, "/healthz") {
		t.Error("the public paths are not listed")
	}
}

func TestAPublicOperationSaysItNeedsNothing(t *testing.T) {
	// The probe is exempt, so the page must not tell a reader it needs a token
	// to call it. A page that over-promises authentication is worse than one
	// that under-promises it.
	body := sectionOf(t, render(), "health")
	if strings.Contains(body, "Accepts bearerAuth") || strings.Contains(body, "Requires") {
		t.Errorf("a public operation claims to need a credential:\n%s", body)
	}
}

func TestDeprecatedIsSaid(t *testing.T) {
	if !strings.Contains(sectionOf(t, render(), "health"), "Deprecated") {
		t.Error("an operation marked deprecated is not marked on the page")
	}
}

func TestMarkupInASourceIsEscapedNotObeyed(t *testing.T) {
	// A description is read out of source text, and source text is text: a
	// title that carried markup would be executed by every reader's browser.
	spec := service()
	spec.Operations[0].Summary = `<script>alert(1)</script>`
	page := Render(spec, ReferenceOptions{})
	if strings.Contains(page, "<script>") {
		t.Errorf("a summary carried markup through:\n%s", page)
	}
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Errorf("the markup was dropped rather than escaped:\n%s", page)
	}
}

func TestTheServerIsShownWhenKnown(t *testing.T) {
	if !strings.Contains(render(), "https://api.rho.dev") {
		t.Error("the address the service answers at is not shown")
	}
	if strings.Contains(Render(service(), ReferenceOptions{}), "The service answers at") {
		t.Error("an address that is not known is invented")
	}
}

func TestAModuleWithNoRoutesSaysSo(t *testing.T) {
	// Most modules are libraries. The page is not added for them by the
	// plugin, but the renderer answers honestly if it is asked.
	spec := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "library", Title: "Library"}
	page := Render(spec, ReferenceOptions{})
	if !strings.Contains(page, "answers no routes") {
		t.Errorf("a module with no routes produced a page that does not say so:\n%s", page)
	}
}

func TestTheBasePathIsAppliedToEveryCall(t *testing.T) {
	page := Render(service(), ReferenceOptions{Prefix: "/api/v1"})
	if !strings.Contains(page, "/api/v1/items") {
		t.Error("the base path is not applied")
	}
}

// sectionOf returns the part of a page about one operation, so an assertion is
// about that operation rather than about the whole page.
func sectionOf(t *testing.T, page, id string) string {
	t.Helper()
	start := strings.Index(page, `id="`+id+`"`)
	if start < 0 {
		t.Fatalf("no section is anchored at %q", id)
	}
	rest := page[start:]
	if end := strings.Index(rest, "</section>"); end > 0 {
		return rest[:end]
	}
	return rest
}
