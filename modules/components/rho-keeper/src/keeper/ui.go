//
// @file ui.go
// @brief The page, and the files it is made of.
//
// There is no layout system and no framework, deliberately. A dashboard this size
// is one page and three files, and a dependency graph that grows with each of
// them is a graph somebody has to keep current. What is here is a template, a
// stylesheet and a script, all embedded in the binary so the thing starts from
// one file and needs nothing installed beside it.
//
// The styling follows the toolchain's own — the same custom properties, the
// same light and dark — so the page looks like part of the project rather than
// something pasted into it. A dark reader reads the same page the same way.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// assets are the files the page is served from, read once at start.
//
// They are embedded rather than read from disk so that the dashboard works from
// wherever it was installed from, which is the whole point of installing a tool
// rather than finding a directory.
//
//go:embed assets
var assets embed.FS

// assetRoot is the embedded tree as the static route serves it: without the
// directory the embed is mounted in, a request for /static/keeper.css would
// look for assets/static/keeper.css and find nothing.
func assetRoot() fs.FS {
	root, err := fs.Sub(assets, "assets")
	if err != nil {
		// The directory is embedded, so it is there; a failure here would be a
		// build that cannot serve a page it carries.
		logf("the page's files cannot be read from this build: %v", err)
		return assets
	}
	return root
}

// renderPage is the dashboard itself.
//
// It is a shell rather than a rendered table: the data comes from the stream
// after the page has loaded, so that a change made in a terminal appears without
// anybody reloading anything. A page rendered once would be correct at the
// moment it was written and wrong from then on, which for a page about a machine
// is the only thing it must not be.
func renderPage(w http.ResponseWriter) {
	data, err := assets.ReadFile("assets/index.html")
	if err != nil {
		http.Error(w, "the dashboard page is missing from this build", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// helpers are the small things a page would otherwise have to be taught.
//
// They are functions rather than methods because Go templates cannot call a
// method on a field they reached through a map, and a page that cannot say "is
// this enabled" in a readable way grows a pile of index expressions instead.
var helpers = template.FuncMap{
	"year": func() string { return strconv.Itoa(time.Now().Year()) },
}

// parseTemplates checks that the embedded page and script are the shape this
// build expects.
//
// It runs once at start rather than on every request, so that a build with a
// broken page says so while it is starting rather than on the first person who
// opens it.
func checkAssets() error {
	for _, name := range []string{"assets/index.html", "assets/keeper.css", "assets/keeper.js"} {
		if _, err := assets.ReadFile(name); err != nil {
			return fmt.Errorf("the dashboard is missing %s: %w", name, err)
		}
	}
	return nil
}

// asOpError unwraps to the network error, so that "address in use" can be told
// from a failure of any other kind.
func asOpError(err error, into **net.OpError) bool {
	return errors.As(err, into)
}

// logf is where the dashboard says something, which is standard error.
//
// Standard output is the page's; anything a person did not ask for goes beside
// it, so that `rho keeper status > file` gets the status and nothing else.
func logf(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "rho keeper: "+strings.TrimRight(format, "\n")+"\n", arguments...)
}
