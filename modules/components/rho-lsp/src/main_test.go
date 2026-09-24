//
// @file main_test.go
// @brief Tests for Rho LSP entry point.
//
// Validates:
// - server starts
// - server handles initialize request
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/rho/rho-lsp/src/server"
)

func TestLSPInitialize(t *testing.T) {
	srv := server.New()

	// Fake LSP initialize request
	payload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///"}}`
	req := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)

	in := bytes.NewBufferString(req)
	out := &bytes.Buffer{}

	err := srv.Start(in, out)
	if err != nil {
		t.Fatalf("server start failed: %v", err)
	}

	resp := out.String()
	if !strings.Contains(resp, `"id":1`) {
		t.Fatalf("missing response id: %s", resp)
	}
	if !strings.Contains(resp, `"capabilities"`) {
		t.Fatalf("missing capabilities: %s", resp)
	}
}
