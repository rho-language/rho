//
// @file session.go
// @brief Running a plugin that outlives one request.
//
// A documentation plugin does its work and exits, so the shared runner starts a
// process, writes one JSON document, reads one back, and the process is finished.
// That is the right shape for that job and the wrong one for a job that holds
// something a caller expects to keep: a database connection, a transaction
// spanning several calls, a warm cache.
//
// A session is the same protocol with a different lifetime. The envelope is
// identical — version, hook, command, component, payload — and the process
// reads one request per line and answers with one response per line instead of
// reading to end of input. A plugin written for the per-request runner still
// works against a session, because a line of JSON is a document: a plugin that
// decodes one request, does its work and writes one response is served
// correctly by either, and one that only ever reads to end of input is the
// documented case for the runner rather than for a session.
//
// The frame is a line, not a length prefix, because a plugin is a program a
// person can run by hand: `echo '{...}' | rho-sqlite` has to work, and it does.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package plugins

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Session is one plugin process, held open across requests.
//
// It is safe for concurrent use. A plugin that cannot serve two requests at
// once says so with a "serial" field in its own answer, and a caller that gets
// one waits rather than starting a second process, because two processes writing
// the same database file is a worse outcome than a request that waited.
type Session struct {
	plugin  Plugin
	command Command
	program string
	// environment is added to the process's own, and is where a component puts
	// what an engine needs to know before it is asked anything: a database path,
	// a connection string, a scratch directory.
	environment []string

	mu      sync.Mutex
	process *exec.Cmd
	// handle is the running process, kept because a kill needs it and a Cmd
	// does not expose one.
	handle  *os.Process
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *bytes.Buffer
	started bool
	closed  bool
}

// SessionOptions are what a session needs that a single run does not: which
// plugin, which of its commands, and what a call should be allowed to take.
type SessionOptions struct {
	// Plugin is the plugin to run.
	Plugin Plugin
	// Command is the command of that plugin to run, which the caller picks by
	// name when a plugin offers several.
	Command string
	// Environment is added to the process's own environment, which is where a
	// database path or a connection string goes.
	Environment []string
	// Timeout bounds a single request, not the session. A session is expected to
	// outlive one request; a request that hangs is still a fault.
	Timeout time.Duration
}

// Open starts a session with a plugin, or returns the one already open.
//
// The session is started lazily: opening it does not start the process, and the
// first request does. A service that configures a database it never queries
// never pays for the connection.
func Open(options SessionOptions) (*Session, error) {
	session := &Session{
		plugin:      options.Plugin,
		environment: append([]string{}, options.Environment...),
		stderr:      &bytes.Buffer{},
	}
	if options.Command != "" {
		// A named command is the one the caller wants; an unnamed one is the
		// plugin's first, which is the only sensible default for a plugin with
		// one.
		command, err := pluginCommand(options.Plugin, options.Command)
		if err != nil {
			return nil, err
		}
		session.command = command
	} else if len(options.Plugin.Commands) > 0 {
		session.command = options.Plugin.Commands[0]
	} else {
		return nil, fmt.Errorf("plugin %s has no command to run", options.Plugin.Name)
	}
	program, err := executable(options.Plugin, splitCommand(session.command.Run)[0])
	if err != nil {
		return nil, err
	}
	session.program = program
	return session, nil
}

// pluginCommand finds a command on a plugin by name.
func pluginCommand(plugin Plugin, name string) (Command, error) {
	for _, command := range plugin.Commands {
		if command.Name == name {
			return command, nil
		}
	}
	return Command{}, fmt.Errorf("plugin %s has no command named %q", plugin.Name, name)
}

// Do sends one request and returns what the plugin answered.
//
// The component is which component the request is for, which is not always the
// plugin's own name: a plugin carries the component it answers, and one that
// answered two would be told which by this. The payload is the component's own,
// exactly as it is for a single run; the difference is only that the process
// answering is the one answering again.
func (s *Session) Do(component, hook, command string, payload any) (Response, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("the %s request could not be encoded: %w", hook, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Response{}, fmt.Errorf("the session with %s is closed", s.plugin.Name)
	}
	if err := s.start(); err != nil {
		return Response{}, err
	}

	request := Request{
		Version:   ProtocolVersion,
		Hook:      hook,
		Command:   command,
		Component: component,
		Payload:   encoded,
	}
	// One request is one line, and one answer is one line. A writer that does
	// not add a newline leaves the reader waiting for a frame that never ends,
	// so the newline is written with the document rather than after it.
	frame, err := json.Marshal(request)
	if err != nil {
		return Response{}, fmt.Errorf("the %s request could not be framed: %w", hook, err)
	}
	if _, err := s.stdin.Write(append(frame, '\n')); err != nil {
		return Response{}, fmt.Errorf("the %s request could not be sent to %s: %w", hook, s.plugin.Name, err)
	}

	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		if err == io.EOF && len(bytes.TrimSpace(line)) == 0 {
			return Response{}, fmt.Errorf(
				"%s closed the session: %s", s.plugin.Name, strings.TrimSpace(s.stderr.String()))
		}
		return Response{}, fmt.Errorf("cannot read the answer from %s: %w", s.plugin.Name, err)
	}
	response := Response{}
	if err := json.Unmarshal(bytes.TrimSpace(line), &response); err != nil {
		return Response{}, fmt.Errorf("cannot read the answer from %s: %w", s.plugin.Name, err)
	}
	return response, nil
}

// start begins the process, once.
func (s *Session) start() error {
	if s.started {
		return nil
	}
	argv := splitCommand(s.command.Run)
	process := exec.Command(s.program, argv[1:]...)
	// The process runs in the plugin's own directory, so a plugin finds what it
	// carries and a data file it ships beside itself.
	process.Dir = s.plugin.Root
	process.Env = append(os.Environ(),
		"RHO_PLUGIN="+s.plugin.Name,
		"RHO_PLUGIN_HOOK="+s.command.Stage(),
		"RHO_PLUGIN_SESSION=1",
	)
	// What the caller set comes last, so it is what the process sees: a
	// component that points an engine at a database of its own is asking for
	// that database rather than for whatever the environment had.
	process.Env = append(process.Env, s.environment...)
	process.Stderr = s.stderr

	stdin, err := process.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return err
	}
	if err := process.Start(); err != nil {
		return fmt.Errorf("cannot start %s: %w", s.plugin.Name, err)
	}
	s.process = process
	s.handle = process.Process
	s.stdin = stdin
	s.stdout = bufio.NewReaderSize(stdout, 64*1024)
	s.started = true
	return nil
}

// Close stops the plugin, and is safe to call more than once.
//
// A plugin holding a database is given the chance to close it: the process is
// asked to end by closing its input, which is the only signal a program reading
// frames can rely on, and a plugin that does not exit is killed. Either way the
// process does not outlive the session.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if !s.started {
		return nil
	}
	// Closing stdin is how a plugin reading frames learns the session is over.
	_ = s.stdin.Close()
	done := make(chan error, 1)
	go func() { done <- s.process.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(SessionShutdownGrace):
		// A plugin that did not stop on the closing input is a plugin that will
		// not stop, and a session that does not end holds its database open.
		_ = s.handle.Kill()
		return <-done
	}
}

// SessionShutdownGrace is how long a session waits for a plugin to stop after
// its input is closed before the process is killed.
const SessionShutdownGrace = 2 * time.Second
