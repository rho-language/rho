package protocol

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	rho "rho/src"
)

// fixedStrings has a `string[N]`, a bare string and scalars around them, so a
// width taken wrongly moves every field after it.
const fixedStrings = `module names
namespace names.v1

message Tag {
    id: u8
    name: string[16]
    label: string[4]
    note: string
    count: u16
}
`

// The values each target encodes. The label is longer than its four bytes, so
// every target has to truncate it the same way; the name is shorter than its
// sixteen, so every target has to pad it.
const (
	tagName  = "pump"
	tagLabel = "overflow"
	tagNote  = "tank 2"
)

// string[N] is N bytes on the wire: the size is what the schema says.
func TestFixedStringIsItsWidth(t *testing.T) {
	schema, err := Parse(fixedStrings)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// version + u8 + string[16] + string[4] + string(64) + u16
	const want = 1 + 1 + 16 + 4 + 64 + 2
	if size := schema.newLayout().messageSize("Tag"); size != want {
		t.Fatalf("Tag wire size = %d, want %d", size, want)
	}
	h, c := schema.EmitC()
	for _, s := range []string{"#define TAG_WIRE_SIZE 88", "char name[16];", "char label[4];", "memcpy(out + off, value->name, 16u);"} {
		if !strings.Contains(h+c, s) {
			t.Errorf("C output missing %q", s)
		}
	}
	g := schema.EmitGo("names")
	for _, s := range []string{"const TagWireSize = 88", "Name string `json:\"name\"`", "copy(out[off:off+16], m.Name)", "m.Name = wireString(data[off : off+16])"} {
		if !strings.Contains(g, s) {
			t.Errorf("Go output missing %q", s)
		}
	}
	r, err := schema.EmitRho("")
	if err != nil {
		t.Fatalf("emit rho: %v", err)
	}
	for _, s := range []string{"name: string", "put_string(value.name, 16)", "get_string(data, off, 16)", "strlen(data) < 88"} {
		if !strings.Contains(r, s) {
			t.Errorf("Rho output missing %q", s)
		}
	}
}

// An array other than a string is still refused by the Rho target, by name.
func TestEmitRhoStillRefusesOtherArrays(t *testing.T) {
	schema, err := Parse("module m\nnamespace m.v1\nmessage Block {\n    data: u8[8]\n}\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := schema.EmitRho(""); err == nil || !strings.Contains(err.Error(), "array") {
		t.Fatalf("EmitRho = %v, want an array refusal", err)
	}
}

// The C, Go and Rho codecs write the same bytes for the same Tag, and each
// reads its own bytes back with the padding gone and the truncation kept.
func TestFixedStringRoundTripsAcrossTargets(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("no C compiler on the path")
	}
	schema, err := Parse(fixedStrings)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Each program prints the wire bytes as decimals, then what it decoded.
	want := expectedTagBytes() + "\n" + tagName + "|" + tagLabel[:4] + "|" + tagNote + "|7\n"

	cOut := runCTag(t, schema)
	goOut := runGoTag(t, schema)
	rhoOut := runRhoTag(t, schema)

	if cOut != want {
		t.Errorf("C printed\n%q\nwant\n%q", cOut, want)
	}
	if goOut != want {
		t.Errorf("Go printed\n%q\nwant\n%q", goOut, want)
	}
	if rhoOut != want {
		t.Errorf("Rho printed\n%q\nwant\n%q", rhoOut, want)
	}
}

// expectedTagBytes is the wire the test Tag must be, built by hand.
func expectedTagBytes() string {
	var wire []byte
	wire = append(wire, 1, 42)
	wire = append(wire, padded(tagName, 16)...)
	wire = append(wire, padded(tagLabel, 4)...)
	wire = append(wire, padded(tagNote, 64)...)
	wire = append(wire, 7, 0)
	parts := make([]string, len(wire))
	for i, c := range wire {
		parts[i] = itoa(int(c))
	}
	return strings.Join(parts, ",")
}

func padded(s string, width int) []byte {
	out := make([]byte, width)
	copy(out, s)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func runCTag(t *testing.T, schema *Schema) string {
	t.Helper()
	dir := t.TempDir()
	h, c := schema.EmitC()
	write(t, dir, "protocol.h", h)
	write(t, dir, "protocol.c", c)
	write(t, dir, "main.c", `#include <stdio.h>
#include "protocol.h"

static void print_text(const char* s, size_t width) {
    for (size_t i = 0; i < width && s[i] != 0; i++) { putchar(s[i]); }
}

int main(void) {
    Tag tag;
    memset(&tag, 0, sizeof tag);
    tag.id = 42;
    strncpy(tag.name, "`+tagName+`", sizeof tag.name);
    strncpy(tag.label, "`+tagLabel+`", sizeof tag.label);
    strncpy(tag.note, "`+tagNote+`", sizeof tag.note);
    tag.count = 7;
    uint8_t wire[TAG_WIRE_SIZE];
    size_t n = Tag_serialize(&tag, wire, sizeof wire);
    for (size_t i = 0; i < n; i++) { printf(i ? ",%u" : "%u", wire[i]); }
    putchar('\n');
    Tag back;
    if (!Tag_deserialize(wire, n, &back)) { return 1; }
    print_text(back.name, sizeof back.name); putchar('|');
    print_text(back.label, sizeof back.label); putchar('|');
    print_text(back.note, sizeof back.note);
    printf("|%u\n", back.count);
    return 0;
}
`)
	bin := filepath.Join(dir, "tag")
	if out, err := exec.Command("cc", "-std=c11", "-Wall", "-Werror", "-Wno-stringop-truncation",
		"-o", bin, filepath.Join(dir, "main.c"), filepath.Join(dir, "protocol.c")).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	return run(t, bin)
}

func runGoTag(t *testing.T, schema *Schema) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module tag\n\ngo 1.21\n")
	write(t, dir, "protocol.go", schema.EmitGo("main"))
	write(t, dir, "main.go", `package main

import (
	"fmt"
	"strings"
)

func main() {
	tag := Tag{Id: 42, Name: "`+tagName+`", Label: "`+tagLabel+`", Note: "`+tagNote+`", Count: 7}
	wire, err := tag.MarshalBinary()
	if err != nil {
		panic(err)
	}
	parts := make([]string, len(wire))
	for i, c := range wire {
		parts[i] = fmt.Sprint(c)
	}
	fmt.Println(strings.Join(parts, ","))
	var back Tag
	if err := back.UnmarshalBinary(wire); err != nil {
		panic(err)
	}
	fmt.Printf("%s|%s|%s|%d\n", back.Name, back.Label, back.Note, back.Count)
}
`)
	bin := filepath.Join(dir, "tag")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return run(t, bin)
}

func runRhoTag(t *testing.T, schema *Schema) string {
	t.Helper()
	generated, err := schema.EmitRho("")
	if err != nil {
		t.Fatalf("emit rho: %v", err)
	}
	// The generated file is a module; the program is that module made main,
	// with a main that encodes, prints and decodes one Tag.
	lines := strings.SplitN(generated, "\n", 3)
	program := "module main\n" + lines[2] + `
fn main() void {
    let id: u8 = u8(42)
    let name: string = "` + tagName + `"
    let label: string = "` + tagLabel + `"
    let note: string = "` + tagNote + `"
    let count: u16 = u16(7)
    let tag: Tag = Tag { id, name, label, note, count }
    let wire: string = tag_marshal(tag)
    var line: string = ""
    var i: u32 = 0
    while i < strlen(wire) {
        if i > 0 {
            line = line + ","
        }
        line = line + int_to_str(u32(str_at(wire, i)))
        i = i + 1
    }
    print(line + "\n")
    if !tag_valid(wire) {
        print("invalid\n")
        return
    }
    let back: Tag = tag_unmarshal(wire)
    print(back.name + "|" + back.label + "|" + back.note + "|" + int_to_str(u32(back.count)) + "\n")
}
`
	root := t.TempDir()
	write(t, root, "module.yml", "module:\n  name: tag\n  type: rho\n  version: 0.1.0\nmain: src/main.rho\nbuild:\n  language: go\n  targets:\n    - go\n")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "src"), "main.rho", program)
	dirs, err := rho.GenerateAllTargets(filepath.Join(root, "src", "main.rho"), "", nil, nil, nil)
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, program)
	}
	bin := filepath.Join(t.TempDir(), "tag")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = dirs[0]
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return run(t, bin)
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, bin string) string {
	t.Helper()
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", filepath.Base(bin), err, out)
	}
	return string(out)
}
