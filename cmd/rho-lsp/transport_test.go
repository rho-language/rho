//
// @file transport_test.go
// @brief Tests for Rho LSP Transport Layer.
//
// Validates:
//   - Content-Length parsing
//   - JSON payload extraction
//   - writeMessage formatting
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"bufio"
	"bytes"
	"testing"
)

func TestReadMessage(t *testing.T) {
	input := "Content-Length: 17\r\n\r\n" +
		`{"jsonrpc":"2.0"}`

	r := bufio.NewReader(bytes.NewBufferString(input))

	msg, err := readMessage(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(msg) != `{"jsonrpc":"2.0"}` {
		t.Fatalf("unexpected payload: %s", string(msg))
	}
}

func TestWriteMessage(t *testing.T) {
	var out bytes.Buffer

	err := writeMessage(&out, []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "Content-Length: 11\r\n\r\n" + `{"ok":true}`
	if out.String() != expected {
		t.Fatalf("unexpected output: %s", out.String())
	}
}
