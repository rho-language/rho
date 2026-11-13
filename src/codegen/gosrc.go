package codegen

import (
	_ "embed"
	"strings"
)

// The Go host sources a generated program may need, kept as real Go files under
// gosrc/ (build-tagged out of this package) and written into a program after
// their marker line.

//go:embed gosrc/service.go
var goServiceSource string

//go:embed gosrc/text.go
var goTextSource string

const goSourceMarker = "// --- emitted below ---\n"

// goSourceBody is the part of a gosrc file a program carries: what follows the
// marker. The package clause and imports above it are there for editors and vet;
// a program's imports are declared with the helper that needs them.
func goSourceBody(source string) string {
	at := strings.Index(source, goSourceMarker)
	if at < 0 {
		return ""
	}
	return strings.TrimLeft(source[at+len(goSourceMarker):], "\n")
}
