//
// @file response.go
// @brief Reading what a handler answers with.
//
// A handler that builds its status through the standard library's helpers says
// what it answers with, in a form a reader can take in: `status_not_found()`
// is a 404 and means a 404. Reading that is the difference between a document
// that describes the service and one that guesses at it.
//
// A status written as a bare number is still a status, and is read as one. What
// is not read is a status computed at run time, because a document cannot
// enumerate what a program will decide; an operation with no status written down
// gets the catch-all response a document is required to carry rather than a
// claim it cannot support.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhorouter

import (
	"net/http"
	"strconv"
	"strings"

	"rho/src/frontend/ast"
)

// standardStatuses are the statuses std.web.http names, and what each is called
// in a document. The name is the reason to read a call rather than the number:
// a document that says `404 Not Found` says what the code meant.
var standardStatuses = map[string]struct {
	status int
	name   string
}{
	"status_ok":                  {200, "OK"},
	"status_created":             {201, "Created"},
	"status_accepted":            {202, "Accepted"},
	"status_no_content":          {204, "No Content"},
	"status_moved":               {301, "Moved Permanently"},
	"status_found":               {302, "Found"},
	"status_not_modified":        {304, "Not Modified"},
	"status_bad_request":         {400, "Bad Request"},
	"status_unauthorized":        {401, "Unauthorized"},
	"status_forbidden":           {403, "Forbidden"},
	"status_not_found":           {404, "Not Found"},
	"status_method_not_allowed":  {405, "Method Not Allowed"},
	"status_conflict":            {409, "Conflict"},
	"status_gone":                {410, "Gone"},
	"status_payload_too_large":   {413, "Content Too Large"},
	"status_unsupported_media":   {415, "Unsupported Media Type"},
	"status_unprocessable":       {422, "Unprocessable Content"},
	"status_too_many_requests":   {429, "Too Many Requests"},
	"status_internal":            {500, "Internal Server Error"},
	"status_not_implemented":     {501, "Not Implemented"},
	"status_bad_gateway":         {502, "Bad Gateway"},
	"status_service_unavailable": {503, "Service Unavailable"},
}

// statusNames are the names the HTTP specifications use, which are the ones a
// document is read against. The Go standard library keeps a few of its older
// names, so the current spelling is stated here where it differs rather than
// taken from a table that may be behind.
var statusNames = map[int]string{
	413: "Content Too Large",
	422: "Unprocessable Content",
	415: "Unsupported Media Type",
}

// statusName returns what a status is called. The table of the specifications'
// own names is consulted first, because a few of them were renamed and the Go
// standard library still carries the older word; the standard library answers
// for everything the table does not, so a status this file has never heard of is
// still named correctly.
func statusName(status int) string {
	if name, known := statusNames[status]; known {
		return name
	}
	if text := http.StatusText(status); text != "" {
		return text
	}
	return "Status " + strconv.Itoa(status)
}

// responseOf reads a call as the status it answers with, and reports whether
// the call is a status at all.
func responseOf(name string) (int, bool) {
	if known, ok := standardStatuses[name]; ok {
		return known.status, true
	}
	return 0, false
}

// responseAt reads a call site as the status it answers with.
//
// A status is written as a call whose argument is a literal, because that is
// the only way a status can be known here. A helper that takes a status as an
// argument — `json(status_not_found(), ...)` — is a helper whose argument is the
// status, so the call is followed one level down.
func responseAt(call ast.CallExpr) (int, bool) {
	callee, ok := call.Callee.(ast.IdentifierExpr)
	if !ok {
		return 0, false
	}
	if status, known := responseOf(callee.Name); known {
		return status, true
	}
	if len(call.Args) == 0 {
		return 0, false
	}
	// The first argument is the status in every helper the standard library
	// offers, and in the ones a project writes beside it.
	switch argument := call.Args[0].(type) {
	case ast.CallExpr:
		return responseAt(argument)
	case ast.NumberExpr:
		status, err := strconv.Atoi(strings.TrimSpace(argument.Value))
		if err != nil {
			return 0, false
		}
		return status, true
	}
	return 0, false
}

// contentTypes are the content types a response builder names, and the media
// type a document calls each by.
var contentTypes = map[string]string{
	"text":  "text/plain",
	"json":  "application/json",
	"html":  "text/html",
	"xml":   "application/xml",
	"csv":   "text/csv",
	"event": "text/event-stream",
	"bytes": "application/octet-stream",
}

// contentTypeOf reads the content type a response answers with, which is the
// one its builder names. A helper takes its body as its second argument, so a
// literal media type written there is the answer.
func contentTypeOf(call ast.CallExpr) string {
	if len(call.Args) >= 2 {
		if literal, ok := call.Args[1].(ast.StringExpr); ok && strings.Contains(literal.Value, "/") {
			return literal.Value
		}
	}
	if callee, ok := call.Callee.(ast.IdentifierExpr); ok {
		if media, known := contentTypes[callee.Name]; known {
			return media
		}
	}
	return ""
}

// literalResponse reads a response written directly, as in
// `return Response{ 404, "no such module", "text/plain" }`. A struct literal
// carries its status in its first field and its content type in its third.
func literalResponse(expression ast.Expr) (int, string, bool) {
	literal, ok := expression.(ast.StructLiteralExpr)
	if !ok || len(literal.Values) == 0 {
		return 0, "", false
	}
	number, ok := literal.Values[0].(ast.NumberExpr)
	if !ok {
		return 0, "", false
	}
	status, err := strconv.Atoi(strings.TrimSpace(number.Value))
	if err != nil {
		return 0, "", false
	}
	contentType := ""
	if len(literal.Values) >= 3 {
		if field, ok := literal.Values[2].(ast.StringExpr); ok {
			contentType = field.Value
		}
	}
	return status, contentType, true
}
