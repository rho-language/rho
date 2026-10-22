//
// @file verify_test.go
// @brief A package nothing vouches for is not installed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/cmd/rho-module/remote"
)

// registryServing is a registry that records the given digest for math@1.0.0 — or
// none, when the digest is empty — and serves the given bytes as the package.
func registryServing(t *testing.T, recorded string, served []byte) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/modules/math":
			document := map[string]any{"name": "math", "versions": []string{"1.0.0"}}
			if recorded != "" {
				document["sha256"] = map[string]string{"1.0.0": recorded}
			}
			_ = json.NewEncoder(w).Encode(document)
		case "/modules/math/1.0.0/rho-pkg":
			_, _ = w.Write(served)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("RHO_HOME", t.TempDir())
	if err := remote.SaveConfig(&remote.Config{Remote: remote.RemoteConfig{URL: server.URL}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
}

// A package whose registry recorded nothing is refused, because there is nothing to
// compare it against.
func TestDownloadRefusesAPackageWithNoRecordedChecksum(t *testing.T) {
	t.Setenv("RHO_ALLOW_UNVERIFIED_PACKAGES", "")
	registryServing(t, "", []byte("whatever is at that address"))

	_, err := DownloadModuleDigest("math@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "recorded no checksum") {
		t.Fatalf("a package with no recorded checksum was accepted: %v", err)
	}
}

// A person who trusts such a registry can say so, and the install says it went
// ahead unverified rather than going ahead silently.
func TestDownloadAcceptsAnUnverifiedPackageWhenToldTo(t *testing.T) {
	t.Setenv("RHO_ALLOW_UNVERIFIED_PACKAGES", "1")
	var said bytes.Buffer
	old := unverifiedNotice
	unverifiedNotice = &said
	t.Cleanup(func() { unverifiedNotice = old })

	registryServing(t, "", archiveWithManifest(t))

	if _, err := DownloadModuleDigest("math@1.0.0"); err != nil {
		t.Fatalf("an unverified package was refused although it was allowed: %v", err)
	}
	if !strings.Contains(said.String(), "without a checksum") {
		t.Fatalf("an unverified install said nothing: %q", said.String())
	}
}

// A registry that answers the checksum request with something that is not a
// document is an error: it is not a way to have the check skipped.
func TestAChecksumDocumentThatCannotBeReadIsAnError(t *testing.T) {
	t.Setenv("RHO_ALLOW_UNVERIFIED_PACKAGES", "1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/modules/math" {
			_, _ = w.Write([]byte("<html>not json</html>"))
			return
		}
		_, _ = w.Write([]byte("package"))
	}))
	defer server.Close()
	t.Setenv("RHO_HOME", t.TempDir())
	if err := remote.SaveConfig(&remote.Config{Remote: remote.RemoteConfig{URL: server.URL}}); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.RecordedSHA256("math", "1.0.0"); err == nil {
		t.Fatal("a checksum document that was not JSON was read as \"no checksum\"")
	}
}

// A recorded value that is not a digest is refused outright.
func TestARecordedChecksumThatIsNotADigestIsRefused(t *testing.T) {
	registryServing(t, "not-a-digest", []byte("package"))
	if _, err := remote.RecordedSHA256("math", "1.0.0"); err == nil {
		t.Fatal("a recorded checksum that is not a SHA-256 was accepted")
	}
}

// What is written to the temporary directory is not at a name anyone can predict.
func TestDownloadedPackageIsNotAtAPredictableName(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	// A symbolic link where the old name would have been: if the download wrote
	// through it, the target would change.
	target := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(target, []byte("untouched"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(scratch, "math-1.0.0.rho-module")); err != nil {
		t.Fatal(err)
	}

	data := archiveWithManifest(t)
	registryServing(t, digestOf(data), data)
	path, _, err := downloadArchive("math", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	if got, _ := os.ReadFile(target); string(got) != "untouched" {
		t.Fatalf("the download wrote through a planted link: %q", got)
	}
	if filepath.Base(path) == "math-1.0.0.rho-module" {
		t.Fatal("the downloaded package is at the predictable name")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the downloaded package is not owner-only: %v %v", info, err)
	}
}

// Only the first @ separates a name from a version.
func TestParseSpecKeepsEverythingAfterTheFirstAt(t *testing.T) {
	name, version := parseSpec("math@1.2.0@extra")
	if name != "math" || version != "1.2.0@extra" {
		t.Fatalf("got %q, %q", name, version)
	}
	if name, version := parseSpec("math"); name != "math" || version != "" {
		t.Fatalf("a bare name: %q, %q", name, version)
	}
}

// archiveWithManifest is a minimal valid package.
func archiveWithManifest(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.rho-module")
	createTestModuleArchive(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
