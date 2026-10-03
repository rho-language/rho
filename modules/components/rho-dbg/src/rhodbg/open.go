//
// @file open.go
// @brief Opening a session: which debugger, which probe, which register map.
//
// The assembly, in one place and in one order, because the order is the substance
// of it:
//
//	1. the debugger, which is the board's or a plugin's or GDB's
//	2. the probe, started and waited for before anything connects to it
//	3. the engine, connected to the probe, with the program loaded or written
//	4. the register map, which is the chip's own and not the debugger's
//
// Steps 2 and 3 are the ones that are easy to get wrong. A debugger that connects
// to a port nothing is listening on says the connection was refused, which is a
// true statement and one that sends a person to look at the debugger. So the probe
// is started, and waited for, before the debugger is asked to connect to it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"errors"
	"fmt"
	"strings"

	"rho/src/plugins"
	"rho/src/targetspec"
)

// OpenRequest is a session that is about to be started.
type OpenRequest struct {
	// Target is the board, as its spec names it, or nil for a program running on
	// this machine.
	Target *targetspec.Target
	// TargetName is what the board was asked for by, for a diagnostic.
	TargetName string
	// Program is the built binary.
	Program string
	// Root is the project directory, which is where a declared path that is relative
	// is read from.
	Root string
	// VendorRoots are the vendor trees of the platforms the project depends on, for
	// a file one of them carries rather than the program: a vendor's register map is
	// megabytes of XML that every program built for the chip would otherwise carry a
	// copy of.
	VendorRoots []string
	// Project is what the project's own manifest says about its chip, which is the
	// answer when it says anything.
	Project *DebugMapping

	// Engine names the debugger, when the launch configuration named one. An empty
	// value means the board's own, or the built-in one.
	Engine string
	// Debugger overrides the debugger program the board names.
	Debugger string
	// ServerPath, ServerArgs, ConfigFiles, Device, Interface, Servertype and Port
	// override what the board says about its probe. They exist because the machine
	// on a desk is the one that differs from the spec, and a session that cannot be
	// pointed at the probe that is actually plugged in is a session that fails on
	// one machine and works on another.
	ServerPath  string
	ServerArgs  []string
	ConfigFiles []string
	Device      string
	Interface   string
	Servertype  string
	Port        string
	// Malloc is the allocator the program is run with. Named here because it is
	// otherwise a `-ex` flag in a debugger's own command line, and a launch
	// configuration is the only place a person can reasonably put one.
	Malloc string
	// Load says whether the program is written to the board. Nil means the board's
	// own answer, which is what a target spec's `debug.load` says.
	Load *bool
	// Plugins is where the debuggers a session may use are found.
	Plugins plugins.Source
}

// Open is a whole session: the probe, the engine and the register map.
//
// Everything the session holds is released by its Close, and Close is what the
// caller must reach even when Open failed half way: a probe that was started and
// a debugger that was not leaves the board held by a process nobody knows about,
// which is the failure the next session reports as a board that cannot be
// reached.
func Open(request OpenRequest, codec *Codec) (*Session, error) {
	available := Discover(request.Plugins)
	spec, err := available.Choose(request.TargetName, request.Engine)
	if err != nil {
		return nil, err
	}

	debugger := request.Debugger
	if debugger == "" && spec.Kind == KindDriver {
		debugger = debuggerFor(request.Target)
	}

	// An adapter engine brings its own everything: it starts the probe server it
	// needs, knows the board's registers, and asks for nothing. So nothing is
	// started around it here, and asking would be a second probe server fighting
	// the first for the board.
	if spec.Kind == KindAdapter {
		engine, err := StartAdapter(spec, request.TargetName, request.Program, codec)
		if err != nil {
			return nil, err
		}
		return NewSession(codec, engine, nil).describeSession(spec, request), nil
	}

	if debugger == "" {
		return nil, fmt.Errorf(
			"nothing says which debugger to use: %s names none, and its compiler was not given either",
			describeTarget(request))
	}

	var probe *Probe
	if needsProbe(request, spec) {
		probeSpec := ProbeSpec{
			Program: firstOf(request.ServerPath, request.Target.Debug.Server),
			Port:    firstOf(request.Port, request.Target.Debug.Port),
		}
		probeSpec.Arguments = probeArguments(request)
		probe, err = StartProbe(probeSpec)
		if err != nil {
			return nil, err
		}
	}

	remote := ""
	if probe != nil {
		remote = "localhost:" + probe.Port
	}

	load := false
	if request.Load != nil {
		load = *request.Load
	} else if request.Target != nil {
		load = request.Target.Debug.Load
	}

	engine, err := StartGDB(debugger, request.Program, GDBOptions{
		Remote: remote,
		Load:   load,
		Malloc: request.Malloc,
	})
	if err != nil {
		_ = probe.Stop()
		return nil, err
	}

	session := NewSession(codec, engine, probe)
	session.describeSession(spec, request)

	// The register map, which is the chip's own and not the debugger's. Read the
	// same way `rho dbg registers` reads it, through the same function, so that a
	// panel showing a chip's peripherals and a session showing a chip's peripherals
	// cannot be about different chips — one of them having found the file somewhere
	// the other did not look.
	found, err := FindRegisterMap(TargetFacts{
		Project:     request.Project,
		TargetName:  request.TargetName,
		Target:      request.Target,
		Root:        request.Root,
		Program:     request.Program,
		VendorRoots: request.VendorRoots,
		Settings:    ReadSettings(),
		FromPlugin: func() (string, string) {
			declared := ""
			if request.Target != nil {
				declared = request.Target.Debug.SVD
			}
			return RegisterMapFromPlugins(request.Plugins, request.TargetName, declared)
		},
	})
	if err != nil {
		// A session on this machine has no registers and needs none, which is not a
		// failure and is not this branch.
		if !errors.Is(err, ErrNoRegisterMap) {
			// A board whose registers cannot be named is a session in which every
			// register view is wrong without saying so. Said as a refusal, with what
			// was tried and where to write the answer, because the one thing the
			// person debugging it can do is name the chip.
			return nil, CannotIdentify(request.TargetName, partOf(request.Target), namesTried(err), err)
		}
		return session, nil
	}
	if found.Path != "" {
		load, loadErr := LoadSVD(found.Path)
		if loadErr != nil {
			session.reportRegisterMap(loadErr.Error())
		} else {
			session.WithRegisterMap(load, found.Path, found.Comment)
			session.reportRegisterMap(fmt.Sprintf("the register map for this chip is %s, from %s", found.Path, found.From))
		}
	}
	return session, nil
}

// needsProbe is whether a probe has to be running before the debugger connects.
//
// The one case where there is no probe to start is an engine that brings its own:
// an adapter starts whatever server it needs, and starting one here as well would
// be two processes fighting over the same board.
func needsProbe(request OpenRequest, spec EngineSpec) bool {
	if request.Target == nil || !request.Target.Debug.Watcher() {
		return false
	}
	return spec.Kind != KindAdapter
}

// probeArguments are the probe server's arguments: the ones the board declares, the
// ones the configuration overrides them with, and the config files the session
// names.
//
// In that order, and the overrides last, so that an override replaces rather than
// adds to: a person who wrote `configFiles` meant the board's config files to be
// the ones they named.
func probeArguments(request OpenRequest) []string {
	arguments := append([]string{}, request.Target.Debug.ServerArgs...)
	arguments = append(arguments, request.ServerArgs...)
	arguments = append(arguments, request.ConfigFiles...)
	if request.Interface != "" {
		arguments = append(arguments, "-f", "interface/"+request.Interface+".cfg")
	}
	return arguments
}

// debuggerFor is the debugger a board's session is run with.
func debuggerFor(target *targetspec.Target) string {
	if target == nil {
		return ""
	}
	if named := strings.TrimSpace(target.Debug.Tool); named != "" {
		return named
	}
	return targetspec.DebuggerFor(target.Toolchain.Compiler)
}

func describeTarget(request OpenRequest) string {
	if request.TargetName != "" {
		return fmt.Sprintf("the target %q", request.TargetName)
	}
	return "this machine"
}

func firstOf(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

/* -------------------------------------------------------------------------- */
/*  What the session says about itself                                         */
/* -------------------------------------------------------------------------- */

// describeSession records how the session was assembled, for the line the session
// prints when it starts.
//
// Said once, at the beginning, in full. A session that behaves oddly is nearly
// always one that was assembled from different facts than the person expected —
// a different probe, a debugger from a different toolchain — and this line is the
// only place those facts are visible afterwards.
func (s *Session) describeSession(spec EngineSpec, request OpenRequest) *Session {
	lines := []string{
		"session: " + spec.Name + " (" + spec.Kind + ")",
		"board: " + describeTarget(request),
		"program: " + request.Program,
	}
	if request.Target != nil && request.Target.Debug.Watcher() && s.probe != nil {
		lines = append(lines, "probe: "+s.probe.Describe())
	}
	if spec.Kind == KindAdapter {
		if executable, arguments := spec.Expansion(request.TargetName, request.Program); executable != "" {
			lines = append(lines, "engine: "+strings.Join(append([]string{executable}, arguments...), " "))
		}
	}
	s.mu.Lock()
	s.opening = lines
	s.mu.Unlock()
	return s
}

// Opening is what a session said about itself as it started, one line each.
func (s *Session) Opening() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.opening...)
}
