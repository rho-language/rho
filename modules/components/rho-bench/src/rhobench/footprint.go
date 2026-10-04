//
// @file footprint.go
// @brief The static figure, asked of the component that owns it.
//
// A benchmark measures what happened while the code ran. It says nothing about what
// the code *costs* to be on the board: the flash it occupies whether it runs or not,
// and the RAM it will need before its first instruction. Those are the numbers
// `rho-mem` exists to produce, from the linker map and the ELF beside it.
//
// So this asks, rather than reading a `.map` a second time.
//
// The call is a component call and not a library import, for the reason it is
// everywhere else in this toolchain: a component that linked another's library would
// be a copy of it that nobody extends, and the moment the other one got better this
// one would quietly be wrong. `rho mem --json` is the seam, and it is a documented
// document rather than a function signature — which is what lets a project replace
// it with a plugin, or with a version of rho-mem that answers differently.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhobench

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Footprint is the static memory a project costs, as `rho mem` reports it.
//
// Only the parts a benchmark run needs: the total, and each region by name so a
// budget can be written against the region rather than the sum. The whole document
// `rho mem` prints has more in it — verdicts, growth, diagnostics — and a benchmark
// report that repeated all of it would be reporting another component's findings
// under its own name.
type Footprint struct {
	Target  string            `json:"target,omitempty"`
	Source  string            `json:"source,omitempty"`
	Total   uint64            `json:"total_bytes"`
	Regions []FootprintRegion `json:"regions"`
}

// FootprintRegion is one region's static usage.
type FootprintRegion struct {
	Name   string `json:"name"`
	Used   uint64 `json:"used_bytes"`
	Length uint64 `json:"length_bytes,omitempty"`
	// Percent is the region's own share of its capacity, which `rho mem` computed
	// and this does not recompute — a percentage computed twice from two code paths
	// is a percentage that will eventually disagree.
	Percent float64 `json:"percent,omitempty"`
}

// memResult is the shape of `rho mem --json`, as much of it as this reads.
//
// Declared here rather than imported, for the reason the whole file is written the
// way it is: the two components agree on a document, not on a header.
type memResult struct {
	Target  string `json:"target"`
	Source  string `json:"source"`
	Total   uint64 `json:"total_bytes"`
	Regions []struct {
		Name    string  `json:"name"`
		Used    uint64  `json:"used_bytes"`
		Length  uint64  `json:"length_bytes"`
		Percent float64 `json:"percent"`
	} `json:"regions"`
}

// FootprintOptions is how the static figure is asked for.
type FootprintOptions struct {
	// RhoBinary is the `rho` CLI to go through, or "" to find it.
	RhoBinary string
	// RhoHome is where the toolchain is installed, which is where `rho mem` is
	// dispatched from when there is no CLI to ask.
	RhoHome string
	// ProjectRoot is the project whose artifacts are read.
	ProjectRoot string
	// Target is the board, when the build was for one.
	Target string
}

// ReadFootprint asks rho-mem for the static figure.
//
// A component that is not installed, or a project with nothing built, is not a
// failure: a benchmark measures runtime and says nothing about flash, so a run with
// no static figure is a run that answered the question it could. Said, rather than
// silently absent, so a reader knows the number is missing and not zero.
func ReadFootprint(options FootprintOptions) (*Footprint, []string) {
	notes := []string{}

	// `-project` is rho-mem's project directory and `--target` is the board within
	// it. Passing a board where a directory belongs is the sort of thing that reads
	// as "no such target" and costs an afternoon.
	arguments := []string{"--json", "--top", "0", "-project", options.ProjectRoot}
	if options.Target != "" && options.Target != "host" {
		arguments = append(arguments, "--target", options.Target)
	}

	// Through the CLI when there is one, because that is how a person runs it and
	// because the CLI is what knows how the component was installed. The flags
	// alone are a component's own command line, and handing them to the CLI without
	// the command name in front of them makes it report an unknown command.
	command, binary := footprintCommand(options, arguments)
	if binary == "" {
		notes = append(notes, "rho-mem was not found, so the static footprint is not in this report")
		return nil, notes
	}

	command.Dir = options.ProjectRoot
	output, err := command.Output()
	if err != nil {
		// A project with nothing built, or a budget that failed: neither is this
		// component's failure, and neither stops a runtime measurement.
		notes = append(notes, fmt.Sprintf("rho-mem could not report a static footprint: %s", firstLine(output)))
		return nil, notes
	}

	parsed := memResult{}
	if err := json.Unmarshal(output, &parsed); err != nil {
		notes = append(notes, fmt.Sprintf("rho-mem's report could not be read: %v", err))
		return nil, notes
	}
	out := &Footprint{Target: parsed.Target, Source: parsed.Source, Total: parsed.Total}
	for _, region := range parsed.Regions {
		out.Regions = append(out.Regions, FootprintRegion{
			Name: region.Name, Used: region.Used, Length: region.Length, Percent: region.Percent,
		})
	}
	return out, notes
}

// footprintCommand is how rho-mem is reached, and the program it ran.
func footprintCommand(options FootprintOptions, arguments []string) (*exec.Cmd, string) {
	if cli := options.RhoBinary; cli != "" {
		return exec.Command(cli, append([]string{"mem"}, arguments...)...), cli
	}
	if found, err := exec.LookPath("rho"); err == nil {
		return exec.Command(found, append([]string{"mem"}, arguments...)...), found
	}
	// No CLI: the component's own binary, which is what the CLI would have run.
	root := options.RhoHome
	if root == "" {
		return nil, ""
	}
	resolved, err := latestInstall(filepath.Join(root, "modules", "installed", "component", "rho-mem"))
	if err != nil {
		return nil, ""
	}
	binary := filepath.Join(resolved, "rho-mem")
	if _, err := os.Stat(binary); err != nil {
		return nil, ""
	}
	return exec.Command(binary, arguments...), binary
}

// latestInstall is the newest installed version of a component directory.
//
// Newest, not alphabetically last: a component can be installed at several versions
// while one is being replaced, and the newest is the one the CLI would have run.
func latestInstall(root string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "*@*"))
	if err != nil {
		return "", err
	}
	best := ""
	var bestTime int64
	for _, candidate := range matches {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().UnixNano() > bestTime {
			best, bestTime = candidate, info.ModTime().UnixNano()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no installed component under %s", root)
	}
	return best, nil
}

// writeFootprint is the static figure in the report, beside the runtime figures.
func writeFootprint(out io.Writer, footprint *Footprint, options Options) {
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "%sStatic footprint (rho-mem):%s\n", paint(options.Color, ansiBold), reset(options.Color))
	for _, region := range footprint.Regions {
		if region.Used == 0 {
			continue
		}
		fmt.Fprintf(out, "  %-10s %10s", region.Name, bytes(region.Used))
		if region.Length > 0 {
			fmt.Fprintf(out, " / %-10s %5.1f%%", bytes(region.Length), region.Percent)
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintf(out, "  %-10s %10s\n", "TOTAL", bytes(footprint.Total))
}

func firstLine(output []byte) string {
	said := strings.TrimSpace(string(output))
	if index := strings.IndexByte(said, '\n'); index >= 0 {
		said = said[:index]
	}
	if said == "" {
		return "it said nothing"
	}
	return said
}
