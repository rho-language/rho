//
// @file plugin.go
// @brief Extending the documentation generator with plugins.
//
// A plugin is an ordinary module of type plugin that is attached to a
// component. It names the component it plugs into and the hooks of that
// component it answers, in its manifest:
//
//	extends: rho-doc
//
//	hooks:
//	  - index
//	  - page
//
//	commands:
//	  - name: table-of-contents
//	    run: rho-doc-toc
//	    hook: index
//
// The generator loads the plugins that name it, and a plugin that names another
// component belongs to that tool. `rho module validate` checks the relation
// against the component before a plugin is published: the component has to
// exist, every hook the plugin declares has to be one the component offers, and
// every command has to answer a hook the plugin declared.
//
// Finding a plugin and running it are the toolchain's, in rho/src/plugins, so
// that every component which takes plugins behaves the same way and a plugin is
// written the same way whoever wrote it for. What this file holds is what is
// specific to a documentation build: the hooks a run goes through, and the shape
// of the pages a plugin is asked about.
//
// A command is an executable. The runner writes a request to its standard input
// as JSON and reads the answer from its standard output as JSON:
//
//	{"version":1,"hook":"index","command":"contents","component":"rho-doc",
//	 "payload":{"site":"Rho","pages":[{"target":"index","title":"Registry"}]}}
//
// The response adds to the site rather than replacing it, so a plugin extends
// the documentation instead of taking it over:
//
//	{"version":1,
//	 "pages":[{"target":"contents","title":"Contents","section":"extensions",
//	            "html":"<nav>...</nav>","plain":"..."}],
//	 "diagnostics":[{"level":"warning","message":"..."}]}
//
// A plugin that fails is reported and skipped. One broken extension must not cost
// a reader the documentation of everything else, which is why a failure is a
// diagnostic and not an error.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import "rho/src/plugins"

/* -------------------------------------------------------------------------- */
/*  What this component offers                                                */
/*                                                                             */
/*  The discovery and the runner are the toolchain's, so every component that   */
/*  takes plugins behaves the same way: a plugin is found because it names the  */
/*  component, it answers only the hooks it declared, and one that fails is     */
/*  reported rather than allowed to stop the run.                              */
/* -------------------------------------------------------------------------- */

// Source is where this generator looks for the plugins it runs.
type Source = plugins.Source

// Plugin is a plugin attached to this component.
type Plugin = plugins.Plugin

// The protocol a documentation build speaks, over the shared envelope. A plugin
// for another component sees the same shapes and reads a different payload.
const ProtocolVersion = plugins.ProtocolVersion

// SiteRequest is what a plugin is asked about a whole build: the site, and the
// pages it has.
type SiteRequest struct {
	Site  string       `json:"site"`
	Out   string       `json:"out,omitempty"`
	Pages []PluginPage `json:"pages"`
}

// PageRequest is what a plugin is asked about one page: the page itself, and
// the site it belongs to.
type PageRequest struct {
	Site string     `json:"site"`
	Page PluginPage `json:"page"`
}

// PluginPage is one page as a plugin sees it: what it is called, where it lives,
// and the rendered body for a per-page hook.
type PluginPage struct {
	Target string `json:"target"`
	Title  string `json:"title"`
	Href   string `json:"href,omitempty"`
	// Section is the sidebar group the page is listed under.
	Section string `json:"section,omitempty"`
	// HTML is the rendered body, present for a per-page hook.
	HTML string `json:"html,omitempty"`
	// Plain is the text of the page without markup, which is what a plugin
	// that rewrites prose needs.
	Plain string `json:"plain,omitempty"`
}

// PluginResponse is what a documentation plugin answers: pages it adds, bodies
// it rewrites, and anything it wants the build to report.
type PluginResponse struct {
	Pages        []PluginPage       `json:"pages,omitempty"`
	Replacements map[string]string  `json:"replacements,omitempty"`
	Diagnostics  []PluginDiagnostic `json:"diagnostics,omitempty"`
}

// PluginDiagnostic is a problem a plugin reports about the documentation it was
// given, rather than about itself.
type PluginDiagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// HookIndex is the whole-site hook: a plugin that builds something over the
// finished set of pages, such as a table of contents.
const HookIndex = "index"

// HookPage is the per-page hook: a plugin that renders or rewrites one page, such
// as a banner saying the page was generated.
const HookPage = "page"

// ComponentName is this component's own name, which a plugin has to say in order
// to be loaded by it. A component knows its own name; that is the one thing it
// does not have to be told.
const ComponentName = "rho-doc"

// OfferedHooks are the hooks a run of this generator goes through, and so the
// hooks a plugin may answer. They are declared in the component's manifest as
// well, because that is what a plugin is validated against; the test beside
// this file checks the two agree, so the list is written once in the manifest
// and read here.
var OfferedHooks = []string{HookIndex, HookPage}

// offers reports whether this generator runs a hook at all. A run asks for
// nothing it does not run, whatever a plugin claims.
func offers(hook string) bool {
	for _, offered := range OfferedHooks {
		if offered == hook {
			return true
		}
	}
	return false
}

// DiscoverPlugins reads the plugins that extend this generator.
func DiscoverPlugins(source Source) []Plugin {
	return source.Discover(ComponentName)
}

// DefaultExtensionSection is the sidebar group a plugin's own pages appear under
// when it says nothing else.
const DefaultExtensionSection = "extensions"
