//
// @file privileged.go
// @brief The two operations that need root, and the guard that says so first.
//
// Almost nothing here needs root, and that is the design rather than an accident:
// a program fetched into the user's own directory belongs to that user, and a
// requirement a package manager can install is installed without privileges. So
// there are exactly two things left that do, both of them files in a system
// directory, and both of them are a copy of a file that is written out somewhere a
// person can read before it is put in place.
//
// Which is why each one asks, and says what it is about to do, rather than
// appearing as a password prompt with no context — and why running the whole
// toolchain as root is refused at the entry point rather than merely discouraged.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package keeper

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"rho/src/privilege"
)

// CheckNotRoot says what is wrong with running this as root.
//
// The same words the CLI uses, because it is the same judgement — and a dashboard
// that said something different from the terminal it is beside would be two tools
// disagreeing about the machine they are both reporting on. It is a warning and
// never a failure, for the same reason: a container runs as root.
func CheckNotRoot(out io.Writer) {
	privilege.Warn(out)
}

// elevationAvailable is whether this machine can ask for privileges at all.
func elevationAvailable() (string, []string, bool) {
	for _, candidate := range []string{"sudo", "doas"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil, true
		}
	}
	return "", nil, false
}

// elevated is one command run with privileges, for one purpose.
//
// Not a general "run this as root": there is no path through this type from a
// request made over HTTP, and no way to reach it with an argument a caller did not
// write out in this file. The only things it runs are the ones below.
type elevated struct {
	command string
	args    []string
}

// run asks for privileges and runs one command, showing both first.
//
// The two streams are connected so that `sudo` can ask for a password: a
// non-interactive sudo fails, and a tool that runs it non-interactively cannot
// install a udev rule at all — which would make the elevation prong useless.
func (e elevated) run(out io.Writer, in io.Reader, what string, args ...string) error {
	command, sudoArgs, ok := elevationAvailable()
	if !ok {
		return fmt.Errorf("%s needs root, and this machine has neither sudo nor doas: run this by hand as an administrator", what)
	}
	full := append(append(append([]string{command}, sudoArgs...), e.command), args...)
	fmt.Fprintf(out, "[rho-keeper] %s\n", what)
	fmt.Fprintf(out, "[%s] %s\n", command, strings.Join(full, " "))
	process := exec.Command(command, append(append(sudoArgs, e.command), args...)...)
	process.Stdin = in
	process.Stdout = out
	process.Stderr = out
	if err := process.Run(); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// InstallUdevRule puts a rule where the host will read it.
//
// Three things happen here that are the difference between this being usable and
// this being something a person turns off:
//
//   - the rule is compared before it is copied, so a second run does nothing and a
//     person who has edited the host's copy does not have it overwritten
//   - the source file is named and shown, because it is a file from a repository
//     about to be written into a system directory
//   - privileges are asked for once, for this copy, and the reload is a second
//     command that is only run if the copy worked
func InstallUdevRule(source string, target string, out io.Writer, in io.Reader) error {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
		return fmt.Errorf("the rule has no source file or no target path, so there is nothing to install")
	}
	contents, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read the rule at %s: %w", source, err)
	}
	if existing, readErr := os.ReadFile(target); readErr == nil {
		if sameBytes(existing, contents) {
			fmt.Fprintf(out, "[rho-keeper] %s is already there and says the same thing.\n", target)
			return nil
		}
		// Different, so somebody has edited the host's copy or the version has moved.
		// Replaced — but said, because a file in /etc that changed without being
		// mentioned is a file nobody will trust afterwards.
		fmt.Fprintf(out, "[rho-keeper] %s differs from %s and will be replaced.\n", target, source)
	}

	if err := (elevated{command: "cp"}).run(out, in,
		fmt.Sprintf("copying %s to %s, which needs root", source, target),
		source, target); err != nil {
		return err
	}
	// Only if the copy worked: reloading rules for a rule that is not there
	// achieves nothing except a slower boot.
	if _, err := exec.LookPath("udevadm"); err == nil {
		if err := (elevated{command: "udevadm"}).run(out, in,
			"reloading the device rules so the copy takes effect",
			"control", "--reload-rules"); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "[rho-keeper] udevadm is not here; the rules take effect on the next boot.\n")
	}
	fmt.Fprintf(out, "[rho-keeper] %s installed. Unplug the probe and plug it back in.\n", target)
	return nil
}

// RemoveUdevRule takes a rule away, which is the other half of putting one there.
func RemoveUdevRule(target string, out io.Writer, in io.Reader) error {
	if _, err := os.Stat(target); err != nil {
		return nil
	}
	return (elevated{command: "rm"}).run(out, in, "removing "+target, target)
}

// sameBytes is whether two files say the same thing.
func sameBytes(left []byte, right []byte) bool {
	leftHash, rightHash := sha256.Sum256(left), sha256.Sum256(right)
	return hex.EncodeToString(leftHash[:]) == hex.EncodeToString(rightHash[:])
}

// RuleSourceFor is where a rule file is in the module that declares it.
//
// Found rather than assumed: a rule is a file somebody wrote and a person should be
// able to read it before it goes into a system directory. A missing one is said, and
// nothing is copied.
func RuleSourceFor(source string, roots []string) (string, bool) {
	if strings.TrimSpace(source) == "" {
		return "", false
	}
	if filepath.IsAbs(source) {
		if _, err := os.Stat(source); err == nil {
			return source, true
		}
		return "", false
	}
	for _, root := range roots {
		candidate := filepath.Join(root, filepath.FromSlash(source))
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}
