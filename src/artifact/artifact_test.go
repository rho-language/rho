package artifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/schema"
)

func writeArtifact(t *testing.T, path, payloadPath string, manifest schema.ArtifactManifest, pub ed25519.PublicKey, priv ed25519.PrivateKey) {
	t.Helper()
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	manifest.SHA256 = hex.EncodeToString(sum[:])
	manifest.Size = int64(len(payload))
	canonical, _ := json.Marshal(manifest)
	if priv != nil {
		manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, canonical))
	}
	manifestJSON, _ := json.Marshal(manifest)

	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	for _, entry := range []struct {
		name string
		data []byte
	}{{manifestName, manifestJSON}, {payloadName, payload}} {
		tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0644, Size: int64(len(entry.data))})
		tw.Write(entry.data)
	}
	tw.Close()
	gz.Close()
	_ = pub
}

func TestVerifyPayloadAndTarget(t *testing.T) {
	dir := t.TempDir()
	payload := filepath.Join(dir, "payload")
	os.WriteFile(payload, []byte("firmware"), 0644)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	artifactPath := filepath.Join(dir, "artifact.tar.gz")
	manifest := schema.ArtifactManifest{
		Name: "firmware", Version: "1", Kind: "firmware", Target: "esp32c3", Entrypoint: "app.bin",
	}
	writeArtifact(t, artifactPath, payload, manifest, pub, priv)
	if _, err := Verify(artifactPath, Options{PublicKey: pub, Target: "esp32c3"}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if _, err := Verify(artifactPath, Options{PublicKey: pub, Target: "rp2040"}); err == nil {
		t.Fatal("wrong target was accepted")
	}
	if _, err := Verify(artifactPath, Options{}); err == nil {
		t.Fatal("signed artifact without a key was accepted")
	}
}

// A payload member that walks out of the extraction directory is refused before
// it is written, and the refusal is not a warning that leaves the file behind.
func TestExtractPayloadRefusesEscapingMember(t *testing.T) {
	dir := t.TempDir()
	payload := filepath.Join(t.TempDir(), "payload.tar.gz")
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	body := []byte("owned")
	if err := tw.WriteHeader(&tar.Header{Name: "../escaped", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	err := extractPayload(payload, dir)
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("an escaping payload member was not refused: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escaped")); statErr == nil {
		t.Fatal("a payload member was written outside the extraction directory")
	}
}
