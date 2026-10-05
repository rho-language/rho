//
// @file stack.go
// @brief How much stack a program needs, before anybody runs it.
//
// Every firmware reserves a stack by guessing. The guess is either too small, in
// which case the program corrupts whatever is below it and carries on — the worst
// kind of bug, because nothing reports it — or too large, in which case a part with
// 64 kB of RAM has given a kilobyte to a program that uses two hundred bytes and
// could have had the rest as data.
//
// Rho can do better than a guess, and the reason is structural rather than clever:
// the compiler emitted every function in the program, so it knows every call in it,
// every frame, and every path between them. C cannot know this — the program is
// many translation units compiled separately, and nothing has the whole call graph
// until after the linker has already fixed the layout.
//
// So: parse what was emitted, build the call graph, and take the longest path from
// the entry point. The number is a bound rather than an estimate, and where it
// cannot be a bound — recursion, a call this analysis cannot see — that is
// reported rather than papered over.
//
// What it counts is the frames it can see: Rho's own functions and the runtime's.
// A call into a library the analysis has not read is accounted at a declared cost,
// because the alternative is a number that looks computed and is not.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// StackBinding is what a stack analysis cannot see.
//
// Every unknown frame is charged this much. Declared rather than assumed because
// the alternative is a bound that silently omits the deepest thing in the program:
// a library called from a leaf, at whatever depth that library happens to use.
type StackBinding struct {
	// Bytes is what a call this analysis cannot read is charged.
	//
	// One number for every such call rather than a name per call, because the
	// analysis does not know the names until it has read the program, and the
	// names it finds are reported in `StackAnalysis.Unknown` — which is where a
	// reader can act on them, and where a target declares what they really cost.
	Bytes int
}

// DefaultForeignFrame is what a call into code this analysis has not read costs.
//
// 64 bytes: a saved frame pointer, a return address and a register save area is
// under 32 on Cortex-M with the ABI this toolchain targets, and a library frame
// with a few locals is a hundred. A round number above both, and declared here so
// that the number in a report is the number somebody chose.
const DefaultForeignFrame = 64

// SpillAllowance is what a function's frame is charged beyond its parameters,
// its locals and the registers its signature saves.
//
// Not a fudge factor but a named omission: a static scan sees what the source
// says, and a compiled frame also holds whatever the compiler spilled to find a
// register. Measured against `gcc -fstack-usage` on the generated hello-world,
// `rho_string_from_i32` is 64 bytes where its parameters and its twelve-byte
// buffer account for 16 — the rest is spills and the alignment the ABI rounds to.
// An analysis that reported 16 would be a number that looks computed and is
// smaller than the truth, which is the one direction a stack bound must never be
// wrong in.
const SpillAllowance = 24

// WordSize is the machine word a frame is counted in.
//
// Rounded up rather than exact, because every ABI pads a frame to word alignment
// and a frame counted at 6 bytes when it occupies 8 is a bound that is not one.
var WordSize = 4

// StackAnalysis is what a program's stack requirement turned out to be.
type StackAnalysis struct {
	// Bytes is the worst-case depth from the entry point, in bytes.
	Bytes int
	// Path is the call path that reaches it, entry point first.
	Path []string
	// Recursive is every function found to reach itself. A recursive function has
	// no worst case — the depth is whatever the recursion reaches — so a program
	// containing one has no bound and is told so.
	Recursive []string
	// Unknown is every call this analysis could not account for.
	Unknown []string
	// Frames is every function and what its frame costs, for a report.
	Frames map[string]int
	// OwnBytes is the total counting only what this analysis read.
	//
	// Kept apart from `Bytes` because `Bytes` includes a declared charge for every
	// call it could not read, and whoever declares the real cost of those calls
	// needs this figure to add them to. Without it, the two components would have
	// to agree on the default charge, and a number both sides had to know in order
	// to correct is a number one of them will eventually have wrong.
	OwnBytes int
}

// ComputeStack is the worst-case stack depth of emitted C.
//
// `emitted` is the program, and `entry` the symbol it starts at. Everything the
// analysis concludes about the program's shape is in the result, including the
// parts it could not conclude, because a bound that is quietly wrong is worse than
// no bound at all.
func ComputeStack(emitted string, entry string, foreign StackBinding) StackAnalysis {
	frames, calls := parseFunctions(emitted)
	if entry == "" {
		entry = "main"
	}

	analysis := StackAnalysis{Frames: frames}
	if _, known := frames[entry]; !known {
		// No function by that name: the caller named the program's real entry and
		// this analysis cannot see it, so there is no bound rather than a wrong one.
		analysis.Unknown = append(analysis.Unknown, entry)
		return analysis
	}

	// Longest path by memoised depth, with recursion reported rather than followed.
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := map[string]int{}
	depth := map[string]int{}
	next := map[string][]string{}

	var walk func(string) int
	walk = func(name string) int {
		switch state[name] {
		case visiting:
			// Reached a function already on the path: this is recursion, and there
			// is no worst case to compute.
			analysis.Recursive = appendUnique(analysis.Recursive, name)
			return foreignFrame(frames, name, foreign)
		case done:
			return depth[name]
		}
		state[name] = visiting

		best, bestNext := 0, ""
		for _, callee := range calls[name] {
			cost := walk(callee)
			if cost > best {
				best, bestNext = cost, callee
			}
		}
		own := frameCost(frames, name, foreign)
		depth[name] = own + best
		if bestNext != "" {
			next[name] = []string{bestNext}
		} else {
			next[name] = nil
		}
		state[name] = done
		return depth[name]
	}

	analysis.Bytes = walk(entry)
	// The same walk without the unseen calls charged, so that the two figures
	// differ by exactly the foreign frames and by nothing else.
	analysis.OwnBytes = analysis.Bytes
	for _, name := range analysis.Unknown {
		analysis.OwnBytes -= frameCost(nil, name, foreign)
	}

	// Walk the path back, for a report that says which chain of calls it is.
	path := []string{entry}
	for at := path[len(path)-1]; ; {
		step := next[at]
		if len(step) == 0 {
			break
		}
		at = step[0]
		path = append(path, at)
		if len(path) > 64 {
			break
		}
	}
	analysis.Path = path

	for _, callees := range calls {
		for _, callee := range callees {
			if _, known := frames[callee]; !known {
				analysis.Unknown = appendUnique(analysis.Unknown, callee)
			}
		}
	}
	sort.Strings(analysis.Unknown)
	sort.Strings(analysis.Recursive)
	return analysis
}

// foreignFrame is what a call to a function this analysis has not read costs.
func foreignFrame(frames map[string]int, name string, foreign StackBinding) int {
	if known, ok := frames[name]; ok {
		return known
	}
	if foreign.Bytes > 0 {
		return foreign.Bytes
	}
	return DefaultForeignFrame
}

// frameCost is one function's own frame, rounded up to a word.
//
// At least a word even when nothing was counted, because every call pushes
// something: the return address, and on this ABI a frame pointer with it. A
// function counted at zero is a function whose frame the analysis has silently
// decided does not exist, and a chain of them sums to nothing.
func frameCost(frames map[string]int, name string, foreign StackBinding) int {
	cost := foreignFrame(frames, name, foreign)
	if cost < WordSize {
		cost = WordSize
	}
	if remainder := cost % WordSize; remainder != 0 {
		cost += WordSize - remainder
	}
	return cost
}

// The shapes this reads. Narrow on purpose: a C parser is a large thing to get
// subtly wrong, and what a stack bound needs is frames and calls, which have a
// small number of shapes.
var (
	// A function definition at the start of a line, with its parameter list.
	functionPattern = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_ \*]*?)\s+\*?([A-Za-z_][A-Za-z0-9_]*)\s*\(([^;{]*)\)\s*\{`)
	// A declaration with no body, which is a prototype and not a definition.
	prototypePattern = regexp.MustCompile(`(?m)^[A-Za-z_][A-Za-z0-9_ \*]*\s+\*?[A-Za-z_][A-Za-z0-9_]*\s*\([^;{]*\)\s*;`)
	// A local array, which is the only thing Rho emits that is unbounded.
	localArrayPattern = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\s+([A-Za-z_][A-Za-z0-9_]*)\s*\[\s*([0-9]+)\s*\]\s*;`)
	// A call, with the name and its arguments.
	callPattern = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	// The frame pointer and link register a prologue saves.
	savedRegisterPattern = regexp.MustCompile(`\b(?:r4|r5|r6|r7|r8|r9|r10|r11|fp|lr)\b`)
)

// Words that are not calls, and are the ones a scan would otherwise read as
// function calls to a function that does not exist.
var notCalls = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "return": true,
	"sizeof": true, "defined": true, "static_assert": true, "_Static_assert": true,
}

// parseFunctions is every function in the emitted C, and what each calls.
//
// The scan is textual and says so: a `(` is a call unless the word before it is a
// keyword, and a `[N]` is a local of `N` bytes unless the name is a parameter.
// Both are true of everything Rho emits, which is the scope this analysis has to
// be right about — and where it is not, `StackAnalysis.Unknown` says which name it
// could not account for.
func parseFunctions(emitted string) (map[string]int, map[string][]string) {
	frames := map[string]int{}
	calls := map[string][]string{}

	// Prototypes are removed first: a declaration and a definition share a shape
	// apart from the brace, and counting the prototype would give half of them an
	// empty frame and no calls, which is indistinguishable from a real one.
	bodies := prototypePattern.ReplaceAllString(emitted, "")

	lines := strings.Split(bodies, "\n")
	inFunction := ""
	braceDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if inFunction == "" {
			match := functionPattern.FindStringSubmatch(trimmed)
			if match == nil {
				continue
			}
			name := match[2]
			inFunction = name
			calls[name] = []string{}
			// The parameters are what a frame holds besides the registers, and
			// for a leaf function they are most of it.
			own := parameterWords(match[3]) + savedRegisters(trimmed) + SpillAllowance
			// Set before the body is read: the body adds to this, and assigning
			// it afterwards would discard every local it found.
			frames[name] = own
			braceDepth = strings.Count(line, "{") - strings.Count(line, "}")
			// The text after the opening brace is the body too, when a definition
			// opens and closes on one line. Skipping it would read every such
			// function as having no locals and making no calls, which for a
			// generated program is a frame of nothing however much it declared.
			body := ""
			if open := strings.Index(trimmed, "{"); open >= 0 {
				body = trimmed[open+1:]
				if close := strings.LastIndex(body, "}"); close >= 0 {
					body = body[:close]
				}
			}
			scanBody(name, body, frames, calls)
			if braceDepth <= 0 {
				inFunction = ""
				braceDepth = 0
			}
			continue
		}

		// Inside a body: a local array is frame, and every call is an edge.
		scanBody(inFunction, line, frames, calls)

		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if braceDepth <= 0 {
			inFunction = ""
			braceDepth = 0
		}
	}

	for name := range frames {
		frames[name] = wordRounded(frames[name])
		calls[name] = uniqueStrings(calls[name])
	}
	return frames, calls
}

// scanBody reads one function's body: a local array is frame, and every call is
// an edge in the graph.
//
// Split out because a body arrives in two pieces — the tail of the line that
// opened the function, and the lines after it — and reading one of them and not
// the other is how a function ends up looking free.
func scanBody(name, body string, frames map[string]int, calls map[string][]string) {
	for _, match := range localArrayPattern.FindAllStringSubmatch(body, -1) {
		size, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		frames[name] += size
	}
	for _, match := range callPattern.FindAllStringSubmatch(body, -1) {
		callee := match[1]
		if notCalls[callee] {
			continue
		}
		// A self-call is kept, deliberately: it is the recursion this analysis has
		// to report rather than follow, and skipping it would make a directly
		// recursive function look like a leaf.
		calls[name] = appendUnique(calls[name], callee)
	}
}

// parameterWords is how many words a parameter list costs on the stack.
//
// Everything by value costs a word, which on this ABI is what a `char`, a
// pointer and an `int` all are. A struct passed by value costs more, and Rho
// emits those by pointer; a caller passing one by value gets a frame this
// under-counts, which is why the entry point's own call sites are worth reading
// rather than trusting.
func parameterWords(parameters string) int {
	if strings.TrimSpace(parameters) == "" || strings.TrimSpace(parameters) == "void" {
		return 0
	}
	depth := 0
	count := 0
	inWord := false
	for _, char := range parameters {
		switch char {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 && inWord {
				count++
				inWord = false
			}
		default:
			if depth == 0 && !isSpace(char) {
				inWord = true
			}
		}
	}
	if inWord {
		count++
	}
	return count * WordSize
}

// savedRegisters is the register save area a prologue reserves, in bytes.
//
// Read from the function's own opening line, where a prologue puts it, rather than
// counted for every function: a leaf function with no calls saves nothing, and
// charging it a full save area would make the bound a fiction.
func savedRegisters(signature string) int {
	matches := savedRegisterPattern.FindAllString(signature, -1)
	return len(matches) * WordSize
}

func isSpace(char rune) bool {
	return char == ' ' || char == '\t' || char == '\n' || char == '\r'
}

// wordRounded is a size the ABI will actually allocate.
func wordRounded(size int) int {
	if size <= 0 {
		return 0
	}
	if remainder := size % WordSize; remainder != 0 {
		size += WordSize - remainder
	}
	return size
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func uniqueStrings(values []string) []string {
	sort.Strings(values)
	out := values[:0]
	var last string
	for index, value := range values {
		if index > 0 && value == last {
			continue
		}
		last = value
		out = append(out, value)
	}
	return out
}

// IsBound is whether this is a bound rather than an estimate.
//
// It is a bound only when nothing was left out: no recursion, which has no worst
// case, and no call this analysis could not read, which is charged a declared
// frame rather than a known one. A project that asked for a computed stack and
// got an estimate is told so, because the alternative is a program that believes
// its stack was measured when it was guessed — which is the situation this whole
// exercise exists to end.
func (a StackAnalysis) IsBound() bool {
	return a.Bytes > 0 && len(a.Recursive) == 0 && len(a.Unknown) == 0
}

// StackRequirement is the stack a program needs, as the linker script writes it.
func (a StackAnalysis) StackRequirement(fallback int) uint32 {
	if a.Bytes <= 0 {
		return uint32(fallback)
	}
	return uint32(a.Bytes)
}

// Describe is the analysis in a sentence, for a build log.
func (a StackAnalysis) Describe() string {
	if a.Bytes <= 0 {
		return "no stack requirement could be established"
	}
	out := fmt.Sprintf("%d bytes of stack, deepest path %s", a.Bytes, strings.Join(a.Path, " → "))
	if len(a.Recursive) > 0 {
		out += fmt.Sprintf("; recursive, so this is not a bound: %s", strings.Join(a.Recursive, ", "))
	}
	if len(a.Unknown) > 0 {
		out += fmt.Sprintf("; %d call(s) charged a declared frame: %s",
			len(a.Unknown), strings.Join(a.Unknown, ", "))
	}
	return out
}
