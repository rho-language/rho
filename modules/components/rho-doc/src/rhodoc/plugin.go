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
// A command is an executable. The generator writes a request to its standard
// input as JSON and reads a response as JSON:
//
//	{"version": 1, "hook": "index", "site": "Rho", "out": "build/docs",
//	 "pages": [{"target": "index", "title": "Registry", "href": "registry.html"}],
//	 "page": {"target": "index", "title": "Registry", "html": "..."}}
//
// The response adds to the site rather than replacing it, so a plugin extends
// the documentation instead of taking it over:
//
//	{"version": 1,
//	 "pages": [{"target": "toc", "title": "Contents", "section": "extensions",
//	            "html": "<nav>...</nav>", "plain": "..."}],
//	 "diagnostics": [{"level": "warning", "message": "..."}]}
//
// A plugin that fails is reported and skipped. One broken extension must not
// cost a reader the documentation of everything else, which is why a failure is
// a diagnostic and not an error.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  The protocol                                                              */
/* -------------------------------------------------------------------------- */

// ProtocolVersion is the version of the request and response shapes. A plugin
// declares the version it was written against, so a generator and a plugin from
// different releases can say so instead of misreading each other.
const ProtocolVersion = 1

// PluginRequest is what a site build sends to a plugin command.
type PluginRequest struct {
	Version int    `json:"version"`
	Hook    string `json:"hook"`
	Command string `json:"command"`
	Site    string `json:"site"`
	Project string `json:"project"`
	// Out is the directory the site is written to, so a plugin that writes a
	// file of its own knows where it belongs.
	Out string `json:"out"`
	// Pages describes the pages the site already has.
	Pages []PluginPage `json:"pages"`
	// Page is the page being rendered, when the command runs per page. It is
	// nil for a whole-site command.
	Page *PluginPage `json:"page,omitempty"`
}

// PluginPage is one page as a plugin sees it: what it is called and where it
// lives, with the rendered body for a per-page command.
type PluginPage struct {
	Target string `json:"target"`
	Title  string `json:"title"`
	Href   string `json:"href"`
	// Section is the sidebar group the page is listed under.
	Section string `json:"section"`
	// HTML is the rendered body, present for a per-page command.
	HTML string `json:"html,omitempty"`
	// Plain is the text of the page without markup, which is what a plugin
	// that rewrites prose needs.
	Plain string `json:"plain,omitempty"`
}

// PluginResponse is what a plugin command returns.
type PluginResponse struct {
	Version int `json:"version,omitempty"`
	// Pages are pages the plugin adds to the site.
	Pages []PluginPage `json:"pages,omitempty"`
	// Replacements are rendered bodies the plugin rewrote, keyed by the target
	// of the page they belong to. A plugin that renders a page itself uses
	// this rather than adding a second page with the same target.
	Replacements map[string]string `json:"replacements,omitempty"`
	// Diagnostics are what the plugin wants the build to report.
	Diagnostics []PluginDiagnostic `json:"diagnostics,omitempty"`
}

// PluginDiagnostic is a problem a plugin reports about the documentation it was
// given, rather than about itself.
type PluginDiagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

/* -------------------------------------------------------------------------- */
/*  Discovery                                                                 */
/* -------------------------------------------------------------------------- */

// Plugin is a discovered extension.
//
// A plugin is not a thing in itself: it answers hooks of one component, and a
// run of that component is the only thing that ever calls it. So a Plugin
// always knows which component it extends and which hooks of that component it
// declared, and a component loads only the plugins that name it.
type Plugin struct {
	// Name is the module name.
	Name string
	// Version is the version it was found at.
	Version string
	// Root is the module root, which a command path is relative to.
	Root string
	// Extends is the component this plugin plugs into.
	Extends string
	// Hooks are the hooks of that component this plugin answers, and the
	// component offers every one of them.
	Hooks []string
	// Section is where the plugin's own pages appear.
	Section string
	// Pages are doc-as-code files the plugin contributes.
	Pages []string
	// Formats are the output formats it can render.
	Formats []string
	// Commands are the executables a build runs through it.
	Commands []PluginCommand
}

// PluginCommand is one executable a plugin answers a hook with.
type PluginCommand struct {
	Name string
	Run  string
	Hook string
}

// Stage is the hook a command answers: the one it names, or its own name when a
// one-hook plugin lets it stand for the stage.
func (c PluginCommand) Stage() string {
	if c.Hook != "" {
		return c.Hook
	}
	return c.Name
}

// Answers reports whether this plugin declared a hook.
func (p Plugin) Answers(hook string) bool {
	for _, offered := range p.Hooks {
		if offered == hook {
			return true
		}
	}
	return false
}

// Plugins is where a build looks for the plugins it runs.
type Plugins struct {
	// Tree is the tree being documented, which may also carry plugins.
	Tree string
	// RhoHome is the installation whose plugins are trusted, or "" for none.
	RhoHome string
	// TrustTree runs the plugins the tree itself carries.
	//
	// A plugin is a program, and running one is running code the tree supplied.
	// An installed plugin was published by someone the user chose to install;
	// a plugin found in the tree being documented was supplied by whoever wrote
	// that tree, which for a repository that was just cloned is a stranger. So
	// the tree's own plugins run only when the caller asks for them.
	TrustTree bool
}

// DiscoverPlugins reads the plugins that extend one component.
//
// A plugin that names another component is left out. It is not a fault, it is
// the relation working: that plugin belongs to another tool, and this one has no
// business calling it.
//
// An installed plugin is preferred over one in the working tree of the same
// name, so a published version is the one a build uses.
func (p Plugins) DiscoverPlugins(component string) []Plugin {
	plugins := []Plugin{}
	for _, plugin := range p.All() {
		if plugin.Extends == component {
			plugins = append(plugins, plugin)
		}
	}
	return plugins
}

// All reads every plugin that may be run, whatever component each one extends.
// It is how a tool reports the plugins that belong to someone else, which a
// reader otherwise cannot account for.
func (p Plugins) All() []Plugin {
	plugins := []Plugin{}
	seen := map[string]bool{}
	if p.RhoHome != "" {
		for _, root := range installedPluginRoots(p.RhoHome) {
			collectPlugin(root, seen, &plugins)
		}
	}
	if p.TrustTree && p.Tree != "" {
		filepath.Walk(p.Tree, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				switch info.Name() {
				case ".git", ".kilo", "node_modules", "build", "vendor":
					return filepath.SkipDir
				}
				return nil
			}
			if info.Name() == "module.yml" {
				collectPlugin(filepath.Dir(path), seen, &plugins)
			}
			return nil
		})
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins
}

// collectPlugin reads one plugin, keeping the first version found of a name so
// an installed plugin wins over the one in the working tree, and keeping only
// the plugins attached to the component being built.
func collectPlugin(root string, seen map[string]bool, out *[]Plugin) {
	module, ok := readModule(root, SourceOptions{})
	if !ok || module.Type != "plugin" {
		return
	}
	extension := module.Manifest.Extends
	if extension == nil {
		return
	}
	if seen[module.Name] {
		return
	}
	seen[module.Name] = true

	plugin := Plugin{
		Name:    module.Name,
		Version: module.Version,
		Root:    root,
		Extends: extension.Component,
		Hooks:   extension.Hooks,
		Section: DefaultExtensionSection,
	}
	for _, command := range extension.Commands {
		plugin.Commands = append(plugin.Commands, PluginCommand{Name: command.Name, Run: command.Run, Hook: command.Hook})
	}
	*out = append(*out, plugin)
}

// DefaultExtensionSection is the sidebar group a plugin's own pages appear under
// when it says nothing else.
const DefaultExtensionSection = "extensions"

// installedPluginRoots is every installed plugin, one directory per name: the
// newest version of each, resolved the way a dependency resolves a module, so
// two installed versions of a plugin are one plugin and not two.
func installedPluginRoots(rhoHome string) []string {
	names := deps.InstalledPluginNames()
	roots := make([]string, 0, len(names))
	for _, name := range names {
		if root := deps.InstalledPluginRoot(name); root != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// PluginResult is what running the plugins produced.
type PluginResult struct {
	// Pages are the pages the plugins added.
	Pages []*SitePage
	// Replacements are rewritten bodies, keyed by page target.
	Replacements map[string]string
	// Diagnostics is what the plugins reported.
	Diagnostics []Diagnostic
}

// RunHooks runs every command a plugin declares for a hook.
//
// The request is the site as it stands, so a command that answers with pages
// sees what the others contributed. A command that fails is recorded and the
// run continues: the documentation of everything else still matters more than
// one extension.
func RunHooks(plugins []Plugin, hook string, request PluginRequest) PluginResult {
	result := PluginResult{Replacements: map[string]string{}}
	if !offers(hook) {
		return result
	}

	for _, plugin := range plugins {
		// A plugin answers only the hooks it declared, and only the ones the
		// component offers: the two lists were checked against each other when
		// the module was validated, and are checked again here because a build
		// must not depend on that having happened.
		if !plugin.Answers(hook) {
			continue
		}
		for _, command := range plugin.Commands {
			if command.Stage() != hook {
				continue
			}
			response, err := runCommand(plugin, command, request)
			if err != nil {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Path:    filepath.Join(plugin.Root, "module.yml"),
					Message: fmt.Sprintf("plugin %s command %s failed: %v", plugin.Name, command.Name, err),
				})
				continue
			}
			mergeResponse(&result, plugin, command, response)
		}
	}
	return result
}

// runCommand runs one plugin executable and reads its response.
func runCommand(plugin Plugin, command PluginCommand, request PluginRequest) (PluginResponse, error) {
	response := PluginResponse{}
	argv := splitCommand(command.Run)
	if len(argv) == 0 {
		return response, fmt.Errorf("command %q names nothing to run", command.Name)
	}
	executable, err := pluginExecutable(plugin, argv[0])
	if err != nil {
		return response, err
	}
	argv[0] = executable

	request.Version = ProtocolVersion
	request.Hook = hookOf(command)
	request.Command = command.Name
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return response, err
	}

	// A plugin that hangs must not hang the build: documentation is not worth
	// waiting for, so a command is given a bounded time to answer.
	var stdout, stderr bytes.Buffer
	process := exec.Command(argv[0], argv[1:]...)
	process.Dir = plugin.Root
	process.Stdin = bytes.NewReader(payload)
	process.Stdout = &stdout
	process.Stderr = &stderr
	process.Env = append(os.Environ(), "RHO_DOC_HOOK="+request.Hook, "RHO_PLUGIN="+plugin.Name)

	if err := runBounded(process, PluginTimeout); err != nil {
		return response, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return response, fmt.Errorf("cannot read the response: %w", err)
	}
	return response, nil
}

func hookOf(command PluginCommand) string {
	if command.Hook != "" {
		return command.Hook
	}
	return command.Name
}

// mergeResponse folds what a plugin returned into the build.
func mergeResponse(result *PluginResult, plugin Plugin, command PluginCommand, response PluginResponse) {
	for target, body := range response.Replacements {
		result.Replacements[target] = body
	}
	for _, page := range response.Pages {
		section := page.Section
		if section == "" {
			section = plugin.Section
		}
		href := page.Href
		if href == "" {
			href = page.Target + ".html"
		}
		result.Pages = append(result.Pages, &SitePage{
			Target:  page.Target,
			Title:   page.Title,
			Href:    href,
			Section: section,
			Module:  plugin.Name,
			Body:    page.HTML,
			Plain:   page.Plain,
		})
	}
	for _, diagnostic := range response.Diagnostics {
		level := diagnostic.Level
		if level == "" {
			level = "warning"
		}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Path:    filepath.Join(plugin.Root, "module.yml"),
			Line:    0,
			Message: fmt.Sprintf("%s (%s/%s): %s", level, plugin.Name, command.Name, diagnostic.Message),
		})
	}
}

// pluginExecutable resolves the program a command runs.
//
// A plugin runs a program it carries, which is the binary its manifest publishes
// and which the package installs beside it, or one found on PATH. A path that
// climbs out of the plugin with `..`, and an absolute path, are refused: a
// plugin is a module with a directory of its own, and a program named outside it
// is either a mistake or an attempt to run something the plugin does not carry.
func pluginExecutable(plugin Plugin, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("plugin %s runs a command with no program", plugin.Name)
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("plugin %s runs %s, which is an absolute path; a plugin runs a program it carries or one on PATH", plugin.Name, name)
	}
	if strings.Contains(filepath.ToSlash(name), "..") {
		return "", fmt.Errorf("plugin %s runs %s, which leaves the plugin directory", plugin.Name, name)
	}
	// A program the plugin carries is preferred: an installed plugin holds the
	// command it publishes, and a build should not depend on it also being on
	// PATH.
	if !strings.ContainsRune(name, filepath.Separator) && !strings.ContainsRune(name, '/') {
		if carried := filepath.Join(plugin.Root, name); exists(carried) {
			return carried, nil
		}
		resolved, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("plugin %s runs %s, which it does not carry and which is not on PATH: %w", plugin.Name, name, err)
		}
		return resolved, nil
	}
	resolved := filepath.Join(plugin.Root, name)
	if _, err := os.Stat(resolved); err != nil {
		return "", fmt.Errorf("plugin %s runs %s, which it does not carry: %w", plugin.Name, name, err)
	}
	return resolved, nil
}


// splitCommand reads the run field of a command into an argument vector.
//
// A plugin usually runs a program with arguments, and a manifest that could
// only name one file would make every such plugin ship a wrapper script. A
// double-quoted argument is kept whole, so a path with a space in it works.
func splitCommand(value string) []string {
	argv := []string{}
	current := strings.Builder{}
	quoted := false
	started := false
	for _, r := range value {
		switch {
		case r == '"':
			quoted = !quoted
			started = true
		case (r == ' ' || r == '\t') && !quoted:
			if started {
				argv = append(argv, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if started {
		argv = append(argv, current.String())
	}
	return argv
}

// PluginTimeout is how long a plugin command may take before the build carries
// on without it.
const PluginTimeout = 30 * time.Second

// runBounded runs a command with a deadline.
func runBounded(process *exec.Cmd, timeout time.Duration) error {
	if err := process.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		// A process that will not stop is killed, so a plugin cannot hold the
		// build open past its deadline.
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		<-done
		return fmt.Errorf("timed out after %s", timeout)
	}
}
