package types

import (
	"strings"
	"testing"
)

// A local named like a C type would be compiled into the type where it is used,
// so it is refused with the reason rather than miscompiled.
func TestALocalNamedLikeACTypeIsRefused(t *testing.T) {
	for _, name := range []string{"signed", "short", "long", "int"} {
		if !backendReservedName(name) {
			t.Fatalf("%s is not reserved", name)
		}
		if reason := backendReservedReason(name); !strings.Contains(reason, "choose another name") {
			t.Fatalf("the reason for %s says nothing useful: %s", name, reason)
		}
	}
	if reason := backendReservedReason("package"); !strings.Contains(reason, "Go") {
		t.Fatalf("the reason for package does not say Go: %s", reason)
	}
	for _, name := range []string{"signed_in", "count", "shorter", "packages", "mapping"} {
		if backendReservedName(name) {
			t.Fatalf("%s is refused", name)
		}
	}
}
