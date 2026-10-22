//
// @file unpack_test.go
// @brief What the keeper's own unpackers refuse, whatever the digest said.
//
// The digest says an archive is the vendor's. It does not say the vendor's archive
// is safe to expand, and the unpackers are the part that has to hold when it is not.
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
	"archive/zip"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarGz writes a gzipped tar to a file and answers its path.
func tarGz(t *testing.T, headers []tar.Header, bodies []string) string {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	for index, header := range headers {
		h := header
		h.Size = int64(len(bodies[index]))
		if err := writer.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		writer.Write([]byte(bodies[index]))
	}
	writer.Close()
	gz.Close()
	path := filepath.Join(t.TempDir(), "a.tar.gz")
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// An entry that climbs out is an error, not something quietly skipped: skipping it
// would unpack the rest of an archive that has just said it is not what it claims.
func TestATarWithAnEscapingPathIsRefused(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "out")
	archive := tarGz(t,
		[]tar.Header{{Name: "ok/file", Mode: 0o644, Typeflag: tar.TypeReg}, {Name: "../escaped", Mode: 0o644, Typeflag: tar.TypeReg}},
		[]string{"fine", "owned"})
	err := unpackTar(archive, dest, true)
	if err == nil || !strings.Contains(err.Error(), "leaves where it is unpacked") {
		t.Fatalf("an escaping path was not refused: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "escaped")); statErr == nil {
		t.Fatal("a file was written outside the destination")
	}
}

// A link is not created: it is how a later entry is made to write somewhere the
// path check never looked.
func TestATarLinkIsNotCreated(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "out")
	outside := t.TempDir()
	archive := tarGz(t,
		[]tar.Header{
			{Name: "bin", Typeflag: tar.TypeSymlink, Linkname: outside},
			{Name: "bin/payload", Mode: 0o644, Typeflag: tar.TypeReg},
		},
		[]string{"", "owned"})
	_ = unpackTar(archive, dest, true)
	if _, err := os.Lstat(filepath.Join(dest, "bin")); err == nil {
		if info, _ := os.Lstat(filepath.Join(dest, "bin")); info.Mode()&os.ModeSymlink != 0 {
			t.Fatal("a symbolic link was created")
		}
	}
	if _, err := os.Stat(filepath.Join(outside, "payload")); err == nil {
		t.Fatal("a file was written through a link")
	}
}

// A file larger than the limit is refused by what it declares.
func TestATarFileOverTheLimitIsRefused(t *testing.T) {
	old := unpackFileLimit
	unpackFileLimit = 10
	t.Cleanup(func() { unpackFileLimit = old })

	archive := tarGz(t,
		[]tar.Header{{Name: "big", Mode: 0o644, Typeflag: tar.TypeReg}},
		[]string{strings.Repeat("x", 11)})
	err := unpackTar(archive, filepath.Join(t.TempDir(), "out"), true)
	if err == nil || !strings.Contains(err.Error(), "a file may be") {
		t.Fatalf("an oversized file was unpacked: %v", err)
	}
}

// A total over the limit is refused, though no single file is.
func TestATarTotalOverTheLimitIsRefused(t *testing.T) {
	oldTotal, oldFile := unpackLimit, unpackFileLimit
	unpackLimit, unpackFileLimit = 15, 10
	t.Cleanup(func() { unpackLimit, unpackFileLimit = oldTotal, oldFile })

	archive := tarGz(t,
		[]tar.Header{{Name: "a", Mode: 0o644, Typeflag: tar.TypeReg}, {Name: "b", Mode: 0o644, Typeflag: tar.TypeReg}},
		[]string{strings.Repeat("x", 8), strings.Repeat("y", 8)})
	err := unpackTar(archive, filepath.Join(t.TempDir(), "out"), true)
	if err == nil {
		t.Fatal("a total over the limit was unpacked")
	}
}

// aZip writes a zip with the named entries.
func aZip(t *testing.T, names []string, bodies []string) string {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for index, name := range names {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write([]byte(bodies[index]))
	}
	writer.Close()
	path := filepath.Join(t.TempDir(), "a.zip")
	if err := os.WriteFile(path, buffer.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The same refusal for a zip, which the tar tests do not reach.
func TestAZipWithAnEscapingPathIsRefused(t *testing.T) {
	root := t.TempDir()
	archive := aZip(t, []string{"ok/file", "../escaped"}, []string{"fine", "owned"})
	err := unpackZip(archive, filepath.Join(root, "out"))
	if err == nil || !strings.Contains(err.Error(), "leaves where it is unpacked") {
		t.Fatalf("an escaping zip path was not refused: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "escaped")); statErr == nil {
		t.Fatal("a zip entry was written outside the destination")
	}
}

// A zip file over the limit is refused.
func TestAZipFileOverTheLimitIsRefused(t *testing.T) {
	old := unpackFileLimit
	unpackFileLimit = 10
	t.Cleanup(func() { unpackFileLimit = old })
	archive := aZip(t, []string{"big"}, []string{strings.Repeat("x", 11)})
	err := unpackZip(archive, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "a file may be") {
		t.Fatalf("an oversized zip file was unpacked: %v", err)
	}
}
