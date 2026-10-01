//
// @file testing.go
// @brief Which of a project's tests this build is eligible to build.
//
// A module that has unit tests says which kind of target they are for. A build
// for another kind does not register them, because a host test links a hosted C
// library and a library built for a microcontroller has no `_kill` or `_getpid`
// for it to resolve against.
//
// What went wrong without this is worth stating, because the symptom pointed
// nowhere near the cause: a bare-metal build failed at the link step, naming a
// symbol from a libc that has no reason to exist on the target. The actual
// decision — whether to build a test at all — had been made implicitly, by a
// build file that registered it unconditionally.
//
// The flags are named for what they mean rather than for what they switch, so a
// build file reads as a statement about the target instead of a switch flipped
// somewhere else.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"fmt"
	"path/filepath"

	"rho/src/deps"
)

// Flag names a build's own statements about what it is building.
//
// They are prefixed so they cannot be mistaken for CMake's, several of which are
// computed by CMake itself rather than set by a caller — `CMAKE_CROSSCOMPILING`
// among them, which a cache entry does not change.
const (
	// FlagBareMetal says the build has no operating system underneath it. It is
	// the difference between a hosted library and one that has to provide its own
	// syscall layer.
	FlagBareMetal = "RHO_TARGET_IS_BAREMETAL"
	// FlagHostTests says the module declared host tests and this target is the
	// one they are for.
	FlagHostTests = "RHO_BUILD_HOST_TESTS"
	// FlagTargetTests says the module declared on-device tests and this target is
	// the one they are for.
	FlagTargetTests = "RHO_BUILD_TARGET_TESTS"
)

// testFlagsFor evaluates what a project's dependencies declared about their
// tests, against the target being built.
//
// Every dependency gets a say, because every dependency may have tests, and a
// build that consulted only the project would silently skip the tests of
// everything it pulled in. A kind is enabled when any dependency declares it for
// this target, because a build file registers each dependency's own tests and
// needs the flag to be true if that dependency has any.
func testFlagsFor(projectRoot, targetName string) []string {
	bareMetal := targetIsBareMetal(targetName)

	flags := []string{
		fmt.Sprintf("-D%s=%s", FlagBareMetal, onOff(bareMetal)),
		fmt.Sprintf("-D%s=OFF", FlagHostTests),
		fmt.Sprintf("-D%s=OFF", FlagTargetTests),
	}

	host := false
	onDevice := false
	for _, dependency := range testingDependencies(projectRoot) {
		if dependency.Testing == nil {
			continue
		}
		if dependency.Testing.HostTests.AppliesTo(targetName) {
			host = true
		}
		if dependency.Testing.TargetTests.AppliesTo(targetName) {
			onDevice = true
		}
	}
	if host {
		flags[1] = fmt.Sprintf("-D%s=ON", FlagHostTests)
	}
	if onDevice {
		flags[2] = fmt.Sprintf("-D%s=ON", FlagTargetTests)
	}
	return flags
}

// targetIsBareMetal reports whether a target has no operating system under it.
//
// The test is the name, and it is a name rather than a computed property on
// purpose: `native` is what the host is called, and a target that was not the
// host is one that has to bring its own runtime. That is the same statement the
// target specs make about themselves, read the same way.
func targetIsBareMetal(targetName string) bool {
	return targetName != "native"
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

// testingDependencies reads every C dependency a project declares, and what those
// dependencies declare, because a dependency's own tests are part of building
// the project that pulled it in.
func testingDependencies(projectRoot string) []*deps.Manifest {
	if projectRoot == "" {
		return nil
	}
	found := []*deps.Manifest{}
	seen := map[string]bool{}

	var walk func(base string, manifest *deps.Manifest, depth int)
	walk = func(base string, manifest *deps.Manifest, depth int) {
		if depth > 16 {
			return
		}
		for _, dep := range manifest.Deps {
			if dep.Type != deps.DepTypeC || dep.Path == "" {
				continue
			}
			root := dep.Path
			if !filepath.IsAbs(root) {
				root = filepath.Join(base, root)
			}
			child, err := deps.LoadManifest(deps.ManifestPath(root))
			if err != nil || seen[root] {
				continue
			}
			seen[root] = true
			found = append(found, child)
			walk(root, child, depth+1)
		}
	}

	if manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot)); err == nil && manifest != nil {
		walk(projectRoot, manifest, 0)
	}
	return found
}
