//
// @file extract_test.go
// @brief What a package is refused for, however it reaches the machine.
//
// A package arrives from a registry, and a registry is not trusted to keep its own
// archive honest. These pin the refusals: a path that leaves the destination, an
// entry that is not a file, and a package that is larger than any package is. Each
// is tried on both ways in — the file on disk and the bytes from the network —
// because the confinement check was once only on one of them.
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

// entry is one thing to put in a test archive.
type entry struct {
	name     string
	body     string
	typeflag byte
	link     string
	mode     int64
}

// archiveOf builds a gzipped tar of the entries, written exactly as given — a
// hostile name included, which is the point.
func archiveOf(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	for _, item := range entries {
		flag := item.typeflag
		if flag == 0 {
			flag = tar.TypeReg
		}
		mode := item.mode
		if mode == 0 {
			mode = 0o644
		}
		header := &tar.Header{Name: item.name, Typeflag: flag, Mode: mode, Linkname: item.link}
		if flag == tar.TypeReg {
			header.Size = int64(len(item.body))
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatalf("write header %q: %v", item.name, err)
		}
		if flag == tar.TypeReg {
			if _, err := writer.Write([]byte(item.body)); err != nil {
				t.Fatalf("write body %q: %v", item.name, err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// bothWays runs one check against the package as bytes and as a file, since they
// are the two ways a package reaches the machine.
func bothWays(t *testing.T, archive []byte, check func(t *testing.T, err error, dest string)) {
	t.Helper()
	t.Run("bytes", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "out")
		check(t, UnpackBytes(archive, dest), dest)
	})
	t.Run("file", func(t *testing.T) {
		root := t.TempDir()
		file := filepath.Join(root, "p.rho-pkg")
		if err := os.WriteFile(file, archive, 0o644); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(root, "out")
		check(t, UnpackArchive(file, dest), dest)
	})
}

// A name that climbs out of the destination writes nothing outside it.
func TestUnpackRefusesAPathThatLeavesTheDestination(t *testing.T) {
	for _, name := range []string{"../escaped.txt", "a/../../escaped.txt", "/etc/escaped.txt"} {
		name := name
		t.Run(name, func(t *testing.T) {
			archive := archiveOf(t, entry{name: name, body: "x"})
			bothWays(t, archive, func(t *testing.T, err error, dest string) {
				if err == nil || !strings.Contains(err.Error(), "outside") {
					t.Fatalf("a path that leaves the destination was not refused: %v", err)
				}
				outside := filepath.Join(filepath.Dir(dest), "escaped.txt")
				if _, statErr := os.Stat(outside); statErr == nil {
					t.Fatalf("%s was written outside the destination", outside)
				}
			})
		})
	}
}

// A link or a device is refused: a link is how a later entry is made to write
// somewhere the path check never looked.
func TestUnpackRefusesALink(t *testing.T) {
	cases := map[string]entry{
		"symbolic link": {name: "link", typeflag: tar.TypeSymlink, link: "/etc"},
		"hard link":     {name: "link", typeflag: tar.TypeLink, link: "module.yml"},
		"device":        {name: "dev", typeflag: tar.TypeChar},
		"named pipe":    {name: "pipe", typeflag: tar.TypeFifo},
	}
	for label, item := range cases {
		label, item := label, item
		t.Run(label, func(t *testing.T) {
			archive := archiveOf(t, entry{name: "module.yml", body: "module: x"}, item)
			bothWays(t, archive, func(t *testing.T, err error, dest string) {
				if err == nil || !strings.Contains(err.Error(), "not a file or a directory") {
					t.Fatalf("%s was not refused: %v", label, err)
				}
			})
		})
	}
}

// A link followed by a file written through it is the attack the refusal above
// exists for; the file must not land where the link pointed.
func TestUnpackDoesNotWriteThroughALink(t *testing.T) {
	target := t.TempDir()
	archive := archiveOf(t,
		entry{name: "bin", typeflag: tar.TypeSymlink, link: target},
		entry{name: "bin/payload", body: "owned"},
	)
	bothWays(t, archive, func(t *testing.T, err error, dest string) {
		if err == nil {
			t.Fatal("a package that links out and writes through the link was accepted")
		}
		if _, statErr := os.Stat(filepath.Join(target, "payload")); statErr == nil {
			t.Fatal("a file was written through a symbolic link")
		}
	})
}
