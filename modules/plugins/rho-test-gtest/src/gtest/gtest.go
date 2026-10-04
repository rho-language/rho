//
// @file gtest.go
// @brief What this plugin does when it is run.
//
// Two ways of being run, and the second is what a run of the first arranges.
//
// As a discover command it is handed the project's declarations and answers with
// the steps that run its suites. As a report step it is handed the file
// GoogleTest wrote and prints what it says, one line per test — because the run
// that spawned it reads a step's output through a parser it already has, and this
// is how a file on disk becomes lines that parser understands.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package gtest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Main runs the plugin and returns a process exit code.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) >= 3 && args[1] == "--report" {
		return sayReport(args[2], stdout, stderr)
	}
	return discover(stdin, stdout, stderr)
}

// discover answers the runner's discover hook.
func discover(stdin io.Reader, stdout, stderr io.Writer) int {
	payload, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "rho-test-gtest:", err)
		return 1
	}
	var envelope Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		fmt.Fprintln(stderr, "rho-test-gtest: the request was not readable:", err)
		return 1
	}
	if envelope.Version != ProtocolVersion {
		// Answered as nothing rather than answered wrongly. The runner sends
		// everything to every plugin on the hook, so a plugin cannot refuse to
		// participate — it can only contribute nothing, and say why on stderr.
		fmt.Fprintf(stderr, "rho-test-gtest: this plugin speaks protocol %d and the run speaks %d; "+
			"it is contributing nothing rather than answering something it would misread\n",
			ProtocolVersion, envelope.Version)
		answer(stdout, DiscoverResponse{})
		return 0
	}
	if envelope.Component != ComponentName {
		answer(stdout, DiscoverResponse{})
		return 0
	}

	var request DiscoverRequest
	if err := json.Unmarshal(envelope.Payload, &request); err != nil {
		fmt.Fprintln(stderr, "rho-test-gtest: the request body was not readable:", err)
		return 1
	}

	answer(stdout, DiscoverResponse{Suites: ResolveAll(request.Root, request.Suites)})
	return 0
}

// sayReport prints what a GoogleTest report says, in the form the run's parser reads.
func sayReport(path string, stdout, stderr io.Writer) int {
	outcomes, err := Read(path)
	if err != nil {
		// An error rather than a pass. A binary that wrote no report did not pass,
		// and reporting nothing here would leave the run saying the suite passed
		// because it had run out of output to complain about.
		fmt.Fprintln(stderr, "rho-test-gtest:", err)
		return 1
	}
	if len(outcomes) == 0 {
		// The framework wrote a report with no tests in it. A suite with no tests has
		// tested nothing, and saying so is the difference between a suite that ran
		// and a suite that exists.
		fmt.Fprintf(stderr, "rho-test-gtest: %s holds no tests, so nothing was tested\n", path)
		return 1
	}
	for _, line := range Lines(outcomes) {
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// answer writes a response on standard output.
func answer(stdout io.Writer, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		fmt.Fprintln(stdout, `{}`)
		return
	}
	envelope := Envelope{Version: ProtocolVersion, Payload: encoded}
	outer, err := json.Marshal(envelope)
	if err != nil {
		fmt.Fprintln(stdout, `{}`)
		return
	}
	stdout.Write(outer)
}

// executablePath is this program's own path.
func executablePath() (string, error) { return os.Executable() }
