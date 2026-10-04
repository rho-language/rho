//
// @file serve.go
// @brief What an engine plugin writes.
//
// An engine answers four hooks and nothing else, so the whole of what a plugin
// author writes is four functions and a Serve. The protocol, the framing and
// the error shapes are here, so a second engine is a driver and four bodies
// rather than a program that has to be right about a protocol.
//
//	package main
//
//	import (
//	    "os"
//	    "github.com/rho/rho-sql/src/rhosql"
//	    "github.com/mattn/go-sqlite3"
//	)
//
//	func main() {
//	    rhosql.Serve(os.Stdin, os.Stdout, &engine{})
//	}
//
// The service is a session: it is started once and holds its connection for as
// long as a caller keeps asking, so a process per statement is never what
// happens, and a transaction may span statements.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhosql

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"rho/src/plugins"
)

// An engine implements what it can and says so by having the method.
//
// The hooks are separate interfaces rather than one with four methods, because
// an engine that cannot run a write should not have to write a Migrate that
// refuses everything: a hook with no method is answered as unsupported, which is
// what it is, and the rest of the engine works.
//
// Only Describe and Query are required. The other two are what an engine offers
// when it can offer them.
type (
	// Describer says what the engine is. Every engine has it: a caller cannot
	// send a statement without knowing what the engine speaks.
	Describer interface {
		Describe() Engine
	}
	// Migrator creates the tables a schema declares.
	Migrator interface {
		Migrate(request MigrateRequest) (MigrateResponse, error)
	}
	// Runner runs a statement.
	Runner interface {
		Query(request QueryRequest) (QueryResponse, error)
	}
	// DescriberOfQuery says what a statement returns without running it, which
	// is what makes a typed result possible.
	DescriberOfQuery interface {
		DescribeQuery(request DescribeQueryRequest) (DescribeQueryResponse, error)
	}
)

// Serve runs an engine against a session.
//
// It reads one request per line and writes one answer per line, which is what a
// session is: a program that is asked many times and answers once per question.
// The envelope is the shared one, so a plugin written for this is a plugin the
// documentation generator could load as well.
//
// The engine is anything; a hook it has no method for is answered as
// unsupported.
func Serve(input io.Reader, output io.Writer, engine any) {
	reader := bufio.NewReaderSize(input, 1<<20)
	writer := bufio.NewWriterSize(output, 1<<20)
	defer writer.Flush()

	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			answer := serveOne(engine, line)
			if writeErr := writeAnswer(writer, answer); writeErr != nil {
				return
			}
			writer.Flush()
		}
		if err != nil {
			// The caller's input ended, which is how a session is closed. There
			// is no answer to give, so the plugin returns.
			return
		}
	}
}

// serveOne answers one request.
func serveOne(engine any, line []byte) plugins.Response {
	request := plugins.Request{}
	if err := json.Unmarshal(line, &request); err != nil {
		return failure(ErrorInvalid, "the request could not be read: "+err.Error(), nil)
	}
	if request.Version != plugins.ProtocolVersion {
		return failure(ErrorMismatch, fmt.Sprintf(
			"this engine speaks protocol %d, the caller speaks %d",
			plugins.ProtocolVersion, request.Version), nil)
	}
	if request.Component != ComponentName {
		return failure(ErrorMismatch, fmt.Sprintf(
			"this engine answers %s, not %s", ComponentName, request.Component), nil)
	}
	switch request.Hook {
	case HookDescribe:
		describer, ok := engine.(Describer)
		if !ok {
			return failure(ErrorUnsupported, request.Hook+" is not something this engine does", nil)
		}
		return payload(DescribeResponse{Engine: describer.Describe()})
	case HookMigrate:
		migrator, ok := engine.(Migrator)
		if !ok {
			return failure(ErrorUnsupported, request.Hook+" is not something this engine does", nil)
		}
		in, err := decode[MigrateRequest](request)
		if err != nil {
			return failure(ErrorInvalid, err.Error(), nil)
		}
		return call(func() (MigrateResponse, error) { return migrator.Migrate(in) })
	case HookQuery:
		runner, ok := engine.(Runner)
		if !ok {
			return failure(ErrorUnsupported, request.Hook+" is not something this engine does", nil)
		}
		in, err := decode[QueryRequest](request)
		if err != nil {
			return failure(ErrorInvalid, err.Error(), nil)
		}
		return call(func() (QueryResponse, error) { return runner.Query(in) })
	case HookDescribeQuery:
		describer, ok := engine.(DescriberOfQuery)
		if !ok {
			return failure(ErrorUnsupported, request.Hook+" is not something this engine does", nil)
		}
		in, err := decode[DescribeQueryRequest](request)
		if err != nil {
			return failure(ErrorInvalid, err.Error(), nil)
		}
		return call(func() (DescribeQueryResponse, error) { return describer.DescribeQuery(in) })
	default:
		return failure(ErrorUnsupported, "there is no hook named "+request.Hook, nil)
	}
}

// call runs one hook and turns what it did into an answer.
//
// A hook that returns an error is answered with the error rather than with an
// empty result, because a caller that cannot tell "no rows" from "the engine
// refused" will read a failure as a fact about the data.
func call[T any](run func() (T, error)) plugins.Response {
	result, err := run()
	if err != nil {
		if failure, ok := err.(EngineError); ok {
			return plugins.Response{
				Version:     plugins.ProtocolVersion,
				Diagnostics: diagnostics(failure.Response),
			}
		}
		return failureResponse(ErrorEngine, err.Error(), nil)
	}
	return payload(result)
}

// payload is an answer carrying a result.
func payload(value any) plugins.Response {
	encoded, err := json.Marshal(value)
	if err != nil {
		return failureResponse(ErrorEngine, "the result could not be encoded: "+err.Error(), nil)
	}
	return plugins.Response{Version: plugins.ProtocolVersion, Payload: encoded}
}

// failure is an answer carrying nothing but a reason.
func failure(kind ErrorKind, message string, details map[string]string) plugins.Response {
	return failureResponse(kind, message, details)
}

// failureResponse turns a kind and a message into an answer a caller acts on.
func failureResponse(kind ErrorKind, message string, details map[string]string) plugins.Response {
	encoded, err := json.Marshal(ErrorResponse{
		Kind: kind, Message: message, Details: details,
	})
	answer := plugins.Response{Version: plugins.ProtocolVersion}
	if err == nil {
		answer.Payload = encoded
	}
	answer.Diagnostics = diagnostics(ErrorResponse{Kind: kind, Message: message, Details: details})
	return answer
}

// diagnostics reports a reason in the envelope's own vocabulary, because a
// caller that reads the envelope does not have to know the payload shape to
// find out what went wrong.
func diagnostics(response ErrorResponse) []plugins.Diagnostic {
	level := "error"
	if response.Kind == ErrorUnsupported || response.Kind == ErrorNotFound {
		level = "warning"
	}
	return []plugins.Diagnostic{{Level: level, Message: response.Message}}
}

// writeAnswer writes one answer as one line.
func writeAnswer(writer *bufio.Writer, answer plugins.Response) error {
	encoded, err := json.Marshal(answer)
	if err != nil {
		return err
	}
	if _, err := writer.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return writer.Flush()
}

// decode reads a hook's request.
func decode[T any](request plugins.Request) (T, error) {
	var out T
	if len(request.Payload) == 0 {
		return out, fmt.Errorf("the request carries no %s payload", request.Hook)
	}
	if err := json.Unmarshal(request.Payload, &out); err != nil {
		return out, fmt.Errorf("the %s payload could not be read: %w", request.Hook, err)
	}
	return out, nil
}

// DescribeResponse is what an engine answers to `describe`.
type DescribeResponse struct {
	Engine Engine `json:"engine"`
}
