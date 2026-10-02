//
// @file protocol.go
// @brief The test runner's protocol, as this plugin sees it.
//
// The protocol is a pair of JSON documents rather than a library, and this plugin
// spells out the fields it reads rather than importing the runner's types. That is
// not squeamishness about dependencies: the runner is a component that moves, and a
// plugin that imported it would break whenever the runner's internals changed —
// which is the opposite of what a plugin is for. So it declares the version it was
// written against and refuses a request in another version rather than reading it
// with types that may no longer mean what they meant.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package gtest

import "encoding/json"

// ProtocolVersion is the version of the request and response shapes this plugin
// was written against. A runner that speaks another version is reported rather
// than answered with something it would misread.
const ProtocolVersion = 1

// ComponentName is the component this plugin extends, which is what makes a test
// run load it at all.
const ComponentName = "rho-test"

// HookDiscover is the hook this plugin answers: once, over what the project
// declared.
const HookDiscover = "discover"

// The kinds this plugin claims. A suite of any other kind is somebody else's,
// and a run may have several plugins asked about the same declarations at once.
const (
	// KindGTest is a suite whose tests are written with GoogleTest.
	KindGTest = "gtest"
	// KindGTestShort is the same, under the other spelling anybody reaches for.
	// Two spellings is a small thing to accept and a needless reason for a
	// project's suites to be reported as having no runner.
	KindGTestShort = "gtest"
)

// Envelope is the toolchain's wrapper: what a request says, and what an answer
// carries.
type Envelope struct {
	Version   int             `json:"version"`
	Hook      string          `json:"hook"`
	Command   string          `json:"command"`
	Component string          `json:"component"`
	Payload   json.RawMessage `json:"payload"`
}

// DiscoverRequest is what the runner asks about.
type DiscoverRequest struct {
	Root     string          `json:"root"`
	Target   string          `json:"target"`
	Manifest string          `json:"manifest"`
	Suites   []DeclaredSuite `json:"suites"`
}

// DeclaredSuite is one suite the project declared.
type DeclaredSuite struct {
	Name string            `json:"name"`
	Kind string            `json:"kind"`
	Tags []string          `json:"tags"`
	Axis map[string]string `json:"axis"`
	// Options is the suite's own settings, as the manifest wrote them and the
	// runner did not read. This plugin's settings are in here, under `gtest`.
	Options map[string]any `json:"options"`
}

// Response is what a plugin answers with.
type Response struct {
	Version int             `json:"version"`
	Payload json.RawMessage `json:"payload"`
	Error   string          `json:"error,omitempty"`
}

// DiscoverResponse is what this plugin answers the discover hook with.
type DiscoverResponse struct {
	Suites []ResolvedSuite `json:"suites"`
}

// ResolvedSuite is one suite this plugin knows how to run.
type ResolvedSuite struct {
	// Declared is the declaration this answers, by name. A suite nobody declared
	// is not run: which tests a project has is the project's to say.
	Declared string `json:"declared"`
	Name     string `json:"name,omitempty"`
	Steps    []Step `json:"steps,omitempty"`
	Skip     string `json:"skip,omitempty"`
}

// Step is one thing a run does to get a suite's result.
type Step struct {
	Name    string   `json:"name,omitempty"`
	Dir     string   `json:"dir,omitempty"`
	Command []string `json:"command,omitempty"`
	// Env is set for this step's programs, on top of the run's own. How a plugin
	// that retrieved something says where it put it, without writing the answer
	// into a project's files.
	Env      []string `json:"env,omitempty"`
	Produces string   `json:"produces,omitempty"`
	Optional bool     `json:"optional,omitempty"`
}
