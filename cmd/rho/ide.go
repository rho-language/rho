//
// @file ide.go
// @brief `rho ide` — write what an editor needs, from what the project declares.
//
// An editor is configured by hand and then left to drift: the launch
// configuration names a path that a build moved, the task runs a command that
// gained a flag. Everything those files hold is already written down in the
// project's manifest and in the target spec it names, so `rho ide` reads it and
// writes the files rather than asking for them again.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	modcore "rho/cmd/rho-module/core"

	"rho/src/deps"
	"rho/src/ide"
)

// ideCLI writes what an editor reads, for one editor, from what a project
// declares.
func ideCLI(args []string) int {
	if len(args) == 0 {
		ideUsage()
		return 2
	}

	editor, err := ide.ParseEditor(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho ide:", err)
		return 2
	}

	var (
		project = "."
		target  = ""
	)
	rest := args[1:]
	for index := 0; index < len(rest); index++ {
		switch {
		case rest[index] == "-target" && index+1 < len(rest):
			target = rest[index+1]
			index++
		case strings.HasPrefix(rest[index], "-target="):
			target = strings.TrimPrefix(rest[index], "-target=")
		case strings.HasPrefix(rest[index], "-"):
			fmt.Fprintf(os.Stderr, "rho ide: unknown flag %q\n", rest[index])
			ideUsage()
			return 2
		default:
			// The project directory is taken as the first path and the rest are
			// refused: naming two directories means one of them was meant for
			// something else, and silently using the first is how a person ends up
			// configuring a project they are not looking at.
			if project != "." {
				fmt.Fprintln(os.Stderr, "rho ide: name one project directory")
				return 2
			}
			project = rest[index]
		}
	}

	result, err := ide.Generate(ide.Request{
		ProjectRoot: project,
		Editor:      editor,
		Target:      target,
		Resolve:     resolveTargetFacts,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho ide:", err)
		return 1
	}

	// The target's facts are a fact about the machine rather than about the
	// project, so a generation that could not read them says so — after writing,
	// because a project with no probe still gets a working configuration and the
	// reader wants to know why it is the simpler one.
	for _, path := range result.Written {
		fmt.Println("Wrote", path)
	}
	for _, path := range result.Merged {
		fmt.Println("Updated", path, "(what was there before was kept)")
	}
	for _, path := range result.Unchanged {
		fmt.Println("Unchanged", path)
	}
	if note := targetNote(project, target); note != "" {
		fmt.Println()
		fmt.Println(note)
	}
	return 0
}

func ideUsage() {
	fmt.Println("Usage: rho ide <editor> [project-dir] [-target name]")
	fmt.Println()
	fmt.Println("Writes the files an editor reads, from what the project declares.")
	fmt.Println("An existing file is updated rather than replaced.")
	fmt.Println()
	fmt.Println("Editors:")
	for _, editor := range ide.Editors() {
		fmt.Printf("  %s\n", editor)
	}
}

// targetNote is what a generation has to say about the target it wrote for, if
// anything.
//
// The interesting case is a target spec that could not be read: the
// configuration that was written is then one for the host rather than for a
// board, which is correct but is not what somebody who declared `build.toolchain:
// pico` asked for. Saying so is the difference between a working session and a
// confusing one.
func targetNote(project, target string) string {
	manifest, err := loadProjectManifest(project)
	if err != nil {
		return ""
	}
	name := target
	if name == "" {
		name = manifest.BuildToolchain
	}
	if name == "" {
		name = "native"
	}
	if _, resolveErr := resolveTargetFacts(project, name); resolveErr != nil {
		return fmt.Sprintf("%s\n%s", resolveErr, ideNextStep())
	}
	return ""
}

func ideNextStep() string {
	return "The configuration that was written is for this machine rather than for that board."
}

// resolveTargetFacts asks the target component what a target is.
//
// It is a component call rather than a library call for the same reason `rho
// build` reaches for rho-target: which targets exist, and what a board's probe
// and register map are, are facts the component carrying the specs holds, and
// extended by its plugins. A machine that has not installed it is told that a
// target needs it, which is a thing it can put right, rather than being handed a
// configuration for the host as though that were what was asked for.
func resolveTargetFacts(project, target string) (*ide.Target, error) {
	if project == "" {
		project = "."
	}
	module, err := modcore.LoadInstalledModule("rho-target")
	if err != nil {
		return nil, fmt.Errorf("a configuration for %s needs the rho-target component, which is not installed: %w", target, err)
	}
	entry := module.Entry()
	if entry == nil {
		return nil, fmt.Errorf("the rho-target component has no entry point")
	}

	// The spec is read as the component reports it — the resolved spec, with a
	// board's chip and architecture already folded in — and captured rather than
	// left on this process's stdout, because stdout here is the console a person
	// is reading.
	said := &bytes.Buffer{}
	read, write, pipeErr := os.Pipe()
	if pipeErr != nil {
		return nil, pipeErr
	}
	copied := make(chan struct{})
	go func() {
		defer close(copied)
		buffer := make([]byte, 4096)
		for {
			count, readErr := read.Read(buffer)
			if count > 0 {
				said.Write(buffer[:count])
			}
			if readErr != nil {
				return
			}
		}
	}()
	runErr := entry.Run([]string{"show", "-json", "-project", project, target}, os.Stdin, write)
	write.Close()
	<-copied
	read.Close()
	if runErr != nil {
		message := strings.TrimSpace(said.String())
		if message == "" {
			return nil, fmt.Errorf("rho target show %s: %w", target, runErr)
		}
		return nil, fmt.Errorf("%s", message)
	}
	return ide.ParseTarget(said.Bytes())
}

// loadProjectManifest is the project's own manifest, read through the loader the
// build reads it with, so `rho ide` and `rho build` cannot disagree about what a
// project declares.
func loadProjectManifest(project string) (*deps.Manifest, error) {
	if project == "" {
		project = "."
	}
	return deps.LoadManifest(deps.ManifestPath(project))
}
