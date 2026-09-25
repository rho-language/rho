//
// @file emit_go_server.go
// @brief Go HTTP server artifacts for generated web projects.
//
// A Rho application compiled with the Go backend can target a long-running
// HTTP service instead of a command line program. This file renders the
// server wiring: configuration from the environment, a health endpoint, JSON
// helpers, and a route registration seam where Rho handlers are attached.
//
// The generated server uses only the Go standard library, so a freshly
// generated project builds and runs with no module downloads.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

import (
	"fmt"
	"strings"

	"rho/src/ir"
)

// DefaultServerAddr is used when RHO_HTTP_ADDR is unset.
const DefaultServerAddr = ":8080"

// StaticDir is the asset folder the generated server serves at /static/.
const StaticDir = "static"

// EmitGoServerEntrypoint returns main.go for a server-kind Go target.
func EmitGoServerEntrypoint(m ir.ModuleIR) string {
	var b strings.Builder
	b.WriteString(goBanner(m, "main.go", "Generated entry point for a Rho web application."))
	b.WriteString("package main\n\nimport (\n\t\"fmt\"\n\t\"os\"\n)\n\n")
	b.WriteString("// main starts the generated Rho HTTP service.\nfunc main() {\n")
	if hasGoInit(m) {
		fmt.Fprintf(&b, "\t%s()\n", AppInitName)
	}
	b.WriteString("\taddr := serverAddr()\n")
	fmt.Fprintf(&b, "\tfmt.Printf(\"rho %s listening on %%s\\n\", addr)\n", m.Name)
	b.WriteString("\tif err := StartServer(addr); err != nil {\n")
	b.WriteString("\t\tfmt.Fprintln(os.Stderr, \"rho server:\", err)\n")
	b.WriteString("\t\tos.Exit(1)\n")
	b.WriteString("\t}\n}\n")
	return b.String()
}

// EmitGoServer returns server.go for a server-kind Go target.
func EmitGoServer(m ir.ModuleIR) string {
	var b strings.Builder
	b.WriteString(goBanner(m, "server.go", "Generated HTTP wiring for a Rho web application."))
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"log\"\n")
	b.WriteString("\t\"net/http\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"os/signal\"\n")
	b.WriteString("\t\"syscall\"\n")
	b.WriteString("\t\"time\"\n")
	b.WriteString(")\n\n")

	// The asset folder the descriptor creates for this target.
	fmt.Fprintf(&b, "// staticDir is the asset directory served under /static/.\nconst staticDir = %q\n\n", StaticDir)

	b.WriteString("// serverAddr returns the listen address, overridable with RHO_HTTP_ADDR.\n")
	b.WriteString("func serverAddr() string {\n")
	fmt.Fprintf(&b, "\tif addr := os.Getenv(\"RHO_HTTP_ADDR\"); addr != \"\" {\n\t\treturn addr\n\t}\n")
	fmt.Fprintf(&b, "\treturn %q\n}\n\n", DefaultServerAddr)

	b.WriteString("// StartServer runs the HTTP service until SIGINT or SIGTERM.\n")
	b.WriteString("func StartServer(addr string) error {\n")
	b.WriteString("\tmux := http.NewServeMux()\n")
	b.WriteString("\tregisterRoutes(mux)\n\n")
	b.WriteString("\tserver := &http.Server{\n")
	fmt.Fprintf(&b, "\t\tAddr:              addr,\n")
	b.WriteString("\t\tHandler:           withLogging(mux),\n")
	b.WriteString("\t\tReadHeaderTimeout: 10 * time.Second,\n")
	b.WriteString("\t}\n\n")
	b.WriteString("\tstop := make(chan os.Signal, 1)\n")
	b.WriteString("\tsignal.Notify(stop, os.Interrupt, syscall.SIGTERM)\n")
	b.WriteString("\tdefer signal.Stop(stop)\n\n")
	b.WriteString("\terrCh := make(chan error, 1)\n")
	b.WriteString("\tgo func() {\n")
	b.WriteString("\t\tif err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {\n")
	b.WriteString("\t\t\terrCh <- err\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}()\n\n")
	b.WriteString("\tselect {\n")
	b.WriteString("\tcase err := <-errCh:\n")
	b.WriteString("\t\treturn err\n")
	b.WriteString("\tcase <-stop:\n")
	b.WriteString("\t\tctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)\n")
	b.WriteString("\t\tdefer cancel()\n")
	b.WriteString("\t\treturn server.Shutdown(ctx)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n\n")

	b.WriteString("// registerRoutes attaches the generated handlers to a mux.\n")
	b.WriteString("// Rho functions exposed over HTTP are wired in here.\n")
	b.WriteString("func registerRoutes(mux *http.ServeMux) {\n")
	b.WriteString("\tmux.HandleFunc(\"/healthz\", func(w http.ResponseWriter, r *http.Request) {\n")
	b.WriteString("\t\twriteJSON(w, http.StatusOK, map[string]string{\"status\": \"ok\"})\n")
	b.WriteString("\t})\n")
	b.WriteString("\n")
	// The descriptor creates static/, so the generated project ships with the
	// asset folder the handler serves.
	b.WriteString("\tif info, err := os.Stat(staticDir); err == nil && info.IsDir() {\n")
	b.WriteString("\t\tmux.Handle(\"/static/\", http.StripPrefix(\"/static/\", http.FileServer(http.Dir(staticDir))))\n")
	b.WriteString("\t}\n")
	b.WriteString("\n")
	b.WriteString("\t// Add project routes below this line.\n")
	b.WriteString("}\n\n")

	b.WriteString("// withLogging records one line per request.\n")
	b.WriteString("func withLogging(next http.Handler) http.Handler {\n")
	b.WriteString("\treturn http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n")
	b.WriteString("\t\tstarted := time.Now()\n")
	b.WriteString("\t\tnext.ServeHTTP(w, r)\n")
	b.WriteString("\t\tlog.Printf(\"%s %s %s\", r.Method, r.URL.Path, time.Since(started))\n")
	b.WriteString("\t})\n")
	b.WriteString("}\n\n")

	b.WriteString("// writeJSON sends a JSON response with the given status code.\n")
	b.WriteString("func writeJSON(w http.ResponseWriter, status int, payload any) {\n")
	b.WriteString("\tw.Header().Set(\"Content-Type\", \"application/json\")\n")
	b.WriteString("\tw.WriteHeader(status)\n")
	b.WriteString("\tif err := json.NewEncoder(w).Encode(payload); err != nil {\n")
	b.WriteString("\t\tlog.Printf(\"write response: %v\", err)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")

	return b.String()
}

// EmitGoModWithRequirements returns go.mod, adding the requirements the
// descriptor asks for.
func EmitGoModWithRequirements(m ir.ModuleIR, modulePath string, requires []string) string {
	path := modulePath
	if path == "" {
		path = goModulePath(m.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo 1.22\n", path)
	if len(requires) == 0 {
		return b.String()
	}
	b.WriteString("\nrequire (\n")
	for _, requirement := range requires {
		fmt.Fprintf(&b, "\t%s\n", requirement)
	}
	b.WriteString(")\n")
	return b.String()
}
