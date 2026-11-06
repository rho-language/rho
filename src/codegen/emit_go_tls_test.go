//
// @file emit_go_tls_test.go
// @brief TLS support in the generated server lifecycle.
//
// A plain `ListenAndServe` leaves every telemetry value on the LAN. The
// lifecycle therefore upgrades to `ListenAndServeTLS` when the deployment
// supplies `RHO_TLS_CERT` and `RHO_TLS_KEY`, and stays plain otherwise so a
// loopback or reverse-proxy deployment is unchanged.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"
)

func TestGeneratedServerSupportsTLS(t *testing.T) {
	lifecycle := goServerLifecycle(ServerConfig{})
	for _, part := range []string{
		"ListenAndServeTLS(cert, key)",
		"RHO_TLS_CERT",
		"RHO_TLS_KEY",
		"ListenAndServe()",
	} {
		if !strings.Contains(lifecycle, part) {
			t.Errorf("server lifecycle is missing %q", part)
		}
	}
}