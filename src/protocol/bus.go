package protocol

import (
	"fmt"
	"os"
	"strings"

	"rho/src/yaml"
)

// Bus is how generated C reaches an event bus: the header that declares it, the
// types a call returns and carries, and the three functions that publish,
// subscribe and unsubscribe.
//
// The names belong to the runtime a node is built on, not to this toolchain, so
// they are read from a binding that runtime's platform ships rather than written
// into the generator. A schema generated with no binding has no bus files: the
// bytes are still framed, and nothing is assumed about who carries them.
type Bus struct {
	// Headers are the includes the bus header needs, in order.
	Headers []string
	// ErrorType is what a bus call returns, and the four results the generated
	// code produces.
	ErrorType     string
	ErrOK         string
	ErrInvalidArg string
	ErrProtocol   string
	ErrNoMem      string
	// EventIDType and SizeType are the types of an event id and of a payload
	// length in the bus's signatures.
	EventIDType string
	SizeType    string
	// Publish, Subscribe and Unsubscribe are the bus's functions.
	Publish     string
	Subscribe   string
	Unsubscribe string
}

// LoadBus reads a bus binding from a YAML file.
func LoadBus(path string) (*Bus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bus binding: %w", err)
	}
	doc, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	text := func(key string) string { return yaml.Unquote(doc.Map[key].Str) }
	bus := &Bus{
		ErrorType: text("error_type"), ErrOK: text("ok"), ErrInvalidArg: text("invalid_arg"),
		ErrProtocol: text("protocol_error"), ErrNoMem: text("no_memory"),
		EventIDType: text("event_id_type"), SizeType: text("size_type"),
		Publish: text("publish"), Subscribe: text("subscribe"), Unsubscribe: text("unsubscribe"),
	}
	for _, item := range doc.Map["headers"].List {
		bus.Headers = append(bus.Headers, yaml.Unquote(item.Str))
	}
	if err := bus.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return bus, nil
}

func (b *Bus) validate() error {
	var missing []string
	for name, value := range map[string]string{
		"error_type": b.ErrorType, "ok": b.ErrOK, "invalid_arg": b.ErrInvalidArg,
		"protocol_error": b.ErrProtocol, "no_memory": b.ErrNoMem,
		"event_id_type": b.EventIDType, "size_type": b.SizeType,
		"publish": b.Publish, "subscribe": b.Subscribe, "unsubscribe": b.Unsubscribe,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("bus binding is missing %s", strings.Join(sortedStrings(missing), ", "))
	}
	return nil
}

func sortedStrings(in []string) []string {
	out := append([]string{}, in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
