//
// @file plugins.go
// @brief Running the plugins that extend a component.
//
// A plugin is a module that answers the hooks of a component. It is not a thing
// in itself: a run of the component it names is the only thing that ever calls
// it, and only the plugins that name a component are loaded by it. The
// declaration lives in the plugin's manifest:
//
//	extends: rho-test
//
//	hooks:
//	  - run
//
//	commands:
//	  - name: junit
//	    run: rho-test-junit
//	    hook: run
//
// A command is an executable. The runner writes a request to its standard input
// as JSON and reads the answer from its standard output as JSON:
//
//	{"version":1,"hook":"run","command":"junit","component":"rho-test",
//	 "payload":{...}}
//
//	{"version":1,"payload":{...},"diagnostics":[{"level":"warning","message":"..."}]}
//
// The envelope is the same for every component, so one mechanism serves them all;
// the payload is whatever the component needs to show a plugin, which is why it
// is opaque here and typed on the component's side.
//
// A plugin that fails is reported and skipped. One broken extension must not cost
// a reader the work of everything else, which is why a failure is a diagnostic
// and not an error.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package plugins

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
// declares the version it was written against, so a component and a plugin from
// different releases can say so instead of misreading each other.
const ProtocolVersion = 1

// Request is what a component sends to a plugin command.
type Request struct {
	Version int `json:"version"`
	// Hook is the stage being run, and Command the one being answered.
	Hook      string `json:"hook"`
	Command   string `json:"command"`
	Component string `json:"component"`
	// Payload is the component's own request body, opaque here and typed by the
	// component: test results for a test run, pages for a documentation build.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Response is what a plugin answers.
type Response struct {
	Version int             `json:"version,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	// Pages and Replacements carry what a documentation build needs of a
	// plugin. A component that has no pages ignores them, and a plugin for such
	// a component leaves them out.
	Pages        []Page            `json:"pages,omitempty"`
	Replacements map[string]string `json:"replacements,omitempty"`
	Diagnostics  []Diagnostic      `json:"diagnostics,omitempty"`
}

// Page is one page a plugin adds, named for the component that shows it.
type Page struct {
	Target  string `json:"target"`
	Title   string `json:"title"`
	Href    string `json:"href,omitempty"`
	Section string `json:"section,omitempty"`
	HTML    string `json:"html,omitempty"`
	Plain   string `json:"plain,omitempty"`
}

// Diagnostic is a problem a plugin reports about what it was given, rather than
// about itself.
type Diagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (d Diagnostic) String() string {
	if d.Level == "" {
		return d.Message
	}
	return d.Level + ": " + d.Message
}

/* -------------------------------------------------------------------------- */
/*  Discovery                                                                 */
/* -------------------------------------------------------------------------- */

// Plugin is a discovered extension.
type Plugin struct {
	// Name is the module name.
	Name string
	// Version is the version it was found at.
	Version string
	// Root is the module root, which a command path is relative to.
	Root string
	// Extends is the component this plugin plugs into.
	Extends string
	// Implements are the contracts of that component this plugin fills, which
	// is what says what the plugin is rather than which hooks it happens to
	// answer. Two plugins may answer the same hook and be unrelated things, and
	// this is what tells them apart when someone decides whether to run one.
	Implements []string
	// Hooks are the hooks of that component this plugin answers, and the
	// component offers every one of them.
	Hooks []string
	// Commands are the executables a run goes through this plugin.
	Commands []Command
}

// Command is one executable a plugin answers a hook with.
type Command struct {
	Name string
	Run  string
	Hook string
}

// Stage is the hook a command answers: the one it names, or its own name when a
// one-hook plugin lets it stand for the stage.
func (c Command) Stage() string {
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

/* -------------------------------------------------------------------------- */
/*  Where to look                                                             */
/* -------------------------------------------------------------------------- */

// Source is where a component looks for the plugins it runs.
type Source struct {
	// Tree is the tree the component is working in, which may carry plugins of
	// its own.
	Tree string
	// RhoHome is the installation whose plugins are trusted, or "" for none.
	RhoHome string
	// TrustTree runs the plugins the tree carries.
	//
	// A plugin is a program, and running one is running code the tree supplied.
	// An installed plugin was published by someone the user chose to install; a
	// plugin found in the tree came from whoever wrote that tree, which for a
	// repository that was just cloned is a stranger. So the tree's own plugins
	// run only when the caller asks for them.
	TrustTree bool
}

// Discover reads the plugins attached to one component.
//
// A plugin that is installed and not active is not here: it is available, and
// this is the set that runs. A component that needs one of them and cannot find
// it says so, rather than carrying on with fewer than it asked for — a build
// that silently dropped a formatter would produce pages nobody asked for the
// shape of.
func (s Source) Discover(component string) []Plugin {
	found := []Plugin{}
	for _, plugin := range s.All() {
		if plugin.Extends == component {
			found = append(found, plugin)
		}
	}
	return found
}

// Contract is what a component needs a plugin to be.
//
// The name is the contract and One says how many plugins may fill it. Hooks say
// how a plugin is called; this says what it is for, which is the question a
// person enabling one is actually asking.
type Contract struct {
	Name        string
	Component   string
	Description string
	One         bool
	Hooks       []string
}

// Contracts reads what a component needs a plugin to be.
//
// A component that declares none has no contract model, and every plugin
// extending it is offered to every run of it — which is what a component whose
// one plugin is the only kind there is wants.
func (s Source) Contracts(component string) []Contract {
	home := s.RhoHome
	if home == "" {
		home = deps.RhoHome()
	}
	if home == "" {
		return nil
	}
	root := deps.InstalledComponentRoot(component)
	if root == "" {
		return nil
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.Provides == nil {
		return nil
	}
	out := make([]Contract, 0, len(manifest.Provides.Contracts))
	for _, contract := range manifest.Provides.Contracts {
		out = append(out, Contract{
			Name:        contract.Name,
			Component:   component,
			Description: contract.Description,
			One:         contract.One,
			Hooks:       contract.Hooks,
		})
	}
	return out
}

// Fills reports whether a plugin fills a contract, and which.
//
// A plugin that names no contract is offered to every run of the component,
// which is what a plugin written before contracts existed does and is why a
// component that needs to tell its plugins apart has to say so.
func (p Plugin) Fills(contract string) bool {
	if contract == "" || len(p.Implements) == 0 {
		return true
	}
	for _, offered := range p.Implements {
		if offered == contract {
			return true
		}
	}
	return false
}

// FillsAny reports whether a plugin fills at least one of the contracts, which
// is how a component asks which of its plugins it could run.
func (p Plugin) FillsAny(contracts []Contract) bool {
	if len(contracts) == 0 {
		return true
	}
	for _, contract := range contracts {
		if p.Fills(contract.Name) {
			return true
		}
	}
	return false
}

// All reads every plugin that may be run, whatever component each one extends.
// It is how a component reports the plugins that belong to someone else, which a
// reader otherwise cannot account for.
func (s Source) All() []Plugin {
	found := []Plugin{}
	seen := map[string]bool{}
	// An installed plugin is preferred over one in the working tree of the same
	// name, so a published version is the one a run uses.
	for _, name := range deps.InstalledPluginNames() {
		if root := deps.InstalledPluginRoot(name); root != "" {
			if plugin, ok := readPlugin(root, seen); ok {
				found = append(found, plugin)
			}
		}
	}
	if s.TrustTree && s.Tree != "" {
		walkTree(s.Tree, func(root string) {
			if plugin, ok := readPlugin(root, seen); ok {
				found = append(found, plugin)
			}
		})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found
}

// walkTree visits every directory that holds a module manifest, so the plugins a
// tree carries are found the way the modules it carries are.
func walkTree(root string, visit func(root string)) {
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
			visit(filepath.Dir(path))
		}
		return nil
	})
}

// readPlugin reads one plugin, keeping the first version found of a name so an
// installed plugin wins over one in the working tree.
func readPlugin(root string, seen map[string]bool) (Plugin, bool) {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil || manifest.ModuleType != "plugin" {
		return Plugin{}, false
	}
	extension := manifest.Extends
	if extension == nil || extension.Component == "" || seen[manifest.ModuleName] {
		return Plugin{}, false
	}
	seen[manifest.ModuleName] = true

	plugin := Plugin{
		Name:       manifest.ModuleName,
		Version:    manifest.Version,
		Root:       root,
		Extends:    extension.Component,
		Implements: extension.Implements,
		Hooks:      extension.Hooks,
	}
	for _, command := range extension.Commands {
		plugin.Commands = append(plugin.Commands, Command{Name: command.Name, Run: command.Run, Hook: command.Hook})
	}
	return plugin, true
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                   */
/* -------------------------------------------------------------------------- */

// Result is what running the plugins produced.
type Result struct {
	// Payload is what the plugins answered, concatenated in the order they ran.
	// A component decides what to do with it.
	Payload []json.RawMessage
	// Pages and Replacements are what a documentation build takes of a plugin.
	Pages        []Page
	Replacements map[string]string
	// Diagnostics is what the plugins reported, about themselves or about what
	// they were given.
	Diagnostics []Diagnostic
}

// RunHooks runs every command the plugins answer a hook with.
//
// The request is the component's own payload for that hook. A command that fails
// is recorded and the run continues: the work of everything else still matters
// more than one extension.
func RunHooks(found []Plugin, component, hook string, payload any) Result {
	result := Result{Replacements: map[string]string{}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Level: "warning", Message: fmt.Sprintf("the %s request could not be encoded: %v", hook, err)})
		return result
	}

	for _, plugin := range found {
		// A plugin answers only the hooks it declared, and only the ones the
		// component offers: the two lists were checked against each other when
		// the module was validated, and are checked again here because a run
		// must not depend on that having happened.
		if !plugin.Answers(hook) {
			continue
		}
		for _, command := range plugin.Commands {
			if command.Stage() != hook {
				continue
			}
			response, err := runCommand(plugin, command, Request{
				Version:   ProtocolVersion,
				Hook:      hook,
				Command:   command.Name,
				Component: component,
				Payload:   encoded,
			})
			if err != nil {
				result.Diagnostics = append(result.Diagnostics, Diagnostic{
					Level:   "warning",
					Message: fmt.Sprintf("plugin %s command %s failed: %v", plugin.Name, command.Name, err),
				})
				continue
			}
			merge(&result, response)
		}
	}
	return result
}

// merge folds what a plugin answered into the run.
func merge(result *Result, response Response) {
	if len(response.Payload) > 0 {
		result.Payload = append(result.Payload, response.Payload)
	}
	result.Pages = append(result.Pages, response.Pages...)
	for target, body := range response.Replacements {
		if result.Replacements == nil {
			result.Replacements = map[string]string{}
		}
		result.Replacements[target] = body
	}
	result.Diagnostics = append(result.Diagnostics, response.Diagnostics...)
}

// runCommand runs one plugin executable and reads its response.
func runCommand(plugin Plugin, command Command, request Request) (Response, error) {
	response := Response{}
	// A plugin names a program and its arguments in one field, because a
	// manifest has no way to say "this program, these arguments" otherwise.
	argv := splitCommand(command.Run)
	if len(argv) == 0 {
		return response, fmt.Errorf("command %q names nothing to run", command.Name)
	}
	program, err := executable(plugin, argv[0])
	if err != nil {
		return response, err
	}
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return response, err
	}

	var stdout, stderr bytes.Buffer
	process := exec.Command(program, argv[1:]...)
	process.Dir = plugin.Root
	process.Stdin = bytes.NewReader(payload)
	process.Stdout = &stdout
	process.Stderr = &stderr
	process.Env = append(os.Environ(), "RHO_PLUGIN="+plugin.Name, "RHO_PLUGIN_HOOK="+request.Hook)

	if err := runBounded(process, Timeout); err != nil {
		return response, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return response, fmt.Errorf("cannot read the response: %w", err)
	}
	return response, nil
}

// executable resolves the program a command runs.
//
// A plugin runs a program it carries, which is the binary its manifest
// publishes and which the package installs beside it, or one found on PATH. A
// path that climbs out of the plugin with "..", and an absolute path, are
// refused: a plugin is a module with a directory of its own, and a program named
// outside it is either a mistake or an attempt to run something the plugin does
// not carry.
func executable(plugin Plugin, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("plugin %s runs a command with no program", plugin.Name)
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("plugin %s runs %s, which is an absolute path; a plugin runs a program it carries or one on PATH", plugin.Name, name)
	}
	if strings.Contains(filepath.ToSlash(name), "..") {
		return "", fmt.Errorf("plugin %s runs %s, which leaves the plugin directory", plugin.Name, name)
	}
	if !strings.ContainsRune(name, filepath.Separator) && !strings.ContainsRune(name, '/') {
		if carried := filepath.Join(plugin.Root, name); fileExists(carried) {
			return carried, nil
		}
		resolved, err := exec.LookPath(name)
		if err != nil {
			return "", fmt.Errorf("plugin %s runs %s, which it does not carry and which is not on PATH: %w", plugin.Name, name, err)
		}
		return resolved, nil
	}
	resolved := filepath.Join(plugin.Root, name)
	if !fileExists(resolved) {
		return "", fmt.Errorf("plugin %s runs %s, which it does not carry", plugin.Name, name)
	}
	return resolved, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// splitCommand reads a run field into a program and its arguments.
//
// A double-quoted argument is kept whole, so a path with a space in it works,
// and a plugin that needs an argument with a space in it can have one.
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

// Timeout is how long a plugin command may take before the run carries on
// without it. A plugin that hangs must not hang the tool that called it.
const Timeout = 30 * time.Second

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
		// run open past its deadline.
		if process.Process != nil {
			_ = process.Process.Kill()
		}
		<-done
		return fmt.Errorf("timed out after %s", timeout)
	}
}
