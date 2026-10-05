//
// @file stackreq.go
// @brief The stack a program needs, read from the project that computed it.
//
// A project's build writes down what its deepest call path needs — see
// `rho-stack.txt` in the generated project — and rho-target puts that in the
// linker script. So the number in the script is the program's requirement rather
// than a number somebody picked, and a project that has one never had to guess.
//
// Precedence, most specific first:
//
//  1. `linker.stack_bytes` in the target spec, or `build.stack_bytes` in the
//     project's own manifest. A person saying a number beats a tool computing one,
//     because a person may know something the analysis cannot see — a board with a
//     different stack, a library that recurses through a dispatcher.
//  2. The computed record, when the project wrote one.
//  3. The target's default.
//
// The computed record is only used when it says it is a bound. `bound: false`
// means the analysis left something out — a recursive function, a call into code
// it could not read — and a figure that is knowingly short of the truth is not
// better than a guess that is honestly a guess. It is reported either way, so the
// project can see what the analysis found and decide.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StackRecordFile is the file a project's build writes its computed stack
// requirement to, relative to the generated project.
//
// Named here as well as in projectgen because this is the side that reads it, and
// two sides agreeing on a name by accident is a bug waiting for a rename.
const StackRecordFile = "rho-stack.txt"

// StackRequirement is what a project said its program needs.
type StackRequirement struct {
	// Bytes is the deepest call path's requirement, or zero when nothing was
	// recorded.
	Bytes uint64
	// OwnBytes is the same total counting only the calls the analysis could read.
	// A target that declares what its own C costs adds those to this.
	OwnBytes uint64
	// Bound says whether that is a bound or an estimate. Only a bound is used as
	// one.
	Bound bool
	// Path is the call chain that reaches it, for a report.
	Path string
	// Unseen are the calls the analysis could not read, which is what makes a
	// figure an estimate.
	Unseen []string
	// Recursive are the functions found to reach themselves.
	Recursive []string
	// Source is the file it was read from, for a diagnostic.
	Source string
}

// ReadStackRequirement is what a generated project recorded, if anything.
//
// A file that is not there is not a failure: a C project has no Rho compiler to
// compute one, and a project built before this existed does not either. Both fall
// back to the target's declared size, which is what they did before.
func ReadStackRequirement(projectRoot string) (StackRequirement, bool) {
	path := filepath.Join(projectRoot, StackRecordFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return StackRequirement{}, false
	}
	out := StackRequirement{Source: path}
	// A record written before `own_bytes` existed, or by something that did not
	// separate the two, falls back to the total: the correction is then skipped
	// rather than applied to a number that already includes the charge.
	if out.OwnBytes == 0 {
		out.OwnBytes = out.Bytes
	}
	for _, line := range strings.Split(string(data), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, found := strings.Cut(text, ":")
		if !found {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch key {
		case "bytes":
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
				out.Bytes = parsed
			}
		case "own_bytes":
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
				out.OwnBytes = parsed
			}
		case "bound":
			out.Bound = value == "true"
		case "path":
			out.Path = value
		case "unseen":
			if value != "" {
				out.Unseen = strings.Split(value, ",")
			}
		case "recursive":
			if value != "" {
				out.Recursive = strings.Split(value, ",")
			}
		}
	}
	return out, out.Bytes > 0
}

// Basis is where the figure came from, for a build log that says what was used.
func (r StackRequirement) Basis(declared map[string]uint64) string {
	if r.Bound {
		return "computed from the program, with nothing left out"
	}
	if len(declared) > 0 {
		return "computed from the program, with this target's own C accounted for by declaration"
	}
	return r.Describe()
}

// Describe is the requirement in a sentence, for a build log.
func (r StackRequirement) Describe() string {
	kind := "estimate"
	if r.Bound {
		kind = "bound"
	}
	out := fmt.Sprintf("%s of %d bytes of stack", kind, r.Bytes)
	if r.Path != "" {
		out += fmt.Sprintf(", deepest path %s", r.Path)
	}
	if len(r.Unseen) > 0 {
		out += fmt.Sprintf("; %d call(s) the analysis could not read", len(r.Unseen))
	}
	if len(r.Recursive) > 0 {
		out += fmt.Sprintf("; recursive (%s), so no worst case exists",
			strings.Join(r.Recursive, ", "))
	}
	return out
}
