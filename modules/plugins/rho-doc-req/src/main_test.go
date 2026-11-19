//
// @file main_test.go
// @brief The protocol: a sources request in, pages out; any other hook, nothing.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestASourcesRequestIsAnswered(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("docreq", "testdata", "workspace"))
	request := `{"version":1,"hook":"sources","command":"requirements","component":"rho-doc","payload":{"site":"T","modules":[` +
		`{"name":"keeper","root":` + quote(filepath.Join(root, "robohub-keeper")) + `,"section":"applications"},` +
		`{"name":"robohub","root":` + quote(filepath.Join(root, "robohub")) + `}]}}`
	var out bytes.Buffer
	if code := run(strings.NewReader(request), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	var reply struct {
		Version int `json:"version"`
		Payload struct {
			Pages []struct {
				Module, Path, Markdown string
				Unlisted               bool
			} `json:"pages"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(out.Bytes(), &reply); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if reply.Version != 1 || len(reply.Payload.Pages) == 0 {
		t.Fatalf("unexpected reply %s", out.String())
	}
	found := map[string]bool{}
	for _, page := range reply.Payload.Pages {
		found[page.Module+"/"+page.Path] = true
	}
	for _, want := range []string{"keeper/requirements/index", "keeper/adr/index", "robohub/requirements/trace"} {
		if !found[want] {
			t.Errorf("expected %s in %v", want, found)
		}
	}
}

func TestAnotherHookIsAnsweredWithNothing(t *testing.T) {
	var out bytes.Buffer
	if code := run(strings.NewReader(`{"version":1,"hook":"index","payload":{}}`), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(out.String()) != `{"version":1}` {
		t.Errorf("unexpected %s", out.String())
	}
}

func quote(text string) string {
	data, _ := json.Marshal(text)
	return string(data)
}
