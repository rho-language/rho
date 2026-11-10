//
// @file main.go
// @brief rho-artifact — pack and verify signed rollouts.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/schema"
)

const manifestName = "artifact.json"
const payloadName = "payload.tar.gz"

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "pack":
		err = pack(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	case "inspect":
		err = inspect(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho artifact:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rho artifact <pack|verify|inspect> [options]")
	os.Exit(2)
}

func pack(args []string) error {
	fs := flag.NewFlagSet("rho artifact pack", flag.ExitOnError)
	name := fs.String("name", "", "artifact name")
	version := fs.String("version", "", "artifact version")
	kind := fs.String("kind", "service", "service or firmware")
	target := fs.String("target", "", "target id")
	entrypoint := fs.String("entrypoint", "", "file to run or flash inside the payload")
	service := fs.String("service", "", "systemd service for a service artifact")
	health := fs.String("health", "/healthz", "health path for a service artifact")
	healthTimeout := fs.Int("health-timeout", 20, "health check seconds")
	keep := fs.Int("keep", 2, "previous versions to keep")
	automatic := fs.Bool("automatic", true, "roll back on a failed health check")
	keyPath := fs.String("key", "", "base64 Ed25519 private key")
	out := fs.String("out", "", "output artifact path")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("pack needs one input directory")
	}
	input := fs.Arg(0)
	if *name == "" || *version == "" || *target == "" || *entrypoint == "" {
		return fmt.Errorf("pack needs name, version, target and entrypoint")
	}
	if *out == "" {
		*out = *name + "-" + *version + ".tar.gz"
	}
	temp, err := os.MkdirTemp("", "rho-artifact-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	payload := filepath.Join(temp, payloadName)
	if err := writePayload(input, payload); err != nil {
		return err
	}
	digest, size, err := fileDigest(payload)
	if err != nil {
		return err
	}
	manifest := schema.ArtifactManifest{
		Name: *name, Version: *version, Kind: *kind, Target: *target,
		Entrypoint: *entrypoint, Service: *service,
		Health:   schema.ArtifactHealth{URL: *health, TimeoutSeconds: *healthTimeout},
		Rollback: schema.ArtifactRollback{Keep: *keep, Automatic: *automatic},
		SHA256:   digest, Size: size,
	}
	if err := schema.ValidateArtifactManifest(manifest); err != nil {
		return err
	}
	if *keyPath != "" {
		key, err := readPrivateKey(*keyPath)
		if err != nil {
			return err
		}
		canonical, err := canonicalManifest(manifest)
		if err != nil {
			return err
		}
		manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, canonical))
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return writeArchive(*out, []archiveEntry{
		{Name: manifestName, Data: append(manifestJSON, '\n'), Mode: 0644},
		{Name: payloadName, Path: payload, Mode: 0644},
	})
}

func verify(args []string) error {
	fs := flag.NewFlagSet("rho artifact verify", flag.ExitOnError)
	pubPath := fs.String("pub", "", "base64 Ed25519 public key")
	keyPath := fs.String("key", "", "alias for -pub")
	target := fs.String("target", "", "require this target")
	extract := fs.String("extract", "", "extract the payload here after verification")
	allowUnsigned := fs.Bool("allow-unsigned", false, "accept an unsigned artifact")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("verify needs one artifact")
	}
	temp, err := os.MkdirTemp("", "rho-artifact-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	manifest, payload, err := unpackArchive(fs.Arg(0), temp)
	if err != nil {
		return err
	}
	if err := schema.ValidateArtifactManifest(manifest); err != nil {
		return err
	}
	if manifest.Signature == "" {
		if !*allowUnsigned {
			return fmt.Errorf("artifact is not signed")
		}
	} else {
		path := *pubPath
		if path == "" {
			path = *keyPath
		}
		if path == "" {
			return fmt.Errorf("a signed artifact needs -pub")
		}
		pub, err := readPublicKey(path)
		if err != nil {
			return err
		}
		signature, err := base64.StdEncoding.DecodeString(manifest.Signature)
		if err != nil {
			return fmt.Errorf("artifact signature is not base64")
		}
		unsigned := manifest
		unsigned.Signature = ""
		canonical, err := canonicalManifest(unsigned)
		if err != nil {
			return err
		}
		if !ed25519.Verify(pub, canonical, signature) {
			return fmt.Errorf("artifact signature does not match")
		}
	}
	if *target != "" && *target != manifest.Target {
		return fmt.Errorf("artifact targets %q, not %q", manifest.Target, *target)
	}
	digest, size, err := fileDigest(payload)
	if err != nil {
		return err
	}
	if digest != manifest.SHA256 || size != manifest.Size {
		return fmt.Errorf("artifact payload checksum does not match the manifest")
	}
	if *extract != "" {
		if err := extractPayload(payload, *extract); err != nil {
			return err
		}
	}
	fmt.Printf("ok %s@%s %s\n", manifest.Name, manifest.Version, manifest.Target)
	return nil
}

func inspect(args []string) error {
	fs := flag.NewFlagSet("rho artifact inspect", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("inspect needs one artifact")
	}
	temp, err := os.MkdirTemp("", "rho-artifact-inspect-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	manifest, _, err := unpackArchive(fs.Arg(0), temp)
	if err != nil {
		return err
	}
	lines := map[string]string{
		"name": manifest.Name, "version": manifest.Version, "kind": manifest.Kind,
		"target": manifest.Target, "entrypoint": manifest.Entrypoint,
		"service": manifest.Service, "health": manifest.Health.URL,
		"health_timeout": fmt.Sprint(manifest.Health.TimeoutSeconds),
		"keep":           fmt.Sprint(manifest.Rollback.Keep),
		"automatic":      fmt.Sprint(manifest.Rollback.Automatic),
		"sha256":         manifest.SHA256, "size": fmt.Sprint(manifest.Size),
		"signed": fmt.Sprint(manifest.Signature != ""),
	}
	keys := make([]string, 0, len(lines))
	for key := range lines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s=%s\n", key, lines[key])
	}
	return nil
}

type archiveEntry struct {
	Name string
	Path string
	Data []byte
	Mode int64
}

func writeArchive(path string, entries []archiveEntry) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tarWriter := tar.NewWriter(gz)
	defer tarWriter.Close()
	for _, entry := range entries {
		data := entry.Data
		if entry.Path != "" {
			data, err = os.ReadFile(entry.Path)
			if err != nil {
				return err
			}
		}
		header := &tar.Header{Name: entry.Name, Mode: entry.Mode, Size: int64(len(data))}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func writePayload(input, out string) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("pack input must be a directory")
	}
	file, err := os.Create(out)
	if err != nil {
		return err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tarWriter := tar.NewWriter(gz)
	defer tarWriter.Close()
	var paths []string
	if err := filepath.Walk(input, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Strings(paths)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(input, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name: filepath.ToSlash(relative), Mode: int64(info.Mode().Perm()),
			Size: int64(len(data)),
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tarWriter.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func unpackArchive(path, dir string) (schema.ArtifactManifest, string, error) {
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
			data, err := io.ReadAll(io.LimitReader(reader, int64(header.Size)+1))
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

func secureExtractPath(root, name string) (string, bool) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", false
	}
	normalized := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(normalized, "/") || filepath.IsAbs(normalized) ||
		filepath.VolumeName(normalized) != "" {
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
		if _, ok := secureExtractPath(dir, header.Name); !ok {
			return fmt.Errorf("payload path escapes the extraction directory: %q", header.Name)
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			continue
		}
		name := filepath.FromSlash(strings.ReplaceAll(header.Name, "\\", "/"))
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

func canonicalManifest(manifest schema.ArtifactManifest) ([]byte, error) {
	manifest.Signature = ""
	return json.Marshal(manifest)
}

func readPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := readBase64(path)
	if err != nil {
		return nil, err
	}
	switch len(data) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(data), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(data), nil
	default:
		return nil, fmt.Errorf("private key must be a 32-byte seed or 64-byte key")
	}
}

func readPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readBase64(path)
	if err != nil {
		return nil, err
	}
	if len(data) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be 32 bytes")
	}
	return ed25519.PublicKey(data), nil
}

func readBase64(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
}
