//
// @file protocol_test.go
// @brief Tests for Rho LSP Protocol Types.
//
// Validates:
// - JSON-RPC request decoding
// - JSON-RPC response encoding
// - basic LSP type marshaling
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package protocol

import (
	"encoding/json"
	"testing"
)

func TestRequestDecode(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":"file:///"}}`

	var req Request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if req.Method != "initialize" {
		t.Fatalf("expected initialize, got %s", req.Method)
	}
}

func TestResponseEncode(t *testing.T) {
	resp := Response{
		JsonRPC: "2.0",
		ID:      1,
		Result:  map[string]any{"ok": true},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	if !json.Valid(data) {
		t.Fatalf("invalid JSON: %s", string(data))
	}
}

func TestPositionMarshal(t *testing.T) {
	pos := Position{Line: 3, Character: 15}
	data, err := json.Marshal(pos)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out Position
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out.Line != 3 || out.Character != 15 {
		t.Fatalf("unexpected position: %+v", out)
	}
}
