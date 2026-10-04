//
// @file privilege.go
// @brief Saying something when this run is root, and only when it should.
//
// Somebody runs `sudo rho build` by accident — it is the first thing anybody tries
// when a permission is denied — and everything after it goes wrong in a way that
// takes an afternoon to trace. The toolchain's cache, the registry, the module
// installs and the build directory are all created root-owned, and the next run
// without sudo fails with "permission denied" on files that did not exist a minute
// earlier. That is worth a warning.
//
// What must not happen is the toolchain refusing to run. A container's default user
// is root, a CI job runs as root, and a person setting a machine up from a root
// shell is doing the thing this whole toolchain is for. So this never fails and
// never exits: it says what it noticed, and carries on.
//
// It is also specific rather than a scolding. "You are root" is true in a container
// where it is entirely normal, and useless in a sudo invocation where it is the
// whole problem. What is said instead is the consequence — who will own the files
// this run creates, and whether anything already does — because that is the part
// that can be acted on.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

// Package privilege notices when this run has more authority than it needs.
//
// It is a notice and not a policy: nothing here decides whether a command may run,
// because a command that cannot run in a container is a toolchain that cannot be
// used to build a container. What it does is say the thing that is expensive to
// discover later, once, in as many words as the situation warrants.
package privilege

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Situation is what a run looks like from the outside.
//
// A value rather than a set of calls, so that the whole judgement — including the
// awkward cases, a container and a CI job — can be decided in a test without a
// machine that is root and one that is not.
type Situation struct {
	// Root is whether this process has uid 0.
	Root bool
	// SudoUser is who ran `sudo`, when sudo was what did it. The difference between
	// a shell that is root and a command that was elevated: the first is somebody's
	// machine, the second is almost always an accident.
	SudoUser string
	// Container is whether this looks like a container, where root is the default
	// user and nobody chose it.
	Container bool
	// CI is whether this looks like a pipeline, for the same reason.
	CI bool
	// RhoHome is where the installation keeps its state — the registry, the caches,
	// the modules. Root-owned here is the specific damage that makes every later
	// run fail.
	RhoHome string
	// WorkDir is where the command is running, which is where a build leaves its
	// output.
	WorkDir string
}

// Containers set one of these, or the marker file a container runtime leaves.
var containerMarkers = []string{"/.dockerenv", "/run/.containerenv"}

// Detect is what this run looks like. Read rather than assumed, and every answer is
// something on the machine or in the environment.
func Detect() Situation {
	home := ""
	if rho := strings.TrimSpace(os.Getenv("RHO_HOME")); rho != "" {
		home = rho
	} else if user, err := os.UserHomeDir(); err == nil {
		home = filepath.Join(user, ".rho")
	}
	work, _ := os.Getwd()

	out := Situation{
		Root:     os.Geteuid() == 0,
		SudoUser: strings.TrimSpace(os.Getenv("SUDO_USER")),
		CI:       isCI(),
		RhoHome:  home,
		WorkDir:  work,
	}
	for _, marker := range containerMarkers {
		if _, err := os.Stat(marker); err == nil {
			out.Container = true
			break
		}
	}
	if !out.Container && looksLikeContainerCgroup() {
		out.Container = true
	}
	return out
}

// isCI is whether this looks like a pipeline.
//
// The two every provider sets. A pipeline running as root is normal and warning
// about it teaches people to ignore the warning that matters.
func isCI() bool {
	for _, name := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "BUILDKITE", "JENKINS_URL", "TF_BUILD"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
		if value == "" || value == "false" || value == "0" || value == "no" ||
			value == "off" || value == "none" || value == "disabled" {
			continue
		}
		return true
	}
	return false
}

// looksLikeContainerCgroup is whether the process is in a container's cgroup, which
// is how a runtime with no marker file is told apart from a machine.
func looksLikeContainerCgroup() bool {
	data, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	text := string(data)
	for _, marker := range []string{"docker", "kubepods", "containerd", "podman", "lxc"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// Check says what this run should be told, and answers whether it said anything.
//
// Three situations, three different sentences, and silence in the fourth:
//
//   - root, not in a container, through sudo: the case worth a paragraph, because a
//     person who did this once has an afternoon ahead of them
//   - root, in a container or a pipeline: one line, because root is what it means
//     there and a warning about it is noise
//   - not root: nothing at all
//
// And in every case: whether anything this toolchain owns has *already* been created
// by root, which is the part that is actually broken and the part nobody notices.
func Check(out io.Writer, situation Situation) bool {
	if !situation.Root {
		return false
	}

	if situation.Container || situation.CI {
		// Said once and briefly, because in a container this is not a mistake and a
		// paragraph about a person being careful would be read as nagging.
		fmt.Fprintln(out, "rho: running as root, which is the default user in a container.")
		return true
	}

	fmt.Fprintln(out, "rho: this is running as root.")
	if situation.SudoUser != "" {
		fmt.Fprintf(out, "  `sudo rho ...` makes everything it writes belong to root: the registry, the\n"+
			"  caches under %s, and whatever the command leaves behind in the project.\n", situation.RhoHome)
		fmt.Fprintln(out, "  The next run without sudo then fails on files that did not exist a moment ago,")
		fmt.Fprintln(out, "  which is a long way from the mistake that caused it.")
		fmt.Fprintf(out, "  Run it as %s instead. If something has already gone wrong,\n", situation.SudoUser)
		fmt.Fprintln(out, "  `ls -ld ~/.rho` and the project's build directory will say who owns them.")
	} else {
		fmt.Fprintln(out, "  Everything it writes will belong to root, and be hard to change later.")
		fmt.Fprintln(out, "  A root shell is fine for setting a machine up; a root `rho` is not what you meant.")
	}

	// The concrete thing. A warning that does not say whether the damage is already
	// done leaves the person with a question they have to answer in another terminal.
	if owned := rootOwned(situation); len(owned) > 0 {
		fmt.Fprintln(out, "  And already:")
		for _, path := range owned {
			fmt.Fprintf(out, "    %s belongs to root\n", path)
		}
		fmt.Fprintln(out, "  Those are fixable with `sudo chown -R $USER`, and nothing else is.")
	}
	return true
}

// rootOwned is which of this toolchain's own places root already owns.
//
// Only the two that matter and that are cheap to ask about: the installation's own
// state and the directory the command is running in. Both are checked rather than
// assumed, because a `chown` advice that turns out to be wrong costs more than the
// warning was worth.
func rootOwned(situation Situation) []string {
	out := []string{}
	for _, path := range []string{situation.RhoHome, situation.WorkDir} {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		// The owner is in the syscall's answer, not in a name: a home directory
		// belonging to a user whose uid is 0 on this machine is root-owned whatever
		// it is called.
		if owner, ok := ownerOf(path); ok && owner == 0 {
			out = append(out, path)
		}
	}
	return out
}

// Warn is the convenience most callers want: notice this run and say so.
func Warn(out io.Writer) bool { return Check(out, Detect()) }

// Silenced is whether a person has asked not to be told.
//
// An environment setting rather than a flag, because the run that needs this is
// usually not a person at a terminal: a pipeline that has read the warning and has
// decided, with sudo somewhere in the middle of it, and has no way to pass a flag.
func Silenced() bool {
	for _, name := range []string{"RHO_NO_PRIVILEGE_WARNING", "RHO_SILENT"} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
		if value == "1" || value == "true" || value == "yes" {
			return true
		}
	}
	return false
}

// ownerOf is the uid that owns a path.
//
// Asked through the operating system rather than guessed from a name, because
// "root-owned" is a statement about a uid and not about a path: a home directory
// whose owner has uid 0 belongs to root whatever it is called.
func ownerOf(path string) (int, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		// A filesystem that does not answer this — a fuse mount, a network share —
		// is not evidence either way, and a warning about it would be a guess.
		return 0, false
	}
	return int(stat.Uid), true
}
