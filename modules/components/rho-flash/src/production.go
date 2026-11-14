//
// @file production.go
// @brief Rho Flash — the production steps a board's platform declares, run only
// when asked for and only after the board's name has been typed.
//
// A production build hardens a board in ways that cannot be undone: an ESP32
// burns eFuses for flash encryption and secure boot, an RP2350 writes its boot
// key hash into OTP. None of that happens on an ordinary `rho flash`. It happens
// with `--production`, which prints every step that will be run — the exact
// command, and whether it is irreversible — and then asks for the board's name
// to be typed. Anything else typed, or nothing, stops before the first step.
//
// What the steps are is the platform's to say, not this program's: the platform
// module ships `production/<name>.json`, looked up by the target's name, its
// preset, its family and finally `production.json`, so a board, a part family or
// the whole platform can declare one. A platform with no plan has no production
// mode, and saying so is better than flashing an ordinary image under that name.
//
// `--dry-run` prints the same plan and the flash command and runs nothing.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// When a production step runs, relative to writing the image.
const (
	StepBeforeFlash = "before-flash"
	StepAfterFlash  = "after-flash"
	// A step the firmware performs itself, the first time the production image
	// boots. It has no command here; it is printed so the person confirming
	// knows that booting the image is itself irreversible.
	StepFirstBoot = "first-boot"
)

// ProductionStep is one thing a production flash does to a board.
type ProductionStep struct {
	// Description says what the step does to the part, in words.
	Description string `json:"description"`
	// Irreversible marks a step that cannot be undone: an eFuse, an OTP row.
	Irreversible bool `json:"irreversible"`
	// When is before-flash, after-flash or first-boot.
	When string `json:"when"`
	// Command is run like a flash command — split, never handed to a shell, with
	// `{binary}` and `{env:NAME}` substituted, and `{platform}` standing for the
	// platform module's directory so a step can run a tool the platform ships.
	// Empty for a first-boot step.
	Command string `json:"command,omitempty"`
	// Show names a file whose contents the step burns — an OTP JSON, a key
	// digest — with the same placeholders as Command. It is printed in full
	// under the step before the confirmation, so what is typed for is what the
	// person has read; a file that is not there stops the flash before it asks.
	Show string `json:"show,omitempty"`
}

// ProductionPlan is what a platform declares for a production flash.
type ProductionPlan struct {
	// Name is the plan's own name, for the printout.
	Name string `json:"name"`
	// Summary says in a sentence what production means on this platform.
	Summary string `json:"summary,omitempty"`
	// Build says how the image has to have been built for the plan to be safe:
	// the production preset, the signing key. It is printed, not checked.
	Build string `json:"build,omitempty"`
	// Steps are run in order: before-flash, the flash, after-flash.
	Steps []ProductionStep `json:"steps"`
	// Notes are printed after the steps: what is reversible and how, what the
	// plan deliberately does not do.
	Notes []string `json:"notes,omitempty"`

	// Path is where the plan was read from, and PlatformRoot the platform module
	// it belongs to.
	Path         string `json:"-"`
	PlatformRoot string `json:"-"`
}

// irreversible reports whether any step of the plan cannot be undone.
func (p ProductionPlan) irreversible() bool {
	for _, step := range p.Steps {
		if step.Irreversible {
			return true
		}
	}
	return false
}

// validate refuses a plan that could not be run as written.
func (p ProductionPlan) validate() error {
	if len(p.Steps) == 0 {
		return fmt.Errorf("%s declares no steps", p.Path)
	}
	for i, step := range p.Steps {
		switch step.When {
		case StepBeforeFlash, StepAfterFlash:
			if strings.TrimSpace(step.Command) == "" {
				return fmt.Errorf("%s: step %d (%s) runs %s but has no command", p.Path, i+1, step.Description, step.When)
			}
		case StepFirstBoot:
			if strings.TrimSpace(step.Command) != "" {
				return fmt.Errorf("%s: step %d (%s) happens at first boot and cannot have a command", p.Path, i+1, step.Description)
			}
		default:
			return fmt.Errorf("%s: step %d (%s) has when %q; it must be %s, %s or %s",
				p.Path, i+1, step.Description, step.When, StepBeforeFlash, StepAfterFlash, StepFirstBoot)
		}
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("%s: step %d has no description", p.Path, i+1)
		}
	}
	return nil
}

// productionCandidates are the file names a plan is looked up by, most specific
// first: the board, its preset, its family, the whole platform.
func productionCandidates(target, preset, family string) []string {
	names := []string{}
	seen := map[string]bool{}
	for _, name := range []string{target, preset, family, "production"} {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || seen[name] || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
			continue
		}
		seen[name] = true
		names = append(names, name+".json")
	}
	return names
}

// loadProductionPlan reads the plan a platform declares for a target, from
// `<platformRoot>/production/`.
func loadProductionPlan(platformRoot, target, preset, family string) (ProductionPlan, error) {
	if platformRoot == "" {
		return ProductionPlan{}, fmt.Errorf("no platform module was found for this project, so no production plan either")
	}
	dir := filepath.Join(platformRoot, "production")
	for _, name := range productionCandidates(target, preset, family) {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		plan := ProductionPlan{}
		if err := json.Unmarshal(data, &plan); err != nil {
			return ProductionPlan{}, fmt.Errorf("%s is not a production plan: %w", path, err)
		}
		plan.Path = path
		plan.PlatformRoot = platformRoot
		if plan.Name == "" {
			plan.Name = strings.TrimSuffix(name, ".json")
		}
		if err := plan.validate(); err != nil {
			return ProductionPlan{}, err
		}
		return plan, nil
	}
	return ProductionPlan{}, fmt.Errorf("the platform at %s declares no production plan for %q (looked for %s in %s); this board has no production mode",
		platformRoot, target, strings.Join(productionCandidates(target, preset, family), ", "), dir)
}

// renderedStep is a step with its command already expanded, so what is printed
// is exactly what will be run.
type renderedStep struct {
	ProductionStep
	Program string
	Args    []string
	// ShowPath and ShowText are the file the step burns and its contents.
	ShowPath string
	ShowText string
}

// maxShownBytes bounds what a step may print: an OTP file is a few hundred
// bytes, and a plan pointing at something much larger is pointing at the
// wrong file.
const maxShownBytes = 16 * 1024

// commandLine is the step's command as it will run, quoted where an argument
// has a space in it.
func (s renderedStep) commandLine() string {
	if s.Program == "" {
		return ""
	}
	parts := []string{quoteArg(s.Program)}
	for _, arg := range s.Args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" || strings.ContainsAny(arg, " \t\"'") {
		return `"` + strings.ReplaceAll(arg, `"`, `\"`) + `"`
	}
	return arg
}

// renderSteps expands every step's command for a binary. A placeholder that
// cannot be filled — an environment variable the plan needs and nobody set —
// is an error before anything is printed or run.
func renderSteps(plan ProductionPlan, binary, platformRoot string) ([]renderedStep, error) {
	out := make([]renderedStep, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		rendered := renderedStep{ProductionStep: step}
		if strings.TrimSpace(step.Command) != "" {
			command := strings.ReplaceAll(step.Command, "{platform}", platformRoot)
			program, args, err := Flash{Command: command}.invocationExact(binary)
			if err != nil {
				return nil, fmt.Errorf("production step %q: %w", step.Description, err)
			}
			rendered.Program, rendered.Args = program, args
		}
		if strings.TrimSpace(step.Show) != "" {
			path, err := expandPlaceholders(strings.ReplaceAll(step.Show, "{platform}", platformRoot), binary)
			if err != nil {
				return nil, fmt.Errorf("production step %q: %w", step.Description, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("production step %q burns the contents of %s, which cannot be read: %w", step.Description, path, err)
			}
			if len(data) > maxShownBytes {
				return nil, fmt.Errorf("production step %q: %s is %d bytes, more than a file it burns should be", step.Description, path, len(data))
			}
			rendered.ShowPath, rendered.ShowText = path, string(data)
		}
		out = append(out, rendered)
	}
	return out, nil
}

// invocationExact renders a command like invocation does, but never appends the
// binary: a production step that does not name the binary does not take it.
func (f Flash) invocationExact(binary string) (string, []string, error) {
	argv := splitCommand(f.Command)
	if len(argv) == 0 {
		return "", nil, fmt.Errorf("the command is empty")
	}
	out := make([]string, 0, len(argv))
	for _, arg := range argv {
		expanded, err := expandPlaceholders(arg, binary)
		if err != nil {
			return "", nil, err
		}
		out = append(out, expanded)
	}
	return out[0], out[1:], nil
}

// printPlan writes what a production flash will do, in the order it does it.
func printPlan(w io.Writer, target, binary string, plan ProductionPlan, steps []renderedStep, flashLine string) {
	fmt.Fprintf(w, "\nPRODUCTION FLASH of %s for board %q\n", binary, target)
	fmt.Fprintf(w, "plan: %s (%s)\n", plan.Name, plan.Path)
	if plan.Summary != "" {
		fmt.Fprintf(w, "\n%s\n", plan.Summary)
	}
	if plan.Build != "" {
		fmt.Fprintf(w, "\nThe image must have been built for production: %s\n", plan.Build)
	}
	fmt.Fprintln(w, "\nThese steps will run, in this order:")
	n := 0
	emit := func(s renderedStep) {
		n++
		mark := "reversible"
		if s.Irreversible {
			mark = "IRREVERSIBLE"
		}
		fmt.Fprintf(w, "\n  %d. [%s] %s\n", n, mark, s.Description)
		if line := s.commandLine(); line != "" {
			fmt.Fprintf(w, "       runs: %s\n", line)
		}
		if s.ShowPath != "" {
			fmt.Fprintf(w, "       burns the contents of %s:\n", s.ShowPath)
			for _, text := range strings.Split(strings.TrimRight(s.ShowText, "\n"), "\n") {
				fmt.Fprintf(w, "         | %s\n", text)
			}
		}
	}
	for _, s := range steps {
		if s.When == StepBeforeFlash {
			emit(s)
		}
	}
	n++
	fmt.Fprintf(w, "\n  %d. [reversible] write the image to the board\n       runs: %s\n", n, flashLine)
	for _, s := range steps {
		if s.When == StepAfterFlash {
			emit(s)
		}
	}
	firstBoot := false
	for _, s := range steps {
		if s.When == StepFirstBoot {
			if !firstBoot {
				fmt.Fprintln(w, "\nThe first time the production image boots, the firmware itself will:")
				firstBoot = true
			}
			emit(s)
		}
	}
	for _, note := range plan.Notes {
		fmt.Fprintf(w, "\nnote: %s\n", note)
	}
	if plan.irreversible() {
		fmt.Fprintln(w, "\nSteps marked IRREVERSIBLE cannot be undone. A wrong key or a wrong image after them can leave the board unusable.")
	}
}

// confirmBoardName asks for the board's name and reads one line. Only the exact
// name continues; anything else, an empty line or the end of the input stops.
// There is deliberately no flag that answers for the person.
func confirmBoardName(in io.Reader, out io.Writer, target string) error {
	fmt.Fprintf(out, "\nType the board name (%s) to continue, anything else to stop: ", target)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("reading the confirmation: %w", err)
	}
	if strings.TrimSpace(line) != target {
		fmt.Fprintln(out)
		return fmt.Errorf("not confirmed: nothing was run")
	}
	return nil
}

// runStep runs one rendered step, its output going to the terminal.
func runStep(s renderedStep) error {
	if _, err := exec.LookPath(s.Program); err != nil {
		return fmt.Errorf("production step %q needs %s, which is not installed: %w", s.Description, s.Program, err)
	}
	fmt.Printf("production: %s\n", s.Description)
	cmd := exec.Command(s.Program, s.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("production step %q failed: %w", s.Description, err)
	}
	return nil
}

// productionOptions are how a production flash is asked for.
type productionOptions struct {
	Production bool
	DryRun     bool
	In         io.Reader
	Out        io.Writer
}

// flashProduction prints the plan, asks for the board's name, and runs the
// before-flash steps, the flash, and the after-flash steps. The first failure
// stops it: a step after a failed one would be run on a board in a state the
// plan did not expect.
func flashProduction(target, binary string, plan ProductionPlan, program string, args []string, options productionOptions) error {
	steps, err := renderSteps(plan, binary, plan.PlatformRoot)
	if err != nil {
		return err
	}
	flashLine := renderedStep{Program: program, Args: args}.commandLine()
	printPlan(options.Out, target, binary, plan, steps, flashLine)
	if options.DryRun {
		fmt.Fprintln(options.Out, "\ndry run: nothing was run")
		return nil
	}
	if err := confirmBoardName(options.In, options.Out, target); err != nil {
		return err
	}
	for _, s := range steps {
		if s.When == StepBeforeFlash {
			if err := runStep(s); err != nil {
				return err
			}
		}
	}
	if err := runFlash(target, program, args, binary); err != nil {
		return err
	}
	for _, s := range steps {
		if s.When == StepAfterFlash {
			if err := runStep(s); err != nil {
				return err
			}
		}
	}
	return nil
}
