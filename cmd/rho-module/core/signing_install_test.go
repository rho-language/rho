//
// @file signing_install_test.go
// @brief A registry that substitutes a package is caught on install.
//
// The unit tests above say what the policy is. This one runs the whole path: a
// registry serves a module, an install pins its signer, and the same registry then
// serves a different package for the next version with a checksum that is correct
// for it — which is exactly what a checksum alone cannot catch.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rho/cmd/rho-module/remote"
)

// substitutingRegistry serves two versions of math. Each has the digest of the
// bytes it serves — a registry that lies about a digest is the easy case — and the
// signature it was given, which for 1.0.1 is whatever the test says.
func substitutingRegistry(t *testing.T, packages map[string][]byte, signatures map[string]Signature) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/modules/math":
			hashes := map[string]string{}
			signed := map[string]map[string]string{}
			versions := []string{}
			for version, data := range packages {
				versions = append(versions, version)
				hashes[version] = digestOf(data)
				if signature, ok := signatures[version]; ok {
					signed[version] = map[string]string{"signer": signature.Signer, "signature": signature.Signature}
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "math", "versions": versions, "sha256": hashes, "signatures": signed,
			})
		case strings.HasPrefix(r.URL.Path, "/modules/math/") && strings.HasSuffix(r.URL.Path, "/rho-pkg"):
			version := strings.Split(r.URL.Path, "/")[3]
			if data, ok := packages[version]; ok {
				_, _ = w.Write(data)
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	if err := remote.SaveConfig(&remote.Config{Remote: remote.RemoteConfig{URL: server.URL}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

// The substitution a checksum cannot see: same registry, a package that is not the
// publisher's, a checksum that matches it. Only the signature gives it away.
func TestARegistryThatSubstitutesAPackageIsCaught(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "")
	t.Setenv("RHO_ALLOW_UNVERIFIED_PACKAGES", "")

	publisher, _ := aKey(t)
	attacker, _ := aKey(t)
	genuine := archiveWithManifest(t)
	malicious := append(append([]byte{}, genuine...), []byte("tampered")...)

	substitutingRegistry(t,
		map[string][]byte{"1.0.0": genuine, "1.0.1": malicious},
		map[string]Signature{
			"1.0.0": SignPackage(publisher, "math", "1.0.0", digestOf(genuine)),
			// The registry has no access to the publisher's key, so the best it can
			// do for 1.0.1 is sign with its own — or send none.
			"1.0.1": SignPackage(attacker, "math", "1.0.1", digestOf(malicious)),
		})

	// The first install pins the publisher.
	if _, err := DownloadModuleDigest("math@1.0.0"); err != nil {
		t.Fatalf("the genuine package was refused: %v", err)
	}
	// The next version's checksum is correct for the bytes served, and it is
	// still refused.
	_, err := DownloadModuleDigest("math@1.0.1")
	if err == nil || !strings.Contains(err.Error(), "pinned to a different signer") {
		t.Fatalf("a substituted package was installed: %v", err)
	}
}

// And the quieter attack: the registry drops the signature altogether.
func TestARegistryThatDropsTheSignatureIsCaught(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	t.Setenv("RHO_REQUIRE_SIGNATURES", "")
	publisher, _ := aKey(t)
	genuine := archiveWithManifest(t)
	malicious := append(append([]byte{}, genuine...), []byte("tampered")...)

	substitutingRegistry(t,
		map[string][]byte{"1.0.0": genuine, "1.0.1": malicious},
		map[string]Signature{"1.0.0": SignPackage(publisher, "math", "1.0.0", digestOf(genuine))})

	if _, err := DownloadModuleDigest("math@1.0.0"); err != nil {
		t.Fatalf("the genuine package was refused: %v", err)
	}
	_, err := DownloadModuleDigest("math@1.0.1")
	if err == nil || !strings.Contains(err.Error(), "lost its signature") {
		t.Fatalf("a package stripped of its signature was installed: %v", err)
	}
}
