//
// @file server_test.go
// @brief Tests for Rho LSP Server.
//
// Validates:
// - JSON-RPC dispatch
// - initialize handler
// - didOpen updates workspace
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package server

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func send(in string) (string, error) {
	srv := New()

	input := bytes.NewBufferString(in)
	output := &bytes.Buffer{}

	err := srv.Start(input, output)
	return output.String(), err
}

func TestServerInitialize(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///"}}`
	req := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)

	resp, err := send(req)
	if err != nil {
		t.Fatalf("server error: %v", err)
	}

	if !strings.Contains(resp, `"capabilities"`) {
		t.Fatalf("missing capabilities: %s", resp)
	}
}

func TestServerDidOpen(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":2,"method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///test.rho","text":"module x"}}}`
	req := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)

	resp, err := send(req)
	if err != nil {
		t.Fatalf("server error: %v", err)
	}

	if !strings.Contains(resp, `"id":2`) {
		t.Fatalf("missing response id: %s", resp)
	}
}

func TestServerAcceptsAdditionalAndLowercaseHeaders(t *testing.T) {
	payload := `{"jsonrpc":"2.0","id":3,"method":"initialize","params":{"rootUri":"file:///"}}`
	input := fmt.Sprintf("content-length: %d\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n%s", len(payload), payload)
	output, err := send(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"capabilities"`) {
		t.Fatalf("missing initialize response: %s", output)
	}
}

func frame(payload string) string {
	return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
}

func TestServerSendsNoResponseToNotifications(t *testing.T) {
	// Real notifications never carry an "id" field.
	didOpen := `{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///test.rho","text":"module x"}}}`
	resp, err := send(frame(didOpen))
	if err != nil {
		t.Fatalf("server error: %v", err)
	}
	if strings.Contains(resp, `"result"`) || strings.Contains(resp, `"error"`) {
		t.Fatalf("server must not respond to a notification: %s", resp)
	}
}

func TestServerIgnoresUnknownNotification(t *testing.T) {
	// $/setTrace has no id; an unsolicited response here would desync clients.
	setTrace := `{"jsonrpc":"2.0","method":"$/setTrace","params":{"value":"verbose"}}`
	resp, err := send(frame(setTrace))
	if err != nil {
		t.Fatalf("server error: %v", err)
	}
	if resp != "" {
		t.Fatalf("expected no output for unknown notification, got: %s", resp)
	}
}

func TestServerRespondsToUnknownRequest(t *testing.T) {
	// A real request (has id) with an unknown method must still get an error response.
	unknown := `{"jsonrpc":"2.0","id":9,"method":"workspace/unknownThing"}`
	resp, err := send(frame(unknown))
	if err != nil {
		t.Fatalf("server error: %v", err)
	}
	if !strings.Contains(resp, `"error"`) || !strings.Contains(resp, `"id":9`) {
		t.Fatalf("expected error response for unknown request: %s", resp)
	}
}
