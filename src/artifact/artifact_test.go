package artifact

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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
