//
// @file artifact.go
// @brief Verify a signed rollout artifact before it is staged or flashed.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
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
			if err := os.MkdirAll(dir, 0755); err != nil {
				return manifest, "", err
			}
			// The payload name is a constant, not the archive's spelling of it.
			target := filepath.Join(dir, payloadName)
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

// secureExtractPath resolves an archive member under root. It refuses an
// absolute path, a Windows drive or UNC path, and a component that walks to the
// parent; the final relative check catches a spelling that gets past those.
func secureExtractPath(root, name string) (string, bool) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", false
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || pathIsAbs(normalized) {
		return "", false
	}
	clean := filepath.Clean(normalized)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	target := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return target, true
}

// pathIsAbs is filepath.IsAbs plus the volume test, so a Windows path in a tar
// written on another host is refused on every host.
func pathIsAbs(name string) bool {
	return filepath.IsAbs(name) || filepath.VolumeName(name) != ""
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
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(strings.ReplaceAll(header.Name, "\\", "/"))
		if _, ok := secureExtractPath(dir, header.Name); !ok {
			return fmt.Errorf("payload path escapes the extraction directory: %q", header.Name)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			// A link is a second path the containment check never saw.
			continue
		}
		if header.FileInfo().IsDir() {
			if err := root.MkdirAll(name, 0755); err != nil {
				return err
			}
			continue
		}
		if err := root.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil {
			return err
		}
		out, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY,
			os.FileMode(header.Mode).Perm())
		if err != nil {
			return err
		}
		if _, err := out.Write(data); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
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
