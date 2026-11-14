package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// A module that declares its own GET /healthz answers the probe itself: the
// built-in one is left out, since a path registered twice stops the service.
func TestADeclaredHealthzReplacesTheBuiltInOne(t *testing.T) {
	own := ir.ModuleIR{Routes: []ir.RouteIR{{Method: "GET", Path: "/healthz", Handler: "health"}}}
	none := ir.ModuleIR{Routes: []ir.RouteIR{{Method: "GET", Path: "/", Handler: "home"}}}
	for name, emit := range map[string]func(ir.ModuleIR) string{"mux": goMountRoutes, "router": goRegisterRoutes} {
		if got := strings.Count(emit(own), `"/healthz"`); got != 1 {
			t.Fatalf("%s: a declared /healthz is registered %d times", name, got)
		}
		if got := strings.Count(emit(none), `"/healthz"`); got != 1 {
			t.Fatalf("%s: the built-in probe is registered %d times", name, got)
		}
	}
}
