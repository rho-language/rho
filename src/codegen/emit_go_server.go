//
// @file emit_go_server.go
// @brief Go HTTP server artifacts for generated web projects.
//
// A Rho application compiled with the Go backend can target a long-running
// HTTP service instead of a command line program. This file renders the
// protocol half of that service: the Request and Response a handler works
// with, the lifecycle around a listener, and the small JSON helpers a handler
// reaches for.
//
// How requests are dispatched is a target's decision, not this file's. A
// target that names no router gets the matcher below, which builds with any Go
// toolchain. A target that names one gets the routing port, which supplies the
// router, the typed parameter binding and the middleware pipeline, and this
// file keeps to the protocol and the lifecycle. Both leave the same project:
// handlers that read a request and write a response, and nothing to wire up.
//
// The generated server uses only the Go standard library, so a freshly
// generated project builds and runs with no module downloads.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"strings"

	"rho/src/ir"
	"rho/src/types"
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

// goServerImports are the imports a generated server.go may need. An import is
// kept only when the file refers to it, so a project that routes through a port
// is not handed an import the compiler refuses.
var goServerImports = []importSpec{
	{Path: `"context"`},
	{Path: `"io"`},
	{Path: `"log"`},
	{Path: `"net/http"`},
	{Path: `"net/url"`},
	{Path: `"os"`},
	{Path: `"os/signal"`},
	{Path: `"strings"`},
	{Path: `"syscall"`},
	{Path: `"time"`},
	// Aliased: the Rho-facing json(...) helper takes the plain name.
	{Alias: "jsonenc", Path: `"encoding/json"`},
}

// importSpec is one import of a generated file: where it comes from, and the
// name the generated source calls it by.
type importSpec struct {
	// Alias is the name the import is bound to, empty for the package's own.
	Alias string
	// Path is the quoted module path the import block prints.
	Path string
}

// Ident is the name the generated source refers to the import by.
func (s importSpec) Ident() string {
	if s.Alias != "" {
		return s.Alias
	}
	path := strings.Trim(s.Path, `"`)
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

// Line is what the import block prints for this import.
func (s importSpec) Line() string {
	if s.Alias == "" {
		return s.Path
	}
	return s.Alias + " " + s.Path
}

// EmitGoServer returns server.go for a server-kind Go target.
func EmitGoServer(m ir.ModuleIR, cfg ServerConfig) string {
	banner := goBanner(m, "server.go", "Generated HTTP wiring for a Rho web application.")

	var b strings.Builder
	b.WriteString(banner)
	b.WriteString("package main\n\n")

	// The asset folder the descriptor creates for this target.
	fmt.Fprintf(&b, "// staticDir is the asset directory served under /static/.\nconst staticDir = %q\n\n", StaticDir)

	// The Request and Response types shared with the Rho handlers.
	b.WriteString(goRequestType(cfg))
	b.WriteString(goResponseType())
	b.WriteString(goRequestHelpers(cfg))
	b.WriteString(goServerLifecycle(cfg))

	// The handlers themselves are the same whatever dispatches them. Only the
	// binding is a target's decision, so only that is a port's.
	if cfg.Routed() {
		b.WriteString(goRouteHandlers(m))
	} else {
		b.WriteString(goRegisterRoutes(m))
		b.WriteString(goRouteHandlers(m))
	}

	b.WriteString(`// writeJSON sends a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := jsonenc.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write response: %v", err)
	}
}

// withLogging records one line per request.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started))
	})
}

`)

	// The banner sits above the package clause, so it is written first and the
	// import block is spliced in underneath it rather than appended.
	source := strings.TrimPrefix(strings.TrimPrefix(b.String(), banner), "package main\n\n")
	used := usedImports(source, goServerImports)
	if len(used) == 0 {
		return banner + "package main\n\n" + source
	}
	var block strings.Builder
	block.WriteString("import (\n")
	for _, spec := range used {
		fmt.Fprintf(&block, "\t%s\n", spec.Line())
	}
	block.WriteString(")\n\n")
	return banner + "package main\n\n" + block.String() + source
}

// goServerLifecycle renders the listener: where it listens, how it stops, and
// which handler it serves.
func goServerLifecycle(cfg ServerConfig) string {
	var b strings.Builder
	b.WriteString("// serverAddr returns the listen address, overridable with RHO_HTTP_ADDR.\n")
	b.WriteString("func serverAddr() string {\n")
	b.WriteString("\tif addr := os.Getenv(\"RHO_HTTP_ADDR\"); addr != \"\" {\n\t\treturn addr\n\t}\n")
	fmt.Fprintf(&b, "\treturn %q\n}\n\n", DefaultServerAddr)

	b.WriteString("// StartServer runs the HTTP service until SIGINT or SIGTERM.\n")
	b.WriteString("func StartServer(addr string) error {\n")
	if cfg.Routed() {
		b.WriteString("\t// The router, the asset folder and the middleware pipeline are the\n")
		b.WriteString("\t// port's, assembled in one step.\n")
		b.WriteString("\thandler, err := newRouter()\n\tif err != nil {\n\t\treturn err\n\t}\n\n")
		b.WriteString("\tserver := &http.Server{\n")
		b.WriteString("\t\tAddr:              addr,\n")
		b.WriteString("\t\tHandler:           withLogging(handler),\n")
		b.WriteString("\t\tReadHeaderTimeout: 10 * time.Second,\n")
		b.WriteString("\t}\n\n")
	} else {
		b.WriteString("\trouter := &router{}\n")
		b.WriteString("\tregisterRoutes(router)\n")
		b.WriteString("\tmux := http.NewServeMux()\n")
		b.WriteString("\tmux.HandleFunc(\"/\", router.serve)\n\n")
		b.WriteString("\tserver := &http.Server{\n")
		b.WriteString("\t\tAddr:              addr,\n")
		b.WriteString("\t\tHandler:           withLogging(mux),\n")
		b.WriteString("\t\tReadHeaderTimeout: 10 * time.Second,\n")
		b.WriteString("\t}\n\n")
	}
	b.WriteString(goServerShutdown())
	return b.String()
}

// goServerShutdown renders the part that stops the service the same way in both
// modes: a signal, a grace period, and a close.
func goServerShutdown() string {
	return `	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}
}

`
}

// goRequestType returns the Request type a Rho handler receives.
func goRequestType(cfg ServerConfig) string {
	var b strings.Builder
	b.WriteString("// Request is what a Rho handler receives for each HTTP request.\n")
	b.WriteString("type Request struct {\n")
	b.WriteString("\tMethod string\n")
	b.WriteString("\tPath   string\n")
	b.WriteString("\tQuery  string\n")
	b.WriteString("\tBody   string\n")
	b.WriteString("\tAuth   string\n")
	b.WriteString("\tParams map[string]string\n")
	if cfg.Routed() {
		// Both are reached through the request rather than beside it. A Rho
		// handler receives the request by value, and a value it keeps is a
		// copy; a map and a pointer are shared by a copy, so a header a header
		// lookup filled and a failure a conversion recorded both outlive the
		// accessor that touched them.
		b.WriteString("\t// head is the request's headers, kept as net/http canonicalised them.\n")
		b.WriteString("\thead map[string]string\n")
		b.WriteString("\t// bind carries what a failed conversion left, so a handler asks once\n")
		b.WriteString("\t// whether the binding held rather than checking every value.\n")
		b.WriteString("\tbind *bindState\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// Param returns a path wildcard value, or \"\" when absent.\n")
	b.WriteString("func (r *Request) Param(name string) string {\n")
	b.WriteString("\tif r.Params == nil {\n\t\treturn \"\"\n\t}\n")
	b.WriteString("\treturn r.Params[name]\n}\n\n")
	return b.String()
}

// goResponseType returns the Response type a Rho handler returns.
func goResponseType() string {
	var b strings.Builder
	b.WriteString("// Response is what a Rho handler returns for each HTTP request.\n")
	b.WriteString("type Response struct {\n")
	b.WriteString("\tStatus      uint32\n")
	b.WriteString("\tBody        string\n")
	b.WriteString("\tcontentType string\n")
	b.WriteString("}\n\n")
	b.WriteString("// write sends the response with its content type.\n")
	b.WriteString("func (resp Response) write(w http.ResponseWriter) {\n")
	b.WriteString("\tcontentType := resp.contentType\n")
	b.WriteString("\tif contentType == \"\" {\n\t\tcontentType = \"text/plain; charset=utf-8\"\n\t}\n")
	b.WriteString("\tw.Header().Set(\"Content-Type\", contentType)\n")
	b.WriteString("\tw.WriteHeader(int(resp.Status))\n")
	b.WriteString("\t_, _ = w.Write([]byte(resp.Body))\n}\n\n")
	return b.String()
}

// goRequestHelpers returns the helpers used to decode an incoming request.
func goRequestHelpers(cfg ServerConfig) string {
	if cfg.Routed() {
		return goRoutedRequestHelpers()
	}
	return `// newRequest decodes an HTTP request into the Rho-facing Request.
func newRequest(r *http.Request) *Request {
	if r == nil {
		return &Request{}
	}
	body, err := readAll(r)
	if err != nil {
		log.Printf("read body: %v", err)
	}
	return &Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   body,
		Auth:   r.Header.Get("Authorization"),
		Params: requestParams(r),
	}
}

// requestParams returns the wildcards the router captured for this request.
func requestParams(r *http.Request) map[string]string {
	if r == nil {
		return map[string]string{}
	}
	if params, ok := r.Context().Value(paramsKey{}).(map[string]string); ok {
		return params
	}
	return map[string]string{}
}

// readAll reads a request body, bounded so a large upload cannot exhaust
// memory.
func readAll(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	defer r.Body.Close()
	const maxBody = 256 << 20
	buf, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// param returns a path wildcard captured by the router, or "" when absent.
func param(req Request, name string) string {
	return req.Param(name)
}

// request_body returns the raw request body, so a binary upload survives.
func requestBody(req Request) string {
	return req.Body
}

// request_auth returns the raw Authorization header.
func requestAuth(req Request) string {
	return req.Auth
}

// request_query_param returns one value from the raw query string.
func requestQueryParam(req Request, name string) string {
	return queryParam(req.Query, name)
}

// queryParam returns a decoded value from the raw query string. Values arrive
// percent-encoded, so a space written as "+" has to be turned back into a
// space rather than shown literally.
func queryParam(query, name string) string {
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		key, value, _ := strings.Cut(part, "=")
		if key != name {
			continue
		}
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return value
		}
		return decoded
	}
	return ""
}

`
}

// goRoutedRequestHelpers is the same decoding, reached through the router the
// target chose. The raw readers live in the routing port; this file keeps only
// what the protocol needs and the names Rho handlers already call.
func goRoutedRequestHelpers() string {
	return `// newRequest decodes an HTTP request into the Rho-facing Request.
func newRequest(r *http.Request) *Request {
	if r == nil {
		return &Request{bind: &bindState{}}
	}
	body, err := readAll(r)
	if err != nil {
		log.Printf("read body: %v", err)
	}
	head := make(map[string]string, len(r.Header))
	for name, values := range r.Header {
		if len(values) > 0 {
			head[name] = values[0]
		}
	}
	return &Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   body,
		Auth:   r.Header.Get("Authorization"),
		Params: rhoRouteParams(r),
		head:   head,
		bind:   &bindState{},
	}
}

// readAll reads a request body, bounded so a large upload cannot exhaust
// memory.
func readAll(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	defer r.Body.Close()
	const maxBody = 256 << 20
	buf, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// param returns a path wildcard captured by the router, or "" when absent.
func param(req Request, name string) string {
	return req.Param(name)
}

// request_body returns the raw request body, so a binary upload survives.
func requestBody(req Request) string {
	return req.Body
}

// request_auth returns the raw Authorization header.
func requestAuth(req Request) string {
	return req.Auth
}

// request_query_param returns one value from the raw query string.
func requestQueryParam(req Request, name string) string {
	value, _ := rhoQueryValue(req.Query, name)
	return value
}

`
}

// goRegisterRoutes emits the mux wiring for a module's routes.
//
// The router matches the patterns itself rather than relying on the Go 1.22
// ServeMux wildcards, so a generated project builds with any Go toolchain. A
// target that names a router asks for the port instead, and gets the standard
// library's mux.
func goRegisterRoutes(m ir.ModuleIR) string {
	var b strings.Builder
	b.WriteString("// registerRoutes attaches the generated handlers to a router.\n")
	b.WriteString("func registerRoutes(router *router) {\n")
	b.WriteString("\trouter.handle(\"GET\", \"/healthz\", func(w http.ResponseWriter, r *http.Request) {\n")
	b.WriteString("\t\twriteJSON(w, http.StatusOK, map[string]string{\"status\": \"ok\"})\n")
	b.WriteString("\t})\n")
	if len(m.Routes) > 0 {
		fmt.Fprintf(&b, "\n\t// Routes declared by the Rho module.\n")
		for _, route := range m.Routes {
			fmt.Fprintf(&b, "\trouter.handle(%q, %q, %s)\n", route.Method, route.Path, goRouteFuncName(route))
		}
	}
	b.WriteString("}\n\n")
	b.WriteString(goRouterType())
	b.WriteString(goRouterServe())
	return b.String()
}

// goRouterServe emits the http.Handler that dispatches through the router.
func goRouterServe() string {
	return `// notImplemented writes the response for a route whose handler is missing.
func notImplemented(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte(message))
}

// notFound writes the response for an unmatched request. It avoids the
// text() helper from std/web/http so a generated project always builds.
func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(404)
	_, _ = w.Write([]byte("not found"))
}

// paramsKey carries the captured path wildcards on the request context.
type paramsKey struct{}

// serve dispatches a request to the matching route, or 404.
func (rt *router) serve(w http.ResponseWriter, r *http.Request) {
	// The asset folder the descriptor creates is served before routing, so a
	// static file never shadows a declared route.
	if strings.HasPrefix(r.URL.Path, "/static/") {
		if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
			http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))).ServeHTTP(w, r)
			return
		}
	}
	handler, params, ok := rt.match(r.Method, r.URL.Path)
	if !ok {
		notFound(w)
		return
	}
	if params != nil {
		r = r.WithContext(context.WithValue(r.Context(), paramsKey{}, params))
	}
	handler(w, r)
}

`
}

// goRouterType emits a small path router. Patterns use {name} for a captured
// segment, which is matched on the decoded path.
func goRouterType() string {
	return `// router matches request paths against the declared patterns. A {name}
// segment captures one path element under that name, and a {name...} segment
// captures every remaining element as one slash-separated path.
type router struct {
	routes []routeEntry
}

// routeEntry is one registered pattern and its handler.
type routeEntry struct {
	method   string
	segments []string
	handler  http.HandlerFunc
}

// handle registers a pattern. The first matching pattern wins.
func (rt *router) handle(method, pattern string, handler http.HandlerFunc) {
	rt.routes = append(rt.routes, routeEntry{
		method:   method,
		segments: splitPath(pattern),
		handler:  handler,
	})
}

// match finds the handler for a request, if any.
func (rt *router) match(method, path string) (http.HandlerFunc, map[string]string, bool) {
	parts := splitPath(path)
	for i := range rt.routes {
		entry := &rt.routes[i]
		if entry.method != method {
			continue
		}
		if params, ok := entry.match(parts); ok {
			return entry.handler, params, true
		}
	}
	return nil, nil, false
}

// match compares the request segments against the pattern.
func (e *routeEntry) match(parts []string) (map[string]string, bool) {
	// A pattern may end in a {name...} segment, which takes every remaining
	// element as one path rather than as one element per name. A path is
	// several elements deep: a documentation page, its stylesheet and the
	// directory of a version's pages are all several.
	catchAll := ""
	prefix := e.segments
	if last := len(prefix) - 1; last >= 0 && strings.HasSuffix(prefix[last], "...}") {
		catchAll = strings.TrimSuffix(strings.Trim(prefix[last], "{}"), "...")
		prefix = prefix[:last]
	}
	if catchAll == "" {
		if len(e.segments) != len(parts) {
			return nil, false
		}
	} else if len(parts) < len(prefix) {
		return nil, false
	}
	var params map[string]string
	for i, segment := range prefix {
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			if params == nil {
				params = map[string]string{}
			}
			params[segment[1:len(segment)-1]] = parts[i]
			continue
		}
		if segment != parts[i] {
			return nil, false
		}
	}
	if catchAll != "" {
		if params == nil {
			params = map[string]string{}
		}
		params[catchAll] = strings.Join(parts[len(prefix):], "/")
	}
	return params, true
}

// splitPath splits a path into its decoded, non-empty segments.
func splitPath(path string) []string {
	segments := []string{}
	for _, part := range strings.Split(path, "/") {
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

`
}

// goRouteFuncName returns the Go handler wrapper name for a route.
func goRouteFuncName(route ir.RouteIR) string {
	return "rho" + goFuncName(route.Handler) + "Handler"
}

// goRouteHandlers emits one net/http wrapper per distinct handler. The
// wrapper decodes the request, calls the Rho function, and writes the response.
func goRouteHandlers(m ir.ModuleIR) string {
	if len(m.Routes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("// HTTP wrappers bridging net/http and the Rho handlers.\n")
	// Several routes can share a handler, so the wrappers are emitted once each
	// and the router binds every path to the same one.
	for _, handler := range types.RouteHandlers(m) {
		name := goRouteFuncName(ir.RouteIR{Handler: handler})

		if !hasGoFunc(m, handler) {
			// A route naming a function this module does not define still has
			// to answer, with a clear status rather than a missing symbol.
			fmt.Fprintf(&b, "// %s stands in for the undefined handler %s.\n", name, handler)
			fmt.Fprintf(&b, "func %s(w http.ResponseWriter, r *http.Request) {\n", name)
			fmt.Fprintf(&b, "\tnotImplemented(w, %q)\n}\n\n", "handler "+handler+" is not defined")
			continue
		}

		// Document every path this handler serves.
		for _, route := range m.Routes {
			if route.Handler == handler {
				fmt.Fprintf(&b, "// %s serves %s %s.\n", name, route.Method, route.Path)
			}
		}
		fmt.Fprintf(&b, "func %s(w http.ResponseWriter, r *http.Request) {\n", name)
		fmt.Fprintf(&b, "\t%s(*newRequest(r)).write(w)\n}\n\n", goFuncName(handler))
	}
	return b.String()
}

// hasGoFunc reports whether a function name is emitted in this module.
func hasGoFunc(m ir.ModuleIR, name string) bool {
	for _, fn := range m.Funcs {
		if !fn.Extern && (irFuncName(fn) == name || goFuncName(irFuncName(fn)) == name) {
			return true
		}
	}
	return false
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

// protocolTypes are the handler signature types the Go server defines itself.
var protocolTypes = map[string]bool{
	"Request":  true,
	"Response": true,
}

// isProtocolType reports whether a struct is an HTTP protocol type owned by
// the server backend.
func isProtocolType(name string) bool { return protocolTypes[name] }
