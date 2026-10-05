//
// @file apidoc.go
// @brief Adding a service's HTTP API to its documentation site.
//
// A module that serves an HTTP API has two interfaces: the one the compiler
// reads out of its Rho sources, and the one a client calls over the wire. The
// first is documented by the module reference every module gets. The second is
// this: a page that lists the endpoints, what they take and what they answer,
// read from the same routes the service dispatches.
//
// It is a page of the same site rather than a site of its own, and that is the
// whole point. A second place to look is a second search box, a second set of
// links to keep working, and a reader who finds one and not the other. Here the
// API reference sits in the same sidebar, under the same section as the module
// reference, and a cross-reference from prose reaches both the same way.
//
// A module with no routes gets no page. Most modules are libraries, and a page
// saying "this module serves no HTTP routes" would be noise on every one of
// them.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package apidoc

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rho/rho-openapi/src/rhoapiref"
	"github.com/rho/rho-router/src/rhorouter"
)

// ProtocolVersion is the version of the plugin protocol this program was
// written against. A generator that speaks another version is reported rather
// than answered with something it would misread.
const ProtocolVersion = 1

// HookIndex is the whole-site hook. This plugin answers it because an API
// reference is a page the build does not have: the per-page hook only rewrites
// pages that already exist.
const HookIndex = "index"

// The address and title the page is added under. The target is what a
// cross-reference names and what the sidebar links to, and it is chosen so it
// sorts beside the module reference rho-doc writes rather than away from it.
const (
	PageTarget  = "http-api"
	PageTitle   = "HTTP API"
	PageSection = "api"
)

// Request is what a documentation build sends: the shared envelope, whose
// payload is the site this build is about.
//
// The envelope is the toolchain's and says which hook is being answered; the
// payload is the component's and says what it is being answered about. A plugin
// decodes the two, which is what every plugin for every component does.
//
//	{"version":1,"hook":"index","command":"api-reference","component":"rho-doc",
//	 "payload":{"site":"Registry","out":"build/docs","root":"/src/registry",
//	            "pages":[{"target":"index","title":"Registry"}]}}
type Request struct {
	Version   int             `json:"version"`
	Hook      string          `json:"hook"`
	Command   string          `json:"command"`
	Component string          `json:"component"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// SitePayload is this component's part of a whole-site request.
type SitePayload struct {
	Site string `json:"site"`
	Out  string `json:"out,omitempty"`
	// Root is the project directory the build read, which is where the routes
	// are read from. It is empty for a build of pages that came from somewhere
	// other than a project directory, and this plugin then has nothing to add.
	Root  string       `json:"root,omitempty"`
	Pages []PluginPage `json:"pages,omitempty"`
}

// PluginPage is one page as the build describes it.
type PluginPage struct {
	Target  string `json:"target"`
	Title   string `json:"title"`
	Href    string `json:"href,omitempty"`
	Section string `json:"section,omitempty"`
}

// Page is one page as a plugin answers with one.
type Page struct {
	Target  string `json:"target"`
	Title   string `json:"title"`
	Href    string `json:"href,omitempty"`
	Section string `json:"section,omitempty"`
	HTML    string `json:"html,omitempty"`
	Plain   string `json:"plain,omitempty"`
}

// Response is what a plugin answers.
type Response struct {
	Version     int          `json:"version,omitempty"`
	Pages       []Page       `json:"pages,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Diagnostic is a problem a plugin reports about what it was given.
type Diagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Main runs the command, reading one request and writing one answer.
func Main() int {
	if err := Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "rho-doc-api:", err)
		return 1
	}
	return 0
}

// Run answers one request.
func Run(input io.Reader, output io.Writer) error {
	content, err := io.ReadAll(input)
	if err != nil {
		return fmt.Errorf("cannot read the request: %w", err)
	}
	var request Request
	if err := json.Unmarshal(content, &request); err != nil {
		return fmt.Errorf("cannot read the request: %w", err)
	}
	if request.Version != ProtocolVersion {
		// A different version is answered with a diagnostic rather than with a
		// guess, so the build reports a plugin that does not fit instead of
		// showing a page that says the wrong thing.
		return json.NewEncoder(output).Encode(Response{
			Version: ProtocolVersion,
			Diagnostics: []Diagnostic{{
				Level: "warning",
				Message: fmt.Sprintf("rho-doc-api speaks protocol %d, the build speaks %d",
					ProtocolVersion, request.Version),
			}},
		})
	}

	// The envelope is not the whole request: the site the build is about
	// travels in the payload, and a plugin that read the envelope's top level
	// would find every field of it empty.
	var payload SitePayload
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &payload); err != nil {
			return json.NewEncoder(output).Encode(Response{
				Version: ProtocolVersion,
				Diagnostics: []Diagnostic{{
					Level:   "warning",
					Message: "the site description could not be read: " + err.Error(),
				}},
			})
		}
	}

	response := Response{Version: ProtocolVersion}
	if request.Hook == HookIndex {
		page, diagnostic := reference(payload)
		if page != nil {
			response.Pages = append(response.Pages, *page)
		}
		if diagnostic != nil {
			response.Diagnostics = append(response.Diagnostics, *diagnostic)
		}
	}
	return json.NewEncoder(output).Encode(response)
}

// reference reads a project's routes and renders the page that describes them.
//
// A project with no routes is answered with no page, because that is what most
// modules are and a page saying so would be noise on every one of them. A
// project whose sources cannot be read is answered with a diagnostic, because a
// page that is missing is worth a reason in the log.
func reference(payload SitePayload) (*Page, *Diagnostic) {
	if strings.TrimSpace(payload.Root) == "" {
		return nil, nil
	}
	result, err := rhorouter.Read(rhorouter.ReadOptions{Root: payload.Root})
	if err != nil {
		// A directory that is not a module has no HTTP API, which is the ordinary
		// state of a repository root and of every build directory in a tree — not a
		// broken one. Reporting it would put a warning on every module in a build
		// and fail a strict build over a directory that was never meant to be a
		// module.
		if rhorouter.IsNotAModule(err) {
			return nil, nil
		}
		return nil, &Diagnostic{
			Level:   "warning",
			Message: "the HTTP API could not be read: " + err.Error(),
		}
	}
	if len(result.Spec.Operations) == 0 {
		return nil, nil
	}
	if len(result.Diagnostics) > 0 {
		// The routes were read; some other source was not. The page is still
		// worth writing, and the reason the reading was incomplete travels
		// with it rather than being dropped.
		first := result.Diagnostics[0]
		return referencePage(result.Spec), &Diagnostic{
			Level: "warning",
			Message: fmt.Sprintf("the HTTP API was read with %d problem(s): %s",
				len(result.Diagnostics), first.Message),
		}
	}
	return referencePage(result.Spec), nil
}

// referencePage renders the page.
func referencePage(spec rhorouter.Spec) *Page {
	html := rhoapiref.Render(spec, rhoapiref.ReferenceOptions{
		Target:  PageTarget,
		Title:   PageTitle,
		Section: PageSection,
		// The site is built for a reader who has the module reference open, so
		// the page introduces the API rather than the service: what the service
		// is has already been said on the page before it.
		Summary: "The endpoints this module serves, and what each of them takes.",
	})
	return &Page{
		Target:  PageTarget,
		Title:   PageTitle,
		Section: PageSection,
		HTML:    html,
		// The plain text is what a per-page hook or a search index reads, so a
		// page a plugin added is searchable like any other.
		Plain: plainText(spec),
	}
}

// plainText is the page as text without markup, which is what a search index
// reads and what a plugin that rewrites prose is given.
func plainText(spec rhorouter.Spec) string {
	var b strings.Builder
	for _, operation := range spec.Operations {
		b.WriteString(operation.Method + " " + operation.Path)
		if operation.Summary != "" {
			b.WriteString(". " + operation.Summary)
		}
		b.WriteString("\n")
	}
	return b.String()
}
