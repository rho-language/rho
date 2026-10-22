//
// @file checksum_test.go
// @brief A toolchain that is not the vendor's file is not unpacked.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/sysdep"
)

// aToolchain is a gzipped tar holding one program, and its digest.
func aToolchain(t *testing.T, extra ...tar.Header) ([]byte, string) {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho thing 1.0\n")
	if err := writer.WriteHeader(&tar.Header{Name: "thing-1.0/bin/thing", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	writer.Write(body)
	for _, header := range extra {
		h := header
		if err := writer.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		writer.Write(make([]byte, h.Size))
	}
	writer.Close()
	gz.Close()
	sum := sha256.Sum256(buffer.Bytes())
	return buffer.Bytes(), hex.EncodeToString(sum[:])
}

// serving answers every request with the bytes.
func serving(t *testing.T, data []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/thing.tar.gz"
}

func aDep(url, digest string) sysdep.ExecutableDep {
	return sysdep.ExecutableDep{
		Name: "thing",
		Fetch: &sysdep.FetchRecipe{
			URL: url, Archive: "tar.gz", Dir: "thing-1.0", Binary: "bin/thing", Version: "1.0", SHA256: digest,
		},
	}
}

// inAHome points the Rho home, which is where a fetched toolchain goes, at a
// directory of its own: the home is read from RHO_HOME.
func inAHome(t *testing.T) {
	t.Helper()
	t.Setenv("RHO_HOME", t.TempDir())
}

// A recipe with no digest is refused before anything is downloaded.
func TestARecipeWithNoDigestIsRefusedBeforeDownloading(t *testing.T) {
	inAHome(t)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()

	_, err := Fetch(aDep(server.URL, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "no sha256") {
		t.Fatalf("a recipe with no digest was fetched: %v", err)
	}
	if requests != 0 {
		t.Errorf("the archive was requested %d times before the recipe was refused", requests)
	}
}

// A recipe whose digest is not a digest is refused as that, not as a mismatch.
func TestARecipeWithAMalformedDigestIsRefused(t *testing.T) {
	inAHome(t)
	_, err := Fetch(aDep("http://127.0.0.1:1/x", "not-a-digest"), nil)
	if err == nil || !strings.Contains(err.Error(), "not a SHA-256") {
		t.Fatalf("a malformed digest was accepted: %v", err)
	}
}

// A download that is not the file the recipe names is not unpacked, and leaves
// nothing behind that looks like the start of an installation.
func TestAMismatchedDownloadIsNotUnpacked(t *testing.T) {
	inAHome(t)
	genuine, digest := aToolchain(t)
	swapped := append(append([]byte{}, genuine...), 0)
	url := serving(t, swapped)

	_, err := Fetch(aDep(url, digest), nil)
	if err == nil || !strings.Contains(err.Error(), "is not the file its recipe names") {
		t.Fatalf("a swapped download was accepted: %v", err)
	}
	if _, statErr := os.Stat(FetchedDir("thing", "1.0")); statErr == nil {
		t.Error("a rejected download left its directory behind")
	}
}

// And the genuine file, with its digest, is unpacked and found.
func TestAMatchingDownloadIsUnpacked(t *testing.T) {
	inAHome(t)
	genuine, digest := aToolchain(t)
	path, err := Fetch(aDep(serving(t, genuine), strings.ToUpper(digest)), nil)
	if err != nil {
		t.Fatalf("the genuine archive was refused: %v", err)
	}
	if filepath.Base(path) != "thing" {
		t.Errorf("the program was found at %q", path)
	}
}
