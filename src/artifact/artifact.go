//
// @file artifact.go
// @brief Verify a signed rollout artifact before it is staged or flashed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package artifact

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rho/src/schema"
)

const manifestName = "artifact.json"
const payloadName = "payload.tar.gz"

// Options are the checks a caller requires beyond the schema.
type Options struct {
	PublicKey     ed25519.PublicKey
	AllowUnsigned bool
	Target        string
	ExtractDir    string
}

// Read returns the manifest from an artifact, without verifying its signature.
func Read(path string) (schema.ArtifactManifest, error) {
	temp, err := os.MkdirTemp("", "rho-artifact-read-")
	if err != nil {
		return schema.ArtifactManifest{}, err
	}
	defer os.RemoveAll(temp)
	manifest, _, err := unpack(path, temp)
	return manifest, err
}

// Verify checks the schema, signature, checksum and optional target, and
// optionally extracts the payload.
func Verify(path string, options Options) (schema.ArtifactManifest, error) {
	temp, err := os.MkdirTemp("", "rho-artifact-verify-")
	if err != nil {
		return schema.ArtifactManifest{}, err
	}
	defer os.RemoveAll(temp)
	manifest, payload, err := unpack(path, temp)
	if err != nil {
		return manifest, err
	}
	if err := schema.ValidateArtifactManifest(manifest); err != nil {
		return manifest, err
	}
	if manifest.Signature == "" {
		if !options.AllowUnsigned {
			return manifest, fmt.Errorf("artifact is not signed")
		}
	} else {
		if len(options.PublicKey) == 0 {
			return manifest, fmt.Errorf("a signed artifact needs a public key")
		}
		signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
		if err != nil {
			return manifest, fmt.Errorf("artifact signature is not base64")
		}
		unsigned := manifest
		unsigned.Signature = ""
		canonical, err := json.Marshal(unsigned)
		if err != nil {
			return manifest, err
		}
		if !ed25519.Verify(options.PublicKey, canonical, signature) {
			return manifest, fmt.Errorf("artifact signature does not match")
		}
	}
	if options.Target != "" && options.Target != manifest.Target {
		return manifest, fmt.Errorf("artifact targets %q, not %q", manifest.Target, options.Target)
	}
	digest, size, err := fileDigest(payload)
	if err != nil {
		return manifest, err
	}
	if digest != manifest.SHA256 || size != manifest.Size {
		return manifest, fmt.Errorf("artifact payload checksum does not match the manifest")
	}
	if options.ExtractDir != "" {
		if err := extractPayload(payload, options.ExtractDir); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}

// PublicKey reads a base64 Ed25519 public key.
func PublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("public key is not base64")
	}
	if len(data) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be 32 bytes")
	}
	return ed25519.PublicKey(data), nil
}

func unpack(path, dir string) (schema.ArtifactManifest, string, error) {
	manifest := schema.ArtifactManifest{}
	payload := ""
	file, err := os.Open(path)
	if err != nil {
		return manifest, "", err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return manifest, "", err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return manifest, "", err
		}
		target := filepath.Join(dir, filepath.Base(header.Name))
		switch header.Name {
		case manifestName:
			data, err := io.ReadAll(io.LimitReader(reader, 1<<20))
			if err != nil {
				return manifest, "", err
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return manifest, "", fmt.Errorf("artifact manifest is not JSON")
			}
		case payloadName:
			data, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
			if err != nil {
				return manifest, "", err
			}
			if err := os.WriteFile(target, data, 0644); err != nil {
				return manifest, "", err
			}
			payload = target
		}
	}
	if payload == "" {
		return manifest, "", fmt.Errorf("artifact has no payload")
	}
	return manifest, payload, nil
}

func extractPayload(payload, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.Open(payload)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	root, _ := filepath.Abs(dir)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(header.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("payload path escapes the extraction directory")
		}
		target := filepath.Join(root, clean)
		if !strings.HasPrefix(target, root+string(os.PathSeparator)) && target != root {
			return fmt.Errorf("payload path escapes the extraction directory")
		}
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, os.FileMode(header.Mode)); err != nil {
			return err
		}
	}
	return nil
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}
