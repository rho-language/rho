//
// @file install.go
// @brief Installing something, as a stream a page can watch.
//
// A fetch takes minutes and a package install asks for a password, so neither is
// something a page should wait for in silence. Both are run here and their output
// streamed as it happens, because the two things a person wants from either — is it
// still going, and did it work — are the two things a spinner does not say.
//
// The stream is deliberate about one thing: loading it does nothing. It answers
// with what it *would* do until the request says it has been asked to, so a link
// followed by accident, a prefetch, or a page somebody else embedded does not start
// a download or reach for privileges on this machine. A person clicking a button is
// what makes it run, and the button is the only thing that sends the question.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package keeper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"rho/src/sysdep"
)

// InstallStream is one installation, as a Server-Sent Events connection.
//
// Three kinds of thing arrive on it, in this order: `plan`, which is what would be
// done and is always sent; then `line`, one per line of what the command printed, as
// it printed it; and last `done`, which says whether the machine has it now. A page
// that reads only the first and the last still gets a truthful answer, and one that
// reads the middle gets to watch.
func InstallStream(w http.ResponseWriter, r *http.Request, module string, dependency string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "this server cannot stream", http.StatusInternalServerError)
		return
	}
	// Set before anything is written, which is not a detail: with no content type
	// declared, `net/http` sniffs the first bytes and decides, and a page's
	// `EventSource` refuses anything that is not `text/event-stream`. The button
	// would then do nothing at all, with the server having no idea why.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	report, problem := installReport()
	if problem != "" {
		// Said as a stream event rather than an HTTP error: the page is already
		// watching this connection, and an error status here would show as a failed
		// load rather than as the sentence that explains it.
		streamLine(w, flusher, problem)
		streamDone(w, flusher, false, problem)
		return
	}

	result, found := findRequirement(report, module, dependency)
	if !found {
		streamLine(w, flusher, "there is no such requirement: "+describeRequirements(report, module))
		streamDone(w, flusher, false, "there is no such requirement")
		return
	}
	if result.Status == sysdep.StatusOK {
		streamDone(w, flusher, true, result.Dep+" is already satisfied")
		return
	}

	out := &streamWriter{writer: w, flusher: flusher}

	// What it would do, always. Sent before anything runs so that a person watching
	// knows what is about to happen — and it is the whole response unless the
	// request says it was asked to.
	streamPlan(w, flusher, result)

	if !confirmed(r) {
		streamLine(w, flusher, "Nothing has been done. Click Install to run it.")
		streamDone(w, flusher, false, "not asked to")
		return
	}

	err := Install(module, dependency, SyncOptions{Writer: out, Reader: r.Body})
	if err != nil {
		streamLine(w, flusher, err.Error())
		streamDone(w, flusher, false, err.Error())
		return
	}

	// Asked what it is now, rather than assuming: an install that reported success
	// and left nothing behind is a thing this has to be able to see.
	after := ReportFor(module, dependency)
	streamDone(w, flusher, true, fmt.Sprintf("%s is %s now", after.Dep, after.Status))
}

// installReport is what the stream acts on.
//
// A variable rather than a call so that what the stream *does* — send the plan first,
// run nothing until asked, leave a satisfied one alone — is a thing that can be
// tested against a report that says what the test means. A test written against
// whatever the machine happens to be missing passes on the machine that wrote it and
// fails on the one where somebody has fixed it, which is exactly the moment the test
// would be most worth running.
var installReport = ReadReport

// withInstallReport runs something against a report that is not this machine's.
func withInstallReport(report sysdep.Report, problem string, body func()) {
	previous := installReport
	installReport = func() (sysdep.Report, string) { return report, problem }
	defer func() { installReport = previous }()
	body()
}

// ReportFor is one requirement as it stands after something changed.
//
// Read from the doctor, like every other answer here — so a page that refreshes
// after an install is showing what a terminal would show, not what the install
// claimed.
func ReportFor(module string, dependency string) sysdep.CheckResult {
	report, problem := installReport()
	if problem != "" {
		return sysdep.CheckResult{Module: module, Dep: dependency, Status: sysdep.StatusUnknown, Message: problem}
	}
	if result, found := findRequirement(report, module, dependency); found {
		return result
	}
	return sysdep.CheckResult{Module: module, Dep: dependency, Status: sysdep.StatusUnknown,
		Message: "it is no longer among what this machine is asked about"}
}

// confirmed is whether this request was asked to do the thing rather than merely to
// say what it would do.
//
// A parameter and not a body, because the page opens this with an `EventSource`,
// which can only ask with a URL — so the question has to travel in one, and a URL
// somebody can follow by accident must not install anything.
func confirmed(r *http.Request) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("confirm")))
	return value == "yes" || value == "true" || value == "1"
}

// describeRequirements is what a module asked for, for a failure that is about the
// name rather than about the machine.
func describeRequirements(report sysdep.Report, module string) string {
	declared := []string{}
	for _, result := range report.Results {
		if result.Module == module {
			declared = append(declared, result.Dep)
		}
	}
	if len(declared) == 0 {
		return module + " declares nothing this machine is asked about"
	}
	return "it needs none of these: " + strings.Join(declared, ", ")
}

/* -------------------------------------------------------------------------- */
/*  Writing                                                                    */
/* -------------------------------------------------------------------------- */

// streamWriter turns what a command prints into stream events, one line at a time.
//
// Line by line because that is the granularity a person watches: a download that
// prints a hundred dots should show a hundred lines, not one line a minute later
// containing all of them.
type streamWriter struct {
	writer  io.Writer
	flusher http.Flusher
	// partial holds the tail of a line that has not ended yet, because a program's
	// output does not always end where it says it does.
	partial string
}

func (s *streamWriter) Write(data []byte) (int, error) {
	s.partial += string(data)
	for {
		index := strings.Index(s.partial, "\n")
		if index < 0 {
			break
		}
		line := strings.TrimRight(s.partial[:index], "\r")
		s.partial = s.partial[index+1:]
		streamLine(s.writer, s.flusher, line)
	}
	return len(data), nil
}

func streamLine(w io.Writer, flusher http.Flusher, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	fmt.Fprintf(w, "event: line\ndata: %s\n\n", jsonLine(line))
	if flusher != nil {
		flusher.Flush()
	}
}

func streamPlan(w io.Writer, flusher http.Flusher, result sysdep.CheckResult) {
	plan := map[string]any{
		"module":  result.Module,
		"dep":     result.Dep,
		"status":  string(result.Status),
		"message": result.Message,
		// The command, in every spelling that would do it: the one a person would
		// type, and the ones this machine's package manager would run.
		"fixCommand": FixCommand(result),
		"recipes":    Recipes(result),
	}
	// The prebuilt archive, when there is one: a person watching wants to know the
	// file is coming from the vendor rather than from a package manager, and where.
	if result.Fetch != nil {
		plan["fetch"] = map[string]any{
			"url":     result.Fetch.URL,
			"version": sysdep.FetchedVersion(depOf(result)),
			"into":    FetchedDir(result.Dep, sysdep.FetchedVersion(depOf(result))),
		}
	}
	if result.Kind == sysdep.KindUdev && result.RuleSource != "" {
		plan["ruleSource"] = result.RuleSource
		plan["targetPath"] = result.FoundPath
		plan["needsRoot"] = true
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: plan\ndata: %s\n\n", encoded)
	if flusher != nil {
		flusher.Flush()
	}
}

// streamDone is the ending. `ok` is a boolean and not a quoted one, because the page
// branches on it and `if ("false")` is true.
func streamDone(w io.Writer, flusher http.Flusher, ok bool, message string) {
	answer := "false"
	if ok {
		answer = "true"
	}
	fmt.Fprintf(w, "event: done\ndata: %s\n\n",
		`{"ok":`+answer+`,"message":`+jsonLine(message)+`}`)
	if flusher != nil {
		flusher.Flush()
	}
}

func jsonLine(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// handleInstall is the route for one requirement, for the mux.
func handleInstall(w http.ResponseWriter, r *http.Request) {
	InstallStream(w, r, r.PathValue("module"), r.PathValue("dep"))
}
