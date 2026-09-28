//
// @file openapi_test.go
// @brief Tests for writing a service's metadata as an OpenAPI 3.1 document.
//
// A document is only useful if a tool can read it and a person can read it, so
// both are checked here: every rendering is parsed as JSON, and the order the
// keys are written in is asserted, because a document that reshuffles itself on
// every run cannot be reviewed in a diff.
//
// The document is written by hand rather than through a marshaller, so the
// tests are as much about the writing as about the content: a missing comma is
// a syntax error, and a comma where a field is not last is one too.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhooas

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rho/rho-router/src/rhorouter"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                    */
/* -------------------------------------------------------------------------- */

// catalog is a spec covering what a document has to say: parameters from every
// source, a body with fields, a body read whole, several statuses, tags and
// comments.
func catalog() rhorouter.Spec {
	return rhorouter.Spec{
		Version:       rhorouter.SpecVersion,
		Module:        "catalog",
		Title:         "Catalogue",
		Description:   "A catalogue of things",
		ModuleVersion: "2.1.0",
		Operations: []rhorouter.Operation{
			{
				ID: "show_item", Method: "GET", Path: "/items/{id}",
				Summary: "Show one item.", Description: "The item is read by its identifier.",
				Tag: "catalogue",
				Parameters: []rhorouter.Parameter{
					{Name: "id", In: rhorouter.InPath, Type: "u32", Required: true},
				},
				Responses: []rhorouter.Response{
					{Status: 200, Description: "OK", ContentType: "application/json"},
					{Status: 404, Description: "Not Found"},
				},
			},
			{
				ID: "make_item", Method: "POST", Path: "/items",
				Summary: "Store one item.", Tag: "catalogue",
				Parameters: []rhorouter.Parameter{
					{Name: "X-Tenant", In: rhorouter.InHeader, Type: "string"},
					{Name: "verbose", In: rhorouter.InQuery, Type: "bool"},
				},
				RequestBody: &rhorouter.Body{
					ContentType: "application/json",
					Fields: []rhorouter.Parameter{
						{Name: "name", In: rhorouter.InBody, Type: "string", Required: true},
						{Name: "count", In: rhorouter.InBody, Type: "u32", Required: true},
					},
				},
				Responses: []rhorouter.Response{
					{Status: 201, Description: "Created", ContentType: "application/json"},
				},
			},
			{
				ID: "upload", Method: "PUT", Path: "/items/archive", Deprecated: true,
				Summary: "Store an archive.",
				RequestBody: &rhorouter.Body{
					ContentType: "application/octet-stream", Raw: true,
				},
				Responses: []rhorouter.Response{
					{Status: 204, Description: "No Content"},
				},
			},
		},
	}
}

// render produces a document and fails the test if it is not valid JSON, so no
// case below can pass on text that no tool would accept.
func render(t *testing.T, spec rhorouter.Spec, options OpenAPIOptions) string {
	t.Helper()
	text := OpenAPI(spec, options).String()
	var parsed interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("the document is not valid JSON: %v\n%s", err, text)
	}
	return text
}

// read parses a document back into a map, so an assertion can look at what a
// tool would see rather than at the text.
func read(t *testing.T, text string) map[string]interface{} {
	t.Helper()
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("the document does not parse: %v\n%s", err, text)
	}
	return parsed
}

// dig walks a parsed document to one value.
func dig(t *testing.T, document map[string]interface{}, path ...string) interface{} {
	t.Helper()
	var current interface{} = document
	for _, step := range path {
		object, ok := current.(map[string]interface{})
		if !ok {
			t.Fatalf("%v is not an object while looking for %q", current, strings.Join(path, "."))
		}
		current, ok = object[step]
		if !ok {
			t.Fatalf("no %q in %v", step, strings.Join(path, "."))
		}
	}
	return current
}

/* -------------------------------------------------------------------------- */
/*  The document                                                                */
/* -------------------------------------------------------------------------- */

func TestTheDocumentSaysWhichVersionItIs(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	if got := dig(t, document, "openapi"); got != OpenAPIVersion {
		t.Errorf("openapi = %v, want %s", got, OpenAPIVersion)
	}
}

func TestTheDocumentTakesTheServicesOwnName(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	// A manifest's title is what the service calls itself, so the document
	// does not invent a name for it.
	if got := dig(t, document, "info", "title"); got != "Catalogue" {
		t.Errorf("title = %v", got)
	}
	if got := dig(t, document, "info", "version"); got != "2.1.0" {
		t.Errorf("version = %v", got)
	}
	if got := dig(t, document, "info", "description"); got != "A catalogue of things" {
		t.Errorf("description = %v", got)
	}
}

func TestTheDocumentFallsBackWhenTheManifestIsSilent(t *testing.T) {
	spec := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare"}
	document := read(t, render(t, spec, OpenAPIOptions{}))
	if got := dig(t, document, "info", "title"); got != "bare" {
		t.Errorf("title = %v, want the module's own name", got)
	}
	if got := dig(t, document, "info", "version"); got != "0.0.0" {
		t.Errorf("version = %v, want a version a reader can rely on", got)
	}
}

func TestTheServerIsWrittenWhenTheTargetKnowsIt(t *testing.T) {
	// A manifest cannot know where a deployment answers, so the address is
	// given rather than guessed.
	document := read(t, render(t, catalog(), OpenAPIOptions{
		Server: "https://api.rho.dev", ServerDescription: "production",
	}))
	servers, ok := dig(t, document, "servers").([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("servers = %v", document["servers"])
	}
	server := servers[0].(map[string]interface{})
	if server["url"] != "https://api.rho.dev" {
		t.Errorf("url = %v", server["url"])
	}
}

func TestNoServerIsWrittenWhenNoneIsKnown(t *testing.T) {
	text := render(t, catalog(), OpenAPIOptions{})
	if strings.Contains(text, `"servers"`) {
		t.Error("a document with no known address claims one")
	}
}

func TestTheBasePathPrefixesEveryPath(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{BasePath: "/api/v1"}))
	paths := dig(t, document, "paths").(map[string]interface{})
	if _, served := paths["/api/v1/items"]; !served {
		t.Errorf("the base path was not applied: %v", keysOf(paths))
	}
}

func TestTagsAreListedOnceEach(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	tags := dig(t, document, "tags").([]interface{})
	if len(tags) != 1 {
		t.Fatalf("tags = %v, want the one group the operations name", tags)
	}
	entry := operation(t, document, "/items", "post")
	if listed := entry["tags"].([]interface{}); listed[0] != "catalogue" {
		t.Errorf("the operation is tagged %v", listed)
	}
}

/* -------------------------------------------------------------------------- */
/*  Operations                                                                  */
/* -------------------------------------------------------------------------- */

func TestTheOperationIdentifierIsTheHandlersName(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	// A handler is already unique within a module, and a name a person wrote
	// is a better identifier than one derived from a path.
	if got := dig(t, document, "paths", "/items/{id}", "get", "operationId"); got != "show_item" {
		t.Errorf("operationId = %v", got)
	}
}

func TestTheCommentBecomesTheSummaryAndTheProse(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	entry := operation(t, document, "/items/{id}", "get")
	if entry["summary"] != "Show one item." {
		t.Errorf("summary = %v", entry["summary"])
	}
	if entry["description"] != "The item is read by its identifier." {
		t.Errorf("description = %v", entry["description"])
	}
}

func TestDeprecatedIsWrittenWhenTheSourcesSaidSo(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	if operation(t, document, "/items/archive", "put")["deprecated"] != true {
		t.Error("an operation the sources marked deprecated is not marked in the document")
	}
	if _, present := operation(t, document, "/items", "post")["deprecated"]; present {
		t.Error("an operation that is not deprecated claims to be")
	}
}

func TestAnOperationWithNoStatusGetsTheCatchAll(t *testing.T) {
	// A document with no response is not a document, and an operation the
	// sources did not pin down may answer with anything.
	spec := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare",
		Operations: []rhorouter.Operation{{ID: "a", Method: "GET", Path: "/a"}}}
	document := read(t, render(t, spec, OpenAPIOptions{}))
	responses := dig(t, document, "paths", "/a", "get", "responses").(map[string]interface{})
	if len(responses) != 1 {
		t.Fatalf("responses = %v", responses)
	}
	if _, present := responses["default"]; !present {
		t.Errorf("responses = %v, want the catch-all", responses)
	}
}

/* -------------------------------------------------------------------------- */
/*  Parameters                                                                  */
/* -------------------------------------------------------------------------- */

func TestAParameterIsWrittenWhereItComesFrom(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	parameters := operation(t, document, "/items/{id}", "get")["parameters"].([]interface{})
	entry := parameters[0].(map[string]interface{})
	if entry["name"] != "id" || entry["in"] != "path" || entry["required"] != true {
		t.Errorf("the path parameter is %v", entry)
	}
}

func TestATypeIsWrittenAsATypeAndNotAsAWord(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	parameters := operation(t, document, "/items/{id}", "get")["parameters"].([]interface{})
	schema := parameters[0].(map[string]interface{})["schema"].(map[string]interface{})
	// A 3.1 schema states a type rather than a string naming one, and an
	// unsigned 32-bit integer is an integer with a format.
	if schema["type"] != "integer" || schema["format"] != "int64" {
		t.Errorf("the id schema is %v", schema)
	}
}

func TestAPathParameterIsRequiredWhateverTheSourcesSaid(t *testing.T) {
	spec := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare",
		Operations: []rhorouter.Operation{{ID: "a", Method: "GET", Path: "/a/{id}",
			Parameters: []rhorouter.Parameter{{Name: "id", In: rhorouter.InPath, Type: "string"}}}}}
	document := read(t, render(t, spec, OpenAPIOptions{}))
	parameters := dig(t, document, "paths", "/a/{id}", "get", "parameters").([]interface{})
	if parameters[0].(map[string]interface{})["required"] != true {
		t.Error("a path parameter is not required")
	}
}

func TestAnOptionalParameterIsNotRequired(t *testing.T) {
	// A client that left a filter out has asked a valid question, and a
	// document that says otherwise is a document a generated client enforces
	// wrongly.
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	parameters := operation(t, document, "/items", "post")["parameters"].([]interface{})
	for _, raw := range parameters {
		entry := raw.(map[string]interface{})
		if entry["required"] == true {
			t.Errorf("the %v parameter is required", entry["name"])
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Bodies                                                                      */
/* -------------------------------------------------------------------------- */

func TestABodyIsWrittenAsAShape(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	schema := dig(t, document, "paths", "/items", "post", "requestBody",
		"content", "application/json", "schema").(map[string]interface{})
	if schema["type"] != "object" {
		t.Errorf("the body is a %v", schema["type"])
	}
	properties := schema["properties"].(map[string]interface{})
	if properties["name"].(map[string]interface{})["type"] != "string" {
		t.Errorf("the name field is %v", properties["name"])
	}
	if properties["count"].(map[string]interface{})["type"] != "integer" {
		t.Errorf("the count field is %v", properties["count"])
	}
	// A client generator needs to know which fields it must send.
	required := schema["required"].([]interface{})
	if len(required) != 2 || required[0] != "count" || required[1] != "name" {
		t.Errorf("required = %v, want the two fields in order", required)
	}
}

func TestABodyFieldIsNotAlsoAParameter(t *testing.T) {
	// A document that lists both is a document that lists it twice, and a
	// generated client then sends it in the wrong place.
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	entry := operation(t, document, "/items", "post")
	if _, present := entry["parameters"]; present {
		parameters := entry["parameters"].([]interface{})
		for _, raw := range parameters {
			if raw.(map[string]interface{})["in"] == rhorouter.InBody {
				t.Error("a body field is listed as a parameter as well")
			}
		}
	}
}

func TestABodyReadWholeIsBinary(t *testing.T) {
	// The bytes of an archive are not a shape, and a document that says
	// otherwise is a document a client will try to encode.
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	schema := dig(t, document, "paths", "/items/archive", "put", "requestBody",
		"content", "application/octet-stream", "schema").(map[string]interface{})
	if schema["type"] != "string" || schema["format"] != "binary" {
		t.Errorf("the archive body is %v", schema)
	}
	if dig(t, document, "paths", "/items/archive", "put", "requestBody", "required") != false {
		t.Error("a body read whole is required; the handler checks for itself")
	}
}

func TestAnOperationThatAcceptsNoBodySaysSo(t *testing.T) {
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	if _, present := operation(t, document, "/items/{id}", "get")["requestBody"]; present {
		t.Error("an operation that accepts nothing was given a body")
	}
}

/* -------------------------------------------------------------------------- */
/*  Responses                                                                   */
/* -------------------------------------------------------------------------- */

func TestAStatusIsTheKeyRatherThanAField(t *testing.T) {
	// A reader looks for "404" in the object and finds the description, and a
	// client generator looks up the same way.
	document := read(t, render(t, catalog(), OpenAPIOptions{}))
	responses := dig(t, document, "paths", "/items/{id}", "get", "responses").(map[string]interface{})
	found := responses["404"].(map[string]interface{})
	if found["description"] != "Not Found" {
		t.Errorf("the 404 is described as %v", found["description"])
	}
}

func TestStatusesAreInNumericOrderWithTheCatchAllLast(t *testing.T) {
	spec := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare",
		Operations: []rhorouter.Operation{{ID: "a", Method: "GET", Path: "/a",
			Responses: []rhorouter.Response{
				{Status: 500}, {Status: 200}, {Status: 404}, {Status: 100},
			}}}}
	text := render(t, spec, OpenAPIOptions{})
	order := []string{`"100"`, `"200"`, `"404"`, `"500"`}
	at := -1
	for _, status := range order {
		found := strings.Index(text, status)
		if found < 0 {
			t.Fatalf("%s is missing from the document", status)
		}
		if found < at {
			t.Errorf("%s is written out of order", status)
		}
		at = found
	}
	// An operation that names its statuses does not also get the catch-all: a
	// document that lists both is a document that over-promises.
	if strings.Contains(text, `"default"`) {
		t.Error("an operation with statuses was also given the catch-all")
	}
}

func TestAnOperationWithNoStatusGetsTheCatchAloneAndNotBoth(t *testing.T) {
	// Listing a catch-all beside statuses a document already names is a
	// document that over-promises, so the catch-all is only there when there
	// is nothing else to say.
	named := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare",
		Operations: []rhorouter.Operation{{ID: "a", Method: "GET", Path: "/a",
			Responses: []rhorouter.Response{{Status: 200}}}}}
	if strings.Contains(render(t, named, OpenAPIOptions{}), `"default"`) {
		t.Error("an operation with a status was also given the catch-all")
	}

	bare := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare",
		Operations: []rhorouter.Operation{{ID: "b", Method: "GET", Path: "/b"}}}
	document := read(t, render(t, bare, OpenAPIOptions{}))
	responses := dig(t, document, "paths", "/b", "get", "responses").(map[string]interface{})
	if len(responses) != 1 {
		t.Fatalf("responses = %v, want the catch-all alone", responses)
	}
	if _, present := responses["default"]; !present {
		t.Errorf("responses = %v, want the catch-all", responses)
	}
}

/* -------------------------------------------------------------------------- */
/*  Determinism                                                                 */
/* -------------------------------------------------------------------------- */

func TestTheSameServiceAlwaysProducesTheSameBytes(t *testing.T) {
	// A document that reshuffles itself on every run cannot be reviewed in a
	// diff, and a diff nobody reads is a contract nobody checks.
	first := OpenAPI(catalog(), OpenAPIOptions{Server: "https://api.rho.dev"}).String()
	for run := 0; run < 20; run++ {
		again := OpenAPI(catalog(), OpenAPIOptions{Server: "https://api.rho.dev"}).String()
		if again != first {
			t.Fatalf("run %d produced a different document", run)
		}
	}
}

func TestPathsAreInTheOrderTheServiceDeclaredThem(t *testing.T) {
	text := render(t, catalog(), OpenAPIOptions{})
	items := strings.Index(text, `"/items/{id}"`)
	upload := strings.Index(text, `"/items/archive"`)
	if items < 0 || upload < 0 || items > upload {
		t.Errorf("the paths are not in declaration order: %d then %d", items, upload)
	}
}

func TestMethodsAreInTheOrderAReaderExpects(t *testing.T) {
	spec := rhorouter.Spec{Version: rhorouter.SpecVersion, Module: "bare",
		Operations: []rhorouter.Operation{
			{ID: "d", Method: "DELETE", Path: "/a"},
			{ID: "p", Method: "POST", Path: "/a"},
			{ID: "g", Method: "GET", Path: "/a"},
		}}
	text := render(t, spec, OpenAPIOptions{})
	get := strings.Index(text, `"get"`)
	post := strings.Index(text, `"post"`)
	remove := strings.Index(text, `"delete"`)
	if get < 0 || post < 0 || remove < 0 || !(get < post && post < remove) {
		t.Errorf("the methods are out of order: get %d, post %d, delete %d", get, post, remove)
	}
}

func TestAHandBuiltDocumentIsWrittenInAStableOrderToo(t *testing.T) {
	// A document assembled in code has no order of its own, so it falls back
	// to one that does not change between runs.
	document := Document{
		Paths: map[string]PathItem{
			"/z": {"get": {OperationID: "z"}},
			"/a": {"get": {OperationID: "a"}},
		},
	}
	first := document.String()
	for run := 0; run < 10; run++ {
		if again := document.String(); again != first {
			t.Fatal("a hand-built document changed between runs")
		}
	}
	if strings.Index(first, `"/a"`) > strings.Index(first, `"/z"`) {
		t.Error("a hand-built document is not in a stable order")
	}
}

/* -------------------------------------------------------------------------- */
/*  Spelling                                                                    */
/* -------------------------------------------------------------------------- */

func TestQuoteEscapesWhatJSONReserves(t *testing.T) {
	cases := map[string]string{
		"plain":           `"plain"`,
		"with \"quotes\"": `"with \"quotes\""`,
		"back\\slash":     `"back\\slash"`,
		"line\nbreak":     `"line\nbreak"`,
		"tab\there":       `"tab\there"`,
		"bell\x07":        `"bell\u0007"`,
	}
	for value, want := range cases {
		if got := quote(value); got != want {
			t.Errorf("quote(%q) = %s, want %s", value, got, want)
		}
	}
}

func TestQuoteSurvivesTheRoundTrip(t *testing.T) {
	// An unescaped value produces a document no tool will accept, so the
	// escaping is checked by decoding rather than by eye.
	values := []string{`a "quoted" word`, "a\nnewline", "a\\backslash", "tab\there", "ünïcödé"}
	for _, value := range values {
		var decoded string
		if err := json.Unmarshal([]byte(quote(value)), &decoded); err != nil {
			t.Errorf("quote(%q) does not decode: %v", value, err)
			continue
		}
		if decoded != value {
			t.Errorf("quote(%q) decoded to %q", value, decoded)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                     */
/* -------------------------------------------------------------------------- */

// operation returns one operation from a parsed document.
func operation(t *testing.T, document map[string]interface{}, path, method string) map[string]interface{} {
	t.Helper()
	item, ok := dig(t, document, "paths", path).(map[string]interface{})
	if !ok {
		t.Fatalf("no path %q", path)
	}
	entry, ok := item[method].(map[string]interface{})
	if !ok {
		t.Fatalf("no %s on %q", method, path)
	}
	return entry
}

// keysOf returns a map's keys, for a failure message.
func keysOf(values map[string]interface{}) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
