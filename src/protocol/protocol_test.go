package protocol

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `module telemetry
namespace telemetry.v1

enum Severity {
    INFO = 0
    WARN = 1
    ERROR = 2
}

message MotorTelemetry {
    timestamp_ms: u32
    current_amps: f32
    rpm: f32
    status: Severity
}

rpc SetMotorSpeed {
    request {
        motor_id: u8
        target_rpm: f32
    }
    response {
        success: bool
        actual_rpm: u16
    }
}
`

func TestParseReadsTheSchema(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if schema.Namespace != "telemetry.v1" {
		t.Fatalf("namespace = %q, want telemetry.v1", schema.Namespace)
	}
	if schema.Version != 1 {
		t.Fatalf("version = %d, want 1", schema.Version)
	}
	if len(schema.Enums) != 1 || len(schema.Messages) != 1 || len(schema.Rpcs) != 1 {
		t.Fatalf("got %d enums, %d messages, %d rpcs; want 1 each",
			len(schema.Enums), len(schema.Messages), len(schema.Rpcs))
	}
	if got := schema.Enums[0].Variants[2]; got.Name != "ERROR" || got.Value != 2 {
		t.Fatalf("Severity[2] = %+v, want ERROR = 2", got)
	}
	if len(schema.Rpcs[0].Request) != 2 || len(schema.Rpcs[0].Response) != 2 {
		t.Fatalf("SetMotorSpeed has %d request and %d response fields, want 2 each",
			len(schema.Rpcs[0].Request), len(schema.Rpcs[0].Response))
	}
	// 1 version byte + u32 + f32 + f32 + enum(i32) = 17.
	if size := schema.newLayout().messageSize("MotorTelemetry"); size != 17 {
		t.Fatalf("MotorTelemetry wire size = %d, want 17", size)
	}
}

func TestEmitC(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	header, source := schema.EmitC()

	for _, want := range []string{
		"#define TELEMETRY_V1_SCHEMA_VERSION 1",
		"SEVERITY_INFO = 0",
		"SEVERITY_ERROR = 2",
		"typedef struct __attribute__((packed)) {",
		"uint32_t timestamp_ms;",
		"float current_amps;",
		"Severity status;",
		"#define MOTOR_TELEMETRY_WIRE_SIZE 17",
		"size_t MotorTelemetry_serialize(",
		"uint8_t motor_id;",
		"#define SET_MOTOR_SPEED_REQUEST_EVENT_ID",
		"#define SET_MOTOR_SPEED_RESPONSE_EVENT_ID",
		"typedef int (*SetMotorSpeed_handler)",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("C header missing %q", want)
		}
	}
	for _, want := range []string{
		"telemetry_v1_put_u8(out + off, (uint8_t)1);",
		"telemetry_v1_put_u32(out + off, (uint32_t)value->timestamp_ms);",
		"telemetry_v1_put_f32(out + off, value->current_amps);",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("C source missing %q", want)
		}
	}
}

func TestEmitCBus(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	header, source := schema.EmitCBus(testBus(t))
	for _, want := range []string{
		"#include \"acme/bus.h\"",
		"AcmeStatus SetMotorSpeed_publish_request(",
		"AcmeStatus SetMotorSpeed_subscribe(",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("C bus header missing %q", want)
		}
	}
	for _, want := range []string{
		"acme_bus_publish(SET_MOTOR_SPEED_REQUEST_EVENT_ID",
		"acme_bus_subscribe(SET_MOTOR_SPEED_REQUEST_EVENT_ID",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("C bus source missing %q", want)
		}
	}
}

// Both generated C headers are wrapped so a C++ translation unit links against
// the C that frames the bytes: without the guard every symbol would be
// name-mangled, and a C++ program could not call the codec its C neighbour uses.
func TestEmitCGuardsCxx(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	protocolHeader, _ := schema.EmitC()
	busHeader, _ := schema.EmitCBus(testBus(t))

	for name, header := range map[string]string{
		"protocol.h":     protocolHeader,
		"protocol_bus.h": busHeader,
	} {
		for _, want := range []string{"#ifdef __cplusplus", "extern \"C\" {", "}\n#endif"} {
			if !strings.Contains(header, want) {
				t.Errorf("%s is missing %q", name, want)
			}
		}
		open := strings.Index(header, "extern \"C\" {")
		closing := strings.Index(header, "}\n#endif")
		if open < 0 || closing < 0 || closing < open {
			t.Errorf("%s: the extern \"C\" guard does not open before it closes", name)
		}
	}
}

func TestEmitGo(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := schema.EmitGo("telemetry")

	for _, want := range []string{
		"package telemetry",
		"const SchemaVersion = 1",
		"type Severity int32",
		"SeverityINFO",
		"type MotorTelemetry struct {",
		"TimestampMs uint32 `json:\"timestamp_ms\"`",
		"CurrentAmps float32 `json:\"current_amps\"`",
		"const MotorTelemetryWireSize = 17",
		"func (m *MotorTelemetry) MarshalBinary() ([]byte, error)",
		"func (m *MotorTelemetry) UnmarshalBinary(data []byte) error",
		"math.Float32bits(m.CurrentAmps)",
		"type SetMotorSpeedFunc func(request SetMotorSpeedRequest) (SetMotorSpeedResponse, error)",
		"func RegisterSetMotorSpeed(mux *http.ServeMux, path string, handler SetMotorSpeedFunc)",
		"\"encoding/binary\"",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Go output missing %q", want)
		}
	}
}

func TestEmitRhoWritesTransportAndVersion(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	framing, err := os.ReadFile("testdata/transport.rho")
	if err != nil {
		t.Fatalf("read transport binding: %v", err)
	}
	out, err := schema.EmitRho(string(framing))
	if err != nil {
		t.Fatalf("emit rho: %v", err)
	}
	if strings.Contains(out, "A made-up framing") {
		t.Error("a framing's # notes were carried into the generated Rho")
	}
	for _, want := range []string{
		// The framing the integration supplied, with the schema's prefix spliced
		// in, so both ends of a link generate one framing.
		"fn telemetry_v1_frame(",
		"fn telemetry_v1_unframe(",
		// The payloads, the version byte first, the same as the C and Go ends.
		"struct MotorTelemetry {",
		"str_from_byte(u8(1))",
		"f32_to_bits(value)",
		"f32_from_bits(",
		// An enum travels as its u32 wire value, never as its own name.
		"put_u32(value.status)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated Rho is missing %q", want)
		}
	}
	// A format-string bug once left an unresolved verb in the output, which
	// then failed to parse rather than failing here.
	if strings.Contains(out, "%!") {
		t.Errorf("generated Rho carries an unresolved format verb")
	}
}

/* -------------------------------------------------------------------------- */
/*  Ordering                                                                  */
/* -------------------------------------------------------------------------- */

// A message that contains another is emitted after it. C and Rho structs need a
// type defined before it is used, and a schema may write the identity after the
// message that carries it — the merge of a shared identity does exactly that.
func TestAllMessagesOrdersDependencies(t *testing.T) {
	source := "module x\nnamespace x.v1\n\n" +
		"message Telemetry {\n    device: DeviceId\n}\n\n" +
		"message DeviceId {\n    area_id: u16\n    node_id: u16\n}\n"
	schema, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}

	position := map[string]int{}
	for i, m := range schema.AllMessages() {
		position[m.Name] = i
	}
	if position["DeviceId"] > position["Telemetry"] {
		t.Fatalf("DeviceId must be emitted before Telemetry, got %v", position)
	}
}

// Messages that do not depend on one another keep the order they were written
// in, so an unrelated change does not churn the generated files.
func TestAllMessagesKeepsIndependentOrder(t *testing.T) {
	source := "module x\nnamespace x.v1\n\n" +
		"message Alpha {\n    a: u16\n}\n\n" +
		"message Beta {\n    b: u16\n}\n"
	schema, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	ordered := schema.AllMessages()
	if len(ordered) != 2 || ordered[0].Name != "Alpha" || ordered[1].Name != "Beta" {
		t.Fatalf("independent messages were reordered: %v", ordered)
	}
}

/* -------------------------------------------------------------------------- */
/*  Includes                                                                  */
/* -------------------------------------------------------------------------- */

// writeSchema writes one schema file, making its directory.
func writeSchema(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func messageNames(schema *Schema) []string {
	names := []string{}
	for _, m := range schema.Messages {
		names = append(names, m.Name)
	}
	return names
}

// An include is merged into the schema that names it, so a caller generates from
// a root and gets the whole wire.
func TestLoadFileFollowsIncludes(t *testing.T) {
	dir := t.TempDir()
	writeSchema(t, dir, "common.rho", "module common\nnamespace acme.v1\n\nmessage DeviceId {\n    area_id: u16\n    node_id: u16\n}\n")
	root := writeSchema(t, dir, "link.rho", "module link\nnamespace acme.v1\n\ninclude \"common.rho\"\n\nmessage Hello {\n    device: DeviceId\n}\n")

	schema, err := LoadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMessage(schema, "DeviceId") || !hasMessage(schema, "Hello") {
		t.Fatalf("expected both declarations, got %v", messageNames(schema))
	}
	if schema.Namespace != "acme.v1" {
		t.Errorf("namespace = %q, want acme.v1", schema.Namespace)
	}
}

// A schema included by two others is one declaration, not two: a diamond is not
// a conflict.
func TestLoadFileDedupesADiamond(t *testing.T) {
	dir := t.TempDir()
	writeSchema(t, dir, "common.rho", "module common\nnamespace acme.v1\n\nmessage DeviceId {\n    area_id: u16\n    node_id: u16\n}\n")
	writeSchema(t, dir, "a.rho", "module a\nnamespace acme.v1\n\ninclude \"common.rho\"\n\nmessage A {\n    device: DeviceId\n}\n")
	writeSchema(t, dir, "b.rho", "module b\nnamespace acme.v1\n\ninclude \"common.rho\"\n\nmessage B {\n    device: DeviceId\n}\n")
	root := writeSchema(t, dir, "root.rho", "module root\nnamespace acme.v1\n\ninclude \"a.rho\"\ninclude \"b.rho\"\n")

	schema, err := LoadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range schema.Messages {
		if m.Name == "DeviceId" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("DeviceId appears %d times, want 1: %v", count, messageNames(schema))
	}
}

// The nearer definition is the one a reader of the file means, so a declaration
// in the including file wins over one it includes.
func TestLoadFileKeepsTheIncludingDeclaration(t *testing.T) {
	dir := t.TempDir()
	writeSchema(t, dir, "common.rho", "module common\nnamespace acme.v1\n\nmessage Shared {\n    a: u16\n}\n")
	root := writeSchema(t, dir, "root.rho", "module root\nnamespace acme.v1\n\ninclude \"common.rho\"\n\nmessage Shared {\n    a: u16\n    b: u16\n}\n")

	schema, err := LoadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range schema.Messages {
		if m.Name == "Shared" {
			if len(m.Fields) != 2 {
				t.Fatalf("expected the including declaration (2 fields), got %d", len(m.Fields))
			}
			return
		}
	}
	t.Fatal("Shared is missing")
}

// A schema that includes itself, directly or through another, is reported rather
// than followed until the stack ends.
func TestLoadFileRejectsACycle(t *testing.T) {
	dir := t.TempDir()
	writeSchema(t, dir, "a.rho", "module a\nnamespace acme.v1\n\ninclude \"b.rho\"\n")
	writeSchema(t, dir, "b.rho", "module b\nnamespace acme.v1\n\ninclude \"a.rho\"\n")
	if _, err := LoadFile(filepath.Join(dir, "a.rho")); err == nil {
		t.Fatal("expected a cycle to be reported")
	}
}

// An include that is not there is a schema that cannot be read, and says which.
func TestLoadFileReportsAMissingInclude(t *testing.T) {
	dir := t.TempDir()
	root := writeSchema(t, dir, "root.rho", "module root\nnamespace acme.v1\n\ninclude \"nope.rho\"\n")
	if _, err := LoadFile(root); err == nil {
		t.Fatal("expected a missing include to be reported")
	}
}

// testBus is a made-up binding (testdata/bus.yml): the generator is tested
// against a binding and carries none of its own.
func testBus(t *testing.T) *Bus {
	t.Helper()
	bus, err := LoadBus("testdata/bus.yml")
	if err != nil {
		t.Fatalf("load bus binding: %v", err)
	}
	return bus
}

func TestLoadBusNamesWhatIsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.yml")
	if err := os.WriteFile(path, []byte("publish: send\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBus(path); err == nil || !strings.Contains(err.Error(), "error_type") {
		t.Fatalf("want an error naming error_type, got %v", err)
	}
}

func TestEmitRhoWithoutFramingSaysNothingAboutTransport(t *testing.T) {
	schema, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	out, err := schema.EmitRho("")
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"fletcher16", "cobs_", "TransportFrame"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("no framing was given, yet the output carries %q", unwanted)
		}
	}
	if !strings.Contains(out, "struct MotorTelemetry {") {
		t.Error("the payloads are generated with or without a framing")
	}
}
