package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func writeKeyPair(t *testing.T, dir string) (string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	privPath := filepath.Join(dir, "build.key")
	pubPath := filepath.Join(dir, "build.pub")
	if err := os.WriteFile(privPath, []byte(base64.StdEncoding.EncodeToString(priv)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pubPath, []byte(base64.StdEncoding.EncodeToString(pub)), 0644); err != nil {
		t.Fatal(err)
	}
	return privPath, pubPath
}

func TestPackVerifyExtract(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "build")
	if err := os.MkdirAll(input, 0755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(input, "aquarium")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\necho aquarium\n"), 0755); err != nil {
		t.Fatal(err)
	}
	priv, pub := writeKeyPair(t, dir)
	artifact := filepath.Join(dir, "aquarium-0.4.0.tar.gz")
	if err := pack([]string{
		"-name", "aquarium", "-version", "0.4.0", "-kind", "service",
		"-target", "host-linux-amd64", "-entrypoint", "aquarium",
		"-service", "aquarium", "-health", "/healthz", "-key", priv,
		"-out", artifact, input,
	}); err != nil {
		t.Fatalf("pack failed: %v", err)
	}
	if err := verify([]string{"-pub", pub, "-target", "host-linux-amd64", artifact}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	extracted := filepath.Join(dir, "extracted")
	if err := verify([]string{"-pub", pub, "-extract", extracted, artifact}); err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(extracted, "aquarium")); err != nil {
		t.Fatalf("entrypoint was not extracted: %v", err)
	}
	if err := verify([]string{"-pub", pub, "-target", "esp32s3", artifact}); err == nil {
		t.Fatal("wrong target was accepted")
	}
}

func TestUnsignedAndTampered(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "build")
	os.MkdirAll(input, 0755)
	os.WriteFile(filepath.Join(input, "app"), []byte("binary"), 0755)
	artifact := filepath.Join(dir, "app.tar.gz")
	if err := pack([]string{
		"-name", "app", "-version", "1", "-kind", "service",
		"-target", "host", "-entrypoint", "app", "-service", "app",
		"-out", artifact, input,
	}); err != nil {
		t.Fatal(err)
	}
	if err := verify([]string{artifact}); err == nil {
		t.Fatal("unsigned artifact was accepted without -allow-unsigned")
	}
	if err := verify([]string{"-allow-unsigned", artifact}); err != nil {
		t.Fatalf("unsigned artifact was refused with the explicit flag: %v", err)
	}
}

// The archive's member names are not paths this tool writes to: the manifest and
// payload are recognized by their exact names and the payload is written under
// the constant payload name. A member that walks out is ignored, not followed.
func TestUnpackArchiveDoesNotWriteAnEscapingMember(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "artifact.rho")
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	entries := []struct {
		name string
		body string
	}{
		{"../escape", "owned"},
		{"artifact.json", "{}"},
		{"payload.tar.gz", "payload"},
	}
	for _, entry := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if _, _, err := unpackArchive(archive, out); err != nil {
		t.Fatalf("a valid artifact with an ignored member failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escape")); err == nil {
		t.Fatal("an archive member was written outside the extraction directory")
	}
}
