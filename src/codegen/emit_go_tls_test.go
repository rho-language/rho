//
// @file emit_go_tls_test.go
// @brief TLS support in the generated server lifecycle.
//
// A plain `Serve` leaves every telemetry value on the LAN. The lifecycle
// therefore upgrades to `ServeTLS` on the same listener when the deployment
// supplies `RHO_TLS_CERT` and `RHO_TLS_KEY`, and stays plain otherwise so a
// loopback or reverse-proxy deployment is unchanged.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package codegen

import (
	"strings"
	"testing"
)

func TestGeneratedServerSupportsTLS(t *testing.T) {
	lifecycle := goServerLifecycle(ServerConfig{})
	for _, part := range []string{
		"ServeTLS(listener, cert, key)",
		"RHO_TLS_CERT",
		"RHO_TLS_KEY",
		"Serve(listener)",
	} {
		if !strings.Contains(lifecycle, part) {
			t.Errorf("server lifecycle is missing %q", part)
		}
	}
}
// Half a certificate pair is a deployment that meant TLS: the service refuses to
// start rather than serve plain HTTP.
func TestGeneratedServerRefusesHalfATLSPair(t *testing.T) {
	lifecycle := goServerLifecycle(ServerConfig{})
	if !strings.Contains(lifecycle, `(cert == "") != (key == "")`) || !strings.Contains(lifecycle, "must be set together") {
		t.Fatalf("expected the lifecycle to refuse half a TLS pair:\n%s", lifecycle)
	}
}
