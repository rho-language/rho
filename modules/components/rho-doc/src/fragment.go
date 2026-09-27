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

	"github.com/rho/rho-doc/src/rhodoc"
)

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
		modules, _ := rhodoc.DiscoverModules(args[1])
		site := rhodoc.BuildSite(modules, rhodoc.ReadApiSet(args[1], "api"), rhodoc.SiteOptions{Title: title})
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
	case "write":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "rho doc --fragment write <root> <out> [title]")
			return 2
		}
		title := "Documentation"
		if len(args) > 3 {
			title = args[3]
		}
		modules, _ := rhodoc.DiscoverModules(args[1])
		site := rhodoc.BuildSite(modules, rhodoc.ReadApiSet(args[1], "api"), rhodoc.SiteOptions{Title: title})
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
