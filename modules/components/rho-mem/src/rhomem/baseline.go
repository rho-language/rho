//
// @file baseline.go
// @brief What the last build looked like.
//
// A capacity says where the ceiling is. It does not say whether the thing under
// it is getting bigger, and that is the question a pull request actually raises:
// this firmware fits in its flash, and this firmware is 4 kB bigger than the one
// on the base branch, and nobody decided that was fine.
//
// So a run can record what it found, and a later run can compare against it. The
// record is a file in the project, which means it is reviewable — a diff on a
// baseline is a diff anybody can read, where a number that appears only in a log
// is a number nobody notices changing — and it means the comparison does not need
// the other build to still exist. A baseline branch can be rebuilt, deleted, or
// never have been checked out on the machine running the check.
//
// What is recorded is deliberately the whole region list and not just a total. A
// total cannot be attributed: "grew by 4 kB" is a question, and the answer is
// which region, which is the next line of the file.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomem

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BaselineVersion is the shape of a recorded baseline.
//
// A version rather than nothing, because a file in a repository outlives the tool
// that wrote it: a baseline recorded by one release of this component and read by
// another is a thing that has to be refused or understood, not silently
// half-parsed.
const BaselineVersion = 1

// Baseline is what a previous run measured.
type Baseline struct {
	Version int `json:"version"`
	// Recorded is when the measurement was taken, which is what makes a stale
	// baseline visible: a baseline from a release ago is a comparison against the
	// wrong thing, and it looks exactly like a real one otherwise.
	Recorded string `json:"recorded"`
	// Target is the board these figures are for. A comparison across boards is a
	// meaningless number — flash and RAM sizes and addresses differ — so a
	// baseline for another target is refused rather than compared.
	Target string `json:"target,omitempty"`
	// Source is what the figures were read from: `map`, `elf` or `map+elf`.
	Source   string            `json:"source,omitempty"`
	Regions  []BaselineRegion  `json:"regions"`
	Total    uint64            `json:"total"`
	Symbols  []BaselineSymbol  `json:"symbols,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// BaselineRegion is one region's recorded usage.
type BaselineRegion struct {
	Name     string `json:"name"`
	Used     uint64 `json:"used"`
	Length   uint64 `json:"length,omitempty"`
	FromLoad uint64 `json:"from_load,omitempty"`
}

// BaselineSymbol is one recorded symbol, for the symbols whose absence is itself
// a finding.
type BaselineSymbol struct {
	Name   string `json:"name"`
	Size   uint64 `json:"size"`
	Object string `json:"object,omitempty"`
}

// Region is a recorded region's usage, and whether there is one.
func (b *Baseline) Region(name string) (BaselineRegion, bool) {
	if b == nil {
		return BaselineRegion{}, false
	}
	for _, one := range b.Regions {
		if strings.EqualFold(one.Name, name) {
			return one, true
		}
	}
	return BaselineRegion{}, false
}

// Record writes a baseline for an analysis.
func Record(analysis *Analysis, top int) *Baseline {
	out := &Baseline{
		Version:  BaselineVersion,
		Recorded: time.Now().UTC().Format(time.RFC3339),
		Target:   analysis.Target,
		Source:   analysis.Source,
	}
	for _, usage := range analysis.Usage {
		out.Regions = append(out.Regions, BaselineRegion{
			Name: usage.Name, Used: usage.Used, Length: usage.Length, FromLoad: usage.FromLoad,
		})
	}
	out.Total = analysis.Total()

	// The largest symbols across every region, because "what got bigger" is
	// answered by a symbol more often than by a region.
	type sized struct {
		symbol PlacedSymbol
		region string
	}
	all := []sized{}
	for _, usage := range analysis.Usage {
		for _, symbol := range usage.Symbols {
			all = append(all, sized{symbol: symbol, region: usage.Name})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].symbol.Size > all[j].symbol.Size })
	if top > 0 && len(all) > top {
		all = all[:top]
	}
	for _, one := range all {
		out.Symbols = append(out.Symbols, BaselineSymbol{
			Name: one.symbol.Name, Size: one.symbol.Size, Object: one.symbol.Object,
		})
	}
	return out
}

// ReadBaseline reads a recorded baseline.
//
// A file that is not there is reported as not there rather than as an error: no
// baseline yet is the ordinary state of a project on its first day, and refusing
// to run over it would mean the check could not be adopted before the first
// measurement had been taken.
func ReadBaseline(path string) (*Baseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no baseline at %s; record one with `--write-baseline`, which is what the first run of a project does", path)
		}
		return nil, err
	}
	out := &Baseline{}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("read the baseline at %s: %w", path, err)
	}
	if out.Version != BaselineVersion {
		return nil, fmt.Errorf("the baseline at %s is version %d and this component writes version %d",
			path, out.Version, BaselineVersion)
	}
	return out, nil
}

// WriteBaseline records a baseline into a project's tree.
func WriteBaseline(path string, baseline *Baseline) error {
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// BaselineFromGit reads the baseline as a git ref carries it.
//
// The other way to have a baseline, and the one a pull request wants: the file
// committed on the base branch, rather than one that was written here and
// committed. A ref that cannot be read is reported with what git said, because
// "no baseline" and "the branch is called something else" are different problems
// and a pipeline should be able to tell them apart.
func BaselineFromGit(root, ref, path string) (*Baseline, error) {
	command := execCommand(root, "git", "show", ref+":"+filepath.ToSlash(path))
	data, err := command.Output()
	if err != nil {
		if detail := strings.TrimSpace(string(errOutput(err))); detail != "" {
			return nil, fmt.Errorf("read %s from %s: %s", path, ref, firstLine(detail))
		}
		return nil, fmt.Errorf("read %s from %s: %w", path, ref, err)
	}
	out := &Baseline{}
	if err := json.Unmarshal(data, out); err != nil {
		return nil, fmt.Errorf("read the baseline at %s from %s: %w", path, ref, err)
	}
	if out.Version != BaselineVersion {
		return nil, fmt.Errorf("the baseline at %s in %s is version %d and this component writes version %d",
			path, ref, out.Version, BaselineVersion)
	}
	return out, nil
}

func firstLine(text string) string {
	if index := strings.IndexByte(text, '\n'); index >= 0 {
		return text[:index]
	}
	return text
}
