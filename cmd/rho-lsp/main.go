//
// @file main.go
// @brief Rho Language Server - Enterprise v1.
//
// Entry point for the Rho LSP. Sets up JSON-RPC, initializes the server,
// and starts the message loop.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package main

import (
	"log"
	"os"

	"rho/cmd/rho-lsp/server"
)

func main() {
	// LSP communicates over STDIN/STDOUT
	in := os.Stdin
	out := os.Stdout

	srv := server.New()

	if err := srv.Start(in, out); err != nil {
		log.Fatalf("rho-lsp: %v", err)
	}
}
