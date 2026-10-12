package core

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/cmd/rho-module/remote"
)

/* -------------------------------------------------------------------------- */
/*  Helper: Create Test Archive                                                */
/* -------------------------------------------------------------------------- */

func createTestModuleArchive(path string) {
	f, _ := os.Create(path)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	tw.WriteHeader(&tar.Header{
		Name: "module.yml",
		Mode: 0644,
		Size: int64(len("module: test")),
	})
	tw.Write([]byte("module: test"))

	tw.WriteHeader(&tar.Header{
		Name: "src/lib.rho",
		Mode: 0644,
		Size: int64(len("module test")),
	})
	tw.Write([]byte("module test"))

	tw.Close()
	gz.Close()
	f.Close()
}

/* -------------------------------------------------------------------------- */
/*  Test: Validate Archive                                                     */
/* -------------------------------------------------------------------------- */

func TestValidateArchive(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "test.rho-module")

	createTestModuleArchive(archive)

	if err := validateArchive(archive); err != nil {
		t.Fatalf("validateArchive failed: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Missing module.yml                                                   */
/* -------------------------------------------------------------------------- */

func TestValidateArchiveMissingModule(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "bad.rho-module")

	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	tw.WriteHeader(&tar.Header{
		Name: "src/lib.rho",
		Mode: 0644,
		Size: int64(len("module test")),
	})
	tw.Write([]byte("module test"))

	tw.Close()
	gz.Close()
	f.Close()

	if err := validateArchive(archive); err == nil {
		t.Fatalf("expected error for missing module.yml")
	}
}

// A package that does not agree with the digest the registry recorded is
// refused before it is cached: installing it would be installing whatever is at
// that address, whatever it is.
func TestDownloadRefusesATamperedPackage(t *testing.T) {
	// The registry records a digest, and serves bytes that are not those.
	recorded := sha256.Sum256([]byte("the package that was published"))
	served := []byte("a different package entirely")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/modules/math":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":     "math",
				"versions": []string{"1.0.0"},
				"sha256":   map[string]string{"1.0.0": hex.EncodeToString(recorded[:])},
			})
		case "/modules/math/1.0.0/rho-pkg":
			_, _ = w.Write(served)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("RHO_HOME", t.TempDir())
	if err := remote.SaveConfig(&remote.Config{Remote: remote.RemoteConfig{URL: server.URL}}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	_, err := DownloadModuleDigest("math@1.0.0")
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a tampered package should be refused, got %v", err)
	}
}
