//
// @file emit_go_gatewayport_test.go
// @brief Tests for the gateway port's generated host primitives.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"strings"
	"testing"

	"rho/src/ir"
)

// The gateway port supplies exactly what a Rho gateway declares: the serial
// link, a datagram, the clock, the shared state, transaction correlation, and
// the deployment's mode and area. A function the Rho source calls but the port
// does not define is a link error, so each is named here.
func TestGatewayPortSuppliesTheGateway(t *testing.T) {
	imports, body := emitGatewayPort(PortContext{IR: ir.ModuleIR{Name: "gateway"}})
	out := strings.Join(imports, "\n") + "\n" + body

	for _, want := range []string{
		"func gatewayConfig()",
		"func gatewayMode() string",
		"func gatewayArea() uint32",
		"func gatewayUplinkPort() uint32",
		"func gatewayUplinkStart(port uint32) bool",
		"func gatewayUplinkRead(connection net.Conn, id uint32)",
		"func gatewayUplinkNext() string",
		"func gatewayUplinkConnection() uint32",
		"func gatewayUplinkBind(childArea uint32, childLink uint32, connection uint32)",
		"func gatewayUplinkSend(childArea uint32, childLink uint32, frame string) bool",
		"func gatewaySerialStart() bool",
		"func gatewaySerialRead(timeoutMs uint32) string",
		"func gatewaySerialWrite(data string) bool",
		"func gatewayUdpStart() bool",
		"func gatewayUdpSend(text string) bool",
		"func gatewayNowMs() uint32",
		"func gatewayF32Str(value float32) string",
		"func gatewayNodeSeen(id uint32)",
		"func gatewayNodeCount() uint32",
		"func gatewayNodeAt(index uint32) uint32",
		"func gatewayStateSet(key string, value string)",
		"func gatewayStateGet(key string) string",
		"func gatewayRpcRegister(tx uint32)",
		"func gatewayRpcWait(tx uint32, timeoutMs uint32) string",
		"func gatewayRpcSignal(tx uint32, payload string) bool",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the gateway port is missing %q", want)
		}
	}

	// The imports the generated code relies on, so a missing one is caught here
	// rather than by the Go compiler inside a generated project.
	for _, want := range []string{`"net"`, `"os"`, `"strconv"`, `"strings"`, `"sync"`, `"time"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the gateway port is missing the import %s", want)
		}
	}
}
