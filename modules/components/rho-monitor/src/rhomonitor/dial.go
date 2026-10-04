//
// @file dial.go
// @brief One TCP connection, and the timeout every backend uses.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

import (
	"fmt"
	"net"
	"time"
)

// dialTCP is a TCP connection with a deadline, which is what detection needs and
// what reading does not: a capture that has gone quiet is not a capture that has
// ended.
func dialTCP(host string, port int, timeout time.Duration) (net.Conn, error) {
	if host == "" {
		host = "localhost"
	}
	if port <= 0 {
		return nil, fmt.Errorf("%d is not a port", port)
	}
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	if timeout <= 0 {
		timeout = time.Second
	}
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}
	return connection, nil
}
