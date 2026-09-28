//
// @file extract_test.go
// @brief Tests for reading a Rho service's HTTP surface out of its sources.
//
// The reading is the contract everything else is built on, so what is checked
// here is that it says what the sources say: the routes as they were declared,
// the values a handler reads with the types it asked for, the statuses it
// answers with, and the comments a reader sees. A reading that quietly drops a
// field produces a document that is wrong in a way nobody notices until a
// client generated from it fails.
//
// The fixtures are written in Rho and read back, rather than a spec being
// written by hand and compared, because the second would only test that the
// test agrees with itself.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhorouter

import (
	"os"
	"path/filepath"
	"testing"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                    */
/* -------------------------------------------------------------------------- */

// manifestText is the manifest a project fixture carries.
const manifestText = `module:
  name: catalog
  type: rho
  version: 2.1.0
  description: A catalogue of things

main: src/main.rho

docs:
  title: Catalogue
`

// serviceSource is a service exercising every shape the reading has to
// recognise: a typed wildcard, a body, a query, a header, a remainder, a
// comment above a route and a comment above a handler.
const serviceSource = `module catalog

import std.web.http

// The routes of the catalogue, one per shape an API needs.

route GET  "/items/{id}"     => show_item
route POST "/items"          => make_item
route GET  "/docs/{path...}" => docs_page
route POST "/upload"         => upload_module
route GET  "/filtered"       => filtered

fn text(status: u32, body: string) Response {
    return Response{ status, body, "text/plain; charset=utf-8" }
}

// show_item answers one item, read by its identifier.
//
// A path that is not a number is a bad request rather than a missing item.
fn show_item(req: Request) Response {
    let id: u32 = path_u32(req, "id")
    if !bind_ok(req) {
        return json(400, bind_error(req))
    }
    return json(200, "ok")
}

// make_item stores one item.
//
// The whole body is taken when a field is not enough.
fn make_item(req: Request) Response {
    let name: string = field_str(req, "name")
    let count: u32 = field_u32(req, "count")
    if !bind_ok(req) {
        return json(400, bind_error(req))
    }
    return json(201, name)
}

// docs_page serves one page of the documentation.
//
// A remainder may be empty, so the page is asked about before it is read.
fn docs_page(req: Request) Response {
    let path: string = ""
    if has_path(req, "path") {
        path = param(req, "path")
    }
    return text(200, path)
}

// upload_module takes a package whole, because an archive is not a form.
fn upload_module(req: Request) Response {
    let payload: string = body_str(req, "")
    return json(201, payload)
}

// filtered lists items, narrowed by a tenant and a page size.
//
// The tenant comes from a header, because a query parameter would let a client
// claim to be a tenant it is not.
fn filtered(req: Request) Response {
    let tenant: string = header_str(req, "X-Tenant")
    let limit: u32 = query_u32(req, "limit")
    if has_query(req, "verbose") {
        return json(200, tenant)
    }
    return json(200, limit)
}
`

// writeProject writes a project fixture and returns its root.
func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// catalog writes the service fixture and reads it.
func catalog(t *testing.T) (ReadResult, Spec) {
	t.Helper()
	root := writeProject(t, map[string]string{
		"module.yml":   manifestText,
		"src/main.rho": serviceSource,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatalf("the fixture did not read: %v", err)
	}
	return result, result.Spec
}

// operation returns one operation by identifier.
func operation(t *testing.T, spec Spec, id string) Operation {
	t.Helper()
	for _, entry := range spec.Operations {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("no operation named %q; read %d", id, len(spec.Operations))
	return Operation{}
}

// parameter returns one parameter by where it comes from and what it is called.
func parameter(t *testing.T, operation Operation, in, name string) Parameter {
	t.Helper()
	for _, entry := range operation.Parameters {
		if entry.In == in && entry.Name == name {
			return entry
		}
	}
	t.Fatalf("no %s parameter named %q on %s", in, name, operation.ID)
	return Parameter{}
}

/* -------------------------------------------------------------------------- */
/*  The contract                                                                */
/* -------------------------------------------------------------------------- */

func TestTheSpecNamesItsVersion(t *testing.T) {
	_, spec := catalog(t)
	// A consumer that does not recognise the version says so rather than
	// guessing at a shape it does not know.
	if spec.Version != SpecVersion {
		t.Errorf("the spec says version %d, want %d", spec.Version, SpecVersion)
	}
}

func TestTheSpecTakesTheManifestsOwnWords(t *testing.T) {
	_, spec := catalog(t)
	if spec.Module != "catalog" {
		t.Errorf("module = %q", spec.Module)
	}
	if spec.Title != "Catalogue" {
		t.Errorf("title = %q, want the docs title the manifest carries", spec.Title)
	}
	if spec.Description != "A catalogue of things" {
		t.Errorf("description = %q", spec.Description)
	}
	if spec.ModuleVersion != "2.1.0" {
		t.Errorf("version = %q", spec.ModuleVersion)
	}
}

func TestAModuleWithNoManifestIsRefused(t *testing.T) {
	root := writeProject(t, map[string]string{"src/main.rho": serviceSource})
	if _, err := Read(ReadOptions{Root: root}); err == nil {
		t.Fatal("a project with no manifest read, so the entry point is unknown")
	}
}

/* -------------------------------------------------------------------------- */
/*  Routes                                                                      */
/* -------------------------------------------------------------------------- */

func TestRoutesAreReadAsDeclared(t *testing.T) {
	_, spec := catalog(t)
	if len(spec.Operations) != 5 {
		t.Fatalf("read %d routes, want 5", len(spec.Operations))
	}
	want := []struct{ method, path, handler string }{
		{"GET", "/items/{id}", "show_item"},
		{"POST", "/items", "make_item"},
		{"GET", "/docs/{path...}", "docs_page"},
		{"POST", "/upload", "upload_module"},
		{"GET", "/filtered", "filtered"},
	}
	for i, expected := range want {
		got := spec.Operations[i]
		if got.Method != expected.method || got.Path != expected.path || got.Handler != expected.handler {
			t.Errorf("route %d = %s %s => %s, want %s %s => %s", i,
				got.Method, got.Path, got.Handler, expected.method, expected.path, expected.handler)
		}
		if got.Source != "src/main.rho" {
			t.Errorf("route %d records source %q", i, got.Source)
		}
	}
}

func TestRoutesInSeveralFilesAreAllRead(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml":   manifestText,
		"src/main.rho": "module catalog\n\nroute GET \"/a\" => a\n\nfn a(req: Request) Response {\n    return json(200, \"a\")\n}\n",
		// A route a module's own file declares is part of the service, and a
		// reader that only looked at the entry point would miss it.
		"src/site/pages.rho": `module catalog.site

route GET "/site" => site

fn site(req: Request) Response {
    return json(200, "site")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spec.Operations) != 2 {
		t.Fatalf("read %d routes across two files, want 2", len(result.Spec.Operations))
	}
	// The entry point comes first, so a reader sees the module it asked about
	// before the modules it imports.
	if result.Spec.Operations[0].ID != "a" {
		t.Errorf("the first route is %q; the entry point is read first", result.Spec.Operations[0].ID)
	}
}

func TestRoutesAreGroupedByTheModuleTheyAreIn(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml":   manifestText,
		"src/main.rho": "module catalog\n\nroute GET \"/a\" => a\n\nfn a(req: Request) Response {\n    return json(200, \"a\")\n}\n",
		"src/site/pages.rho": `module catalog.site

route GET "/site" => site

fn site(req: Request) Response {
    return json(200, "site")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := operation(t, result.Spec, "site").Tag; got != "site" {
		t.Errorf("a route in src/site is tagged %q, want the module it is in", got)
	}
}

func TestATestIsNotARoute(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml":        manifestText,
		"src/main.rho":      serviceSource,
		"src/item_test.rho": "module catalog\n\nroute GET \"/test\" => t\n\nfn t(req: Request) Response {\n    return json(200, \"\")\n}\n",
		"tests/edge.rho":    "module catalog\n\nroute GET \"/edge\" => e\n\nfn e(req: Request) Response {\n    return json(200, \"\")\n}\n",
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range result.Spec.Operations {
		if entry.ID == "t" || entry.ID == "e" {
			t.Errorf("a test was read as a route: %s", entry.ID)
		}
	}
}

func TestASourceThatDoesNotParseIsAWarningRatherThanAFailure(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml":     manifestText,
		"src/main.rho":   "module catalog\n\nroute GET \"/a\" => a\n\nfn a(req: Request) Response {\n    return json(200, \"a\")\n}\n",
		"src/broken.rho": "module catalog\n\nfn ((( \n",
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatalf("one unreadable source stopped the whole reading: %v", err)
	}
	if len(result.Spec.Operations) != 1 {
		t.Errorf("read %d routes, want the one that could be read", len(result.Spec.Operations))
	}
	if len(result.Diagnostics) == 0 {
		t.Error("the unreadable source was not reported")
	}
}

/* -------------------------------------------------------------------------- */
/*  Parameters                                                                  */
/* -------------------------------------------------------------------------- */

func TestATypedWildcardIsReadWithTheTypeItWasAskedFor(t *testing.T) {
	_, spec := catalog(t)
	// The handler is where the type was asked for, so the document says what
	// the program does rather than what a route could have meant.
	if got := parameter(t, operation(t, spec, "show_item"), InPath, "id").Type; got != "u32" {
		t.Errorf("the id parameter is a %q, want u32", got)
	}
}

func TestAPathWildcardIsRequiredWhetherOrNotItIsRead(t *testing.T) {
	_, spec := catalog(t)
	// The router will not have matched the route without it.
	if !parameter(t, operation(t, spec, "show_item"), InPath, "id").Required {
		t.Error("a path wildcard is not required")
	}
}

func TestACatchAllWildcardIsNamedWithoutItsDots(t *testing.T) {
	_, spec := catalog(t)
	// A handler asks for the name without the dots, so the document does too.
	entry := parameter(t, operation(t, spec, "docs_page"), InPath, "path")
	if entry.Type != "string" {
		t.Errorf("the remainder is a %q", entry.Type)
	}
}

func TestEverySourceIsRead(t *testing.T) {
	_, spec := catalog(t)
	make := operation(t, spec, "make_item")
	// A body value belongs to the body rather than to the parameter list, so
	// it is read from the request body the operation carries.
	if make.RequestBody == nil {
		t.Fatal("an operation that reads body fields has no body")
	}
	for _, field := range make.RequestBody.Fields {
		switch field.Name {
		case "name":
			if field.Type != "string" {
				t.Errorf("a body field reads as a %q", field.Type)
			}
		case "count":
			if field.Type != "u32" {
				t.Errorf("a numeric body field reads as a %q", field.Type)
			}
		}
	}
	// And it is not listed as a parameter as well: a consumer that had to
	// filter one out would be reading the same thing twice.
	for _, entry := range make.Parameters {
		if entry.In == InBody {
			t.Errorf("a body value is listed as a parameter: %+v", entry)
		}
	}
	filtered := operation(t, spec, "filtered")
	if got := parameter(t, filtered, InHeader, "X-Tenant").Type; got != "string" {
		t.Errorf("a header reads as a %q", got)
	}
	if got := parameter(t, filtered, InQuery, "limit").Type; got != "u32" {
		t.Errorf("a query parameter reads as a %q", got)
	}
}

func TestOnlyAPathWildcardIsRequired(t *testing.T) {
	_, spec := catalog(t)
	// The router will not have matched a route without its wildcards, so those
	// are the requirements the route itself states.
	if !parameter(t, operation(t, spec, "show_item"), InPath, "id").Required {
		t.Error("a path wildcard is not required")
	}
	// Nothing else can be decided from the sources. A guard is not a statement
	// about a value: `bind_ok(req)` takes the request and nothing else, so which
	// read a guard was for is not something a reading can recover. Claiming a
	// query parameter was required because its handler reads it would make a
	// generated client insist on a value the service accepts without.
	if parameter(t, operation(t, spec, "filtered"), InQuery, "limit").Required {
		t.Error("a query parameter is required because its handler reads it")
	}
	// A header a handler reads is no more required for being read, and a
	// presence read is a filter rather than a value: neither is a requirement
	// the sources state.
	if got := parameter(t, operation(t, spec, "filtered"), InHeader, "X-Tenant"); got.Required {
		t.Error("a header is required because its handler reads it")
	}
}

func TestAValueReadTwiceIsReadOnce(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

fn a(req: Request) Response {
    let one: string = query_str(req, "q")
    let two: string = query_str(req, "q")
    return json(200, one + two)
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	entry := operation(t, result.Spec, "a")
	if len(entry.Parameters) != 1 {
		t.Errorf("a value read twice is listed %d times", len(entry.Parameters))
	}
}

func TestTheSameNameFromTwoSourcesIsTwoValues(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a/{id}" => a

fn a(req: Request) Response {
    let from_path: u32 = path_u32(req, "id")
    let from_query: u32 = query_u32(req, "id")
    return json(200, int_to_str(from_path + from_query))
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	entry := operation(t, result.Spec, "a")
	if len(entry.Parameters) != 2 {
		t.Fatalf("two values named id from two sources are listed %d times", len(entry.Parameters))
	}
}

func TestAVariableAccessIsNotAValue(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

fn a(req: Request) Response {
    let name: string = "id"
    let value: u32 = query_u32(req, name)
    return json(200, int_to_str(value))
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	// A name built at run time cannot be read here, and a value read that way
	// is documented by hand rather than by this tool.
	if got := len(operation(t, result.Spec, "a").Parameters); got != 0 {
		t.Errorf("a parameter whose name is a variable was listed %d times", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Bodies                                                                      */
/* -------------------------------------------------------------------------- */

func TestAJSONBodyIsDescribedByItsFields(t *testing.T) {
	_, spec := catalog(t)
	body := operation(t, spec, "make_item").RequestBody
	if body == nil {
		t.Fatal("a handler that reads fields of a body has no body described")
	}
	if body.ContentType != "application/json" {
		t.Errorf("content type = %q", body.ContentType)
	}
	if body.Raw {
		t.Error("a body read as fields was described as raw")
	}
	if len(body.Fields) != 2 {
		t.Fatalf("read %d fields, want 2", len(body.Fields))
	}
	if body.Fields[0].Name != "name" || body.Fields[0].Type != "string" {
		t.Errorf("the first field is %+v", body.Fields[0])
	}
}

func TestABodyReadWholeIsDescribedAsRaw(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route POST "/upload" => upload

fn upload(req: Request) Response {
    let payload: string = body_str(req, "")
    return json(201, payload)
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	body := operation(t, result.Spec, "upload").RequestBody
	if body == nil || !body.Raw {
		t.Fatalf("a body read whole is described as %+v", body)
	}
	if body.ContentType != "application/octet-stream" {
		t.Errorf("content type = %q, want one that does not claim a shape", body.ContentType)
	}
}

func TestAHandlerThatReadsNoBodyHasNone(t *testing.T) {
	_, spec := catalog(t)
	if got := operation(t, spec, "docs_page").RequestBody; got != nil {
		t.Errorf("a handler that reads no body was given one: %+v", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Responses                                                                   */
/* -------------------------------------------------------------------------- */

func TestStatusesAreReadWithTheNameTheSourcesGiveThem(t *testing.T) {
	_, spec := catalog(t)
	entry := operation(t, spec, "show_item")
	want := map[int]string{200: "OK", 400: "Bad Request"}
	if len(entry.Responses) != len(want) {
		t.Fatalf("read %d statuses, want %d: %+v", len(entry.Responses), len(want), entry.Responses)
	}
	for _, response := range entry.Responses {
		if want[response.Status] != response.Description {
			t.Errorf("status %d is called %q, want %q", response.Status,
				response.Description, want[response.Status])
		}
	}
}

func TestAStatusWrittenAsANumberIsStillAStatus(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

fn a(req: Request) Response {
    return Response{ 404, "no such thing", "text/plain" }
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	entry := operation(t, result.Spec, "a")
	if len(entry.Responses) != 1 || entry.Responses[0].Status != 404 {
		t.Fatalf("a response written directly was read as %+v", entry.Responses)
	}
	if entry.Responses[0].ContentType != "text/plain" {
		t.Errorf("content type = %q", entry.Responses[0].ContentType)
	}
}

func TestStatusesAreInOrderAndWrittenOnce(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

fn a(req: Request) Response {
    if true {
        return json(500, "one")
    }
    return json(404, "two")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	entry := operation(t, result.Spec, "a")
	if len(entry.Responses) != 2 {
		t.Fatalf("read %d statuses, want 2", len(entry.Responses))
	}
	if entry.Responses[0].Status != 404 || entry.Responses[1].Status != 500 {
		t.Errorf("statuses are out of order: %+v", entry.Responses)
	}
}

func TestAHandlerThatAnswersNoNamedStatusHasNone(t *testing.T) {
	_, spec := catalog(t)
	// An operation with no status written down is an absence, not a claim.
	upload := operation(t, spec, "filtered")
	for _, response := range upload.Responses {
		if response.Status == 0 {
			t.Error("a status of zero was recorded")
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Comments                                                                    */
/* -------------------------------------------------------------------------- */

func TestAHandlerCommentBecomesTheSummaryAndTheProse(t *testing.T) {
	_, spec := catalog(t)
	entry := operation(t, spec, "show_item")
	if entry.Summary != "show_item answers one item, read by its identifier." {
		t.Errorf("summary = %q", entry.Summary)
	}
	if !contains(entry.Description, "a bad request") {
		t.Errorf("description = %q, want the prose under the heading", entry.Description)
	}
}

func TestAOneLineCommentIsTheHeadingAndNotAHeadingRepeated(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

// a answers the root.
fn a(req: Request) Response {
    return json(200, "a")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	entry := operation(t, result.Spec, "a")
	if entry.Summary != "a answers the root." {
		t.Errorf("summary = %q", entry.Summary)
	}
	if entry.Description != "" {
		t.Errorf("a one-line comment was repeated as prose: %q", entry.Description)
	}
}

func TestARouteCommentIsRead(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

// health answers a liveness probe.
//
// It is the one route a deployment can reach without a credential.
route GET "/healthz" => health

fn health(req: Request) Response {
    return json(200, "ok")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	entry := operation(t, result.Spec, "health")
	if entry.Summary != "health answers a liveness probe." {
		t.Errorf("summary = %q", entry.Summary)
	}
	if !contains(entry.Description, "without a credential") {
		t.Errorf("description = %q", entry.Description)
	}
}

func TestACommentSeparatedByABlankLineIsNotTheDeclarations(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

// The section below handles the catalogue.

route GET "/a" => a

fn a(req: Request) Response {
    return json(200, "a")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	// A note about a section is not a description of the route under it, and
	// putting it there would put prose where a reader expects a heading.
	if got := operation(t, result.Spec, "a").Summary; got != "" {
		t.Errorf("a section note became a summary: %q", got)
	}
}

func TestACommentInsideAHandlerIsNotTheHandlersDescription(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

fn a(req: Request) Response {
    // The default is what a client gets when it asks for nothing.
    return json(200, "a")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := operation(t, result.Spec, "a").Summary; got != "" {
		t.Errorf("a comment inside the body became a summary: %q", got)
	}
}

func TestDeprecatedIsReadFromTheComment(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

// old answers the way it always did. Deprecated: use /a instead.
route GET "/old" => old

fn old(req: Request) Response {
    return json(200, "old")
}
`,
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !operation(t, result.Spec, "old").Deprecated {
		t.Error("a comment saying deprecated did not mark the operation")
	}
}

/* -------------------------------------------------------------------------- */
/*  The vocabulary                                                              */
/* -------------------------------------------------------------------------- */

func TestEveryAccessorIsSpelledOnceAcrossTheSources(t *testing.T) {
	// A name listed twice would be read once and the second entry would be a
	// dead entry, which is the kind of thing that survives for years.
	seen := map[string]bool{}
	for _, name := range AccessorNames() {
		if seen[name] {
			t.Errorf("the accessor %q is listed twice", name)
		}
		seen[name] = true
	}
}

func TestDescribeAccessorSaysSomethingUseful(t *testing.T) {
	if got := DescribeAccessor("path_u32"); !contains(got, "path parameter") || !contains(got, "u32") {
		t.Errorf("path_u32 is described as %q", got)
	}
	if got := DescribeAccessor("query_str"); !contains(got, "query parameter") {
		t.Errorf("query_str is described as %q", got)
	}
	if got := DescribeAccessor("body_str"); !contains(got, "body") {
		t.Errorf("body_str is described as %q", got)
	}
	if got := DescribeAccessor("not_an_accessor"); got != "not a binding accessor" {
		t.Errorf("a name that is not an accessor is described as %q", got)
	}
}

func TestSchemaTypeMapsToWhatADocumentCallsAType(t *testing.T) {
	cases := map[string][2]string{
		"string": {"string", ""},
		"u32":    {"integer", "int64"},
		"i32":    {"integer", "int32"},
		"f64":    {"number", "double"},
		"bool":   {"boolean", ""},
	}
	for rho, want := range cases {
		kind, format := SchemaType(rho)
		if kind != want[0] || format != want[1] {
			t.Errorf("SchemaType(%q) = %q, %q, want %q, %q", rho, kind, format, want[0], want[1])
		}
	}
}

func TestWildcardNamesHandlesEveryShapeOfPattern(t *testing.T) {
	cases := map[string][]string{
		"/items":                {},
		"/items/{id}":           {"id"},
		"/a/{x}/b/{y}":          {"x", "y"},
		"/docs/{path...}":       {"path"},
		"/a/{x}/docs/{path...}": {"x", "path"},
		"/a/{}":                 {},
		"/a/{unterminated":      {},
	}
	for pattern, want := range cases {
		got := wildcardNames(pattern)
		if len(got) != len(want) {
			t.Errorf("wildcardNames(%q) = %v, want %v", pattern, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("wildcardNames(%q) = %v, want %v", pattern, got, want)
				break
			}
		}
	}
}

func TestStatusNameFollowsTheSpecifications(t *testing.T) {
	cases := map[int]string{
		200: "OK",
		404: "Not Found",
		413: "Content Too Large",
		422: "Unprocessable Content",
		499: "", // not a status the specifications name
	}
	for status, want := range cases {
		got := statusName(status)
		if want == "" {
			if got == "" {
				t.Errorf("statusName(%d) = %q, want something even for an unnamed status", status, got)
			}
			continue
		}
		if got != want {
			t.Errorf("statusName(%d) = %q, want %q", status, got, want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  The spec's own views                                                         */
/* -------------------------------------------------------------------------- */

func TestPathsGroupsByPathAndMethod(t *testing.T) {
	_, spec := catalog(t)
	paths := spec.Paths()
	if len(paths) != 5 {
		t.Fatalf("grouped into %d paths, want 5", len(paths))
	}
	// The wildcard is part of the path, so the read of one item and the write
	// of another are two methods on two paths.
	if _, served := paths["/items/{id}"]["get"]; !served {
		t.Error("the read route is not grouped under get /items/{id}")
	}
	if _, served := paths["/items"]["post"]; !served {
		t.Error("the write route is not grouped under post /items")
	}
}

func TestTagsAreInTheOrderTheyFirstAppear(t *testing.T) {
	root := writeProject(t, map[string]string{
		"module.yml":        manifestText,
		"src/main.rho":      "module catalog\n\nroute GET \"/a\" => a\n\nfn a(req: Request) Response {\n    return json(200, \"\")\n}\n",
		"src/zeta/one.rho":  "module catalog.zeta\n\nroute GET \"/z\" => z\n\nfn z(req: Request) Response {\n    return json(200, \"\")\n}\n",
		"src/alpha/two.rho": "module catalog.alpha\n\nroute GET \"/b\" => b\n\nfn b(req: Request) Response {\n    return json(200, \"\")\n}\n",
	})
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	tags := result.Spec.Tags()
	// The entry point is read first and its route is untagged, so the groups
	// follow the rest of the sources in order rather than a map's order.
	if len(tags) != 2 || tags[0] != "alpha" || tags[1] != "zeta" {
		t.Errorf("tags are %v, want [alpha zeta] in the order the sources are read", tags)
	}
}

func TestHasOperationAnswersForTheFold(t *testing.T) {
	_, spec := catalog(t)
	if !spec.HasOperation("show_item") {
		t.Error("an operation that is present is reported as absent")
	}
	if spec.HasOperation("absent") {
		t.Error("an operation that is absent is reported as present")
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

/* -------------------------------------------------------------------------- */
/*  The manifest                                                                */
/* -------------------------------------------------------------------------- */

func TestAModuleNameIsTheGroupItsSourcesAreIn(t *testing.T) {
	cases := map[string]string{
		"src/registry/web.rho": "registry",
		"src/main.rho":         "",
		"main.rho":             "",
		"src/a/b/c.rho":        "b",
	}
	for source, want := range cases {
		if got := moduleOf(source); got != want {
			t.Errorf("moduleOf(%q) = %q, want %q", source, got, want)
		}
	}
}
