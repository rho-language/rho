package protocol

import (
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
	header, source := schema.EmitCBus()
	for _, want := range []string{
		"#include \"robohub/core/event.h\"",
		"RoboHubError SetMotorSpeed_publish_request(",
		"RoboHubError SetMotorSpeed_subscribe(",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("C bus header missing %q", want)
		}
	}
	for _, want := range []string{
		"robohub_core_event_publish(SET_MOTOR_SPEED_REQUEST_EVENT_ID",
		"robohub_core_event_subscribe(SET_MOTOR_SPEED_REQUEST_EVENT_ID",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("C bus source missing %q", want)
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
	out, err := schema.EmitRho()
	if err != nil {
		t.Fatalf("emit rho: %v", err)
	}
	for _, want := range []string{
		// The robohub framing, generated so the gateway's copy cannot drift
		// from robohub-transport's.
		"fn telemetry_v1_fletcher16(",
		"fn telemetry_v1_cobs_encode(",
		"fn telemetry_v1_cobs_decode(",
		"fn telemetry_v1_frame(",
		"fn telemetry_v1_unframe(",
		"struct TransportFrame {",
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
