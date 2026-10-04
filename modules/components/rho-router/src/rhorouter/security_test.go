//
// @file security_test.go
// @brief Tests for how a service says what a caller has to prove.
//
// The reading of credentials is the one place a description depends on a
// project saying something rather than on its code, because a check inside a
// handler can be conditional and a document has to distinguish the two. What is
// tested here is that a declared scheme is read, that an enforced one is
// distinguished from an accepted one, and that a project which said nothing
// says nothing.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorouter

import (
	"os"
	"path/filepath"
	"testing"
)

// descriptor writes a project descriptor and returns the project root.
func descriptor(t *testing.T, target string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"module.yml": manifestText,
		"src/main.rho": `module catalog

route GET "/a" => a

fn a(req: Request) Response {
    return json(200, "a")
}
`,
		DescriptorFileName: "version: 1\noutput_dir: build/generated\nbuild_dir: build\ntargets:\n" + target,
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// serverTarget is a server target carrying whatever the case under test says.
func serverTarget(extra string) string {
	return "  - id: go-server\n    emitter: go\n    kind: server\n    router: serveMux\n" + extra +
		"    files:\n      - path: app.go\n        role: implementation\n"
}

func read(t *testing.T, root string) Spec {
	t.Helper()
	result, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatalf("the project did not read: %v", err)
	}
	if len(result.Diagnostics) != 0 {
		t.Errorf("the project read with problems: %+v", result.Diagnostics)
	}
	return result.Spec
}

func TestAProjectThatSaidNothingIsTakenToAskForNothing(t *testing.T) {
	// A document that demanded a credential nobody enforces would teach a client
	// to send one it has no reason to hold, so silence is silence.
	if got := read(t, descriptor(t, serverTarget(""))).Security; got != nil {
		t.Errorf("a project that declared nothing carries a scheme: %+v", got)
	}
}

func TestADeclaredSchemeIsRead(t *testing.T) {
	spec := read(t, descriptor(t, serverTarget(`    api:
      security: bearer
      security_description: "A publish token, from POST /auth/token."
      public_paths: "/healthz, /a"
`)))
	security := spec.Security
	if security == nil {
		t.Fatal("a declared scheme was not read")
	}
	if security.Scheme != SecurityBearer {
		t.Errorf("scheme = %q", security.Scheme)
	}
	// The description is the one thing a document cannot derive from anything,
	// so it has to survive being read.
	if security.Description != "A publish token, from POST /auth/token." {
		t.Errorf("description = %q", security.Description)
	}
	if len(security.PublicPaths) != 2 {
		t.Errorf("the public paths read as %v", security.PublicPaths)
	}
}

func TestADeclaredSchemeIsAcceptedRatherThanEnforced(t *testing.T) {
	// The service declares the scheme but nothing refuses a request without
	// one, so an operation that only reads a credential may or may not be given
	// it. A document has to say that, and it does with an empty alternative.
	security := read(t, descriptor(t, serverTarget(`    api:
      security: bearer
`))).Security
	if security == nil {
		t.Fatal("a declared scheme was not read")
	}
	if security.Required {
		t.Error("a declared scheme was read as enforced, which it is not")
	}
}

func TestTheJWTWrapperEnforcesACredential(t *testing.T) {
	// A wrapper refuses the request before any handler sees it, so every
	// operation that is not exempt requires one. That is a different claim from
	// what a handler's own check makes, and the document distinguishes them.
	security := read(t, descriptor(t, serverTarget(`    middleware:
      - name: request_id
      - name: jwt
        options:
          issuer: "https://auth.rho.dev"
          skip_paths: "/healthz"
`))).Security
	if security == nil {
		t.Fatal("a wrapped router carries no scheme")
	}
	if !security.Required {
		t.Error("a wrapped router is not read as requiring a credential")
	}
	if security.Scheme != SecurityBearer {
		t.Errorf("scheme = %q", security.Scheme)
	}
	// The exempt paths are the wrapper's own settings, because they are what
	// it refuses to enforce them on.
	if len(security.PublicPaths) != 1 || security.PublicPaths[0] != "/healthz" {
		t.Errorf("the exempt paths read as %v", security.PublicPaths)
	}
}

func TestADeclaredDescriptionWinsOverTheWrapper(t *testing.T) {
	// A project that wrote a description down knows more than a wrapper's name
	// does about where a credential comes from, so its words are the ones kept.
	security := read(t, descriptor(t, serverTarget(`    api:
      security: bearer
      security_description: "A registry publish token."
    middleware:
      - name: jwt
`))).Security
	if security == nil || security.Description != "A registry publish token." {
		t.Errorf("description = %+v", security)
	}
	// The wrapper still says what is enforced, because a description does not
	// change what the service refuses.
	if security == nil || !security.Required {
		t.Errorf("a declared scheme overrode what the wrapper enforces: %+v", security)
	}
}

func TestANonServerTargetIsNotDescribed(t *testing.T) {
	// A target that serves no HTTP API says nothing about credentials, whatever
	// its kind of target is.
	spec := read(t, descriptor(t, `  - id: go
    emitter: go
    kind: cli
    files:
      - path: app.go
        role: implementation
`))
	if spec.Security != nil {
		t.Errorf("a command line target carries a scheme: %+v", spec.Security)
	}
}

func TestAProjectWithNoDescriptorIsAnAnswerRatherThanAFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifestText), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(ReadOptions{Root: root}); err != nil {
		t.Errorf("a project with no descriptor did not read: %v", err)
	}
}

func TestTheChoiceOfTargetIsHonoured(t *testing.T) {
	// A project that serves more than one API needs to say which one a document
	// is about, and the flag is how it says so.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifestText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.rho"),
		[]byte("module catalog\n\nroute GET \"/a\" => a\n\nfn a(req: Request) Response {\n    return json(200, \"a\")\n}\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	content := `version: 1
output_dir: build/generated
build_dir: build
targets:
  - id: internal
    emitter: go
    kind: server
    api:
      security: none
    files:
      - path: app.go
        role: implementation
  - id: public
    emitter: go
    kind: server
    api:
      security: bearer
    files:
      - path: app.go
        role: implementation
`
	if err := os.WriteFile(filepath.Join(root, DescriptorFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Named: the target the caller asked for is the one described.
	named, err := Read(ReadOptions{Root: root, Target: "public"})
	if err != nil {
		t.Fatal(err)
	}
	if named.Spec.Security == nil || named.Spec.Security.Scheme != SecurityBearer {
		t.Errorf("the named target's scheme was not read: %+v", named.Spec.Security)
	}
	// Unnamed: the first, which is what a caller who named nothing means.
	first, err := Read(ReadOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if first.Spec.Security != nil {
		t.Errorf("the first target's declaration was not the one read: %+v", first.Spec.Security)
	}
}

func TestSecurityAsksTheRightQuestions(t *testing.T) {
	security := &Security{PublicPaths: []string{"/healthz"}}
	if !security.Public("/healthz") {
		t.Error("an exempt path is not reported as exempt")
	}
	if security.Public("/a") {
		t.Error("a path that is not exempt is reported as exempt")
	}
	// A service that says nothing is exempt from everything, which is what a
	// path on a service with no declared security means.
	var absent *Security
	if !absent.Public("/anything") {
		t.Error("a service with no declared security is not exempt from everything")
	}
	// And a described scheme gets a name and a default description, so a
	// document never refers to a scheme that has neither.
	described := (&Security{Scheme: SecurityBearer}).Describe()
	if described.Name != "bearerAuth" {
		t.Errorf("name = %q", described.Name)
	}
	if described.Description == "" {
		t.Error("a scheme with no description has no default one")
	}
}
