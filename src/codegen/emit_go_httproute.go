//
// @file emit_go_httproute.go
// @brief The HTTP routing port.
//
// Routing is a target's decision, so it is a target's setting: a server names
// the router it dispatches through and the middleware wrapped around it, and
// the generator writes the code that does both.
//
//	targets:
//	  - id: go-server
//	    emitter: go
//	    kind: server
//	    router: serveMux
//	    middleware:
//	      - name: request_id
//	      - name: cors
//	        options:
//	          origin: "*"
//
// Standard library first is not a preference, it is the default. Go 1.22 gave
// `http.ServeMux` the method-and-wildcard patterns a REST API is written in,
// and a generated module already declares that version, so a service routes
// its requests with nothing fetched. `{name}` and `{name...}` are the syntax a
// Rho route is declared with, so the pattern crosses into the mux unchanged.
//
// The adapters exist for the projects that want the rest of a framework. Each
// is a registration change and a way to read a captured value, never a second
// router: the handlers, the binding and the middleware are the same whatever
// dispatches them. That is what keeps a project that moves between routers
// moving code rather than rewriting it.
//
// Middleware is the Go shape, unchanged. Every wrapper is a
// `func(http.Handler) http.Handler`, composed in the order the target listed
// them, outermost first. Anything a wrapper can do as a Go value is a wrapper;
// there is no Rho-side pipeline to learn and no registration to sequence.
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
	"sort"
	"strings"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  The vocabulary                                                              */
/* -------------------------------------------------------------------------- */

// Routers a server target may name. The standard library comes first and is
// the default; the rest are adapters, and each costs a dependency the project
// asked for rather than one it inherited.
const (
	RouterServeMux = "serveMux"
	RouterChi      = "chi"
	RouterGin      = "gin"
	RouterFiber    = "fiber"
)

// DefaultRouter is the router a target naming none is given: the standard
// library, because a generated project should build with `go build` and
// nothing fetched.
const DefaultRouter = RouterServeMux

// The rate limit a target that named the wrapper but set nothing gets, written
// into the generated project by name so that a reader can see what the service
// enforces without running it.
const (
	defaultRateLimitRequests = 100
	defaultRateLimitWindow   = "1m"
)

// Middleware wrappers the port supplies. Each is a func(http.Handler)
// http.Handler composed around the router.
const (
	// MiddlewareRequestID names every request and echoes the name back, so a
	// log line and a client report refer to the same request.
	MiddlewareRequestID = "request_id"
	// MiddlewareCORS answers the preflight and sets the cross-origin headers.
	MiddlewareCORS = "cors"
	// MiddlewareRateLimit bounds how often one client may call the service.
	MiddlewareRateLimit = "rate_limit"
	// MiddlewareJWT verifies a bearer token and refuses the request without
	// one that is valid.
	MiddlewareJWT = "jwt"
)

// registeredRouters lists the routers a target may name.
var registeredRouters = []string{RouterServeMux, RouterChi, RouterGin, RouterFiber}

// RegisteredRouters lists the routers a target may name.
func RegisteredRouters() []string {
	out := make([]string, len(registeredRouters))
	copy(out, registeredRouters)
	return out
}

// HasRouter reports whether a router name is available.
func HasRouter(name string) bool {
	for _, known := range registeredRouters {
		if known == name {
			return true
		}
	}
	return false
}

// routerRequire is the module an adapter needs, and the version the generated
// go.mod records for it. A project that names an adapter gets its dependency
// written for it, so choosing a router is one line rather than three.
var routerRequire = map[string]string{
	RouterChi:   "github.com/go-chi/chi/v5 v5.1.0",
	RouterGin:   "github.com/gin-gonic/gin v1.10.0",
	RouterFiber: "github.com/gofiber/fiber/v2 v2.52.4",
}

// RouterRequirement returns the go.mod line an adapter needs, or "" when the
// router is the standard library.
func RouterRequirement(router string) string { return routerRequire[router] }

// adapterImports returns the import lines an adapter needs, or none for the
// standard library.
func adapterImports(router string) []importSpec {
	switch router {
	case RouterChi:
		return []importSpec{{Path: `"github.com/go-chi/chi/v5"`}}
	case RouterGin:
		return []importSpec{{Path: `"github.com/gin-gonic/gin"`}}
	case RouterFiber:
		return []importSpec{
			{Path: `"github.com/gofiber/fiber/v2"`},
			{Path: `"github.com/gofiber/fiber/v2/middleware/adaptor"`},
		}
	default:
		return nil
	}
}

/* -------------------------------------------------------------------------- */
/*  Server configuration                                                        */
/* -------------------------------------------------------------------------- */

// MiddlewareSpec is one wrapper of a target's pipeline, with the settings it is
// composed with. Every option a middleware understands has a default, so a
// project names one and overrides what it cares about.
type MiddlewareSpec struct {
	Name    string
	Options map[string]string
}

// ServerConfig is how a target routes and what it wraps the router in.
type ServerConfig struct {
	// Router names the router to dispatch through. Empty means the standard
	// library, which is the default.
	Router string
	// Middleware lists the wrappers composed around the router, outermost
	// first, in the order the target declared them.
	Middleware []MiddlewareSpec
}

// Routed reports whether the target routes through a named router, rather than
// through the backend's own matcher.
func (c ServerConfig) Routed() bool { return c.Router != "" || len(c.Middleware) > 0 }

// EffectiveRouter returns the router a target dispatches through.
func (c ServerConfig) EffectiveRouter() string {
	if c.Router == "" {
		return DefaultRouter
	}
	return c.Router
}

// option returns a middleware setting, or a default when it declared none.
func option(spec MiddlewareSpec, name, fallback string) string {
	if value, ok := spec.Options[name]; ok && value != "" {
		return value
	}
	return fallback
}

/* -------------------------------------------------------------------------- */
/*  The port                                                                   */
/* -------------------------------------------------------------------------- */

// emitHTTPRoutePort renders the routing, binding and middleware support a
// server target that named a router asked for.
func emitHTTPRoutePort(ctx PortContext) ([]string, string) {
	var b strings.Builder
	b.WriteString("// Generated by the httproute port.\n\n")
	b.WriteString(goRouterConstruction(ctx))
	b.WriteString(goMountRoutes(ctx.IR))
	b.WriteString(goCapturedParams(ctx))
	b.WriteString(emitGoBinding())
	b.WriteString(goMiddlewareChain(ctx.Server.Middleware))

	source := b.String()
	candidates := []importSpec{
		{Path: `"net/http"`},
		{Path: `"net/url"`},
		{Path: `"os"`},
		{Path: `"strconv"`},
		{Path: `"strings"`},
		// The middleware, each of which needs what its own shape needs.
		{Path: `"context"`},
		{Path: `"crypto/rand"`},
		{Path: `"crypto/hmac"`},
		{Path: `"crypto/sha256"`},
		{Path: `"encoding/base64"`},
		{Path: `"encoding/hex"`},
		{Path: `"net"`},
		{Path: `"sync"`},
		{Path: `"time"`},
	}
	candidates = append(candidates, adapterImports(ctx.Server.EffectiveRouter())...)

	lines := []string{}
	for _, spec := range usedImports(source, candidates) {
		lines = append(lines, spec.Line())
	}
	return lines, source
}

// usedImports keeps the imports the emitted source actually refers to, so a
// project that asks for no middleware and the standard library is not handed a
// file the compiler refuses.
func usedImports(source string, candidates []importSpec) []importSpec {
	out := []importSpec{}
	for _, candidate := range candidates {
		if usesPackage(source, candidate.Ident()) {
			out = append(out, candidate)
		}
	}
	// An import block is ordered by path, which is what gofmt does, so a
	// generated file needs no second formatting pass to be gofmt-clean.
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// usesPackage reports whether a source refers to a package by name.
//
// A comment is stripped first, because a comment that happens to mention
// `os.Getenv` would otherwise keep the import the comment is describing, and a
// file carrying an import nothing uses is a file the compiler refuses.
func usesPackage(source, name string) bool {
	marker := name + "."
	source = stripGoComments(source)
	for at := 0; ; {
		index := strings.Index(source[at:], marker)
		if index < 0 {
			return false
		}
		start := at + index
		if start == 0 || !isIdentByte(source[start-1]) {
			return true
		}
		at = start + len(marker)
	}
}

// isIdentByte reports whether a byte is part of an identifier, so `net` is not
// found inside some longer name that happens to end the same way.
func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// stripGoComments removes the comments from a Go source, so a search for a
// package name is not answered by a sentence that happens to mention it.
func stripGoComments(source string) string {
	var out strings.Builder
	for i := 0; i < len(source); {
		switch {
		case strings.HasPrefix(source[i:], "//"):
			end := strings.IndexByte(source[i:], '\n')
			if end < 0 {
				return out.String()
			}
			i += end
		case strings.HasPrefix(source[i:], "/*"):
			end := strings.Index(source[i+2:], "*/")
			if end < 0 {
				return out.String()
			}
			i += end + 4
		default:
			out.WriteByte(source[i])
			i++
		}
	}
	return out.String()
}

/* -------------------------------------------------------------------------- */
/*  Construction                                                               */
/* -------------------------------------------------------------------------- */

// goRouterConstruction renders the handler the service serves: the router, the
// static folder in front of it, and the pipeline wrapped around the result.
func goRouterConstruction(ctx PortContext) string {
	var b strings.Builder
	b.WriteString(`// newRouter builds the handler the service serves: the routes the module
// declared, the static folder ahead of them, and the pipeline wrapped around
// the result.
func newRouter() (http.Handler, error) {
	routed, err := rhoNewRouter()
	if err != nil {
		return nil, err
	}
	// The asset folder is served before routing, so a static file never
	// shadows a declared route.
	return rhoPipeline(rhoStaticFiles(routed)), nil
}

// rhoStaticFiles serves the asset folder under /static/ and passes everything
// else to the router. A project that ships no assets gets a handler that does
// nothing rather than a folder that is missing.
func rhoStaticFiles(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
				http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))).ServeHTTP(w, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

`)
	b.WriteString(goRouterConstructor(ctx.Server.EffectiveRouter()))
	return b.String()
}

// goRouterConstructor renders the router itself, which is the one part of
// routing an adapter has to replace.
func goRouterConstructor(router string) string {
	switch router {
	case RouterChi:
		return `// rhoNewRouter returns the chi router the routes are bound to.
func rhoNewRouter() (http.Handler, error) {
	r := chi.NewRouter()
	rhoMountRoutes(func(method, pattern string, handler http.HandlerFunc) {
		r.Method(method, pattern, handler)
	})
	return r, nil
}
`
	case RouterGin:
		return `// rhoNewRouter returns the gin engine the routes are bound to.
func rhoNewRouter() (http.Handler, error) {
	r := gin.New()
	rhoMountRoutes(func(method, pattern string, handler http.HandlerFunc) {
		// gin keeps the values it captures on its own context rather than on
		// the request, so the bridge copies them onto one before the handler
		// reads a path wildcard. The handler is otherwise the same net/http
		// handler every other router serves.
		r.Handle(method, rhoAdapterPattern(pattern), gin.WrapF(func(c *gin.Context) {
			handler(c.Writer, rhoGinRequest(c, pattern))
		}))
	})
	return r, nil
}

// rhoAdapterPattern rewrites a ServeMux wildcard to the one gin reads, and
// leaves every other segment as it was. A trailing {name...} is gin a *name,
// because it is the rest of the path rather than one segment.
func rhoAdapterPattern(pattern string) string {
	var out strings.Builder
	for _, segment := range strings.Split(pattern, "/") {
		if out.Len() > 0 {
			out.WriteString("/")
		}
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			name := strings.TrimSuffix(segment[1:len(segment)-1], "...")
			if name != segment[1:len(segment)-1] {
				out.WriteString("*" + name)
			} else {
				out.WriteString(":" + name)
			}
			continue
		}
		out.WriteString(segment)
	}
	return out.String()
}

// rhoGinRequest returns the request with the values gin captured copied onto
// it, so a handler reads a path wildcard the same way whichever router
// dispatched it.
func rhoGinRequest(c *gin.Context, pattern string) *http.Request {
	values := map[string]string{}
	for _, name := range pathWildcardNames(pattern) {
		values[name] = c.Param(name)
	}
	request := c.Request.WithContext(context.WithValue(c.Request.Context(), rhoParamsKey{}, values))
	return request
}
`
	case RouterFiber:
		return `// rhoNewRouter returns the fiber app the routes are bound to.
func rhoNewRouter() (http.Handler, error) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	rhoMountRoutes(func(method, pattern string, handler http.HandlerFunc) {
		app.Add(method, rhoAdapterPattern(pattern), func(c *fiber.Ctx) error {
			// fiber answers through its own context rather than through a
			// response writer, so the handler is given one that collects what
			// it writes. One handler then serves whichever router dispatched
			// it, and the response is copied across afterwards.
			recorder := &rhoRecorder{header: http.Header{}, status: http.StatusOK}
			handler(recorder, rhoFiberRequest(c, pattern))
			for name, values := range recorder.header {
				for _, value := range values {
					c.Set(name, value)
				}
			}
			return c.Status(recorder.status).SendString(recorder.body.String())
		})
	})
	return adaptor.FiberApp(app), nil
}

// rhoAdapterPattern rewrites a ServeMux wildcard to the one fiber reads, and
// leaves every other segment as it was. A trailing {name...} is fiber a *name,
// because it is the rest of the path rather than one segment.
func rhoAdapterPattern(pattern string) string {
	var out strings.Builder
	for _, segment := range strings.Split(pattern, "/") {
		if out.Len() > 0 {
			out.WriteString("/")
		}
		if len(segment) > 2 && segment[0] == '{' && segment[len(segment)-1] == '}' {
			name := strings.TrimSuffix(segment[1:len(segment)-1], "...")
			if name != segment[1:len(segment)-1] {
				out.WriteString("*" + name)
			} else {
				out.WriteString(":" + name)
			}
			continue
		}
		out.WriteString(segment)
	}
	return out.String()
}

// rhoRecorder is a response writer a fiber handler can write to, so the same
// net/http handler serves whichever router dispatched it.
type rhoRecorder struct {
	header http.Header
	status int
	body   strings.Builder
}

func (rec *rhoRecorder) Header() http.Header            { return rec.header }
func (rec *rhoRecorder) WriteHeader(status int)         { rec.status = status }
func (rec *rhoRecorder) Write(data []byte) (int, error) { return rec.body.Write(data) }

// rhoFiberRequest returns the request with the values fiber captured copied
// onto it, the same bridge gin gets.
func rhoFiberRequest(c *fiber.Ctx, pattern string) *http.Request {
	values := map[string]string{}
	for _, name := range pathWildcardNames(pattern) {
		values[name] = c.Params(name)
	}
	request := c.Context().Request
	return request.WithContext(context.WithValue(request.Context(), rhoParamsKey{}, values))
}
`
	default:
		return `// rhoNewRouter returns the standard library's mux, which has carried
// method-and-wildcard patterns since Go 1.22. A generated project therefore
// routes its requests with nothing fetched.
func rhoNewRouter() (http.Handler, error) {
	mux := http.NewServeMux()
	rhoMountRoutes(func(method, pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(method+" "+pattern, handler)
	})
	return mux, nil
}
`
	}
}

/* -------------------------------------------------------------------------- */
/*  Mounting                                                                   */
/* -------------------------------------------------------------------------- */

// goMountRoutes renders the binding of every declared route onto whichever
// router the target chose.
//
// A ServeMux pattern is a method, a space and the path the Rho route was
// declared with, so nothing rewrites it. An adapter rewrites only the wildcard
// syntax, in the constructor above.
func goMountRoutes(m ir.ModuleIR) string {
	var b strings.Builder
	b.WriteString("// rhoMountRoutes binds the module's declared routes. The binder is the\n")
	b.WriteString("// router's own registration, so the handlers below are net/http handlers\n")
	b.WriteString("// whichever router is in use.\n")
	b.WriteString("func rhoMountRoutes(register func(method, pattern string, handler http.HandlerFunc)) {\n")
	b.WriteString("\t// Every service answers a liveness probe, whether or not the module\n")
	b.WriteString("\t// declared one.\n")
	b.WriteString("\tregister(\"GET\", \"/healthz\", func(w http.ResponseWriter, r *http.Request) {\n")
	b.WriteString("\t\twriteJSON(w, http.StatusOK, map[string]string{\"status\": \"ok\"})\n")
	b.WriteString("\t})\n")
	if len(m.Routes) > 0 {
		b.WriteString("\n\t// Routes declared by the Rho module.\n")
		for _, route := range m.Routes {
			if !hasGoFunc(m, route.Handler) {
				// A route naming a function this module does not define still
				// has to answer, with a clear status rather than a missing
				// symbol. The stand-in itself is written beside the other
				// handlers, because that is where every handler lives.
				fmt.Fprintf(&b, "\t// %s stands in for the undefined handler %s.\n",
					goRouteFuncName(route), route.Handler)
			}
			fmt.Fprintf(&b, "\tregister(%q, %q, %s)\n",
				route.Method, route.Path, goRouteFuncName(route))
		}
	}
	b.WriteString("}\n\n")
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Reading what the router captured                                            */
/* -------------------------------------------------------------------------- */

// goCapturedParams renders the reader that puts the router's captured values
// back on the request, so `param(req, "name")` reads the same whichever router
// dispatched the call.
func goCapturedParams(ctx PortContext) string {
	var b strings.Builder
	b.WriteString(`// rhoParamsKey carries the values a router captured. The standard library
// reaches them through r.PathValue and chi through its own context; an
// adapter that keeps them somewhere else puts them here, and every router then
// reads one place.
type rhoParamsKey struct{}

// rhoRouteParams returns the values the router captured for this request.
func rhoRouteParams(r *http.Request) map[string]string {
	if r == nil {
		return map[string]string{}
	}
	values := map[string]string{}
	for name, value := range rhoRouterValues(r) {
		values[name] = value
	}
	if captured, ok := r.Context().Value(rhoParamsKey{}).(map[string]string); ok {
		for name, value := range captured {
			values[name] = value
		}
	}
	return values
}

`)
	// The router's own reader. Every name a module's routes declare is read by
	// name, so a generated project names its parameters rather than
	// reflecting over the request to discover them.
	router := ctx.Server.EffectiveRouter()
	if router == RouterChi {
		b.WriteString("// rhoRouterValues returns the values chi captured. chi keeps them in the\n")
		b.WriteString("// request context, one read per declared name.\n")
		b.WriteString("func rhoRouterValues(r *http.Request) map[string]string {\n\tvalues := map[string]string{}\n")
		for _, name := range declaredWildcards(ctx.IR) {
			fmt.Fprintf(&b, "\tvalues[%q] = chi.URLParam(r, %q)\n", name, name)
		}
		b.WriteString("\treturn values\n}\n\n")
		return b.String()
	}
	if router == RouterGin || router == RouterFiber {
		b.WriteString("// rhoRouterValues is empty for this adapter: it has already put the values\n")
		b.WriteString("// it captured on the request context, where rhoRouteParams finds them.\n")
		b.WriteString("func rhoRouterValues(r *http.Request) map[string]string {\n\treturn nil\n}\n\n")
		return b.String()
	}
	b.WriteString("// rhoRouterValues returns the values the standard library captured.\n")
	b.WriteString("//\n")
	b.WriteString("// r.PathValue hands back the decoded segment, so a percent-encoded name\n")
	b.WriteString("// arrives as the name rather than as the bytes someone typed.\n")
	b.WriteString("func rhoRouterValues(r *http.Request) map[string]string {\n\tvalues := map[string]string{}\n")
	for _, name := range declaredWildcards(ctx.IR) {
		fmt.Fprintf(&b, "\tvalues[%q] = r.PathValue(%q)\n", name, name)
	}
	b.WriteString("\treturn values\n}\n\n")
	return b.String()
}

// declaredWildcards returns the distinct `{name}` placeholders a module's
// routes declare, in the order they first appear.
func declaredWildcards(m ir.ModuleIR) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, route := range m.Routes {
		for _, name := range pathWildcardNames(route.Path) {
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// pathWildcardNames returns the `{name}` placeholders in a route pattern. A
// trailing `{name...}` names the rest of the path and is written without the
// dots, because that is the name a handler asks for.
func pathWildcardNames(pattern string) []string {
	out := []string{}
	for i := 0; i < len(pattern); {
		if pattern[i] != '{' {
			i++
			continue
		}
		end := strings.IndexByte(pattern[i:], '}')
		if end < 0 {
			break
		}
		out = append(out, strings.TrimSuffix(pattern[i+1:i+end], "..."))
		i += end + 1
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Middleware                                                                 */
/* -------------------------------------------------------------------------- */

// middlewareEmitters are the wrappers the port supplies, in the vocabulary the
// descriptor validates against. Each one writes the wrapper under the name
// middlewareFuncs gives it, so the pipeline and the implementation cannot
// disagree about what a middleware is called.
var middlewareEmitters = map[string]func(name string, spec MiddlewareSpec) string{
	MiddlewareRequestID: emitRequestIDMiddleware,
	MiddlewareCORS:      emitCORSMiddleware,
	MiddlewareRateLimit: emitRateLimitMiddleware,
	MiddlewareJWT:       emitJWTMiddleware,
}

// middlewareFuncs are the Go names of the wrappers. The spellings follow the
// usual Go convention, where an initialism keeps its capitals.
var middlewareFuncs = map[string]string{
	MiddlewareRequestID: "withRequestID",
	MiddlewareCORS:      "withCORS",
	MiddlewareRateLimit: "withRateLimit",
	MiddlewareJWT:       "withJWT",
}

// RegisteredMiddlewares lists the wrappers a target may compose.
func RegisteredMiddlewares() []string {
	out := make([]string, 0, len(middlewareEmitters))
	for name := range middlewareEmitters {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// HasMiddleware reports whether a middleware name is available.
func HasMiddleware(name string) bool {
	_, ok := middlewareEmitters[name]
	return ok
}

// goMiddlewareChain renders the pipeline a target declared.
//
// Composition is the Go one. A wrapper takes a handler and returns a handler,
// so the first name in the list is the outermost wrapper: a request meets the
// request-id stamp, then the cross-origin headers, then the rate limit, and
// only then the router.
func goMiddlewareChain(pipeline []MiddlewareSpec) string {
	var b strings.Builder
	b.WriteString("// rhoPipeline composes the middleware a target declared, outermost first.\n")
	b.WriteString("//\n")
	b.WriteString("// A wrapper is an ordinary func(http.Handler) http.Handler, so a project\n")
	b.WriteString("// that wants a wrapper the port does not supply writes one and adds it\n")
	b.WriteString("// to this list rather than reaching for a different mechanism.\n")
	b.WriteString("func rhoPipeline(next http.Handler) http.Handler {\n")
	if len(pipeline) == 0 {
		b.WriteString("\treturn next\n}\n")
		return b.String()
	}
	// The last wrapper declared is the innermost, so the list is applied from
	// the end and each one closes over what has been built so far.
	for i := len(pipeline) - 1; i >= 0; i-- {
		if _, ok := middlewareEmitters[pipeline[i].Name]; !ok {
			// The descriptor is validated first, so this only reports a caller
			// that reached past it.
			continue
		}
		fmt.Fprintf(&b, "\tnext = %s(next)\n", goMiddlewareFunc(pipeline[i].Name))
	}
	b.WriteString("\treturn next\n}\n\n")

	// The wrappers themselves, each written under the name the pipeline calls.
	for _, spec := range pipeline {
		emit, ok := middlewareEmitters[spec.Name]
		if !ok {
			continue
		}
		b.WriteString(emit(goMiddlewareFunc(spec.Name), spec))
		b.WriteString("\n")
	}
	return b.String()
}

// goMiddlewareFunc returns the Go name of a middleware wrapper, or the
// underscored name spelled out when a caller reached past the table.
func goMiddlewareFunc(name string) string {
	if spelling, ok := middlewareFuncs[name]; ok {
		return spelling
	}
	return name
}
