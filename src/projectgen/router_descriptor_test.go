//
// @file router_descriptor_test.go
// @brief Tests for routing and the middleware pipeline in a project descriptor.
//
// A descriptor is a project's declaration of how its service routes. The
// mistakes that matter here are the ones a build would otherwise discover late:
// a router on a target that is not a server, a pipeline that names a wrapper
// nobody supplies, a middleware named twice, an option that is not a string.
// Each of those is a message at generate time rather than a Go build failure
// pointing at generated code.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import (
	"sort"
	"strings"
	"testing"

	"rho/src/codegen"
	"rho/src/schema"
)

/* -------------------------------------------------------------------------- */
/*  Fixtures                                                                    */
/* -------------------------------------------------------------------------- */

// serverDescriptor wraps a target's keys in a descriptor.
func serverDescriptor(target string) []byte {
	return []byte("version: 1\noutput_dir: build/generated\nbuild_dir: build\ntargets:\n" + target)
}

// routedTarget is a server target that routes through the standard library with
// a pipeline, which is the shape a real service declares.
const routedTarget = `  - id: go-server
    emitter: go
    kind: server
    router: serveMux
    middleware:
      - name: request_id
      - name: cors
        options:
          origin: "https://app.rho.dev"
    files:
      - path: app.go
        role: implementation
      - path: server.go
        role: server
      - path: go.mod
        role: build
`

/* -------------------------------------------------------------------------- */
/*  Reading                                                                     */
/* -------------------------------------------------------------------------- */

func TestDescriptorReadsTheRouterAndThePipeline(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(routedTarget))
	if err != nil {
		t.Fatalf("a descriptor with a router did not parse: %v", err)
	}
	if len(descriptor.Targets) != 1 {
		t.Fatalf("the descriptor carries %d targets", len(descriptor.Targets))
	}
	target := descriptor.Targets[0]
	if target.Router != codegen.RouterServeMux {
		t.Errorf("router = %q", target.Router)
	}
	if len(target.Middleware) != 2 {
		t.Fatalf("the pipeline carries %d wrappers, want 2", len(target.Middleware))
	}
	if target.Middleware[0].Name != "request_id" {
		t.Errorf("the first wrapper is %q; the order is the order it runs in", target.Middleware[0].Name)
	}
	if target.Middleware[1].Options["origin"] != "https://app.rho.dev" {
		t.Errorf("a middleware option did not read: %v", target.Middleware[1].Options)
	}
}

func TestServerConfigCarriesTheDescriptorThrough(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(routedTarget))
	if err != nil {
		t.Fatal(err)
	}
	config := ServerConfigFor(descriptor.Targets[0])
	if config.EffectiveRouter() != codegen.RouterServeMux {
		t.Errorf("the backend was handed router %q", config.EffectiveRouter())
	}
	if !config.Routed() {
		t.Error("a target that named a router is not routed")
	}
	if len(config.Middleware) != 2 {
		t.Errorf("the backend was handed %d wrappers", len(config.Middleware))
	}
}

func TestATargetThatNamesNoRouterIsNotRouted(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(`  - id: go-server
    emitter: go
    kind: server
    files:
      - path: app.go
        role: implementation
`))
	if err != nil {
		t.Fatal(err)
	}
	config := ServerConfigFor(descriptor.Targets[0])
	if config.Routed() {
		t.Error("a target that named no router is routed")
	}
	// The default is still the standard library, which is what a target asking
	// for the port without naming a router gets.
	if config.EffectiveRouter() != codegen.DefaultRouter {
		t.Errorf("the default router is %q", config.EffectiveRouter())
	}
}

/* -------------------------------------------------------------------------- */
/*  The routing port is implied                                                 */
/* -------------------------------------------------------------------------- */

func TestNamingARouterAsksForTheRoutingPort(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(routedTarget))
	if err != nil {
		t.Fatal(err)
	}
	ports := descriptor.Targets[0].EffectivePorts()
	// One rule rather than two: a target cannot declare a router and forget
	// the port that supplies it.
	if len(ports) != 1 || ports[0] != RoutingPortName {
		t.Errorf("a target that named a router asked for %v", ports)
	}
}

func TestComposingMiddlewareAlsoAsksForTheRoutingPort(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(routedTarget))
	if err != nil {
		t.Fatal(err)
	}
	// The pipeline is meaningless without the router it wraps, so the two come
	// from the same declaration.
	if !containsString(descriptor.Targets[0].EffectivePorts(), RoutingPortName) {
		t.Error("a pipeline did not pull in the routing port")
	}
}

func TestDeclaringTheRoutingPortDirectlyIsNotADuplicate(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(`  - id: go-server
    emitter: go
    kind: server
    router: chi
    ports:
      - httproute
      - logemit
    files:
      - path: app.go
        role: implementation
`))
	if err != nil {
		t.Fatal(err)
	}
	ports := descriptor.Targets[0].EffectivePorts()
	// Naming the port as well is not a second copy of it, which would be
	// written into ports.go twice.
	count := 0
	for _, name := range ports {
		if name == RoutingPortName {
			count++
		}
	}
	if count != 1 {
		t.Errorf("the routing port was requested %d times: %v", count, ports)
	}
	if !containsString(ports, "logemit") {
		t.Errorf("a declared port was dropped: %v", ports)
	}
}

func TestAnAdapterWritesItsOwnRequirement(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(`  - id: go-server
    emitter: go
    kind: server
    router: chi
    files:
      - path: app.go
        role: implementation
`))
	if err != nil {
		t.Fatal(err)
	}
	// A project that chooses an adapter should not also have to remember the
	// module version, which is the detail most likely to be wrong.
	requirement := descriptor.Targets[0].RouterRequirement()
	if !strings.HasPrefix(requirement, "github.com/go-chi/chi") {
		t.Errorf("the chi adapter's requirement is %q", requirement)
	}
	if got := descriptor.Targets[0].RouterRequirement(); got == "" {
		t.Error("no requirement for the adapter")
	}
}

func TestTheStandardRouterWritesNoRequirement(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(routedTarget))
	if err != nil {
		t.Fatal(err)
	}
	if got := descriptor.Targets[0].RouterRequirement(); got != "" {
		t.Errorf("the standard library router recorded a requirement: %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Refusals                                                                    */
/* -------------------------------------------------------------------------- */

func TestDescriptorRefusesWhatItCannotHonour(t *testing.T) {
	cases := []struct {
		name   string
		target string
		wants  string
	}{
		{
			name: "a router that does not exist",
			target: `  - id: go-server
    emitter: go
    kind: server
    router: kratos
    files:
      - path: app.go
        role: implementation
`,
			wants: "router",
		},
		{
			name: "a router on a target that is not a server",
			target: `  - id: go
    emitter: go
    kind: cli
    router: serveMux
    files:
      - path: app.go
        role: implementation
`,
			wants: "kind",
		},
		{
			name: "a pipeline with no router to wrap",
			target: `  - id: go-server
    emitter: go
    kind: server
    middleware:
      - name: cors
    files:
      - path: app.go
        role: implementation
`,
			wants: "names no router",
		},
		{
			name: "a wrapper that does not exist",
			target: `  - id: go-server
    emitter: go
    kind: server
    router: serveMux
    middleware:
      - name: teleport
    files:
      - path: app.go
        role: implementation
`,
			wants: "middleware",
		},
		{
			name: "the same wrapper twice",
			target: `  - id: go-server
    emitter: go
    kind: server
    router: serveMux
    middleware:
      - name: cors
      - name: cors
    files:
      - path: app.go
        role: implementation
`,
			wants: "more than once",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseDescriptor(serverDescriptor(test.target))
			if err == nil {
				t.Fatal("the descriptor parsed, so the mistake is only a Go build failure later")
			}
			if !strings.Contains(err.Error(), test.wants) {
				t.Errorf("the message does not say what is wrong: %v", err)
			}
		})
	}
}

/* -------------------------------------------------------------------------- */
/*  The vocabularies agree                                                      */
/* -------------------------------------------------------------------------- */

func TestDescriptorAndBackendAgreeOnTheVocabulary(t *testing.T) {
	// Each side declares what it accepts, because each is answering for itself:
	// the schema validates a document, and the emitter writes the code. The two
	// lists are written once each, and this is the check that they match, so a
	// new wrapper is one line in two places rather than a silent gap.
	if got, want := sortedCopy(codegen.RegisteredRouters()), validRouters(); !equalStrings(got, want) {
		t.Errorf("the routers differ:\\n backend: %v\\n schema:   %v", got, want)
	}
	if got, want := codegen.RegisteredMiddlewares(), validMiddlewares(); !equalStrings(got, want) {
		t.Errorf("the middleware differ:\\n backend: %v\\n schema:   %v", got, want)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// validRouters and validMiddlewares are the vocabularies the schema accepts,
// read from the schema rather than copied, so the check above compares the two
// lists and not a test's memory of them.
func validRouters() []string { return sortedCopy(schema.ValidRouters) }

func validMiddlewares() []string { return sortedCopy(schema.ValidMiddlewares) }

// sortedCopy returns a copy in order, because the two vocabularies are
// declared in the order a document reads them and the order the backend finds
// them in a map, and the comparison is about the contents.
func sortedCopy(values []string) []string {
	out := make([]string, len(values))
	copy(out, values)
	sort.Strings(out)
	return out
}

/* -------------------------------------------------------------------------- */
/*  The api section                                                             */
/* -------------------------------------------------------------------------- */

func TestDescriptorReadsWhatTheServiceSaysAboutItself(t *testing.T) {
	descriptor, err := ParseDescriptor(serverDescriptor(`  - id: go-server
    emitter: go
    kind: server
    router: serveMux
    api:
      security: bearer
      security_description: "A publish token, from POST /auth/token."
      public_paths: "/healthz, /"
    files:
      - path: app.go
        role: implementation
`))
	if err != nil {
		t.Fatalf("a descriptor with an api section did not parse: %v", err)
	}
	// The section is read by the component that writes a description, not by
	// the generator, so the generator validates it and otherwise leaves it be.
	// What the test here pins is that the generator accepts a shape the
	// component reads, which is the two halves agreeing on one spelling.
	if len(descriptor.Targets) != 1 {
		t.Fatalf("the descriptor carries %d targets", len(descriptor.Targets))
	}
}

func TestDescriptorRefusesASchemeItCannotDescribe(t *testing.T) {
	// A scheme nothing here can describe would be written into a document as
	// something it is not, so it is refused where a person can see the refusal.
	_, err := ParseDescriptor(serverDescriptor(`  - id: go-server
    emitter: go
    kind: server
    router: serveMux
    api:
      security: kerberos
    files:
      - path: app.go
        role: implementation
`))
	if err == nil {
		t.Fatal("a scheme that cannot be described was accepted, so a document would state it wrongly")
	}
}

func TestTheVocabularyAgreesAcrossTheThreeHomes(t *testing.T) {
	// The scheme names are written in the schema (which validates a document)
	// and in the reading (which writes one). They have to be the same list, or
	// a project would be accepted by one and refused by the other.
	if !equalStrings(sortedCopy(schema.ValidSecuritySchemes), []string{"bearer", "none"}) {
		t.Errorf("the schema's schemes are %v", schema.ValidSecuritySchemes)
	}
	for _, scheme := range schema.ValidSecuritySchemes {
		switch scheme {
		case "bearer":
		case "none":
		default:
			t.Errorf("the schema offers %q, which the reading does not know", scheme)
		}
	}
	// The middleware the reading treats as enforcing a credential is named in
	// the schema as well, because that is what a project composes.
	if schema.MiddlewareJWT == "" {
		t.Error("the schema does not name the wrapper that enforces a token")
	}
}
