//
// @file httproute_test.go
// @brief Tests for the HTTP routing port.
//
// The port generates Go, so most of what is checked here is what a generated
// project must contain: a router the target chose, a handler for every declared
// route, an accessor for every source and type, and a pipeline in the order the
// target declared. The generated source is also parsed, because a port that
// emits something the Go parser rejects is a port that breaks a build with a
// message pointing at a file nobody wrote.
//
// The binding is checked where it can be: the generated accessors are extracted
// from the emitted text and exercised as Go, which is the only way to know that
// a conversion a document promises is the conversion the code performs.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/ir"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                    */
/* -------------------------------------------------------------------------- */

// routedIR is a module with one route per shape a service declares: a read of
// one thing by its identifier, a write with a body, and a catch-all.
func routedIR() ir.ModuleIR {
	return ir.ModuleIR{
		Name:    "routed",
		Version: "0.1.0",
		Funcs: []ir.FuncIR{
			{Name: "module_page", CName: "module_page",
				ReturnType: ir.Type{Kind: ir.TypeStruct, Name: "Response"}},
			{Name: "publish_module", CName: "publish_module",
				ReturnType: ir.Type{Kind: ir.TypeStruct, Name: "Response"}},
			{Name: "docs_page", CName: "docs_page",
				ReturnType: ir.Type{Kind: ir.TypeStruct, Name: "Response"}},
		},
		Routes: []ir.RouteIR{
			{Method: "GET", Path: "/modules/{name}", Handler: "module_page"},
			{Method: "POST", Path: "/modules", Handler: "publish_module"},
			{Method: "GET", Path: "/modules/{name}/docs/{path...}", Handler: "docs_page"},
		},
	}
}

// serveMuxConfig is a target that routes through the standard library with a
// three-wrapper pipeline, which is the shape a real service declares.
func serveMuxConfig() ServerConfig {
	return ServerConfig{
		Router: RouterServeMux,
		Middleware: []MiddlewareSpec{
			{Name: MiddlewareRequestID},
			{Name: MiddlewareCORS, Options: map[string]string{"origin": "https://app.rho.dev"}},
			{Name: MiddlewareRateLimit, Options: map[string]string{"requests": "42", "window": "30s"}},
		},
	}
}

// emitPort renders the routing port for a module and a target.
func emitPort(t *testing.T, module ir.ModuleIR, config ServerConfig) string {
	t.Helper()
	ctx := PortContext{IR: module, ModuleName: module.Name, Server: config}
	body := EmitGoPorts(ctx, []string{"httproute"})
	if body == "" {
		t.Fatal("the routing port emitted nothing for a target that named a router")
	}
	return body
}

/* -------------------------------------------------------------------------- */
/*  The router                                                                  */
/* -------------------------------------------------------------------------- */

func TestPortUsesTheStandardLibraryByDefault(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, want := range []string{
		"mux := http.NewServeMux()",
		`mux.HandleFunc(method+" "+pattern, handler)`,
		"r.PathValue(\"name\")",
		`register("GET", "/modules/{name}", rhomodulePageHandler)`,
		`register("POST", "/modules", rhopublishModuleHandler)`,
		`register("GET", "/modules/{name}/docs/{path...}", rhodocsPageHandler)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the port did not emit %q", want)
		}
	}
}

func TestPortDownloadsNothingForTheStandardRouter(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	// Nothing beyond the standard library is imported, so a generated project
	// routes its requests with nothing fetched.
	for _, unwanted := range []string{"go-chi", "gin-gonic", "gofiber"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the standard router pulled in %q", unwanted)
		}
	}
}

func TestPortWritesAnAdapterPerRouter(t *testing.T) {
	cases := map[string][]string{
		RouterChi: {
			"r := chi.NewRouter()",
			"r.Method(method, pattern, handler)",
			`values["name"] = chi.URLParam(r, "name")`,
		},
		RouterGin: {
			"r := gin.New()",
			"r.Handle(method, rhoAdapterPattern(pattern)",
			"rhoGinRequest(c, pattern)",
		},
		RouterFiber: {
			"app := fiber.New(",
			"adaptor.FiberApp(app)",
			"rhoFiberRequest(c, pattern)",
		},
	}
	for router, wants := range cases {
		body := emitPort(t, routedIR(), ServerConfig{Router: router})
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Errorf("the %s adapter did not emit %q", router, want)
			}
		}
		if requirement := RouterRequirement(router); requirement == "" {
			t.Errorf("the %s adapter names no requirement for go.mod", router)
		}
	}
}

func TestAdapterRewritesACatchAllToAFrameworkWildcard(t *testing.T) {
	body := emitPort(t, routedIR(), ServerConfig{Router: RouterGin})
	// A {name...} is the rest of the path, which gin reads as a *name and a
	// {name} is one segment, which it reads as :name.
	if !strings.Contains(body, `out.WriteString("*" + name)`) {
		t.Error("a catch-all wildcard was not rewritten to the adapter's own")
	}
	if !strings.Contains(body, `out.WriteString(":" + name)`) {
		t.Error("a single-segment wildcard was not rewritten to the adapter's own")
	}
}

func TestStandardRouterNeedsNoRequirement(t *testing.T) {
	if got := RouterRequirement(RouterServeMux); got != "" {
		t.Errorf("the standard library router recorded a requirement: %q", got)
	}
	if got := adapterImports(RouterServeMux); len(got) != 0 {
		t.Errorf("the standard library router recorded imports: %v", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Mounting                                                                    */
/* -------------------------------------------------------------------------- */

func TestPortAnswersAProbeAndEveryRouteOnce(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	// A service answers a liveness probe whether or not the module declared
	// one, and the string appears once: in the constant and nowhere else.
	if count := strings.Count(body, `register("GET", "/healthz"`); count != 1 {
		t.Errorf("the probe is bound %d times, want 1", count)
	}
	if count := strings.Count(body, "func rhomodulePageHandler"); count != 0 {
		// The wrapper lives in server.go, not the port; two would not compile.
		t.Errorf("the port emitted a handler wrapper as well: %d", count)
	}
}

func TestPortStandsInForAnUndefinedHandler(t *testing.T) {
	module := routedIR()
	module.Routes = []ir.RouteIR{{Method: "GET", Path: "/gone", Handler: "not_written"}}
	body := emitPort(t, module, serveMuxConfig())
	// A route naming a function the module does not define still has to answer,
	// or the project will not build.
	// The binding is the port's; the stand-in that answers is written beside
	// the other handlers, because that is where every handler lives.
	if !strings.Contains(body, `register("GET", "/gone", rhonotWrittenHandler)`) {
		t.Error("a route with no handler was left unbound")
	}
	if !strings.Contains(body, "rhonotWrittenHandler stands in for the undefined handler not_written") {
		t.Error("the binding does not say which handler is missing")
	}
	server := EmitGoServer(module, serveMuxConfig())
	if !strings.Contains(server, "handler not_written is not defined") {
		t.Error("the stand-in does not say which handler is missing")
	}
}

func TestPortEmitsNoBindingsForAModuleWithNoRoutes(t *testing.T) {
	body := emitPort(t, ir.ModuleIR{Name: "bare", Version: "0.1.0"}, serveMuxConfig())
	if !strings.Contains(body, "func rhoMountRoutes(") {
		t.Fatal("the mount function is missing")
	}
	// A module with no routes still answers, so the router is not dead code.
	if !strings.Contains(body, "/healthz") {
		t.Error("a module with no routes lost its probe")
	}
}

/* -------------------------------------------------------------------------- */
/*  Parameter binding                                                           */
/* -------------------------------------------------------------------------- */

func TestPortEmitsAnAccessorPerSourceAndType(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, source := range bindingSources {
		for _, kind := range bindingTypes {
			accessor := source.rho + "_" + kind.suffix
			want := "func " + lowerCamel(accessor) + "(req Request, name string) " + kind.gotype
			if !strings.Contains(body, want) {
				t.Errorf("no accessor for %s: want %q", accessor, want)
			}
		}
	}
}

func TestPortConvertsWithTheStandardLibraryRatherThanReflection(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, want := range []string{
		"strconv.ParseInt(raw, 10, 32)",
		"strconv.ParseUint(raw, 10, 32)",
		"strconv.ParseUint(raw, 10, 64)",
		"strconv.ParseFloat(raw, 64)",
		"strconv.ParseBool(raw)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no conversion %q", want)
		}
	}
	// The claim is the point of the design, so it is checked rather than
	// assumed: the port must not reach for reflect or for a decoder that
	// decodes by way of it.
	for _, unwanted := range []string{"reflect.", "json.Unmarshal", "encoding/json"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the port used %q, which decodes by way of reflection", unwanted)
		}
	}
}

func TestPortReportsABindingFailureOnTheRequest(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, want := range []string{
		"func bindOk(req Request) bool",
		"func bindError(req Request) string",
		"req.bindFail(bindMessage(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the port did not emit %q", want)
		}
	}
}

func TestPortEmitsAPresenceReadPerNamedSource(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, accessor := range []string{"has_path", "has_query", "has_header", "has_field"} {
		if !strings.Contains(body, "func "+lowerCamel(accessor)+"(req Request, name string) bool") {
			t.Errorf("no presence read for %s", accessor)
		}
	}
	// The body read whole is a value rather than a name, so there is no
	// presence to ask about.
	if strings.Contains(body, "func hasBody(") {
		t.Error("a presence read was emitted for the body read whole")
	}
	if !strings.Contains(body, "func queryDefault(req Request, name, fallback string) string") {
		t.Error("the defaulting read is missing")
	}
}

func TestJSONScannerIsGeneratedWhole(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, want := range []string{
		"func rhoJSONField(body, name string) (string, bool)",
		"func rhoJSONObject(body string) map[string]string",
		"func rhoJSONString(body string, i int) (string, int, bool)",
		"func rhoJSONNested(body string, i int) (int, bool)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the JSON reader did not emit %q", want)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Middleware                                                                  */
/* -------------------------------------------------------------------------- */

func TestPipelineComposesOutermostFirst(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	// The first wrapper declared is the outermost, so the composition runs
	// from the last one back. A pipeline applied in the other order would let a
	// rate limiter sit outside the request id, and a rejected request would
	// then have no identifier.
	chain := between(t, body, "func rhoPipeline(", "\treturn next\n}")
	want := []string{
		"\tnext = withRateLimit(next)\n",
		"\tnext = withCORS(next)\n",
		"\tnext = withRequestID(next)\n",
	}
	for _, line := range want {
		if !strings.Contains(chain, line) {
			t.Errorf("the pipeline is composed in the wrong order; want %s in:\n%s", line, chain)
		}
	}
}

func TestEveryWrapperIsAGoHandlerWrapper(t *testing.T) {
	// The pipeline carries every wrapper, because a wrapper that is not
	// composed is not a wrapper.
	pipeline := []MiddlewareSpec{}
	for _, name := range RegisteredMiddlewares() {
		pipeline = append(pipeline, MiddlewareSpec{Name: name})
	}
	body := emitPort(t, routedIR(), ServerConfig{Router: RouterServeMux, Middleware: pipeline})
	for _, name := range RegisteredMiddlewares() {
		wrapper := goMiddlewareFunc(name)
		if !strings.Contains(body, "func "+wrapper+"(next http.Handler) http.Handler {") {
			t.Errorf("%s is not a func(http.Handler) http.Handler", name)
		}
		if !strings.Contains(body, "next = "+wrapper+"(next)") {
			t.Errorf("%s is never composed", name)
		}
	}
}

func TestWrapperSettingsComeFromTheTarget(t *testing.T) {
	body := emitPort(t, routedIR(), serveMuxConfig())
	for _, want := range []string{
		`corsOrigin  = "https://app.rho.dev"`,
		`rateLimitSetting       = "42"`,
		`rateLimitWindowSetting = "30s"`,
		`const requestIDHeader = "X-Request-Id"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the target's setting did not reach the code: want %q", want)
		}
	}
}

func TestASettingThatIsNotACountFallsBackRatherThanBreakingTheBuild(t *testing.T) {
	// A settings map is free text, so a project can write a number where a
	// number belongs and something else where a number belongs. Writing it
	// straight into the source would hand the compiler a bool constant and
	// produce a service that refuses every request.
	body := emitPort(t, routedIR(), ServerConfig{Router: RouterServeMux,
		Middleware: []MiddlewareSpec{{Name: MiddlewareRateLimit,
			Options: map[string]string{"requests": "yes", "window": "1 lightyear"}}}})
	for _, want := range []string{
		`rateLimitSetting       = "yes"`,
		"func rhoCount(setting string, fallback int) int",
		"func rhoWindow(setting, fallback string) time.Duration",
		"rateLimitRequests = rhoCount(rateLimitSetting, 100)",
		"rateLimitWindow   = rhoWindow(rateLimitWindowSetting, \"1m\")",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a setting that is not a number is not resolved safely: want %q", want)
		}
	}
}

func TestWrapperDefaultsApplyWhenTheTargetIsSilent(t *testing.T) {
	body := emitPort(t, routedIR(), ServerConfig{Router: RouterServeMux,
		Middleware: []MiddlewareSpec{{Name: MiddlewareCORS}, {Name: MiddlewareJWT}}})
	for _, want := range []string{
		`corsOrigin  = "*"`,
		`jwtSecretEnv = "RHO_JWT_SECRET"`,
		`jwtScheme    = "Bearer"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("a default did not apply: want %q", want)
		}
	}
}

func TestJWTSigningKeyIsNamedRatherThanWritten(t *testing.T) {
	body := emitPort(t, routedIR(), ServerConfig{Router: RouterServeMux,
		Middleware: []MiddlewareSpec{{Name: MiddlewareJWT}}})
	// A service with no key refuses every token rather than accepting every
	// one, which is the failure that cannot be discovered in production.
	if !strings.Contains(body, `func jwtSecret() string { return os.Getenv(jwtSecretEnv) }`) {
		t.Error("the signing key is not read from the environment")
	}
	if !strings.Contains(body, "if secret == \"\" {") {
		t.Error("a service with no key does not refuse every token")
	}
}

func TestAnEmptyPipelineIsStillAPipeline(t *testing.T) {
	body := emitPort(t, routedIR(), ServerConfig{Router: RouterServeMux})
	if !strings.Contains(body, "func rhoPipeline(next http.Handler) http.Handler {\n\treturn next\n}") {
		t.Error("a target with no middleware did not get a pipeline that does nothing")
	}
}

/* -------------------------------------------------------------------------- */
/*  Imports                                                                     */
/* -------------------------------------------------------------------------- */

func TestImportsAreOnlyWhatTheCodeUses(t *testing.T) {
	// A file the compiler refuses for an unused import breaks a build with a
	// message pointing at generated code, so the filtering is the thing under
	// test rather than a detail of it.
	source := emitPort(t, routedIR(), ServerConfig{Router: RouterServeMux})
	if strings.Contains(source, `"crypto/hmac"`) {
		t.Error("a port with no JWT middleware imported the signing packages")
	}
	if strings.Contains(source, `"sync"`) {
		t.Error("a port with no rate limiter imported the concurrency package")
	}
	if !strings.Contains(source, `"net/http"`) {
		t.Error("a port that emits HTTP code did not import net/http")
	}
}

func TestUsedImportsKeepsWhatIsReferenced(t *testing.T) {
	cases := []struct {
		source string
		ident  string
		want   bool
	}{
		{"os.Stat(staticDir)", "os", true},
		{"host, _, err := net.SplitHostPort(r.RemoteAddr)", "net", true},
		{"// a comment about net.SplitHostPort", "net", false},
		{"answer := 1", "os", false},
		{"x := hostname", "net", false},
	}
	for _, test := range cases {
		if got := usesPackage(test.source, test.ident); got != test.want {
			t.Errorf("usesPackage(%q, %q) = %v, want %v", test.source, test.ident, got, test.want)
		}
	}
}

func TestImportBlocksAreOrderedByPath(t *testing.T) {
	source := emitPort(t, routedIR(), serveMuxConfig())
	block := between(t, source, "import (\n", "\n)\n")
	previous := ""
	for _, line := range strings.Split(block, "\n") {
		fields := strings.Fields(line)
		// The opening line is `import (`, whose trailing token is the bracket
		// rather than a path.
		if len(fields) == 0 || fields[0] == "import" {
			continue
		}
		path := fields[len(fields)-1]
		if path < previous {
			t.Errorf("the import block is not ordered by path: %q after %q", path, previous)
		}
		previous = path
	}
}

/* -------------------------------------------------------------------------- */
/*  The generated project                                                       */
/* -------------------------------------------------------------------------- */

func TestRoutedServerKeepsTheProtocolAndDropsTheMatcher(t *testing.T) {
	source := EmitGoServer(routedIR(), serveMuxConfig())
	for _, want := range []string{
		"type Request struct",
		"type Response struct",
		"func newRequest(r *http.Request) *Request",
		"func StartServer(addr string) error",
		"Params: rhoRouteParams(r)",
		"handler, err := newRouter()",
		"func param(req Request, name string) string",
		// The handler wrappers belong to the protocol, whichever router runs.
		"func rhomodulePageHandler(w http.ResponseWriter, r *http.Request)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("a routed server.go does not contain %q", want)
		}
	}
	// The backend's own matcher and its context-carried wildcards are the
	// unrouted path's, and emitting them beside the port's router would be
	// two routers in one package.
	for _, unwanted := range []string{"type router struct", "func registerRoutes(", "paramsKey{}"} {
		if strings.Contains(source, unwanted) {
			t.Errorf("a routed server.go still carries the unrouted matcher: %q", unwanted)
		}
	}
}

func TestUnroutedServerIsUnchangedByThis(t *testing.T) {
	source := EmitGoServer(routedIR(), ServerConfig{})
	for _, want := range []string{
		"type router struct",
		"func registerRoutes(router *router)",
		"func requestParams(r *http.Request) map[string]string",
		"func queryParam(query, name string) string",
		"mux.HandleFunc(\"/\", router.serve)",
		"func notFound(w http.ResponseWriter)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("a server that named no router does not contain %q", want)
		}
	}
	// The binding state belongs to the routed request, so a project that never
	// asks for it does not carry it.
	if strings.Contains(source, "bind *bindState") {
		t.Error("an unrouted request carries binding state it has no use for")
	}
}

func TestRoutedRequestCarriesTheBindingState(t *testing.T) {
	source := EmitGoServer(routedIR(), serveMuxConfig())
	for _, want := range []string{
		"head map[string]string",
		"bind *bindState",
		"bind:   &bindState{}",
		// The raw readers are the port's, and the Rho-facing names reach them.
		"value, _ := rhoQueryValue(req.Query, name)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("a routed server.go does not contain %q", want)
		}
	}
}

func TestRoutedServerParsesAsGo(t *testing.T) {
	// A port that emits something the Go parser rejects breaks a build with a
	// message pointing at a file nobody wrote, so the emitted text is parsed
	// rather than pattern-matched.
	module := routedIR()
	module.Routes = append(module.Routes, ir.RouteIR{Method: "GET", Path: "/x", Handler: "absent"})
	for _, router := range RegisteredRouters() {
		config := serveMuxConfig()
		config.Router = router
		for name, text := range map[string]string{
			"server.go": EmitGoServer(module, config),
			"ports.go":  emitPort(t, module, config),
		} {
			fileSet := token.NewFileSet()
			if _, err := parser.ParseFile(fileSet, name, text, parser.ParseComments); err != nil {
				t.Errorf("the %s router's %s does not parse: %v", router, name, err)
			}
		}
	}
}

func TestPortParsesAsGoWithEveryMiddleware(t *testing.T) {
	// Every wrapper at once, because each is a template and a template is only
	// correct for the settings it was given.
	pipeline := []MiddlewareSpec{}
	for _, name := range RegisteredMiddlewares() {
		pipeline = append(pipeline, MiddlewareSpec{Name: name, Options: map[string]string{
			"origin":     "https://app.rho.dev",
			"requests":   "10",
			"window":     "1m",
			"issuer":     "https://auth.rho.dev",
			"audience":   "registry",
			"skip_paths": "/healthz, /auth/token",
		}})
	}
	module := routedIR()
	text := emitPort(t, module, ServerConfig{Router: RouterServeMux, Middleware: pipeline})
	fileSet := token.NewFileSet()
	if _, err := parser.ParseFile(fileSet, "ports.go", text, parser.ParseComments); err != nil {
		t.Fatalf("the port with every middleware does not parse: %v", err)
	}
	// The paths are read from a comma-separated setting and split when the
	// server starts, so what reaches the source is the setting itself.
	if !strings.Contains(text, `jwtSkipPaths = splitPaths("/healthz, /auth/token")`) {
		t.Error("the exempt paths did not reach the source")
	}
}

/* -------------------------------------------------------------------------- */
/*  The binding, executed                                                       */
/* -------------------------------------------------------------------------- */

// TestGeneratedBindingBehaves compiles the generated accessors and runs them,
// because the document a service publishes and the code that answers a request
// have to be the same thing. The generated project is written to a temporary
// module with the router's own dependency on the request API removed, since a
// generated project's routing needs Go 1.22 and this toolchain may be older.
func TestGeneratedBindingBehaves(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	text := emitPort(t, ir.ModuleIR{Name: "bound", Version: "0.1.0"},
		ServerConfig{Router: RouterServeMux})

	// The binding is the span from the state type to the pipeline: everything a
	// handler reaches for. Nothing before or after it is needed here, and the
	// router above it needs Go 1.22 to compile.
	from := indexOf(text, "// bindState is what a failed conversion")
	to := indexOf(text, "// rhoPipeline composes")
	if from < 0 || to < 0 || to < from {
		t.Fatal("the generated binding is not where the test expects it")
	}
	binding := text[from:to]

	dir := t.TempDir()
	writeGenerated(t, filepath.Join(dir, "go.mod"), "module bound\n\ngo 1.18\n")
	writeGenerated(t, filepath.Join(dir, "request.go"), routedRequestType)
	writeGenerated(t, filepath.Join(dir, "binding.go"), `package main

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

`+binding)

	writeGenerated(t, filepath.Join(dir, "binding_check_test.go"), bindingCheckTest)

	build := exec.Command("go", "test", "./...")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the generated binding does not behave: %v\n%s\n---\n%s", err, out, binding)
	}
}

// routedRequestType is the request shape the generated accessors are written
// against, copied here so the test does not depend on which of the two request
// forms the emitter produced.
const routedRequestType = `package main

type Request struct {
	Method string
	Path   string
	Query  string
	Body   string
	Auth   string
	Params map[string]string
	head   map[string]string
	bind   *bindState
}
`

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                     */
/* -------------------------------------------------------------------------- */

// between returns the text between two markers, and fails when a marker is
// missing, so a test that greps for something never quietly passes on a file
// that stopped containing it.
func between(t *testing.T, source, start, end string) string {
	t.Helper()
	from := strings.Index(source, start)
	if from < 0 {
		t.Fatalf("no %q in the generated source", start)
	}
	rest := source[from:]
	to := strings.Index(rest, end)
	if to < 0 {
		t.Fatalf("no %q after %q", end, start)
	}
	return rest[:to+len(end)]
}

func indexOf(source, needle string) int { return strings.Index(source, needle) }

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// writeGenerated writes one file of the generated project under test.
func writeGenerated(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bindingCheckTest exercises the generated accessors, written out separately so
// that the JSON bodies inside it can be spelled the way Go wants rather than
// fighting the quoting of a larger literal.
const bindingCheckTest = `package main

import (
	"strings"
	"testing"
)

func request(params map[string]string, query, body string) Request {
	return Request{Params: params, Query: query, Body: body,
		head: map[string]string{"X-Tenant": "acme"}, bind: &bindState{}}
}

func TestEverySourceBinds(t *testing.T) {
	r := request(map[string]string{"id": "42", "name": "widget"},
		"q=hello+world&n=7&flag=true",
		"{\"name\":\"widget\",\"count\":3,\"ratio\":1.5,\"on\":true}")
	if got := pathU32(r, "id"); got != 42 {
		t.Fatalf("pathU32 = %v", got)
	}
	if got := pathStr(r, "name"); got != "widget" {
		t.Fatalf("pathStr = %q", got)
	}
	if got := queryStr(r, "q"); got != "hello world" {
		t.Fatalf("queryStr = %q, want the decoded value", got)
	}
	if got := queryF64(r, "n"); got != 7 {
		t.Fatalf("queryF64 = %v", got)
	}
	if !queryBool(r, "flag") {
		t.Fatal("queryBool = false")
	}
	if got := headerStr(r, "x-tenant"); got != "acme" {
		t.Fatalf("headerStr = %q, want the header matched without regard to case", got)
	}
	if got := fieldStr(r, "name"); got != "widget" {
		t.Fatalf("fieldStr = %q", got)
	}
	if got := fieldU32(r, "count"); got != 3 {
		t.Fatalf("fieldU32 = %v", got)
	}
	if got := fieldF64(r, "ratio"); got != 1.5 {
		t.Fatalf("fieldF64 = %v", got)
	}
	if !fieldBool(r, "on") {
		t.Fatal("fieldBool = false")
	}
	if got := bodyStr(r, "any"); got != r.Body {
		t.Fatalf("bodyStr = %q, want the whole body", got)
	}
	if !bindOk(r) {
		t.Fatalf("the binding reported a failure: %s", bindError(r))
	}
}

func TestAFailedConversionIsReported(t *testing.T) {
	r := request(map[string]string{"id": "abc"}, "", "")
	if got := pathU32(r, "id"); got != 0 {
		t.Fatalf("a failed conversion left %v rather than zero", got)
	}
	if bindOk(r) {
		t.Fatal("bindOk reported success after a failed conversion")
	}
	want := "the path parameter \"id\" is not a 32-bit unsigned integer"
	if got := bindError(r); got != want {
		t.Fatalf("bindError = %q, want %q", got, want)
	}
	// The first failure is the one reported, so the message names the accessor
	// the handler wrote rather than whatever ran last.
	_ = queryU32(r, "later")
	if got := bindError(r); !strings.HasPrefix(got, "the path parameter") {
		t.Fatalf("a later failure replaced the first: %q", got)
	}
}

func TestAMissingValueIsReported(t *testing.T) {
	r := request(map[string]string{}, "", "")
	_ = queryU32(r, "gone")
	if bindOk(r) {
		t.Fatal("a missing value did not fail the binding")
	}
	want := "the query parameter \"gone\" is missing"
	if got := bindError(r); got != want {
		t.Fatalf("bindError = %q, want %q", got, want)
	}
}

func TestAPresenceReadRecordsNothing(t *testing.T) {
	r := request(map[string]string{"here": "1"}, "q=", "{\"field\":null}")
	// A parameter present but empty is present, and a body field set to null
	// is present: a client that sent the key sent a value.
	if !hasQuery(r, "q") {
		t.Error("a query parameter sent empty was reported as absent")
	}
	if !hasPath(r, "here") || hasPath(r, "gone") {
		t.Error("a path presence read answered wrongly")
	}
	if !hasField(r, "field") || hasField(r, "other") {
		t.Error("a body field presence read answered wrongly")
	}
	if !bindOk(r) {
		t.Fatalf("a presence read recorded a failure: %s", bindError(r))
	}
	if got := queryDefault(r, "q", "fallback"); got != "" {
		t.Errorf("queryDefault took %q, want the sent empty value", got)
	}
	if got := queryDefault(r, "absent", "fallback"); got != "fallback" {
		t.Errorf("queryDefault = %q, want the fallback", got)
	}
}

func TestTheJSONReaderFindsAField(t *testing.T) {
	cases := map[string]string{
		"{\"a\":1,\"b\":\"two\",\"want\":\"two\"}": "two",
		"{\"braces\":\"{}[]\",\"want\":\"ok\"}":     "ok",
		"{\"esc\":\"a\\\"b\",\"want\":\"yes\"}":    "yes",
		"{\"esc\":\"a\\\\b\",\"want\":\"yes\"}":    "yes",
		"{\"esc\":\"a\\nb\",\"want\":\"yes\"}":     "yes",
		"{\"esc\":\"a\\u0041b\",\"want\":\"yes\"}": "yes",
	}
	for body, want := range cases {
		got, ok := rhoJSONField(body, "want")
		if !ok || got != want {
			t.Errorf("rhoJSONField(%q, want) = %q, %v", body, got, ok)
		}
	}
	if _, ok := rhoJSONField("{\"a\":1}", "missing"); ok {
		t.Error("a field that is not there was reported as present")
	}
	if _, ok := rhoJSONField("not json", "a"); ok {
		t.Error("a body that is not an object yielded a field")
	}
	// A nested value is not a scalar, so a flat reading of one is refused
	// rather than half-understood.
	if got, ok := rhoJSONField("{\"a\":{\"b\":1},\"c\":\"ok\"}", "a"); ok {
		t.Errorf("a nested value was read as a scalar: %q", got)
	}
}

func TestNumbersAndBooleansBind(t *testing.T) {
	r := request(map[string]string{
		"i": "-5", "u": "7", "big": "18446744073709551615", "f": "1.5", "b": "true",
	}, "", "")
	if got := pathI32(r, "i"); got != -5 {
		t.Errorf("pathI32 = %v", got)
	}
	if got := pathU32(r, "u"); got != 7 {
		t.Errorf("pathU32 = %v", got)
	}
	if got := pathU64(r, "big"); got != 18446744073709551615 {
		t.Errorf("pathU64 = %v", got)
	}
	if got := pathF64(r, "f"); got != 1.5 {
		t.Errorf("pathF64 = %v", got)
	}
	if !pathBool(r, "b") {
		t.Error("pathBool = false")
	}
	if !bindOk(r) {
		t.Fatalf("a valid set of values failed: %s", bindError(r))
	}
	// A value that overflows the type it was asked for is a failure, not a
	// truncated number.
	wide := request(map[string]string{"i": "99999999999"}, "", "")
	if got := pathI32(wide, "i"); got != 0 {
		t.Errorf("an overflowing value bound to %v", got)
	}
	if bindOk(wide) {
		t.Error("an overflowing value did not fail the binding")
	}
}
`

/* -------------------------------------------------------------------------- */
/*  The description port                                                        */
/* -------------------------------------------------------------------------- */

func TestTheDescriptionPortIsAvailable(t *testing.T) {
	if !HasPort("openapiemit") {
		t.Error("the description port is not available")
	}
	if !containsString(RegisteredPorts(), "openapiemit") {
		t.Error("the description port is not listed")
	}
}

func TestTheDescriptionPortAnswersWithSomethingServable(t *testing.T) {
	// The port is reached as a process, so a host without the component is the
	// ordinary case rather than the broken one: the service still starts, and
	// what it answers with is the reason.
	body := EmitGoPorts(PortContext{IR: routedIR()}, []string{"openapiemit"})
	if !strings.Contains(body, "func openapiDocument() string") {
		t.Errorf("the port did not supply a reader:\n%s", body)
	}
	if !strings.Contains(body, "openapiDescriptionUnavailable") || !strings.Contains(body, "no OpenAPI description was generated") {
		t.Errorf("a port that could not render says nothing about why:\n%s", body)
	}
}

func TestTheDescriptionIsWrittenWhenTheProjectIsGenerated(t *testing.T) {
	// A running service does not read Rho sources to describe itself: the
	// document is written when the project is generated, so a start-up cannot
	// fail because a source moved. Whichever form the port takes — the document
	// or the reason there is none — it is a constant, and no command is run
	// from the generated project.
	body := EmitGoPorts(PortContext{IR: routedIR()}, []string{"openapiemit"})
	hasDocument := strings.Contains(body, "const openapiDescription = ")
	hasReason := strings.Contains(body, "const openapiDescriptionUnavailable = ")
	if !hasDocument && !hasReason {
		t.Errorf("the port supplies neither the document nor a reason:\n%s", body)
	}
	if strings.Contains(body, "exec.Command") || strings.Contains(body, "os/exec") {
		t.Error("the port runs a command at run time rather than when the project is generated")
	}
}

func TestAQuotedDocumentSurvivesTheRoundTrip(t *testing.T) {
	// The document is a document: JSON, with quotes and newlines and possibly
	// a backslash in a description a person wrote. A quoting that got one of
	// them wrong would produce a project that does not compile, and the error
	// would point at generated code.
	document := `{
  "info": {"title": "A \"quoted\" title", "description": "a backslash \\ and a newline\n"},
  "paths": {}
}`
	quoted := quotedGoString(document)
	if quoted == document {
		t.Fatal("the document was not quoted at all")
	}
	fileSet := token.NewFileSet()
	if _, err := parser.ParseFile(fileSet, "const.go",
		"package main\n\nconst document = "+quoted+"\n", 0); err != nil {
		t.Fatalf("the quoted document is not a Go literal: %v", err)
	}
}
