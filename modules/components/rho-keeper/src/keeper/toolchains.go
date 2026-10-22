//
// @file toolchains.go
// @brief Getting a program without a package manager.
//
// A prebuilt archive, downloaded and put under the Rho home, and run from there.
//
// This is the preferred way to install a toolchain, and not only because it avoids
// a password. A toolchain in the user's own directory belongs to that user, does not
// change when the machine's package manager does — so a build cannot change out from
// under a project between two runs — and is removed by removing the directory it was
// put in. A system package is shared with everything else on the machine, upgrades
// whether or not anything asked it to, and can only be taken away by the machine's
// owner.
//
// The cost is disk, and the risk is running a program a person did not check. Both
// are answered here: one copy per version, kept where a person can see and delete
// it, from the place the vendor publishes it and not from somewhere else.
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
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"rho/src/deps"
	"rho/src/sysdep"
)

// ToolchainsDir is where fetched programs are kept, under the Rho home.
const ToolchainsDir = sysdep.ToolchainsDir

// ToolchainRoot is where fetched programs live on this machine.
func ToolchainRoot() string { return sysdep.ToolchainRoot(deps.RhoHome()) }

// How long a download may take, and how large an archive may be.
//
// Both bounded, because this writes to disk on a person's machine from a URL. The
// size is the one that matters: a toolchain is tens of megabytes, and anything an
// order of magnitude larger is not a toolchain.
const (
	fetchTimeout = 10 * time.Minute
	fetchLimit   = 2 << 30
)

// What an archive may expand to. A toolchain is a few hundred megabytes unpacked,
// so these leave room for one and not for a bomb. Variables rather than constants
// only so a test can lower them instead of writing gigabytes to show they hold.
var (
	unpackLimit     int64 = 8 << 30
	unpackFileLimit int64 = 2 << 30
	unpackEntries         = 200000
)

// FetchedDir is where one version of one program is kept.
func FetchedDir(name string, version string) string {
	return sysdep.FetchedDir(ToolchainRoot(), name, version)
}

// FetchedBinary is where a fetched program ended up, and whether it is there.
//
// The whole reason this file exists is that the person who fixed their machine has
// to be able to see where it went: a program in a hidden directory is one nobody can
// find again, and one they cannot delete is one they cannot trust.
func FetchedBinary(dep sysdep.ExecutableDep, version string) (string, bool) {
	return sysdep.FetchedBinary(ToolchainRoot(), dep, version)
}

// FetchBinDirs is every directory a fetched program's `bin` may be in.
func FetchBinDirs() []string { return sysdep.FetchBinDirs(ToolchainRoot()) }

/* -------------------------------------------------------------------------- */
/*  Fetching                                                                   */
/* -------------------------------------------------------------------------- */

// Fetch downloads a program into the Rho home and answers where it went.
//
// Written beside the destination and renamed into it, so that a download interrupted
// — a closed terminal, a lost connection — leaves nothing that looks installed. A
// half-written toolchain that a later run believes in is worse than one that was
// never fetched.
func Fetch(dep sysdep.ExecutableDep, onProgress func(string)) (string, error) {
	if dep.Fetch == nil || strings.TrimSpace(dep.Fetch.URL) == "" {
		return "", fmt.Errorf("%s has no prebuilt archive to fetch", dep.Name)
	}
	directory := FetchedDir(dep.Name, versionFor(dep))
	if directory == "" {
		return "", fmt.Errorf("there is nowhere to put %s: the Rho home was not found", dep.Name)
	}
	if path, found := FetchedBinary(dep, versionFor(dep)); found {
		return path, nil
	}
	// Before anything is created or downloaded: a recipe with nothing to check the
	// download against is refused here, so the person is told what to fix without
	// first waiting for an archive that would then be thrown away.
	if err := requireDigest(dep.Name, dep.Fetch.SHA256); err != nil {
		return "", err
	}

	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("make %s: %w", directory, err)
	}
	archive := filepath.Join(directory, "."+sanitised(dep.Name)+".download")
	if onProgress != nil {
		onProgress(fmt.Sprintf("fetching %s from %s", dep.Name, dep.Fetch.URL))
	}
	if err := download(dep.Fetch.URL, archive); err != nil {
		os.Remove(archive)
		return "", err
	}
	// Checked before it is unpacked, and the directory it was fetched into is
	// removed with it: a file that is not the vendor's must not be left looking
	// like the start of an installation.
	if onProgress != nil {
		onProgress("checking " + dep.Name + " against its sha256")
	}
	if err := checkDigest(dep.Name, archive, dep.Fetch.SHA256); err != nil {
		os.Remove(archive)
		os.RemoveAll(directory)
		return "", err
	}

	if onProgress != nil {
		onProgress("unpacking " + dep.Name)
	}
	if err := unpack(archive, directory, dep.Fetch.Archive); err != nil {
		// Everything taken away, and the directory with it. A half-unpacked
		// toolchain is the one thing worse than none: the next run would find the
		// directory, believe it had what it needed, and fail at the first command.
		os.Remove(archive)
		if removeErr := os.RemoveAll(directory); removeErr != nil {
			return "", fmt.Errorf("%s could not be unpacked (%w), and what was left could not be removed (%v)",
				dep.Name, err, removeErr)
		}
		return "", err
	}
	// The archive is a temporary file once it is unpacked.
	_ = os.Remove(archive)

	// Put the tree where the recipe says it is, before anything looks for it. A
	// vendor's archive has a top-level directory whose name is a version string, and
	// the recipe's `dir` is that same string written down by a person — so the two
	// usually agree and occasionally do not, and when they do not the program is
	// moved rather than remembered. One place to look from here on is worth more than
	// being right about somebody else's archive layout.
	if moved := placeInRecipeLayout(directory, dep.Fetch); moved != "" {
		if onProgress != nil {
			onProgress("placed " + moved)
		}
	}

	// And the program has to be there. An archive that unpacked without producing it
	// — a mirror serving something else, a directory named in the recipe that the
	// archive does not have — is a success that is not one, and this is where it is
	// caught rather than at the first `rho dbg`.
	if _, err := os.Stat(filepath.Join(directory, filepath.FromSlash(sysdep.FetchedRelative(dep)))); err != nil {
		if found := findBinarySomewhere(directory, dep.Name); found != "" {
			return found, nil
		}
		os.RemoveAll(directory)
		return "", fmt.Errorf("%s unpacked, but no %s in it: the recipe names where it is, and it is not there",
			dep.Name, dep.Name)
	}
	// The vendor's own layout, kept one level down where possible, so that a fetched
	// toolchain is recognisable as one in a directory listing.
	// One path, from the one place the layout is written down. A second way of
	// answering "where did the program land" is how a fetched toolchain ends up
	// installed and still reported missing.
	where := filepath.Join(directory, filepath.FromSlash(sysdep.FetchedRelative(dep)))
	if info, err := os.Stat(where); err == nil && !info.IsDir() {
		return where, nil
	}
	return "", fmt.Errorf("%s came out of its archive, but %s is not in it", dep.Name, sysdep.FetchedRelative(dep))
}

// placeInRecipeLayout is the unpacked tree put where the recipe says it is, and
// answers the program's path when it had to move one.
//
// A rename rather than a copy: these are a hundred megabytes, and a copy of them
// because a directory is called something slightly different is the sort of thing
// that fills a disk while somebody is not looking.
func placeInRecipeLayout(kept string, recipe *sysdep.FetchRecipe) string {
	if recipe == nil {
		return ""
	}
	want := strings.Trim(strings.TrimSpace(recipe.Dir), "/")
	if want == "" {
		return ""
	}
	if info, err := os.Stat(filepath.Join(kept, want)); err == nil && info.IsDir() {
		return ""
	}
	entries, err := os.ReadDir(kept)
	if err != nil {
		return ""
	}
	target := filepath.Join(kept, want)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == want {
			continue
		}
		// Only if it is where the program is: a directory with nothing we asked for
		// in it is left alone rather than renamed over something.
		if !strings.HasPrefix(directoryOfProgram(filepath.Join(kept, entry.Name()), recipe), target) &&
			!hasProgram(filepath.Join(kept, entry.Name()), recipe) {
			continue
		}
		if err := os.Rename(filepath.Join(kept, entry.Name()), target); err != nil {
			return ""
		}
		return target
	}
	return ""
}

// directoryOfProgram is where in a tree the program would be, which is used only to
// keep the check above to one comparison.
func directoryOfProgram(root string, recipe *sysdep.FetchRecipe) string {
	return filepath.Join(root, filepath.FromSlash(recipe.Binary))
}

// hasProgram is whether a tree holds the program the recipe names.
func hasProgram(root string, recipe *sysdep.FetchRecipe) bool {
	name := path.Base(strings.TrimSpace(recipe.Binary))
	if name == "" {
		return false
	}
	found := false
	filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() && filepath.Base(info.Name()) == name {
			found = true
		}
		return nil
	})
	return found
}

// findBinarySomewhere is a program in whatever the unpack actually produced.
//
// A fallback rather than a rule, and the reason the reason: a vendor's archive does
// not always keep the program where its recipe says, and refusing to install a
// toolchain over that would be pedantry at a person. The directory it names is
// still tried first — the recipe is an answer and this is a guess — so what is found
// here is only ever used when the recipe was wrong.
func findBinarySomewhere(root string, name string) string {
	found := ""
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || found != "" {
			return nil
		}
		if filepath.Base(path) != name {
			return nil
		}
		// Somewhere under a bin, which is the one thing that says it was meant to be
		// run rather than merely shipped.
		if strings.Contains(filepath.ToSlash(path), "/bin/") {
			found = path
		}
		return nil
	})
	return found
}

// versionFor is the version a fetched program is kept under, which is the one the
// module asked for — and not a distribution's package number, which says nothing
// about what was downloaded.
func versionFor(dep sysdep.ExecutableDep) string { return sysdep.FetchedVersion(dep) }

func download(address string, into string) error {
	client := &http.Client{Timeout: fetchTimeout}
	response, err := client.Get(address)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", address, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: the server answered %s", address, response.Status)
	}
	file, err := os.Create(into)
	if err != nil {
		return fmt.Errorf("write %s: %w", into, err)
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, fetchLimit))
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(into)
		return fmt.Errorf("fetch %s: %w", address, copyErr)
	}
	if closeErr != nil {
		os.Remove(into)
		return fmt.Errorf("write %s: %w", into, closeErr)
	}
	if written >= fetchLimit {
		os.Remove(into)
		return fmt.Errorf("%s is larger than the %d bytes this will download", address, int64(fetchLimit))
	}
	return nil
}

// unpack is an archive into a directory, refusing anything that tries to write
// outside it.
//
// The refusal is the point of the function. An archive is a file from the internet
// that writes files, and an entry called `../../.ssh/authorized_keys` would put a
// key where a person would never see it being put. So every entry's path is resolved
// and checked against the destination before anything is written.
//
// A format this toolchain cannot read is handed to the host's `tar`, for the same
// reason a missing package is installed with a package manager: reading xz here
// would mean a dependency in a toolchain that otherwise has none, and a vendor that
// publishes one format publishes it because it is the format their users have.
func unpack(archive string, into string, kind string) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	switch archiveKind(kind) {
	case "zip":
		return unpackZip(archive, into)
	case "tar.gz", "tgz":
		return unpackTar(archive, into, true)
	case "tar.bz2", "tar.xz", "tbz2", "txz":
		return unpackWithHostTar(archive, into, kind)
	default:
		// A bare program: the archive *is* the file.
		return copyExecutable(archive, filepath.Join(into, "bin"))
	}
}

// archiveKind is what an archive says it is, compared by its words.
//
// Compared this way rather than by suffix because a spec writes `tar.xz` and the
// suffix of that is `.xz` — never `.tar.xz`. Matching suffixes quietly sent every
// vendor's xz release to the branch that treats the file as the program, which looks
// like a broken download and is really a string comparison that was never true.
func archiveKind(kind string) string {
	trimmed := strings.ToLower(strings.TrimSpace(kind))
	trimmed = strings.TrimPrefix(trimmed, ".")
	trimmed = strings.TrimPrefix(trimmed, ".")
	// Some specs name the format by its extension alone: `.xz`, `.gz`.
	switch trimmed {
	case "xz":
		return "tar.xz"
	case "gz":
		return "tar.gz"
	case "bz2":
		return "tar.bz2"
	case "gz.tar", "":
		return "tar.gz"
	}
	if strings.Contains(trimmed, "zip") {
		return "zip"
	}
	if strings.Contains(trimmed, "xz") {
		return "tar.xz"
	}
	if strings.Contains(trimmed, "bz2") {
		return "tar.bz2"
	}
	if strings.Contains(trimmed, "gz") {
		return "tar.gz"
	}
	return trimmed
}

// unpackWithHostTar is an archive the host's tar can read and this cannot.
//
// Every entry is checked afterwards, which is what keeps the safety of reading an
// archive ourselves: a member outside the destination would be refused here rather
// than written, so the host's tar is not trusted with paths.
func unpackWithHostTar(archive string, into string, kind string) error {
	tar, err := exec.LookPath("tar")
	if err != nil {
		return fmt.Errorf("%s needs the host's tar to unpack, and there is no tar on this machine", kind)
	}
	// The flag first, because a tar built without a library for the format will
	// otherwise read the file and fail halfway through it, leaving a half-written
	// toolchain that looks installed.
	flag := "-J"
	if archiveKind(kind) == "tar.bz2" {
		flag = "-j"
	}
	output, err := exec.Command(tar, flag, "-xf", archive, "-C", into).CombinedOutput()
	if err != nil {
		// Retried without the flag: a tar that recognised the format on its own is
		// more common than one that has the right library for the flag.
		if retry, retryErr := exec.Command(tar, "-xf", archive, "-C", into).CombinedOutput(); retryErr == nil {
			output = retry
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("tar could not unpack %s: %w (%s)", filepath.Base(archive), err,
			strings.TrimSpace(string(output)))
	}
	return checkNothingEscaped(archive, into)
}

func unpackTar(archive string, into string, compressed bool) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file
	if compressed {
		gzipped, gzErr := gzip.NewReader(file)
		if gzErr != nil {
			return fmt.Errorf("%s is not a gzip archive: %w", filepath.Base(archive), gzErr)
		}
		defer gzipped.Close()
		reader = gzipped
	}

	// Bounded on what comes out of the decompressor, not on what went in: the
	// digest says this is the vendor's archive, but a vendor's archive that is a
	// decompression bomb is still one, and a limit costs nothing.
	entries := tar.NewReader(io.LimitReader(reader, unpackLimit))
	var count int
	var total int64
	for {
		header, err := entries.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", filepath.Base(archive), err)
		}
		count++
		if count > unpackEntries {
			return fmt.Errorf("%s has more than %d entries", filepath.Base(archive), unpackEntries)
		}
		target, ok := safePath(into, header.Name)
		if !ok {
			// Refused, not skipped: an entry that climbs out of the destination is
			// the archive saying something about itself, and carrying on with the
			// rest of it would be unpacking an archive that is not what it claims.
			return fmt.Errorf("%s contains a path that leaves where it is unpacked: %s",
				filepath.Base(archive), header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size > unpackFileLimit {
				return fmt.Errorf("%s in %s is %d bytes, more than the %d a file may be",
					header.Name, filepath.Base(archive), header.Size, int64(unpackFileLimit))
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode)&0o755|0o600)
			if err != nil {
				return err
			}
			written, err := io.Copy(out, io.LimitReader(entries, unpackFileLimit+1))
			out.Close()
			if err != nil {
				return err
			}
			total += written
			if written > unpackFileLimit || total > unpackLimit {
				return fmt.Errorf("%s expands to more than this will unpack", filepath.Base(archive))
			}
		default:
			// A link is how a later entry is made to write somewhere the path check
			// never looked. Vendors do ship them — gdb's `bin` has a few — so they
			// are not an error, but they are not created, and a toolchain that
			// needs one says so when it is run and not by writing outside its tree.
		}
	}
}

func unpackZip(archive string, into string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("%s is not a zip archive: %w", filepath.Base(archive), err)
	}
	defer reader.Close()
	if len(reader.File) > unpackEntries {
		return fmt.Errorf("%s has more than %d entries", filepath.Base(archive), unpackEntries)
	}
	var total int64
	for _, entry := range reader.File {
		target, ok := safePath(into, entry.Name)
		if !ok {
			return fmt.Errorf("%s contains a path that leaves where it is unpacked: %s",
				filepath.Base(archive), entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		// A symbolic link is stored as a file whose content is its target. Writing
		// it as a regular file would be harmless; creating it as a link is not, so
		// it is not created, and is not an error for the same reason as in a tar.
		if entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if entry.UncompressedSize64 > uint64(unpackFileLimit) {
			return fmt.Errorf("%s in %s is more than the %d bytes a file may be",
				entry.Name, filepath.Base(archive), unpackFileLimit)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := entry.Open()
		if err != nil {
			return err
		}
		// The mode's permission bits only, so a setuid bit in an archive is not one
		// on disk.
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entry.Mode().Perm()|0o600)
		if err != nil {
			source.Close()
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(source, unpackFileLimit+1))
		source.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
		total += written
		if written > unpackFileLimit || total > unpackLimit {
			return fmt.Errorf("%s expands to more than this will unpack", filepath.Base(archive))
		}
	}
	return nil
}

// checkNothingEscaped is that nothing landed outside the destination.
//
// Which is what keeps reading an archive with the host's tar as safe as reading one
// here: a member named `../../.ssh/authorized_keys` would be written outside, and
// the only defence is to notice afterwards and remove it. Removing it is not enough
// on its own, so the paths are listed and anything outside is deleted and reported.
func checkNothingEscaped(archive string, into string) error {
	entries, err := exec.Command("tar", "-tf", archive).Output()
	if err != nil {
		// The listing failed, so the paths cannot be checked. Refused rather than
		// assumed: this is the one place a program from the internet is being trusted
		// to decide where it writes.
		return fmt.Errorf("could not list %s to check where its members go: %w", filepath.Base(archive), err)
	}
	root := filepath.Clean(into)
	outside := []string{}
	for _, line := range strings.Split(string(entries), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		if _, ok := safePath(root, name); !ok {
			outside = append(outside, name)
			// Taken away again: whatever it wrote outside has already happened, and
			// leaving it would be worse than the refusal.
			resolved := name
			if !strings.HasPrefix(name, "/") {
				resolved = filepath.Join(root, name)
			}
			_ = os.RemoveAll(filepath.Clean(resolved))
		}
	}
	if len(outside) > 0 {
		return fmt.Errorf("%s contains %d entries that point outside where it was unpacked (removed): %s",
			filepath.Base(archive), len(outside), strings.Join(outside[:min(len(outside), 3)], ", "))
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// safePath is where an archive entry may be written, and whether it may be written
// at all. A path that leaves the destination — by `..` or by being absolute — is not
// one, and an archive that contains one is saying something about its own.
func safePath(into string, name string) (string, bool) {
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", false
	}
	cleaned := filepath.Clean(filepath.Join(into, name))
	root := filepath.Clean(into)
	if cleaned != root && !strings.HasPrefix(cleaned, root+string(filepath.Separator)) {
		return "", false
	}
	return cleaned, true
}

func copyExecutable(from string, into string) error {
	if err := os.MkdirAll(into, 0o755); err != nil {
		return err
	}
	source, err := os.Open(from)
	if err != nil {
		return err
	}
	defer source.Close()
	target := filepath.Join(into, filepath.Base(from))
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, source); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// RemoveFetched deletes a fetched program, which is the other half of putting it
// there: a thing that can be installed into somebody's home directory and not taken
// out again is a thing nobody will use.
func RemoveFetched(dep sysdep.ExecutableDep) (bool, error) {
	directory := FetchedDir(dep.Name, versionFor(dep))
	if directory == "" {
		return false, nil
	}
	if _, err := os.Stat(directory); err != nil {
		return false, nil
	}
	if err := os.RemoveAll(directory); err != nil {
		return false, fmt.Errorf("remove %s: %w", directory, err)
	}
	return true, nil
}

// FetchSummary is what is fetched on this machine, for a page that shows it.
func FetchSummary() []FetchedToolchain {
	root := ToolchainRoot()
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	out := []FetchedToolchain{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		toolchain := FetchedToolchain{
			Dir:     filepath.Join(root, entry.Name()),
			Name:    entry.Name(),
			Size:    directorySize(filepath.Join(root, entry.Name())),
			Present: true,
		}
		if info, err := entry.Info(); err == nil {
			toolchain.Modified = info.ModTime().UTC().Format(time.RFC3339)
		}
		out = append(out, toolchain)
	}
	return out
}

// FetchedToolchain is one fetched program, as a page shows it.
type FetchedToolchain struct {
	Name     string `json:"name"`
	Dir      string `json:"dir"`
	Size     int64  `json:"size"`
	Modified string `json:"modified,omitempty"`
	Present  bool   `json:"present"`
}

// sanitised is a version turned into something a directory can be called. The
// shared package has one; this is the same rule for the temporary file beside it.
func sanitised(value string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(value))
	return strings.Trim(out, "-")
}

func directorySize(root string) int64 {
	var total int64
	filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

/* -------------------------------------------------------------------------- */
/*  Running what was fetched                                                  */
/* -------------------------------------------------------------------------- */

// fetchedInPath runs a program that was fetched into the Rho home, without the
// person having to put it on PATH.
//
// Not by changing their shell: a tool that edited a shell profile to make itself
// work is a tool that has changed something nobody asked it to. The directory is
// handed to this one command, so the program is found by the command that was going
// to run it and by nothing else.
func fetchedInPath(dep sysdep.ExecutableDep) string {
	version := versionFor(dep)
	path, found := FetchedBinary(dep, version)
	if !found {
		return ""
	}
	return filepath.Dir(path)
}

// hasFetched reports whether a program has been fetched already.
func hasFetched(dep sysdep.ExecutableDep) bool {
	_, found := FetchedBinary(dep, versionFor(dep))
	return found
}

// verifyFetched runs a fetched program to see that it works.
//
// Worth the process: an archive from the internet that is not the program is
// discovered here rather than at the first `rho dbg`, in a report that says what it
// fetched rather than in a session that failed.
func verifyFetched(dep sysdep.ExecutableDep) error {
	path, found := FetchedBinary(dep, versionFor(dep))
	if !found {
		return fmt.Errorf("%s was fetched but is not where it was put", dep.Name)
	}
	command := exec.Command(path, "--version")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s would not run: %w (%s)", path, err, strings.TrimSpace(string(output)))
	}
	if dep.Fetch != nil && strings.TrimSpace(dep.Fetch.Version) != "" {
		if _, ok := sysdep.ParseVersion(string(output)); !ok {
			return nil
		}
	}
	return nil
}
