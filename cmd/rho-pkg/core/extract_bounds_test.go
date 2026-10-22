//
// @file extract_bounds_test.go
// @brief What a package may weigh: entries, files, totals and a decompression bomb.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The mode that travels is the permission bits and nothing else.
func TestUnpackDoesNotHonourSetuid(t *testing.T) {
	archive := archiveOf(t, entry{name: "tool", body: "x", mode: 0o4755})
	bothWays(t, archive, func(t *testing.T, err error, dest string) {
		if err != nil {
			t.Fatal(err)
		}
		info, statErr := os.Stat(filepath.Join(dest, "tool"))
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode()&os.ModeSetuid != 0 {
			t.Fatal("a setuid bit in a package was applied")
		}
	})
}

// An ordinary package still unpacks, with its directories and its modes.
func TestUnpackStillUnpacksAPackage(t *testing.T) {
	archive := archiveOf(t,
		entry{name: "module.yml", body: "module: x"},
		entry{name: "src/", typeflag: tar.TypeDir, mode: 0o755},
		entry{name: "src/main.rho", body: "module main"},
		entry{name: "rho-x", body: "#!/bin/sh\n", mode: 0o755},
	)
	bothWays(t, archive, func(t *testing.T, err error, dest string) {
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Stat(filepath.Join(dest, "src", "main.rho")); statErr != nil {
			t.Fatalf("src/main.rho missing: %v", statErr)
		}
		info, statErr := os.Stat(filepath.Join(dest, "rho-x"))
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm()&0o100 == 0 {
			t.Fatalf("the command lost its executable bit: %v", info.Mode())
		}
	})
}

// lowered sets the bounds for one test and puts them back.
func lowered(t *testing.T, entries int, file, total int64) {
	t.Helper()
	oldEntries, oldFile, oldTotal := maxPackageEntries, maxPackageFileBytes, maxPackageBytes
	maxPackageEntries, maxPackageFileBytes, maxPackageBytes = entries, file, total
	t.Cleanup(func() {
		maxPackageEntries, maxPackageFileBytes, maxPackageBytes = oldEntries, oldFile, oldTotal
	})
}

// A package with more entries than any package has is refused.
func TestUnpackRefusesTooManyEntries(t *testing.T) {
	lowered(t, 3, 1<<20, 1<<20)
	archive := archiveOf(t,
		entry{name: "a", body: "1"}, entry{name: "b", body: "2"},
		entry{name: "c", body: "3"}, entry{name: "d", body: "4"},
	)
	bothWays(t, archive, func(t *testing.T, err error, dest string) {
		if err == nil || !strings.Contains(err.Error(), "entries") {
			t.Fatalf("too many entries were accepted: %v", err)
		}
	})
}

// A file larger than any file in a package is refused, by what it declares.
func TestUnpackRefusesAFileThatIsTooLarge(t *testing.T) {
	lowered(t, 100, 10, 1<<20)
	archive := archiveOf(t, entry{name: "big", body: strings.Repeat("x", 11)})
	bothWays(t, archive, func(t *testing.T, err error, dest string) {
		if err == nil || !strings.Contains(err.Error(), "file in a package may be") {
			t.Fatalf("a file over the limit was accepted: %v", err)
		}
	})
}

// A package whose files are each small but together too large is refused: the
// total is what a bomb is made of.
func TestUnpackRefusesATotalThatIsTooLarge(t *testing.T) {
	lowered(t, 100, 10, 15)
	archive := archiveOf(t,
		entry{name: "a", body: strings.Repeat("x", 8)},
		entry{name: "b", body: strings.Repeat("y", 8)},
	)
	bothWays(t, archive, func(t *testing.T, err error, dest string) {
		if err == nil || !strings.Contains(err.Error(), "expands to more than") {
			t.Fatalf("a package whose total is over the limit was accepted: %v", err)
		}
	})
}

// A header that lies about its size is caught by what is actually written: the
// limit is on what comes out of the stream, not on what the header claims.
func TestUnpackStopsADecompressionBomb(t *testing.T) {
	lowered(t, 100, 1<<20, 1<<20)

	// 64 MiB of zeros compresses to about 64 KiB, and is sixty times what a
	// package may be here.
	var buffer bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buffer, gzip.BestCompression)
	writer := tar.NewWriter(gz)
	const size = int64(64 << 20)
	if err := writer.WriteHeader(&tar.Header{Name: "bomb", Mode: 0o644, Size: size, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for written := int64(0); written < size; written += int64(len(chunk)) {
		if _, err := writer.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	writer.Close()
	gz.Close()
	if buffer.Len() > 4<<20 {
		t.Fatalf("the test archive is %d bytes; it was meant to be small", buffer.Len())
	}

	bothWays(t, buffer.Bytes(), func(t *testing.T, err error, dest string) {
		if err == nil {
			t.Fatal("a decompression bomb was accepted")
		}
		if info, statErr := os.Stat(filepath.Join(dest, "bomb")); statErr == nil && info.Size() > 1<<20+1 {
			t.Fatalf("%d bytes of the bomb were written", info.Size())
		}
	})
}

// A manifest larger than a page of text is refused before it is parsed.
func TestReadArchiveManifestRefusesAHugeManifest(t *testing.T) {
	old := maxManifestBytes
	maxManifestBytes = 16
	t.Cleanup(func() { maxManifestBytes = old })

	archive := archiveOf(t, entry{name: "module.yml", body: strings.Repeat("a: b\n", 100)})
	if _, err := ReadArchiveManifest(archive); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("an oversized manifest was read: %v", err)
	}
}
