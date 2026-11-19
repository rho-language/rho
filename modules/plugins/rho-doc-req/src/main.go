//
// @file main.go
// @brief The rho-doc-req command: rho-doc's sources hook, answered.
//
// The documentation generator runs this program with a request on standard
// input and reads the answer from standard output, both JSON. The request names
// the modules a site build found; the answer is the pages written for the ones
// that keep requirements or architecture decisions.
//
// @project Rho
// @version 0.1
// @date 2026-11-19
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rho/rho-doc-req/src/docreq"
)

// request is the shared envelope a component sends a plugin.
type request struct {
	Version int             `json:"version"`
	Hook    string          `json:"hook"`
	Payload json.RawMessage `json:"payload"`
}

// sourcesPayload is rho-doc's part of a sources request.
type sourcesPayload struct {
	Site    string `json:"site"`
	Root    string `json:"root"`
	Modules []struct {
		Name    string `json:"name"`
		Root    string `json:"root"`
		Section string `json:"section"`
	} `json:"modules"`
}

// response is the shared envelope a plugin answers with; the sources answer is
// its payload.
type response struct {
	Version int `json:"version"`
	Payload any `json:"payload,omitempty"`
}

type answer struct {
	Pages []docreq.Page `json:"pages"`
	Notes []string      `json:"notes,omitempty"`
}

func main() {
	os.Exit(run(os.Stdin, os.Stdout))
}

func run(in io.Reader, out io.Writer) int {
	data, err := io.ReadAll(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho-doc-req:", err)
		return 1
	}
	var req request
	if err := json.Unmarshal(data, &req); err != nil {
		fmt.Fprintln(os.Stderr, "rho-doc-req: the request is not JSON:", err)
		return 1
	}
	reply := response{Version: 1}
	if req.Hook == "sources" {
		var payload sourcesPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			fmt.Fprintln(os.Stderr, "rho-doc-req: the sources payload cannot be read:", err)
			return 1
		}
		modules := []docreq.Module{}
		for _, module := range payload.Modules {
			modules = append(modules, docreq.Module{Name: module.Name, Root: module.Root, Section: module.Section})
		}
		// The site documents the tree it was given: the repositories of a family
		// are found beside each other there, not wherever the environment of the
		// person building it points.
		os.Unsetenv("ROBOHUB_WORKSPACE")
		os.Unsetenv("RHO_WORKSPACE")
		pages, notes := docreq.Pages(modules)
		reply.Payload = answer{Pages: pages, Notes: notes}
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho-doc-req:", err)
		return 1
	}
	out.Write(encoded)
	return 0
}
