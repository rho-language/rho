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
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		"func gatewayWallTimeMs() string",
		"func gatewayF32Str(value float32) string",
		"func gatewayNodeSeen(id uint32)",
		"func gatewayNodeCount() uint32",
		"func gatewayNodeAt(index uint32) uint32",
		"func gatewayStateSet(key string, value string)",
		"func gatewayStateGet(key string) string",
		"func gatewayRpcRegister(tx uint32)",
		"func gatewayRpcWait(tx uint32, timeoutMs uint32) string",
		"func gatewayRpcSignal(tx uint32, payload string) bool",
		"func gatewayF32FromStr(value string) float32",
		"func gatewayOutboxOpen(path string) bool",
		"func gatewayOutboxAppend(areaID uint32, nodeID uint32, field string, value float32, status uint32, receivedMS string) uint32",
		"func gatewayOutboxAppendU32(areaID uint32, nodeID uint32, field string, value uint32, status uint32, receivedMS string) uint32",
		"func gatewayOutboxFetch(after uint32, limit uint32) string",
		"func gatewayOutboxAck(seq uint32) uint32",
		"func gatewayOutboxStatus() string",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the gateway port is missing %q", want)
		}
	}

	// The imports the generated code relies on, so a missing one is caught here
	// rather than by the Go compiler inside a generated project.
	for _, want := range []string{`"bufio"`, `"encoding/json"`, `"net"`, `"os"`, `"path/filepath"`, `"strconv"`, `"strings"`, `"sync"`, `"time"`} {
		if !strings.Contains(out, want) {
			t.Errorf("the gateway port is missing the import %s", want)
		}
	}
}

// The outbox is what makes the gateway a feed rather than a latest-value
// snapshot, so it is exercised as a generated program: append two readings,
// read them back, ack one, and check the second survives the compaction.
func TestGatewayOutboxAppendFetchAck(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	imports, body := emitGatewayPort(PortContext{IR: ir.ModuleIR{Name: "gateway"}})
	port := "package main\n\nimport (\n"
	for _, one := range imports {
		port += "\t" + one + "\n"
	}
	port += ")\n\n" + body

	root := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ports.go", port)
	write("main.go", `package main

import "fmt"

func main() {
	if !gatewayOutboxOpen("") {
		fmt.Println("open failed")
		return
	}
	gatewayOutboxAppend(7, 1, "temperature_c", 24.5, 0, "1700000000000")
	gatewayOutboxAppend(7, 1, "pv_power_w", 100, 0, "1700000000001")
	fmt.Println("before", gatewayOutboxFetch(0, 10))
	fmt.Println("removed", gatewayOutboxAck(1))
	fmt.Println("after", gatewayOutboxFetch(0, 10))
	fmt.Println("status", gatewayOutboxStatus())
}
`)
	write("go.mod", "module outboxtest\n\ngo 1.18\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := exec.CommandContext(ctx, "go", "run", ".")
	run.Dir = root
	run.Env = append(os.Environ(), "GOWORK=off",
		"OUTBOX_PATH="+filepath.Join(root, "outbox.jsonl"))
	out, err := run.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("the outbox program did not finish: %v\n%s", ctx.Err(), out)
	}
	if err != nil {
		t.Fatalf("the outbox program failed: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{
		`"field":"temperature_c"`, `"field":"pv_power_w"`,
		"removed 1", `"last_seq":2`, `"depth":1`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the outbox did not report %q:\n%s", want, text)
		}
	}
}


// The uplink must be able to confine itself to one interface, so a deployment
// on a shared network can keep relay dial-ins off it.
func TestGatewayPortHonoursUplinkBind(t *testing.T) {
	port := EmitGoPorts(PortContext{ModuleName: "gateway"}, []string{"gateway"})
	for _, part := range []string{"UPLINK_BIND", `bind + ":" + strconv.FormatUint`} {
		if !strings.Contains(port, part) {
			t.Errorf("gateway port missing %q", part)
		}
	}
}
