package codegen

import (
	"strings"
	"testing"
)

const shippedPort = `package main

import (
	"fmt"
	str "strings"
)

// hello is a platform's host primitive.
func hello() string { return fmt.Sprint(str.ToUpper("hi")) }
`

func TestSplitPortSourceSeparatesImportsFromBody(t *testing.T) {
	imports, body, err := SplitPortSource("hello", shippedPort)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(imports, ",") != `"fmt",str "strings"` {
		t.Errorf("imports = %v", imports)
	}
	if !strings.HasPrefix(body, "// hello is") || strings.Contains(body, "import") {
		t.Errorf("body = %q", body)
	}
	if _, _, err := SplitPortSource("x", "package other\n"); err == nil {
		t.Error("a port outside package main must be refused")
	}
}

func TestEmitGoPortsMergesAPlatformPort(t *testing.T) {
	out := EmitGoPorts(PortContext{ModuleName: "app", PlatformPorts: map[string]string{"hello": shippedPort}}, []string{"hello"})
	for _, want := range []string{`"fmt"`, `str "strings"`, "func hello() string"} {
		if !strings.Contains(out, want) {
			t.Errorf("merged port file is missing %q", want)
		}
	}
	// The toolchain carries no gateway or telemetry port of its own.
	for _, name := range []string{"gateway", "telemetry"} {
		if HasPort(name) {
			t.Errorf("the toolchain still carries the %q port", name)
		}
	}
}
