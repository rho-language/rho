//
// @file main.go
// @brief The rho-doc component.
//
// Renders the documentation a Rho tree carries: the doc-as-code sources each
// module ships, the API reference generated from the Rho sources themselves,
// and whatever the plugins add to the result.
//
// Usage:
//
//	rho doc [path] [flags]
//
//	-out DIR                 where to write the site (default build/docs)
//	-api DIR                 generate the API reference from DIR
//	-api-section NAME        where the reference appears in the sidebar
//	-section NAME            put the pages of a path in this section
//	-title TITLE             the site title
//	-plugins / -no-plugins   run the installed plugins (default: run them)
//	-tree-plugins            also run the plugins the tree being documented carries
//	-list-plugins            list the plugins that extend the generator
//	-strict                  fail when a source has a diagnostic
//
// The component is late bound like every other component: the Rho CLI reaches
// it through the command its manifest publishes, and a project that depends on
// it reaches the same implementation through the docemit port. Both arrive at
// this entry point, so there is one implementation of a page rather than two
// that have to agree.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rho/src/deps"

	"github.com/rho/rho-doc/src/rhodoc"
	rholog "github.com/rho/rho-log/src/logger"
)

// log is the generator's own logger, so a doc build is identifiable in a shared
// log without every line having to say so.
var log = rholog.Default("rho-doc")

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// A ported project calls the same entry point the CLI does, with a
	// fragment request instead of flags.
	if len(args) > 0 && args[0] == "--fragment" {
		return runFragment(args[1:])
	}
	return runCommand(args)
}

func runCommand(args []string) int {
	fs := flag.NewFlagSet("rho doc", flag.ExitOnError)
	out := fs.String("out", "", "where to write the site (default build/docs)")
	apiDir := fs.String("api", "", "generate the API reference from this tree (default: the tree itself)")
	apiSection := fs.String("api-section", "api", "the sidebar section the reference appears in")
	section := fs.String("section", "", "the sidebar section a path's pages appear in")
	title := fs.String("title", "", "the site title (default: the project name)")
	usePlugins := fs.Bool("plugins", true, "run the plugins that extend the build")
	// A flag parser that spells booleans positively makes "-no-plugins" the
	// flag a person actually types when they want a build with no extensions.
	noPlugins := fs.Bool("no-plugins", false, "do not run the plugins that extend the build")
	strict := fs.Bool("strict", false, "fail when a source reports a problem")
	// A plugin is a program, and the tree being documented may have been
	// written by someone the caller does not know, so running the tree's own
	// plugins is a decision the caller makes rather than a default.
	treePlugins := fs.Bool("tree-plugins", false, "run the plugins the tree being documented carries, not only the installed ones")
	listPlugins := fs.Bool("list-plugins", false, "list the plugins that extend the generator and exit")
	// Flags are read wherever they appear, because `rho doc <path> -out <dir>`
	// is what a person types first and a flag parser that stops at the path
	// would silently ignore the rest.
	flagArgs, rest := splitArgs(args, map[string]bool{
		"out": true, "api": true, "api-section": true, "section": true, "title": true,
	})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	// A path is what is being documented; with none the working directory is
	// the tree, which is what a build in a repository means.
	root := "."
	if len(rest) > 0 {
		root = rest[0]
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho doc:", err)
		return 1
	}
	if info, err := os.Stat(absolute); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "rho doc: %s is not a directory\n", root)
		return 2
	}

	if *noPlugins {
		*usePlugins = false
	}
	discovery := rhodoc.Source{Tree: absolute, RhoHome: deps.RhoHome(), TrustTree: *treePlugins}
	plugins := []rhodoc.Plugin{}
	if *usePlugins {
		plugins = discovery.Discover(rhodoc.ComponentName)
	}
	if *listPlugins {
		listPluginsFound(plugins)
		listOtherPlugins(discovery)
		return 0
	}

	options := rhodoc.SiteOptions{
		Title:   *title,
		Out:     *out,
		Project: projectName(absolute),
		Root:    absolute,
		Version: projectVersion(absolute),
	}
	if options.Out == "" {
		options.Out = filepath.Join(absolute, "build", "docs")
	}

	modules, unreadable := rhodoc.DiscoverModules(absolute)
	if *section != "" {
		// A path documented on its own belongs where the caller says it does,
		// which is what makes a single module's pages land in the right place
		// without editing its manifest.
		for index := range modules {
			if strings.HasPrefix(modules[index].Root, absolute) {
				modules[index].Section = *section
			}
		}
	}
	api := rhodoc.ReadApiSet(apiDirOr(absolute, *apiDir), *apiSection)

	site := rhodoc.BuildSiteWithPlugins(modules, api, options, plugins)
	site.RunPlugins(options.Out)

	report(site, unreadable)
	if *strict && (len(site.Diagnostics) > 0 || len(site.Requests) > 0) {
		return 1
	}

	if err := site.Write(options.Out); err != nil {
		fmt.Fprintln(os.Stderr, "rho doc:", err)
		return 1
	}
	log.Info("documentation written", "out", options.Out, "pages", strconv.Itoa(len(site.Pages)))
	fmt.Printf("Wrote %d pages to %s\n", len(site.Pages), options.Out)
	return 0
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                   */
/* -------------------------------------------------------------------------- */

// splitArgs separates the flags from the path a command was given.
//
// A flag that takes a value may be written as -name value or -name=value, so
// the value is consumed as well: without that, `rho doc -out build` would read
// the output directory as the tree to document.
func splitArgs(args []string, takesValue map[string]bool) (flags []string, rest []string) {
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			rest = append(rest, argument)
			continue
		}
		flags = append(flags, argument)
		name := strings.TrimLeft(argument, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if takesValue[name] && index+1 < len(args) {
			index++
			flags = append(flags, args[index])
		}
	}
	return flags, rest
}

// apiDirOr is the tree the API reference is generated from: the one the caller
// named, or the whole tree.
func apiDirOr(absolute, named string) string {
	if named != "" {
		return named
	}
	return absolute
}

// readStdin reads a request body, which is how a ported project passes a source
// that is larger than a command line comfortably carries.
func readStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}

// itoa renders a count, so a ported project reads a number rather than a word.
func itoa(value int) string { return strconv.Itoa(value) }

// listPluginsFound reports the extensions that are available, which is how a
// reader finds out what a documentation build can do.
func listPluginsFound(plugins []rhodoc.Plugin) {
	if len(plugins) == 0 {
		fmt.Println("No plugin extends the documentation generator.")
		return
	}
	fmt.Printf("%d plugin(s) extend the documentation generator:\n", len(plugins))
	for _, plugin := range plugins {
		detail := ""
		if len(plugin.Commands) > 0 {
			hooks := []string{}
			for _, command := range plugin.Commands {
				hook := command.Hook
				if hook == "" {
					hook = command.Name
				}
				hooks = append(hooks, hook)
			}
			detail = " hooks: " + strings.Join(hooks, ", ")
		}
		fmt.Printf("  %s %s%s\n", plugin.Name, plugin.Version, detail)
	}
}

// listOtherPlugins names the plugins that extend some other component.
//
// A plugin is loaded by the component it names, so one written for another
// tool is inert here. Saying so is the difference between a plugin that does
// nothing and a plugin whose author cannot tell why.
func listOtherPlugins(discovery rhodoc.Source) {
	for _, plugin := range discovery.All() {
		if plugin.Extends == rhodoc.ComponentName {
			continue
		}
		fmt.Printf("  %s %s extends %s, so this generator does not load it\n",
			plugin.Name, plugin.Version, plugin.Extends)
	}
}

// report prints what the build found, so a documentation build says as much
// about its own health as the pages it wrote.
func report(site *rhodoc.Site, unreadable []string) {
	for _, path := range unreadable {
		fmt.Fprintf(os.Stderr, "rho doc: skipped %s: no readable module manifest\n", path)
	}
	for _, diagnostic := range site.Diagnostics {
		fmt.Fprintf(os.Stderr, "rho doc: %s\n", diagnostic)
	}
	for _, request := range site.Requests {
		fmt.Fprintf(os.Stderr, "rho doc: %s\n", request)
	}
}

// projectName is the name of the root module of a tree, which is what a site
// calls itself when the caller names nothing.
func projectName(root string) string {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil {
		return filepath.Base(root)
	}
	return manifest.ModuleName
}

// projectVersion is the version of the root module of a tree.
func projectVersion(root string) string {
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil || manifest == nil {
		return ""
	}
	return manifest.Version
}
