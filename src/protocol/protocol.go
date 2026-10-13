// Package protocol turns a Rho schema — enums, messages and rpcs — into the
// code both ends of a link need: packed little-endian C for a node, Go with the
// same wire and JSON for an edge or cloud service. One file is written once and
// both sides are generated from it, so the two can only disagree if the schema
// disagrees with itself.
//
// This is deliberately separate from the language's own C and Go backends. Those
// compile a program; this emits an interface. A schema is not a program, and its
// output is not a translation of one.
package protocol

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rho/src/frontend/ast"
	"rho/src/frontend/lexer"
	"rho/src/frontend/parser"
)

// Schema is a parsed protocol file: its namespace, and the declarations a wire
// cares about.
type Schema struct {
	// Namespace is the schema's versioned name, such as "telemetry.v1".
	Namespace string
	// Version is the number in the namespace's last component, or 1.
	Version  int
	Enums    []ast.EnumDecl
	Messages []ast.MessageDecl
	Rpcs     []ast.RpcDecl
	// Includes are the schemas this one includes, relative to it. A schema that
	// shares its identity with another writes it once and includes it, so the
	// identity is one declaration rather than two kept in step.
	Includes []string
}

// Parse reads a schema from source.
func Parse(source string) (*Schema, error) {
	tokens, err := lexer.Lex(source)
	if err != nil {
		return nil, err
	}
	mod, err := parser.Parse(tokens)
	if err != nil {
		return nil, err
	}
	return FromModule(mod), nil
}

// FromModule collects the schema declarations of a parsed module. A file with no
// schema declarations is an empty schema, not an error: a program may include
// one for the types and none for the wire.
func FromModule(mod *ast.Module) *Schema {
	schema := &Schema{Namespace: mod.Namespace}
	for _, inc := range mod.Includes {
		schema.Includes = append(schema.Includes, inc.Path)
	}
	for _, decl := range mod.Decls {
		switch d := decl.(type) {
		case ast.EnumDecl:
			schema.Enums = append(schema.Enums, d)
		case ast.MessageDecl:
			schema.Messages = append(schema.Messages, d)
		case ast.RpcDecl:
			schema.Rpcs = append(schema.Rpcs, d)
		}
	}
	schema.Version = namespaceVersion(schema.Namespace)
	return schema
}

/* -------------------------------------------------------------------------- */
/*  Includes                                                                  */
/* -------------------------------------------------------------------------- */

// LoadFile reads a schema and every schema it includes, and merges them into
// one, so a caller generates from a root and gets the whole wire.
//
// An include is a path relative to the file that wrote it. A schema included by
// two others is merged once — the names are deduplicated first-wins — so a
// diamond is not a conflict; a schema that includes itself, directly or through
// another, is a cycle and is reported rather than followed until the stack ends.
func LoadFile(path string) (*Schema, error) {
	return loadSchema(path, map[string]bool{})
}

func loadSchema(path string, visiting map[string]bool) (*Schema, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if visiting[absolute] {
		return nil, fmt.Errorf("%s: include cycle", path)
	}
	visiting[absolute] = true
	defer delete(visiting, absolute)

	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, err
	}
	parsed, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	merged := &Schema{}
	// The including file's own declarations are merged first, so a name it
	// declares wins over one it includes: the nearer definition is the one a
	// reader of this file means.
	Merge(merged, parsed)
	for _, include := range parsed.Includes {
		child, childErr := loadSchema(filepath.Join(filepath.Dir(absolute), filepath.FromSlash(include)), visiting)
		if childErr != nil {
			return nil, childErr
		}
		Merge(merged, child)
	}
	return merged, nil
}

// Merge adds a schema's declarations to another's, dropping a name already
// present. Namespace is taken from the first schema that has one.
func Merge(into, add *Schema) {
	if into.Namespace == "" {
		into.Namespace = add.Namespace
		into.Version = add.Version
	}
	for _, e := range add.Enums {
		if !hasEnum(into, e.Name) {
			into.Enums = append(into.Enums, e)
		}
	}
	for _, m := range add.Messages {
		if !hasMessage(into, m.Name) {
			into.Messages = append(into.Messages, m)
		}
	}
	for _, r := range add.Rpcs {
		if !hasRpc(into, r.Name) {
			into.Rpcs = append(into.Rpcs, r)
		}
	}
}

func hasEnum(schema *Schema, name string) bool {
	for _, e := range schema.Enums {
		if e.Name == name {
			return true
		}
	}
	return false
}

func hasMessage(schema *Schema, name string) bool {
	for _, m := range schema.Messages {
		if m.Name == name {
			return true
		}
	}
	return false
}

func hasRpc(schema *Schema, name string) bool {
	for _, r := range schema.Rpcs {
		if r.Name == name {
			return true
		}
	}
	return false
}

// namespaceVersion reads the number out of a namespace's last component, so
// "telemetry.v1" is version 1 and a namespace with no number is version 1.
func namespaceVersion(namespace string) int {
	if namespace == "" {
		return 1
	}
	parts := strings.Split(namespace, ".")
	last := parts[len(parts)-1]
	digits := strings.TrimPrefix(last, "v")
	if digits == "" {
		return 1
	}
	if value, err := strconv.Atoi(digits); err == nil {
		return value
	}
	return 1
}

// AllMessages is every message the wire carries: the ones the schema declared,
// and the request and response of each rpc, named after it.
//
// They are ordered so that a message comes after every message it contains. A
// language whose types must be defined before they are used — C, and Rho's
// structs — cannot lay out a struct that holds one not yet defined, and a schema
// author writing the identity after a message that carries it is writing the
// obvious thing, not a mistake to be reported.
func (s *Schema) AllMessages() []ast.MessageDecl {
	out := append([]ast.MessageDecl{}, s.Messages...)
	for _, r := range s.Rpcs {
		out = append(out, ast.MessageDecl{Name: r.Name + "Request", Fields: r.Request})
		out = append(out, ast.MessageDecl{Name: r.Name + "Response", Fields: r.Response})
	}
	return orderByDependency(out)
}

// orderByDependency is a stable topological sort of the messages: each is
// emitted after the messages its fields are, and messages that do not depend on
// one another keep the order they were declared in.
func orderByDependency(messages []ast.MessageDecl) []ast.MessageDecl {
	byName := map[string]ast.MessageDecl{}
	for _, message := range messages {
		byName[message.Name] = message
	}
	// 0 is unvisited, 1 is being visited — a cycle, which the layout reports
	// rather than this — and 2 is emitted.
	state := map[string]int{}
	out := make([]ast.MessageDecl, 0, len(messages))
	var visit func(name string)
	visit = func(name string) {
		if state[name] != 0 {
			return
		}
		state[name] = 1
		message := byName[name]
		for _, field := range message.Fields {
			if _, isMessage := byName[field.TypeExpr.Name]; isMessage {
				visit(field.TypeExpr.Name)
			}
		}
		state[name] = 2
		out = append(out, message)
	}
	for _, message := range messages {
		visit(message.Name)
	}
	return out
}

/* -------------------------------------------------------------------------- */
/*  Layout                                                                    */
/* -------------------------------------------------------------------------- */

// scalarWire is the width of each scalar on the wire, in bytes.
var scalarWire = map[string]int{
	"u8": 1, "i8": 1, "bool": 1,
	"u16": 2, "i16": 2,
	"u32": 4, "i32": 4, "f32": 4,
	"u64": 8, "i64": 8, "f64": 8,
}

// stringWire is the width a bare `string` takes. A fixed layout has to give a
// string a length, and a schema that cares should write `string[N]`; this is the
// default for one that did not.
const stringWire = 64

// enumWire is the width an enum takes: a signed 32-bit value, which is what C
// gives an enum and what every target can write without negotiation.
const enumWire = 4

// layout resolves a schema into sizes: how wide one field is, and how wide a
// whole message is on the wire. A message is its own fields, preceded by the
// version byte.
type layout struct {
	enumNames map[string]bool
	fields    map[string][]ast.Field
	sizes     map[string]int
	visiting  map[string]bool
}

func (s *Schema) newLayout() *layout {
	l := &layout{
		enumNames: map[string]bool{},
		fields:    map[string][]ast.Field{},
		sizes:     map[string]int{},
		visiting:  map[string]bool{},
	}
	for _, e := range s.Enums {
		l.enumNames[e.Name] = true
	}
	for _, m := range s.AllMessages() {
		l.fields[m.Name] = m.Fields
	}
	return l
}

// kind reports what a type name is: a scalar, an enum, a message or a string.
func (l *layout) kind(name string) string {
	if _, ok := scalarWire[name]; ok {
		return "scalar"
	}
	if name == "string" {
		return "string"
	}
	if l.enumNames[name] {
		return "enum"
	}
	if _, ok := l.fields[name]; ok {
		return "message"
	}
	return "unknown"
}

// sizeOfField is a field's wire width, arrays included.
func (l *layout) sizeOfField(f ast.Field) int {
	element := l.sizeOfType(f.TypeExpr.Name)
	if f.TypeExpr.ArrayLen != nil {
		return element * *f.TypeExpr.ArrayLen
	}
	return element
}

func (l *layout) sizeOfType(name string) int {
	switch l.kind(name) {
	case "scalar":
		return scalarWire[name]
	case "string":
		return stringWire
	case "enum":
		return enumWire
	case "message":
		return l.messageSize(name)
	}
	return 1
}

// messageSize is a message's wire width: the version byte and its fields. A
// message that contains itself has no size, and is reported where it is found
// rather than recursing until the stack ends.
func (l *layout) messageSize(name string) int {
	if size, ok := l.sizes[name]; ok {
		return size
	}
	if l.visiting[name] {
		panic(fmt.Sprintf("message %s contains itself, which cannot be packed", name))
	}
	l.visiting[name] = true
	size := 0
	for _, f := range l.fields[name] {
		size += l.sizeOfField(f)
	}
	delete(l.visiting, name)
	l.sizes[name] = 1 + size
	return l.sizes[name]
}

/* -------------------------------------------------------------------------- */
/*  Naming                                                                    */
/* -------------------------------------------------------------------------- */

var cScalars = map[string]string{
	"u8": "uint8_t", "u16": "uint16_t", "u32": "uint32_t", "u64": "uint64_t",
	"i8": "int8_t", "i16": "int16_t", "i32": "int32_t", "i64": "int64_t",
	"f32": "float", "f64": "double", "bool": "bool",
}

var goScalars = map[string]string{
	"u8": "uint8", "u16": "uint16", "u32": "uint32", "u64": "uint64",
	"i8": "int8", "i16": "int16", "i32": "int32", "i64": "int64",
	"f32": "float32", "f64": "float64", "bool": "bool",
}

// cBase is the C type a scalar, enum, message or string element is written as.
func (l *layout) cBase(name string) string {
	if c, ok := cScalars[name]; ok {
		return c
	}
	if name == "string" {
		return "char"
	}
	if l.kind(name) == "enum" || l.kind(name) == "message" {
		return name
	}
	return "uint8_t /* unknown type " + name + " */"
}

// cDeclaration is a struct field's C declaration: a type and a name, an array
// where the schema gave it a length.
func (l *layout) cDeclaration(f ast.Field) string {
	name := snake(f.Name)
	if f.TypeExpr.ArrayLen != nil {
		return fmt.Sprintf("%s %s[%d]", l.cBase(f.TypeExpr.Name), name, *f.TypeExpr.ArrayLen)
	}
	if f.TypeExpr.Name == "string" {
		return fmt.Sprintf("char %s[%d]", name, stringWire)
	}
	return l.cBase(f.TypeExpr.Name) + " " + name
}

func (l *layout) goType(f ast.Field) string {
	base := goScalars[f.TypeExpr.Name]
	if base == "" {
		if f.TypeExpr.Name == "string" {
			base = "string"
		} else if l.kind(f.TypeExpr.Name) == "enum" || l.kind(f.TypeExpr.Name) == "message" {
			base = f.TypeExpr.Name
		} else {
			base = "uint8 /* unknown type " + f.TypeExpr.Name + " */"
		}
	}
	if f.TypeExpr.ArrayLen != nil {
		return fmt.Sprintf("[%d]%s", *f.TypeExpr.ArrayLen, base)
	}
	return base
}

// snake lower-cases a name and separates its words with underscores:
// SetMotorSpeed becomes set_motor_speed, and a run of capitals — an initialism
// like INFO — stays one word rather than becoming i_n_f_o.
func snake(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			afterLower := i > 0 && (runes[i-1] >= 'a' && runes[i-1] <= 'z' || runes[i-1] >= '0' && runes[i-1] <= '9')
			beforeLower := i > 0 && i+1 < len(runes) &&
				runes[i-1] >= 'A' && runes[i-1] <= 'Z' && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if afterLower || beforeLower {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// exported upper-cases the first letter of every underscore-separated part, the
// Go name for a schema field.
func exported(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, "")
}

// macroName is the upper-case, underscored form a C macro or enum constant
// takes: Severity_INFO becomes SEVERITY_INFO and SetMotorSpeed becomes
// SET_MOTOR_SPEED.
func macroName(name string) string {
	return strings.ToUpper(snake(name))
}

func namespaceOr(namespace, fallback string) string {
	if namespace == "" {
		return fallback
	}
	return namespace
}

// prefix is the lower-case name generated C helpers are mangled with, derived
// from the namespace so two schemas can be linked without colliding.
func (s *Schema) prefix() string {
	return snake(strings.ReplaceAll(namespaceOr(s.Namespace, "protocol"), ".", "_"))
}

// namespaceMacro is the namespace as a C macro prefix: telemetry.v1 becomes
// TELEMETRY_V1, and an unnamed schema becomes PROTOCOL.
func namespaceMacro(namespace string) string {
	return macroName(strings.ReplaceAll(namespaceOr(namespace, "protocol"), ".", "_"))
}
