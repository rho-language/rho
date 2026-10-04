//
// @file plugin.go
// @brief Extending the route metadata with plugins.
//
// A plugin is an ordinary module of type plugin attached to this component. It
// names the component it plugs into and the hooks of that component it answers,
// in its manifest:
//
//	extends: rho-router
//
//	hooks:
//	  - operation
//	  - openapi
//
//	commands:
//	  - name: response-shapes
//	    run: rho-router-shapes
//	    hook: operation
//
// Finding a plugin and running it is the toolchain's, in rho/src/plugins, so
// that every component which takes plugins behaves the same way. What this file
// holds is what is specific to reading a web service: the three points a run
// goes through, and the shape of what a plugin is asked about.
//
// The request is the shared envelope, and the payload is this component's:
//
//	{"version":1,"hook":"spec","command":"response-shapes","component":"rho-router",
//	 "payload":{"version":1,"module":"registry","operations":[...]}}
//
// The answer adds to what was read rather than replacing it, so a plugin
// extends the metadata instead of taking it over. An operation already present
// is left alone, which is how a plugin that wants to fill a gap fills the gap
// and a plugin that wants to add an operation adds one:
//
//	{"version":1,
//	 "operations":[{"operationId":"health","method":"GET","path":"/healthz",
//	                "responses":[{"status":200,"description":"OK"}]}],
//	 "diagnostics":[{"level":"warning","message":"..."}]}
//
// A plugin that fails is reported and skipped. One broken extension must not
// cost a reader the API of everything else.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorouter

import (
	"encoding/json"
	"fmt"

	"rho/src/plugins"
)

// Source is where this component looks for the plugins it runs.
type Source = plugins.Source

// Plugin is a plugin attached to this component.
type Plugin = plugins.Plugin

// ProtocolVersion is the version of the shared envelope.
const ProtocolVersion = plugins.ProtocolVersion

// The hooks a run goes through, and so the hooks a plugin may answer.
const (
	// HookSpec is the whole-service hook, asked before the operations are laid
	// out. A plugin answers it to add an operation no source declares, or to
	// report what it thinks is missing.
	HookSpec = "spec"
	// HookOperation is the per-operation hook, asked once per route. A plugin
	// answers it to fill in a description, a tag or a response the sources
	// leave to be guessed at.
	HookOperation = "operation"
	// HookOpenAPI is the finished-document hook, asked once the document is
	// written. A plugin answers it to reorder, filter or extend it.
	HookOpenAPI = "openapi"
)

// ComponentName is this component's own name, which a plugin has to say in order
// to be loaded by it. A component knows its own name; that is the one thing it
// does not have to be told.
const ComponentName = "rho-router"

// OfferedHooks are the hooks a run goes through. They are declared in the
// component's manifest as well, because that is what a plugin is validated
// against; the test beside this file checks the two agree, so the list is
// written once in the manifest and read here.
var OfferedHooks = []string{HookSpec, HookOperation, HookOpenAPI}

// offers reports whether this component runs a hook at all. A run asks for
// nothing it does not run, whatever a plugin claims.
func offers(hook string) bool {
	for _, offered := range OfferedHooks {
		if offered == hook {
			return true
		}
	}
	return false
}

// DiscoverPlugins reads the plugins that extend this component.
func DiscoverPlugins(source Source) []Plugin {
	return source.Discover(ComponentName)
}

/* -------------------------------------------------------------------------- */
/*  What a plugin is asked                                                      */
/* -------------------------------------------------------------------------- */

// SpecRequest is what a plugin is asked about a whole service. The operations
// are the ones the sources declared; a plugin adds to them.
type SpecRequest struct {
	// Spec is the metadata as it was read.
	Spec Spec `json:"spec"`
}

// OperationRequest is what a plugin is asked about one route.
type OperationRequest struct {
	// Spec is the service the operation belongs to.
	Spec Spec `json:"spec"`
	// Operation is the route, as the sources declared it.
	Operation Operation `json:"operation"`
}

// OpenAPIRequest is what a plugin is asked about a finished document. The
// document is passed as the text that will be written, because that is what a
// plugin that filters it has to work on.
type OpenAPIRequest struct {
	Spec     Spec   `json:"spec"`
	Document string `json:"document"`
}

// PluginResponse is what a plugin answers: the operations it adds, and anything
// it wants the run to report.
type PluginResponse struct {
	// Operations are added to the spec. An operation whose identifier is
	// already present is left alone, so a plugin extends rather than replaces.
	Operations []Operation `json:"operations,omitempty"`
	// Replacements rewrite a value already read, keyed by the operation's
	// identifier. A value here wins over what the sources said, which is how a
	// plugin states something the language has nowhere to say.
	Replacements map[string]Operation `json:"replacements,omitempty"`
	// Document replaces the finished document whole, for a plugin that filters
	// or reformats it rather than adding to it.
	Document string `json:"document,omitempty"`
	// Diagnostics are problems with the metadata the plugin was given, rather
	// than with itself.
	Diagnostics []PluginDiagnostic `json:"diagnostics,omitempty"`
}

// PluginDiagnostic is a problem a plugin reports about the service it was given.
type PluginDiagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                    */
/* -------------------------------------------------------------------------- */

// RunSpec asks every plugin what it makes of the whole service, and folds in
// what they answered. A plugin that fails is a diagnostic, not a failure: one
// broken extension must not cost a reader the API of everything else.
func RunSpec(found []Plugin, spec Spec) (Spec, []Diagnostic) {
	raw, diagnostics := answer(found, HookSpec, SpecRequest{Spec: spec})
	response := PluginResponse{}
	diagnostics = append(diagnostics, decode(raw, &response)...)
	return fold(spec, response, diagnostics)
}

// RunOperation asks every plugin about one route, and folds in what they
// answered.
func RunOperation(found []Plugin, spec Spec, operation Operation) (Operation, []Diagnostic) {
	raw, diagnostics := answer(found, HookOperation, OperationRequest{Spec: spec, Operation: operation})
	response := PluginResponse{}
	diagnostics = append(diagnostics, decode(raw, &response)...)
	if replacement, ok := response.Replacements[operation.ID]; ok {
		return mergeOperation(operation, replacement), append(diagnostics, responseDiagnostics(response)...)
	}
	return operation, append(diagnostics, responseDiagnostics(response)...)
}

// RunOpenAPI asks every plugin about the finished document.
func RunOpenAPI(found []Plugin, spec Spec, document string) (string, []Diagnostic) {
	raw, diagnostics := answer(found, HookOpenAPI, OpenAPIRequest{Spec: spec, Document: document})
	response := PluginResponse{}
	diagnostics = append(diagnostics, decode(raw, &response)...)
	// A plugin that answered with nothing about the document leaves it as it
	// was, rather than emptying it.
	if response.Document == "" {
		return document, append(diagnostics, responseDiagnostics(response)...)
	}
	return response.Document, append(diagnostics, responseDiagnostics(response)...)
}

// answer runs one hook and returns what the plugins wrote.
func answer(found []Plugin, hook string, payload any) ([]json.RawMessage, []Diagnostic) {
	if !offers(hook) || len(found) == 0 {
		return nil, nil
	}
	result := plugins.RunHooks(found, ComponentName, hook, payload)
	out := []Diagnostic{}
	for _, item := range result.Diagnostics {
		out = append(out, Diagnostic{Level: item.Level, Message: item.Message})
	}
	return result.Payload, out
}

// decode reads what the plugins wrote, one answer at a time. An answer that
// cannot be read is a warning rather than a failure, which is the same choice
// the runner makes about a command that cannot run.
func decode(raw []json.RawMessage, into any) []Diagnostic {
	out := []Diagnostic{}
	for _, item := range raw {
		if err := json.Unmarshal(item, into); err != nil {
			out = append(out, Diagnostic{Level: "warning",
				Message: fmt.Sprintf("a plugin answer could not be read: %v", err)})
		}
	}
	return out
}

// fold applies a plugin's answer to a spec.
func fold(spec Spec, response PluginResponse, diagnostics []Diagnostic) (Spec, []Diagnostic) {
	// A replacement wins over an addition, because a replacement says what the
	// operation is and an addition only says it exists.
	for id, replacement := range response.Replacements {
		for i, operation := range spec.Operations {
			if operation.ID == id {
				spec.Operations[i] = mergeOperation(operation, replacement)
			}
		}
	}
	for _, operation := range response.Operations {
		if spec.HasOperation(operation.ID) {
			continue
		}
		spec.Operations = append(spec.Operations, operation)
	}
	return spec, append(diagnostics, responseDiagnostics(response)...)
}

// mergeOperation lets a plugin fill a gap in what was read without having to
// restate what it read. An answer that says nothing about a field leaves the
// field as it was.
func mergeOperation(read, answered Operation) Operation {
	if answered.Summary != "" {
		read.Summary = answered.Summary
	}
	if answered.Description != "" {
		read.Description = answered.Description
	}
	if answered.Tag != "" {
		read.Tag = answered.Tag
	}
	if len(answered.Parameters) > 0 {
		read.Parameters = answered.Parameters
	}
	if answered.RequestBody != nil {
		read.RequestBody = answered.RequestBody
	}
	if len(answered.Responses) > 0 {
		read.Responses = answered.Responses
	}
	if answered.Deprecated {
		read.Deprecated = true
	}
	return read
}

// responseDiagnostics converts what a plugin reported into this run's own kind.
func responseDiagnostics(response PluginResponse) []Diagnostic {
	out := []Diagnostic{}
	for _, item := range response.Diagnostics {
		out = append(out, Diagnostic{Level: item.Level, Message: item.Message})
	}
	return out
}
