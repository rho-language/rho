//
// @file openapi.go
// @brief Writing a service's route metadata as an OpenAPI 3.1 document.
//
// A REST API is a contract, and a contract that is not written down is a
// contract two people argue about. This turns the spec a Rho service's own
// sources describe into that writing, so the document cannot disagree with the
// program: there is one reading of the sources, and this is where it is laid
// out.
//
// 3.1 rather than 3.0, because a document produced by a compiler may state facts
// the older version has nowhere to put: a type is a type rather than a string
// naming one, a `null` is part of a type rather than a flag, and a value may be
// any shape at all. Writing the newer version from the start costs nothing and
// means the facts are not lost on the way out.
//
// The document is emitted through a small ordered writer rather than through
// `encoding/json` on a struct. A document is something people read in a diff,
// and a marshaller offers no say in the order of the keys, which is the whole
// content of a diff. It is also reflection again, and a file whose whole point
// is that nothing on the path is should not start using it at the end.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

// Package rhoopenapi turns the route metadata a Rho service describes itself
// with into an OpenAPI 3.1 document.
//
// It is a package of its own rather than a file beside the reading, because the
// two have different owners. The metadata is the contract between everything
// that reads a Rho service — this writer, a generated client, a contract test —
// and it should not carry a third party's type names. An OpenAPI document is one
// reading of that contract, and a later one will be a second.
package rhoopenapi

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/rho/rho-router/src/rhorouter"
)

// The metadata this writer reads. It is the contract, aliased rather than
// copied, so a change to the contract reaches this writer without anything
// being written twice.
type (
	// Spec is everything a module exposes over HTTP.
	Spec = rhorouter.Spec
	// Body is what a handler reads out of a request body.
	Body = rhorouter.Body
	// Declared is one route as the sources declare it, before it is laid out as
	// a document operation. The two share a name and not a shape: what a source
	// says and what a document says are different things.
	Declared = rhorouter.Operation
)

// OpenAPIVersion is the specification version this writer emits.
const OpenAPIVersion = "3.1.0"

// Document is an OpenAPI 3.1 description of a service, ready to be written.
type Document struct {
	// Info describes the service the document is about.
	Info Info `json:"info"`
	// Servers are where the service answers.
	Servers []Server `json:"servers,omitempty"`
	// Paths are the operations, grouped by path.
	Paths map[string]PathItem `json:"paths"`
	// Components are the reusable schemas the operations refer to.
	Components *Components `json:"components,omitempty"`
	// Tags are the groups the operations are listed under, in order.
	Tags []Tag `json:"tags,omitempty"`
	// Security is what a caller has to prove for the service as a whole.
	Security []map[string][]string `json:"security,omitempty"`

	// order records the order the service declared its paths in, so a
	// regenerated document produces no diff when nothing changed. It is not
	// part of the document; it is how this one is written.
	order struct {
		paths []string
	}
}

// Info is what the document says about the service.
type Info struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

// Server is one place the service answers.
type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// PathItem holds the operations served on one path.
type PathItem map[string]Operation

// Operation is one documented method on a path.
type Operation struct {
	OperationID string              `json:"operationId"`
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
	// Security is what a caller has to prove for this operation, or nil to
	// take whatever the document says for the service as a whole. An entry
	// with an empty alternative means a credential is accepted but not
	// required, which is how a conditional check is written down.
	Security   []map[string][]string  `json:"security,omitempty"`
	Deprecated bool                   `json:"deprecated,omitempty"`
	Extra      map[string]interface{} `json:"-"`
}

// Parameter is one documented value of a request.
type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
	Example     string  `json:"example,omitempty"`
}

// RequestBody is what an operation accepts.
type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required"`
	Content     map[string]MediaType `json:"content"`
}

// MediaType is one representation of a body.
type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

// Response is one documented answer.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

// Schema is a value's shape. A 3.1 schema is a type rather than a string naming
// one, so a number a handler refuses to convert is a document's `integer` with
// a format, and not a word.
type Schema struct {
	Type        string             `json:"type,omitempty"`
	Format      string             `json:"format,omitempty"`
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Enum        []string           `json:"enum,omitempty"`
}

// Components holds what operations refer to by name.
type Components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme is how a caller proves who it is, as a document describes it.
type SecurityScheme struct {
	Type        string `json:"type"`
	Scheme      string `json:"scheme,omitempty"`
	In          string `json:"in,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	// BearerFormat is a hint to a client about what the token is, which
	// `JWT` states for the token shape this toolchain issues.
	BearerFormat string `json:"bearerFormat,omitempty"`
}

// Tag is a group of operations.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

/* -------------------------------------------------------------------------- */
/*  Building                                                                   */
/* -------------------------------------------------------------------------- */

// OpenAPIOptions say what a document should say beyond what the sources do.
type OpenAPIOptions struct {
	// Server is the URL the service is reached at, which a manifest cannot
	// know because a deployment sets it.
	Server string
	// ServerDescription is what to call that address.
	ServerDescription string
	// Title overrides the manifest's own name for the service.
	Title string
	// BasePath is prefixed to every path, for a service mounted under one.
	BasePath string
}

// OpenAPI turns a spec into a document.
//
// The operations are laid out in the order the sources declare them, and the
// paths in the order they first appear, so that regenerating a document produces
// no diff when nothing changed.
func OpenAPI(spec Spec, options OpenAPIOptions) Document {
	document := Document{
		Info: Info{
			Title:       firstNonEmpty(options.Title, spec.Title, spec.Module),
			Version:     firstNonEmpty(spec.ModuleVersion, "0.0.0"),
			Description: spec.Description,
		},
		Paths:      map[string]PathItem{},
		Components: &Components{},
	}
	if options.Server != "" {
		document.Servers = []Server{{URL: options.Server, Description: options.ServerDescription}}
	}

	for _, tag := range spec.Tags() {
		document.Tags = append(document.Tags, Tag{Name: tag})
	}

	// How a caller proves who it is is stated once, and the operations that
	// need it refer to it by name. A service that asks for nothing gets no
	// scheme and no requirements, because a document that demanded a
	// credential nobody enforces teaches a client to send one it has no
	// reason to hold.
	security := spec.Security.Describe()
	scheme := ""
	if security.Scheme != rhorouter.SecurityNone {
		scheme = security.Name
		document.Components.SecuritySchemes = map[string]SecurityScheme{
			scheme: securitySchemeFor(security),
		}
		if security.Required {
			document.Security = []map[string][]string{{scheme: {}}}
		}
	}

	// The order the paths first appear is recorded so a regenerated document
	// produces no diff. Every operation is laid out regardless: a path is
	// served by more than one method as a rule rather than as an exception.
	for _, operation := range spec.Operations {
		path := options.BasePath + operation.Path
		item, present := document.Paths[path]
		if !present {
			item = PathItem{}
			document.Paths[path] = item
			document.order.paths = append(document.order.paths, path)
		}
		laid := documentOperation(operation)
		laid.Security = operationSecurity(operation, path, scheme, security)
		item[strings.ToLower(operation.Method)] = laid
	}
	if len(document.Components.SecuritySchemes) == 0 && len(document.Components.Schemas) == 0 {
		document.Components = nil
	}
	return document
}

// securitySchemeFor describes a declared scheme the way a document does.
func securitySchemeFor(security rhorouter.Security) SecurityScheme {
	scheme := SecurityScheme{Description: security.Description}
	switch security.Scheme {
	case rhorouter.SecurityBearer:
		scheme.Type = "http"
		scheme.Scheme = "bearer"
		// The toolchain issues JSON Web Tokens, and saying so tells a client
		// what it is holding rather than only how to send it.
		scheme.BearerFormat = "JWT"
	default:
		// A scheme this writer has not met is named rather than guessed at,
		// so a document that cannot describe one says less instead of wrong.
		scheme.Type = "http"
		scheme.Scheme = security.Scheme
	}
	return scheme
}

// operationSecurity returns what a caller has to prove for one operation, or
// nil to take what the document says for the service.
//
// An operation the pipeline exempts, or one whose handler checks a credential
// without the pipeline enforcing it, gets an entry with an empty alternative:
// a caller may present a credential or may not. That is exactly the shape of a
// conditional check, and the alternative is what makes it a document a client
// can act on rather than one it has to guess about.
func operationSecurity(operation rhorouter.Operation, path, scheme string,
	security rhorouter.Security) []map[string][]string {

	if scheme == "" {
		return nil
	}
	if security.Public(path) {
		return nil
	}
	if security.Required {
		return nil
	}
	if !operation.NeedsAuth {
		return nil
	}
	return []map[string][]string{{scheme: {}}, {}}
}

// documentOperation lays one operation out.
func documentOperation(operation Declared) Operation {
	out := Operation{
		OperationID: operation.ID,
		Summary:     operation.Summary,
		Description: operation.Description,
		Deprecated:  operation.Deprecated,
		Responses:   map[string]Response{},
	}
	if operation.Tag != "" {
		out.Tags = []string{operation.Tag}
	}
	for _, parameter := range operation.Parameters {
		// A body field is described by the body, not as a parameter of its
		// own: a document that lists both is a document that lists it twice.
		if parameter.In == rhorouter.InBody {
			continue
		}
		kind, format := rhorouter.SchemaType(parameter.Type)
		entry := Parameter{
			Name:        parameter.Name,
			In:          parameter.In,
			Description: parameter.Description,
			Required:    parameter.Required || parameter.In == rhorouter.InPath,
			Schema:      &Schema{Type: kind, Format: format, Description: parameter.Description},
		}
		out.Parameters = append(out.Parameters, entry)
	}
	out.RequestBody = documentBody(operation.RequestBody)

	for _, response := range operation.Responses {
		key := strconv.Itoa(response.Status)
		entry := Response{Description: response.Description}
		if response.ContentType != "" {
			entry.Content = map[string]MediaType{
				response.ContentType: {Schema: &Schema{Type: "string"}},
			}
		}
		out.Responses[key] = entry
	}
	if len(out.Responses) == 0 {
		// A document with no response is not a document. The catch-all is
		// what an operation answers when nothing else matches, and an
		// operation the sources did not pin down may answer with it.
		out.Responses["default"] = Response{Description: "Unexpected response"}
	}
	return out
}

// documentBody lays out what an operation accepts, or returns nil for one that
// accepts nothing.
func documentBody(body *Body) *RequestBody {
	if body == nil {
		return nil
	}
	out := &RequestBody{
		Required: !body.Raw,
		Content:  map[string]MediaType{},
	}
	media := body.ContentType
	if media == "" {
		media = "application/json"
	}
	if body.Raw {
		// A body read whole has no fields to describe, and a binary payload is
		// described by its length rather than by a shape.
		out.Content[media] = MediaType{Schema: &Schema{
			Type:        "string",
			Format:      "binary",
			Description: "The request body, read as a whole.",
		}}
		return out
	}
	schema := &Schema{Type: "object", Properties: map[string]*Schema{}}
	required := []string{}
	for _, field := range body.Fields {
		kind, format := rhorouter.SchemaType(field.Type)
		schema.Properties[field.Name] = &Schema{
			Type: kind, Format: format, Description: field.Description,
		}
		if field.Required {
			required = append(required, field.Name)
		}
	}
	sort.Strings(required)
	schema.Required = required
	out.Content[media] = MediaType{Schema: schema}
	return out
}

// firstNonEmpty returns the first value that says something.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// String renders the document as the JSON a reader and a tool both accept.
//
// The writing is explicit, for two reasons. Nothing on the path from a Rho
// source to a document is reflection, and the order of the keys is the order a
// reader reads them in. It is also deterministic: the same service always
// produces the same bytes, because a document that reshuffles itself on every
// run cannot be reviewed in a diff.
func (d Document) String() string {
	out := &writer{}
	root := out.root()

	root.field("openapi", "%s", quote(OpenAPIVersion))
	writeInfo(out, root.obj("info"), d.Info)
	writeServers(out, root, d.Servers)
	writeTags(out, root, d.Tags)
	writePaths(out, root, d)
	writeSecurity(root, d.Security)
	writeComponents(out, root, d.Components)

	root.close()
	return out.String()
}

/* -------------------------------------------------------------------------- */
/*  Writing                                                                    */
/* -------------------------------------------------------------------------- */

// writer lays out a document with the indentation a reader wants in a diff.
type writer struct {
	b strings.Builder
}

func (w *writer) String() string { return w.b.String() }

// root opens the document itself.
func (w *writer) root() *block {
	w.b.WriteString("{\n")
	return &block{w: w, depth: 1}
}

// block is one open object or array, and the commas inside it.
//
// The comma a block's fields need is written before each field rather than
// after, so the last one is never left with a trailing comma. That is the only
// way a document stays valid JSON without every block having to know in
// advance how many fields it will hold.
type block struct {
	w     *writer
	depth int
	// fields is how many entries have been written, which is what decides
	// whether the next one needs a comma.
	fields int
}

// field writes one named entry.
func (b *block) field(key, format string, args ...interface{}) {
	b.comma()
	b.w.b.WriteString(indent(b.depth) + quote(key) + ": ")
	fmt.Fprintf(&b.w.b, format, args...)
}

// elem writes one entry of an array, which has no key.
func (b *block) elem(format string, args ...interface{}) {
	b.comma()
	b.w.b.WriteString(indent(b.depth))
	fmt.Fprintf(&b.w.b, format, args...)
}

// obj opens a nested object under a key.
func (b *block) obj(key string) *block {
	b.field(key, "{\n")
	return &block{w: b.w, depth: b.depth + 1}
}

// arr opens a nested array under a key.
func (b *block) arr(key string) *block {
	b.field(key, "[\n")
	return &block{w: b.w, depth: b.depth + 1}
}

// elemObj opens an object as an entry of an array.
func (b *block) elemObj() *block {
	b.elem("{\n")
	return &block{w: b.w, depth: b.depth + 1}
}

// comma writes the separator before the next entry.
func (b *block) comma() {
	if b.fields > 0 {
		b.w.b.WriteString(",\n")
	}
	b.fields++
}

// close ends an object.
func (b *block) close() {
	if b.fields > 0 {
		b.w.b.WriteString("\n")
	}
	b.w.b.WriteString(indent(b.depth-1) + "}")
}

// closeArray ends an array.
func (b *block) closeArray() {
	if b.fields > 0 {
		b.w.b.WriteString("\n")
	}
	b.w.b.WriteString(indent(b.depth-1) + "]")
}

// indent is the leading whitespace for a depth.
func indent(depth int) string { return strings.Repeat("  ", depth) }

/* -------------------------------------------------------------------------- */
/*  The document's parts                                                       */
/* -------------------------------------------------------------------------- */

func writeInfo(_ *writer, info *block, value Info) {
	info.field("title", "%s", quote(value.Title))
	info.field("version", "%s", quote(value.Version))
	if value.Description != "" {
		info.field("description", "%s", quote(value.Description))
	}
	info.close()
}

func writeServers(_ *writer, root *block, servers []Server) {
	if len(servers) == 0 {
		return
	}
	list := root.arr("servers")
	for _, server := range servers {
		entry := list.elemObj()
		entry.field("url", "%s", quote(server.URL))
		if server.Description != "" {
			entry.field("description", "%s", quote(server.Description))
		}
		entry.close()
	}
	list.closeArray()
}

func writeTags(_ *writer, root *block, tags []Tag) {
	if len(tags) == 0 {
		return
	}
	list := root.arr("tags")
	for _, tag := range tags {
		entry := list.elemObj()
		entry.field("name", "%s", quote(tag.Name))
		if tag.Description != "" {
			entry.field("description", "%s", quote(tag.Description))
		}
		entry.close()
	}
	list.closeArray()
}

// methodOrder is the order methods are written in, which is the order the
// specification lists them and the order a reader expects to find them in.
var methodOrder = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

func writePaths(_ *writer, root *block, document Document) {
	paths := root.obj("paths")
	for _, path := range document.PathOrder() {
		item := document.Paths[path]
		body := paths.obj(path)
		for _, method := range methodOrder {
			operation, served := item[method]
			if !served {
				continue
			}
			writeOperation(body.obj(method), operation)
		}
		body.close()
	}
	paths.close()
}

// writeOperation lays one operation out.
func writeOperation(item *block, operation Operation) {

	item.field("operationId", "%s", quote(operation.OperationID))
	if operation.Summary != "" {
		item.field("summary", "%s", quote(operation.Summary))
	}
	if operation.Description != "" {
		item.field("description", "%s", quote(operation.Description))
	}
	if len(operation.Tags) > 0 {
		list := item.arr("tags")
		for _, tag := range operation.Tags {
			list.elem("%s", quote(tag))
		}
		list.closeArray()
	}
	if operation.Deprecated {
		item.field("deprecated", "true")
	}
	writeRequirements(item, operation.Security)
	if len(operation.Parameters) > 0 {
		list := item.arr("parameters")
		for _, parameter := range operation.Parameters {
			writeParameter(list.elemObj(), parameter)
		}
		list.closeArray()
	}
	if operation.RequestBody != nil {
		writeRequestBody(item.obj("requestBody"), operation.RequestBody)
	}
	writeResponses(item.obj("responses"), operation.Responses)
	item.close()
}

func writeParameter(entry *block, parameter Parameter) {
	entry.field("name", "%s", quote(parameter.Name))
	entry.field("in", "%s", quote(parameter.In))
	if parameter.Description != "" {
		entry.field("description", "%s", quote(parameter.Description))
	}
	// A path wildcard is required whatever the sources said: the router will
	// not have matched the route without it.
	entry.field("required", "%t", parameter.Required || parameter.In == rhorouter.InPath)
	writeSchema(entry.obj("schema"), parameter.Schema)
	entry.close()
}

func writeRequestBody(body *block, value *RequestBody) {
	if value.Description != "" {
		body.field("description", "%s", quote(value.Description))
	}
	body.field("required", "%t", value.Required)
	writeContent(body.obj("content"), value.Content)
	body.close()
}

// writeContent writes the representations a body may be sent or answered in.
// The media type names the member, because that is what a client generator
// looks up.
func writeContent(content *block, values map[string]MediaType) {
	for _, media := range sortedKeys(values) {
		// The media type names the member and the member holds a schema, which
		// is the two levels a client generator walks. The schema closes itself;
		// the member it sits in is this function's to close.
		entry := content.obj(media)
		writeSchema(entry.obj("schema"), values[media].Schema)
		entry.close()
	}
	content.close()
}

// writeResponses writes the statuses in numeric order with the catch-all last,
// which is the order a reader scans them in and a stable one to diff.
func writeResponses(responses *block, values map[string]Response) {
	for _, status := range sortedStatuses(values) {
		// The status is the member's name, not a field inside it: a reader
		// looks for "404" in the object and finds the description.
		writeResponse(responses.obj(status), values[status])
	}
	responses.close()
}

func writeResponse(entry *block, value Response) {
	entry.field("description", "%s", quote(value.Description))
	if len(value.Content) > 0 {
		writeContent(entry.obj("content"), value.Content)
	}
	entry.close()
}

// writeSecurity writes what the service as a whole asks a caller to prove.
func writeSecurity(root *block, security []map[string][]string) {
	if len(security) == 0 {
		return
	}
	list := root.arr("security")
	for _, requirement := range security {
		entry := list.elemObj()
		for _, name := range sortedKeys(requirement) {
			scopes := entry.arr(name)
			for _, scope := range requirement[name] {
				scopes.elem("%s", quote(scope))
			}
			scopes.closeArray()
		}
		entry.close()
	}
	list.closeArray()
}

// writeRequirements writes what one operation asks a caller to prove. An entry
// with an empty alternative is written as an empty object, which is how a
// document says that a credential is accepted but not required.
func writeRequirements(item *block, security []map[string][]string) {
	if len(security) == 0 {
		return
	}
	list := item.arr("security")
	for _, requirement := range security {
		if len(requirement) == 0 {
			list.elem("{}")
			continue
		}
		entry := list.elemObj()
		for _, name := range sortedKeys(requirement) {
			scopes := entry.arr(name)
			for _, scope := range requirement[name] {
				scopes.elem("%s", quote(scope))
			}
			scopes.closeArray()
		}
		entry.close()
	}
	list.closeArray()
}

func writeComponents(_ *writer, root *block, components *Components) {
	if components == nil ||
		(len(components.Schemas) == 0 && len(components.SecuritySchemes) == 0) {
		return
	}
	body := root.obj("components")
	if len(components.SecuritySchemes) > 0 {
		schemes := body.obj("securitySchemes")
		for _, name := range sortedKeys(components.SecuritySchemes) {
			scheme := components.SecuritySchemes[name]
			entry := schemes.obj(name)
			entry.field("type", "%s", quote(scheme.Type))
			if scheme.Scheme != "" {
				entry.field("scheme", "%s", quote(scheme.Scheme))
			}
			if scheme.In != "" {
				entry.field("in", "%s", quote(scheme.In))
			}
			if scheme.Name != "" {
				entry.field("name", "%s", quote(scheme.Name))
			}
			if scheme.BearerFormat != "" {
				entry.field("bearerFormat", "%s", quote(scheme.BearerFormat))
			}
			if scheme.Description != "" {
				entry.field("description", "%s", quote(scheme.Description))
			}
			entry.close()
		}
		schemes.close()
	}
	if len(components.Schemas) > 0 {
		schemas := body.obj("schemas")
		for _, name := range sortedKeys(components.Schemas) {
			writeSchema(schemas.obj(name), components.Schemas[name])
		}
		schemas.close()
	}
	body.close()
}

/* -------------------------------------------------------------------------- */
/*  Schemas                                                                    */
/* -------------------------------------------------------------------------- */

// writeSchema writes a shape, or the empty object that stands for a shape
// nothing is known about. A missing type is left out rather than guessed at: a
// document that claims `string` for a value it did not read is a document that
// is wrong in a way nobody will notice.
func writeSchema(body *block, schema *Schema) {
	if schema == nil {
		body.close()
		return
	}
	// A schema is written as its own object, so the object a caller opened for
	// it is closed here. Opening one level deeper is the caller's business.
	defer body.close()
	if schema.Type != "" {
		body.field("type", "%s", quote(schema.Type))
	}
	if schema.Format != "" {
		body.field("format", "%s", quote(schema.Format))
	}
	if schema.Description != "" {
		body.field("description", "%s", quote(schema.Description))
	}
	if len(schema.Enum) > 0 {
		list := body.arr("enum")
		for _, value := range schema.Enum {
			list.elem("%s", quote(value))
		}
		list.closeArray()
	}
	if len(schema.Properties) > 0 {
		properties := body.obj("properties")
		for _, name := range sortedKeys(schema.Properties) {
			writeSchema(properties.obj(name), schema.Properties[name])
		}
		properties.close()
	}
	if len(schema.Required) > 0 {
		required := append([]string{}, schema.Required...)
		sort.Strings(required)
		list := body.arr("required")
		for _, name := range required {
			list.elem("%s", quote(name))
		}
		list.closeArray()
	}
}

/* -------------------------------------------------------------------------- */
/*  Order                                                                      */
/* -------------------------------------------------------------------------- */

// PathOrder returns the document's paths in the order the service declared
// them, falling back to alphabetical order for a document built by hand. A
// document read from sources reads in source order; one assembled in code has
// no order of its own, and needs one that does not change between runs.
func (d Document) PathOrder() []string {
	if len(d.order.paths) == 0 {
		return sortedKeys(d.Paths)
	}
	out := make([]string, 0, len(d.order.paths))
	for _, path := range d.order.paths {
		if _, served := d.Paths[path]; served {
			out = append(out, path)
		}
	}
	return out
}

// sortedKeys returns a map's keys in order.
func sortedKeys[V any](values map[string]V) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// sortedStatuses returns a response map's keys in the order a reader scans
// them: the codes in numeric order, with the catch-all last because it is the
// one that is not a code.
func sortedStatuses(values map[string]Response) []string {
	codes := []string{}
	other := []string{}
	for key := range values {
		if _, err := strconv.Atoi(key); err == nil {
			codes = append(codes, key)
			continue
		}
		other = append(other, key)
	}
	sort.Slice(codes, func(i, j int) bool {
		left, _ := strconv.Atoi(codes[i])
		right, _ := strconv.Atoi(codes[j])
		return left < right
	})
	sort.Strings(other)
	return append(codes, other...)
}

/* -------------------------------------------------------------------------- */
/*  Spelling                                                                   */
/* -------------------------------------------------------------------------- */

// quote spells a Go string as a JSON string, escaping what JSON reserves and
// leaving everything else as it was written.
func quote(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Checking                                                                    */
/* -------------------------------------------------------------------------- */

// Problem is something wrong with a description that a document cannot state.
//
// The checks are the ones a description can fail at while still being a
// document: a generator that read one would produce a client that cannot be
// called, and the failure would appear in the client rather than here.
type Problem struct {
	// Operation is the operation the problem is about, which is its identifier
	// or the route when it has none.
	Operation string
	// Message says what is wrong.
	Message string
}

// Check reports what a description cannot express, in the order it was read.
//
// What is checked is what the document format requires and the reading cannot
// invent:
//
//   - every operation has an identifier, because a generated client names its
//     methods after one;
//   - no two operations claim the same identifier, because a client cannot
//     distinguish them;
//   - every path wildcard is declared, because a document that omitted one
//     would describe a caller's parameter as unnameable;
//   - every declared path parameter is captured, because a caller could not
//     supply one the route has nowhere to put.
//
// An operation that names no status is not a problem. A handler can delegate
// past the level a reading follows, and a document expresses that as the
// catch-all response: the service answers something, and the description says
// it does not know what.
//
// What is deliberately not checked is whether a description matches the
// program. It cannot fail that: it is read from the program.
func Check(spec Spec) []Problem {
	problems := []Problem{}
	seen := map[string]string{}

	for _, operation := range spec.Operations {
		where := firstNonEmpty(operation.ID, operation.Method+" "+operation.Path)
		if strings.TrimSpace(operation.ID) == "" {
			problems = append(problems, Problem{
				Operation: where,
				Message:   "the operation has no identifier, so a client cannot name it",
			})
		} else if previous, taken := seen[operation.ID]; taken {
			problems = append(problems, Problem{
				Operation: where,
				Message:   "the identifier " + operation.ID + " is already claimed by " + previous,
			})
		} else {
			seen[operation.ID] = operation.Method + " " + operation.Path
		}

		declared := map[string]bool{}
		for _, parameter := range operation.Parameters {
			if parameter.In == rhorouter.InPath {
				declared[parameter.Name] = true
			}
		}
		for _, name := range wildcards(operation.Path) {
			if !declared[name] {
				problems = append(problems, Problem{
					Operation: where,
					Message:   "the path captures " + name + " and no parameter declares it",
				})
			}
		}

		for _, parameter := range operation.Parameters {
			if parameter.In == rhorouter.InPath && !captures(operation.Path, parameter.Name) {
				problems = append(problems, Problem{
					Operation: where,
					Message: "the parameter " + parameter.Name +
						" is declared in the path and the path captures nothing called that",
				})
			}
		}
	}
	return problems
}

// captures reports whether a path captures a name.
func captures(path, name string) bool {
	for _, captured := range wildcards(path) {
		if captured == name {
			return true
		}
	}
	return false
}

// wildcards returns the `{name}` placeholders in a path.
func wildcards(path string) []string {
	out := []string{}
	for i := 0; i < len(path); {
		if path[i] != '{' {
			i++
			continue
		}
		end := strings.IndexByte(path[i:], '}')
		if end < 0 {
			break
		}
		name := strings.TrimSuffix(path[i+1:i+end], "...")
		if name != "" {
			out = append(out, name)
		}
		i += end + 1
	}
	return out
}
