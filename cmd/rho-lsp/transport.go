//
// @file transport.go
// @brief Rho LSP Transport Layer - Enterprise v1.
//
// Implements JSON-RPC transport over stdio:
//   - reads Content-Length header
//   - reads JSON payload
//   - forwards to server
//   - writes JSON-RPC responses
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
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	serverpkg "rho/cmd/rho-lsp/server"
)

type Server = serverpkg.Server

/* -------------------------------------------------------------------------- */
/*  ReadMessage                                                                 */
/* -------------------------------------------------------------------------- */

func readMessage(r *bufio.Reader) ([]byte, error) {
	// Read headers
	var contentLength int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break // end of headers
		}

		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, _ = strconv.Atoi(val)
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}

	// Read JSON payload
	buf := make([]byte, contentLength)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

/* -------------------------------------------------------------------------- */
/*  WriteMessage                                                                */
/* -------------------------------------------------------------------------- */

func writeMessage(w io.Writer, data []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	_, err := w.Write([]byte(header))
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

/* -------------------------------------------------------------------------- */
/*  Transport Loop                                                              */
/* -------------------------------------------------------------------------- */

func transportLoop(server *Server) {
	reader := bufio.NewReader(os.Stdin)

	for {
		msg, err := readMessage(reader)
		if err != nil {
			return
		}

		resp := server.HandleMessage(msg)
		if resp == nil {
			continue
		}

		writeMessage(os.Stdout, resp)
	}
}
