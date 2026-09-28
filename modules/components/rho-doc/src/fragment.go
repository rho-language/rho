//
// @file fragment.go
// @brief Answering a port's request.
//
// A generated project reaches this component through the docemit port. The port
// calls the same binary the CLI does, with a fragment request, so the page a
// service renders and the page a build writes are produced by one
// implementation: two generators would be two definitions of a page, and they
// would disagree.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rho/rho-doc/src/rhodoc"
	"rho/src/deps"
)

// fragmentSite builds a site the way a build does, plugins included.
//
// A fragment is how a generated project reaches this component, and a project
// that documents a published module is documenting a tree it did not write. A
// plugin that adds pages to a site therefore has to work here as well as it
// does from the command line, or a page a build would write would be missing
// from the site a service serves.
//
// Only the plugins the toolchain installed are run, never the ones the tree
// carries. The tree here is an unpacked package that arrived from a registry,
// so it came from someone else: running code out of a freshly downloaded
// package is running a stranger's program, and a documentation build is not a
// reason to do that.
func fragmentSite(root, title, home, homeTitle string) (*rhodoc.Site, []rhodoc.Plugin) {
	modules, _ := rhodoc.DiscoverModules(root)
	found := []rhodoc.Plugin{}
	if rhodocHome := deps.RhoHome(); rhodocHome != "" {
		found = rhodoc.Source{RhoHome: rhodocHome}.Discover(rhodoc.ComponentName)
	}
	site := rhodoc.BuildSiteWithPlugins(modules, rhodoc.ReadApiSet(root, "api"), rhodoc.SiteOptions{
		Title:     title,
		Root:      root,
		Home:      home,
		HomeTitle: homeTitle,
	}, found)
	if len(found) > 0 {
		// A build that writes a site runs the plugin pass before it writes, so
		// a page a plugin added is a page on disk like any other.
		site.RunPlugins(root)
		// A plugin that failed is reported rather than swallowed: a page
		// missing from a served site needs a reason in the log, or an absence
		// is all anyone sees.
		for _, diagnostic := range site.Diagnostics {
			fmt.Fprintf(os.Stderr, "rho doc: %s\n", diagnostic)
		}
	}
	return site, found
}

// homeAddress reads the address a site is published under from a fragment's
// arguments, so a caller that serves the site can say where it is and the
// generator can write a link back to it.
//
// The arguments are the two flags rather than more positionals, because a
// fragment's arguments are positional and the address is optional: a site built
// on its own passes neither.
func homeAddress(args []string) (string, string, error) {
	home, title := "", ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--home":
			if index+1 >= len(args) {
				return "", "", fmt.Errorf("--home needs an address")
			}
			home = args[index+1]
			index++
		case "--home-title":
			if index+1 >= len(args) {
				return "", "", fmt.Errorf("--home-title needs a name")
			}
			title = args[index+1]
			index++
		default:
			return "", "", fmt.Errorf("unknown argument %q", args[index])
		}
	}
	return home, title, nil
}

func mainFragmentUsage() string {
	return "rho doc --fragment <render|site|api|diagnostics|write> [arguments]"
}

func runFragment(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, mainFragmentUsage())
		return 2
	}
	switch args[0] {
	case "render":
		// The source arrives on standard input, because a rendered page can be
		// larger than a command line comfortably carries.
		source, err := readStdin()
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho doc:", err)
			return 1
		}
		path := "page.md"
		if len(args) > 1 {
			path = args[1]
		}
		document := rhodoc.Parse(path, string(source))
		fmt.Print(rhodoc.RenderPage(document).Body)
		return 0
	case "site":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "rho doc --fragment site <root> [title]")
			return 2
		}
		title := "Documentation"
		if len(args) > 2 {
			title = args[2]
		}
		site, _ := fragmentSite(args[1], title, "", "")
		fmt.Print(rhodoc.SiteListing(site))
		return 0
	case "api":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "rho doc --fragment api <root>")
			return 2
		}
		set := rhodoc.ReadApiSet(args[1], "api")
		fmt.Print(rhodoc.ApiListing(set))
		return 0
	case "diagnostics":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "rho doc --fragment diagnostics <root>")
			return 2
		}
		modules, unreadable := rhodoc.DiscoverModules(args[1])
		site := rhodoc.BuildSite(modules, rhodoc.ReadApiSet(args[1], "api"), rhodoc.SiteOptions{})
		fmt.Print(rhodoc.DiagnosticListing(site, unreadable))
		return 0
	case "publish-docs":
		// Documentation for a module a registry has just received.
		//
		// A published package carries the manifest, the README and the
		// sources, and nothing else: the documentation is generated from what
		// is there, which is the interface the compiler reads and the prose the
		// module wrote about itself. The registry is the caller, so this is the
		// one entry point that knows a package rather than a working tree.
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "rho doc --fragment publish-docs <source> <out> [title] [--home URL] [--home-title TEXT]")
			return 2
		}
		source, out, title := args[1], args[2], "Documentation"
		rest := args[3:]
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			title = rest[0]
			rest = rest[1:]
		}
		home, homeTitle, err := homeAddress(rest)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho doc:", err)
			return 2
		}
		if info, err := os.Stat(source); err != nil || !info.IsDir() {
			fmt.Fprintf(os.Stderr, "rho doc: %s is not a directory\n", source)
			return 2
		}
		modules, unreadable := rhodoc.DiscoverModulesWith(source, rhodoc.SourceOptions{Readme: true})
		site := rhodoc.BuildSite(modules, rhodoc.ReadApiSet(source, "api"), rhodoc.SiteOptions{
			Title:     title,
			Out:       out,
			Project:   projectName(source),
			Version:   projectVersion(source),
			Home:      home,
			HomeTitle: homeTitle,
		})
		if err := site.Write(out); err != nil {
			fmt.Fprintln(os.Stderr, "rho doc:", err)
			return 1
		}
		// The problems travel with the count, so a registry can record what it
		// could not read rather than reporting a build that looks clean.
		for _, path := range unreadable {
			fmt.Fprintf(os.Stderr, "rho doc: skipped %s: no readable module manifest\n", path)
		}
		for _, diagnostic := range site.Diagnostics {
			fmt.Fprintf(os.Stderr, "rho doc: %s\n", diagnostic)
		}
		fmt.Fprint(os.Stdout, itoa(len(site.Pages)))
		return 0
	case "write":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "rho doc --fragment write <root> <out> [title] [--home URL] [--home-title TEXT]")
			return 2
		}
		title := "Documentation"
		rest := args[3:]
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			title = rest[0]
			rest = rest[1:]
		}
		home, homeTitle, err := homeAddress(rest)
		if err != nil {
			fmt.Fprintln(os.Stderr, "rho doc:", err)
			return 2
		}
		site, _ := fragmentSite(args[1], title, home, homeTitle)
		if err := site.Write(args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "rho doc:", err)
			return 1
		}
		fmt.Fprint(os.Stdout, itoa(len(site.Pages)))
		return 0
	default:
		fmt.Fprintln(os.Stderr, mainFragmentUsage())
		return 2
	}
}
