//
// @file artifacts.go
// @brief Finding what a build produced.
//
// A project does not have to say where its artifacts are, and it should not have
// to: the build already knows, and it wrote them where its own settings say. So
// the search follows what a build actually produces — an ELF with the project's
// own name, a linker map beside it — rather than asking for paths.
//
// The search is bounded and ordered, because a build tree holds a great deal
// that is not the program: object files, CMake caches, and one intermediate
// directory per board a project supports. Walking all of it looking for an ELF
// would find the ELF in `build/generated/c/build/nucleo-h743zi/` of a project
// being built for `nucleo-h753zi`, which is precisely the wrong answer.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Artifacts is what a project's build produced, and where.
type Found struct {
	// ELFFile is the program, or "" when none was found.
	ELFFile string
	// MapFile is the linker map beside it, or "" when the build wrote none.
	MapFile string
	// Target is the board or chip the program was built for, when it could be
	// worked out from where it was found.
	Target string
	// Notes is what the search had to decide, reported rather than hidden: a
	// project whose build tree holds several programs is a project whose numbers
	// this run is guessing between.
	Notes []string
}

// openLimited opens a file for reading a few bytes of.
func openLimited(path string) (*os.File, error) { return os.Open(path) }

// Find locates a project's build artifacts.
//
// `outputDir` is where the project's own build puts finished programs, which is
// the first place to look and usually the only place that needs looking: the
// binary a flash command is given is the one in there. Underneath that, the CMake
// build tree is searched for a program, taking the one nearest the top of the tree
// so that a project built for several boards reports the one it most recently
// built rather than an arbitrary one of them.
func Find(projectRoot, outputDir string) Found {
	out := Found{}
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		root = projectRoot
	}

	output := outputDir
	if output == "" {
		output = "build/output"
	}
	outputDirPath := filepath.Join(root, filepath.FromSlash(output))

	// The linker map first, because it is what names the memory regions and what
	// names the board: it is written by the link into the build tree, into a
	// directory called after the target, which is the only place either fact is
	// written down. Looking for it first is what lets a report say which board it
	// is talking about rather than guessing from a file name.
	if mapFile, target := newestMapUnder(root); mapFile != "" {
		out.MapFile = mapFile
		out.Target = target
		if elf := programFor(root, outputDirPath, mapFile); elf != "" {
			out.ELFFile = elf
			return out
		}
		out.Notes = append(out.Notes, "found the linker map for "+target+" but no program beside it; "+
			"the map alone still answers every budget question")
		return out
	}

	// No map: the project was built without one, which is what `--no-link-map` is
	// for. The program is still worth reading, and its regions are inferred.
	if elf := newestELFIn(outputDirPath); elf != "" {
		out.ELFFile = elf
		return out
	}
	if generated := filepath.Join(root, "build", "generated"); isDir(generated) {
		for _, elf := range elfsUnder(generated, 4) {
			out.ELFFile = elf
			out.Target = targetFromPath(elf)
			out.Notes = append(out.Notes, "no linker map was found, so the regions were inferred from "+
				"section names; build with a map for figures that name the board's own regions")
			return out
		}
	}
	if build := filepath.Join(root, "build"); isDir(build) {
		for _, elf := range elfsUnder(build, 5) {
			out.ELFFile = elf
			out.Target = targetFromPath(elf)
			return out
		}
	}
	return out
}

// programFor is the program a linker map belongs to.
//
// The map is written into the CMake build tree and the program is copied out of
// it into the project's output directory, so the two are in different places and
// which is which matters: a project that built for several boards has one
// directory per board, and the map in a directory is the map of the program built
// there.
func programFor(root, outputDirPath, mapFile string) string {
	directory := filepath.Dir(mapFile)
	if elf := newestELFIn(directory); elf != "" {
		return elf
	}
	if outputDirPath != "" && outputDirPath != directory {
		if elf := newestELFIn(outputDirPath); elf != "" {
			return elf
		}
	}
	return ""
}

// newestMapUnder is the most recently written linker map in a project's build
// tree, and the target it was written for.
//
// Searched by name rather than by extension because the linker decides the name:
// rho-target writes `<target>.map`, a project's own build may write something
// else, and the one thing every linker map has in common is where it is — a
// directory named after the target, inside a CMake build tree.
func newestMapUnder(root string) (string, string) {
	builds := filepath.Join(root, "build", "generated")
	if !isDir(builds) {
		builds = filepath.Join(root, "build")
	}
	if !isDir(builds) {
		return "", ""
	}
	type candidate struct {
		path    string
		target  string
		modTime int64
	}
	found := []candidate{}
	var walk func(directory string, level int)
	walk = func(directory string, level int) {
		if level > 5 {
			return
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			return
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(directory, name)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.IsDir() {
				if name == "CMakeFiles" || name == ".git" {
					continue
				}
				walk(path, level+1)
				continue
			}
			if !strings.HasSuffix(name, ".map") {
				continue
			}
			found = append(found, candidate{
				path:    path,
				target:  targetFromPath(path),
				modTime: info.ModTime().UnixNano(),
			})
		}
	}
	walk(builds, 0)
	if len(found) == 0 {
		return "", ""
	}
	// Newest first: a project built for several boards has one map per board, and
	// the one written last is the one it most recently built.
	sort.Slice(found, func(i, j int) bool { return found[i].modTime > found[j].modTime })
	best := found[0]
	target := best.target
	if target == "" {
		target = strings.TrimSuffix(filepath.Base(best.path), filepath.Ext(best.path))
	}
	return best.path, target
}

// targetFromPath is the board a build was for, read from the path the build
// wrote it to.
//
// The generated tree is written per target — `build/generated/c/build/<target>/` —
// and the linker map is named after the target as well, so both say it. The
// directory name is preferred over the file name because the file name is the
// program's, which a project chooses.
//
// A heuristic, and reported as one: it recognises the directory a CMake build ran
// in, which is the one place the answer is written down when a project built for
// several boards and did not say which one it meant.
func targetFromPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	// The directory a CMake build ran in: the one whose parent is `build`.
	for index := len(parts) - 1; index > 0; index-- {
		if parts[index-1] == "build" && parts[index] != "build" {
			return parts[index]
		}
	}
	// A map named after its target, when it is not in such a directory.
	if base := filepath.Base(path); strings.HasSuffix(base, ".map") {
		return strings.TrimSuffix(base, ".map")
	}
	return ""
}

// newestELFIn is the most recently written ELF in a directory.
//
// Newest rather than first, because a build that produces several programs in one
// directory has produced the one it most recently built last, and that is the one
// somebody looking at the directory meant.
func newestELFIn(directory string) string {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ""
	}
	type candidate struct {
		path    string
		modTime int64
	}
	found := []candidate{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if !looksLikeELF(path) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		found = append(found, candidate{path: path, modTime: info.ModTime().UnixNano()})
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool { return found[i].modTime > found[j].modTime })
	return found[0].path
}

// elfsUnder is every ELF at or below a directory, deepest-last, at most `depth`
// directories down.
//
// Sorted by depth and then by path so that the answer is the same on every run:
// a build tree is walked in the order the filesystem hands it back otherwise, and
// a tool whose number changes when nothing was rebuilt is a tool nobody trusts in
// a pipeline.
func elfsUnder(root string, depth int) []string {
	found := []string{}
	var walk func(directory string, level int)
	walk = func(directory string, level int) {
		if level > depth {
			return
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			return
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(directory, name)
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.IsDir() {
				// A directory whose name says it is a build system is not worth
				// walking: it holds no program and a great deal of file.
				if name == "CMakeFiles" || name == ".git" {
					continue
				}
				walk(path, level+1)
				continue
			}
			if looksLikeELF(path) {
				found = append(found, path)
			}
		}
	}
	walk(root, 0)
	return found
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// readAtMost is the first bytes of a file, for a format check.
func readAtMost(path string, count int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, int64(count)))
}
