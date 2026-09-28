//
// @file spec.go
// @brief The route metadata a Rho web service describes itself with.
//
// This is the contract. A Rho application declares its HTTP surface in its own
// sources, as routes and handler bodies; everything a machine needs to know
// about that surface — the operation, its parameters and their types, what it
// returns — is read out of those sources here and written as this shape.
//
// The shape is deliberately plain and serialised, because it has two consumers
// that are allowed to be built at different times. The command in this component
// turns it into an OpenAPI document. A separate `rho-openapi` component turns
// the same shape into whatever else a project publishes: a client, a mock, a
// contract test. Neither re-reads the Rho sources, and neither can disagree
// with the other about what the service exposes, because there is one reading
// and one shape.
//
// That is the whole reason this file exists rather than a reading living inside
// the OpenAPI writer. A second reader is a second dialect, and it drifts.
//
// The version is part of the contract. A consumer that does not recognise the
// version says so rather than guessing at a shape it does not know.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhorouter

import "strings"

// SpecVersion is the version of this contract. It rises when a field changes
// meaning or a field is removed, and not when one is added: a consumer that
// ignores an unknown field still reads a document it knows.
const SpecVersion = 1

// Spec is everything a module exposes over HTTP, as read from its sources.
type Spec struct {
	// Version is this contract's version, so a consumer knows what it is
	// holding.
	Version int `json:"version"`
	// Module is the Rho module the routes were declared in.
	Module string `json:"module"`
	// Title is what the service calls itself, which a documentation page needs
	// before it knows the first operation.
	Title string `json:"title,omitempty"`
	// Description is the module manifest's description.
	Description string `json:"description,omitempty"`
	// ModuleVersion is the manifest's version.
	ModuleVersion string `json:"moduleVersion,omitempty"`
	// Source is the project the routes were read from.
	Source string `json:"source,omitempty"`
	// Security is how a caller proves who it is, or nil for a service that
	// asks for nothing. It comes from the descriptor of the target that serves
	// the routes, because it is a fact about the deployment rather than about
	// a handler.
	Security *Security `json:"security,omitempty"`
	// Operations are the routes, in declaration order.
	Operations []Operation `json:"operations"`
}

// Operation is one HTTP operation: a method and a path bound to the Rho
// function that serves it.
type Operation struct {
	// ID is the operation's identifier in the document, which is the handler's
	// name. A handler is already unique within a module, and a name a person
	// wrote is a better identifier than one derived from a path.
	ID string `json:"operationId"`
	// Method is the HTTP method, upper case.
	Method string `json:"method"`
	// Path is the ServeMux pattern, wildcards included.
	Path string `json:"path"`
	// Handler is the Rho function serving the route.
	Handler string `json:"handler"`
	// Source is the file the route was declared in, relative to the project.
	Source string `json:"source,omitempty"`
	// Summary is the first line of the route's comment, and Description the
	// rest. The split is the one a documentation tool already knows: a heading
	// line and the prose under it.
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	// Parameters are the values the handler reads out of the request, with the
	// type each one is converted to.
	Parameters []Parameter `json:"parameters,omitempty"`
	// RequestBody describes what the handler reads out of the body, or is nil
	// for an operation that reads none.
	RequestBody *Body `json:"requestBody,omitempty"`
	// Responses are the statuses the handler can answer with, as far as the
	// sources say. A handler that builds its status through the standard
	// library's helpers has them written down; one that does not has none
	// listed, which is an absence rather than a claim.
	Responses []Response `json:"responses,omitempty"`
	// NeedsAuth is true when the handler checks a credential, so a caller may
	// need a token. It is "may" rather than "must": a check can be
	// conditional, and a document says so with a security entry that has an
	// empty alternative.
	NeedsAuth bool `json:"needsAuth,omitempty"`
	// Tag groups the operation with others. A tag is the module the route was
	// declared in, unless the sources say otherwise.
	Tag string `json:"tag,omitempty"`
	// Deprecated marks an operation a comment says is on its way out.
	Deprecated bool `json:"deprecated,omitempty"`
}

// Parameter is one value a handler reads out of a request.
type Parameter struct {
	// Name is the name the handler asks for, which is the wildcard's name, the
	// query key or the header.
	Name string `json:"name"`
	// In is where the value comes from: path, query, header or body.
	In string `json:"in"`
	// Type is the Rho type the raw text is converted to.
	Type string `json:"type"`
	// Required is true when the handler cannot proceed without it. A path
	// wildcard always is, because the router will not have matched the route
	// without it; the rest are true when the handler reads them and no default
	// makes them optional.
	Required bool `json:"required"`
	// Description is the comment above the accessor, when the sources gave one.
	Description string `json:"description,omitempty"`
}

// Where a value comes from. These are the four the binding reads.
const (
	InPath   = "path"
	InQuery  = "query"
	InHeader = "header"
	InBody   = "body"
)

// Body is what a handler reads out of a request body.
type Body struct {
	// ContentType is what the body is read as, which is what the handler's
	// content types imply.
	ContentType string `json:"contentType"`
	// Fields are the body's top-level fields the handler reads.
	Fields []Parameter `json:"fields,omitempty"`
	// Raw is true when the handler reads the body whole rather than as fields,
	// which is what a binary upload or a package does.
	Raw bool `json:"raw,omitempty"`
}

// Response is one status an operation can answer with.
type Response struct {
	// Status is the HTTP status code.
	Status int `json:"status"`
	// Description says what the status means here, from the standard library's
	// own name for it.
	Description string `json:"description,omitempty"`
	// ContentType is what the body carries, where the sources say.
	ContentType string `json:"contentType,omitempty"`
	// Schema is the name of the shape the body has, where one is written down.
	Schema string `json:"schema,omitempty"`
}

// HasOperation reports whether the spec already carries an operation under an
// identifier, which is how a plugin's addition is checked before it is made
// rather than after it collides.
func (s Spec) HasOperation(id string) bool {
	for _, operation := range s.Operations {
		if operation.ID == id {
			return true
		}
	}
	return false
}

// Paths groups the operations by path, which is the shape a document is written
// in: one entry per path, holding every method served on it.
func (s Spec) Paths() map[string]map[string]Operation {
	out := map[string]map[string]Operation{}
	for _, operation := range s.Operations {
		methods, seen := out[operation.Path]
		if !seen {
			methods = map[string]Operation{}
			out[operation.Path] = methods
		}
		methods[strings.ToLower(operation.Method)] = operation
	}
	return out
}

// Tags returns the distinct tags the operations carry, in the order they first
// appear, so a document lists its groups the way the sources do rather than in
// whatever order a map yields.
func (s Spec) Tags() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, operation := range s.Operations {
		if operation.Tag == "" || seen[operation.Tag] {
			continue
		}
		seen[operation.Tag] = true
		out = append(out, operation.Tag)
	}
	return out
}
