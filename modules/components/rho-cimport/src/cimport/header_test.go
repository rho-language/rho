//
// @file header_test.go
// @brief Tests for reading C declarations out of a header.
//
// A binding is only as good as what the reader understood of the header it came
// from, and a C header is written for a compiler rather than for a reader. So
// these tests are about the shapes a real interface is written in: comments that
// run over many lines, declarations that run over several, a macro that is not a
// constant, a const pointer, and a table of function pointers.
//
// The two failures that matter most are silence and noise: a declaration the
// reader missed must be reported, and a line of documentation must not be
// reported as a declaration it could not read.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package cimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// header writes a header and reads it back.
func header(t *testing.T, text string) Header {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.h")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	read := ReadHeader(path, []string{dir}, map[string]bool{}, 0)
	for _, diagnostic := range read.Diagnostics {
		if strings.Contains(diagnostic.Message, "cannot read") {
			t.Errorf("the reader reported a line it should have understood: %s", diagnostic)
		}
	}
	return read
}

/* -------------------------------------------------------------------------- */
/*  Comments                                                                   */
/* -------------------------------------------------------------------------- */

func TestAMultiLineCommentIsNotReadAsDeclarations(t *testing.T) {
	// Every C header documents itself in a block comment that runs over many
	// lines, and a reader that treats the continuation as code reports every
	// documented line as unreadable.
	read := header(t, `/**
 * @file test.h
 * @brief A header.
 *
 * SPDX-License-Identifier: MIT
 */

#pragma once

typedef unsigned int rh_u32;
`)
	if len(read.Decls) != 1 {
		t.Fatalf("expected one declaration, got %d: %+v", len(read.Decls), read.Decls)
	}
	if read.Decls[0].Name != "rh_u32" {
		t.Fatalf("expected the alias, got %+v", read.Decls[0])
	}
}

func TestACommentInsideADeclarationIsNotPartOfItsValue(t *testing.T) {
	// A C header documents an enumeration member and a struct field by putting a
	// comment next to them, which is the normal way and not an edge case.
	read := header(t, `typedef enum {
    ROBOT_OK = 0,        /* Success */
    ROBOT_ERR_IO = -8     /* I/O error */
} RobotError;
`)
	if len(read.Decls) != 1 {
		t.Fatalf("expected one enumeration, got %+v", read.Decls)
	}
	fields := read.Decls[0].Fields
	if len(fields) != 2 {
		t.Fatalf("expected two members, got %+v", fields)
	}
	if fields[0].Name != "ROBOT_OK" || fields[0].Comment != "0" {
		t.Errorf("expected the first member, got %+v", fields[0])
	}
	if fields[1].Name != "ROBOT_ERR_IO" || fields[1].Comment != "-8" {
		t.Errorf("expected the second member, got %+v", fields[1])
	}
}

func TestACommentAboveADeclarationIsCarriedAcross(t *testing.T) {
	// The comment is where a C interface explains itself, and a binding should
	// carry the explanation rather than make the reader open the header.
	read := header(t, `/* Initialise the registry. */
void robohub_registry_init(void);
`)
	if len(read.Decls) != 1 {
		t.Fatalf("expected one function, got %+v", read.Decls)
	}
	if !strings.Contains(read.Decls[0].Comment, "Initialise the registry") {
		t.Errorf("expected the documentation to be carried, got %q", read.Decls[0].Comment)
	}
}

/* -------------------------------------------------------------------------- */
/*  Macros                                                                     */
/* -------------------------------------------------------------------------- */

func TestAMacroIsNotAConstant(t *testing.T) {
	// A macro expands in the source; a constant has a value a binding can carry.
	// Reading one as the other would put `((err) != OK)` in a generated header.
	read := header(t, `#define ROBOT_OK 0
#define ROBOT_SUCCEEDED(err) ((err) == ROBOT_OK)
#define ROBOT_MAX_NAME 32
`)
	names := []string{}
	for _, decl := range read.Decls {
		names = append(names, decl.Name)
	}
	if len(names) != 2 {
		t.Fatalf("expected the two constants, got %+v", names)
	}
	if names[0] != "ROBOT_OK" || names[1] != "ROBOT_MAX_NAME" {
		t.Fatalf("expected the constants that are constants, got %+v", names)
	}
}

func TestAConstantKeepsItsValue(t *testing.T) {
	read := header(t, "#define ROBOT_TIMEOUT_MS 250\n")
	if len(read.Decls) != 1 || read.Decls[0].Value != "250" {
		t.Fatalf("expected the value, got %+v", read.Decls)
	}
}

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

func TestAConstPointerIsAPointerToItsElement(t *testing.T) {
	// `const char*` is a pointer to characters. Reading the qualifier as part of
	// the name types it wrongly, and the pointer is most of what a name here is.
	read := header(t, "typedef const char* rh_name;\n")
	if len(read.Decls) != 1 {
		t.Fatalf("expected one alias, got %+v", read.Decls)
	}
	if read.Decls[0].Type != "char*" {
		t.Fatalf("expected a pointer to characters, got %q", read.Decls[0].Type)
	}
}

func TestAnAggregateDeclaredWithATagIsReadAsAStruct(t *testing.T) {
	// A HAL is written with a tag and a typedef together, and the tag is what
	// the members refer to.
	read := header(t, `typedef struct RobotSPI {
    rh_name name;
    void*   context;
    int32_t (*transfer)(struct RobotSPI* self, const rh_u8* tx, rh_u8* rx, rh_size len);
} RobotSPI;
`)
	if len(read.Decls) != 1 {
		t.Fatalf("expected the struct, got %d: %+v", len(read.Decls), read.Diagnostics)
	}
	decl := read.Decls[0]
	if decl.Kind != KindStruct || decl.Name != "RobotSPI" {
		t.Fatalf("expected the struct, got %+v", decl)
	}
	if len(decl.Fields) != 3 {
		t.Fatalf("expected three members, got %+v", decl.Fields)
	}
}

func TestAFunctionPointerMemberKeepsItsSignature(t *testing.T) {
	// The shim that makes the call Rho cannot make is written from this
	// signature, so losing it here would lose it everywhere.
	read := header(t, `typedef struct RobotGPIO {
    rh_name name;
    int32_t (*write)(struct RobotGPIO* self, rh_u32 pin, rh_u32 value);
} RobotGPIO;
`)
	decl := read.Decls[0]
	var member *Field
	for index := range decl.Fields {
		if decl.Fields[index].Ptr {
			member = &decl.Fields[index]
		}
	}
	if member == nil {
		t.Fatalf("expected a function-pointer member, got %+v", decl.Fields)
	}
	if member.Name != "write" || member.Result != "int32_t" {
		t.Fatalf("expected the member and its result, got %+v", member)
	}
	if len(member.Params) != 3 {
		t.Fatalf("expected three parameters, got %+v", member.Params)
	}
	if member.Params[0].Name != "self" || member.Params[0].Type != "struct RobotGPIO*" {
		t.Fatalf("expected the table as the first parameter, got %+v", member.Params[0])
	}
	if member.Params[1].Name != "pin" || member.Params[1].Type != "rh_u32" {
		t.Fatalf("expected the pin, got %+v", member.Params[1])
	}
}

func TestAMultiLineFunctionIsOneDeclaration(t *testing.T) {
	read := header(t, `RoboHubError robohub_spi_transfer(RoboHubSPI* self,
                                          const rh_u8* tx,
                                          rh_u8* rx,
                                          rh_size len);
`)
	if len(read.Decls) != 1 {
		t.Fatalf("expected one function, got %+v (%+v)", read.Decls, read.Diagnostics)
	}
	if len(read.Decls[0].Params) != 4 {
		t.Fatalf("expected four parameters, got %+v", read.Decls[0].Params)
	}
	if read.Decls[0].Result != "RoboHubError" {
		t.Fatalf("expected the result, got %q", read.Decls[0].Result)
	}
}

func TestALinkageBlockIsNotADeclaration(t *testing.T) {
	// Every header that exports an interface wraps it in `extern "C"`, and a
	// reader that gathers a statement from it swallows the rest of the file.
	read := header(t, `#pragma once
#ifdef __cplusplus
extern "C" {
#endif

void robohub_init(void);

#ifdef __cplusplus
}
#endif
`)
	if len(read.Decls) != 1 || read.Decls[0].Name != "robohub_init" {
		t.Fatalf("expected the one declaration, got %+v (%+v)", read.Decls, read.Diagnostics)
	}
	if len(read.Diagnostics) != 0 {
		t.Fatalf("expected a header with guards to read cleanly, got %+v", read.Diagnostics)
	}
}

/* -------------------------------------------------------------------------- */
/*  Includes and failures                                                      */
/* -------------------------------------------------------------------------- */

func TestAHeaderIsReadThroughItsIncludes(t *testing.T) {
	// A type used by a header the project named is a type the binding needs, and
	// that header is found the way a compiler finds it.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "types.h"), []byte("typedef unsigned int rh_u32;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spi.h"), []byte("#include \"types.h\"\nint32_t robohub_spi_init(rh_u32 speed);\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	read := ReadHeader(filepath.Join(dir, "spi.h"), []string{dir}, map[string]bool{}, 0)
	names := []string{}
	for _, decl := range read.Decls {
		names = append(names, decl.Name)
	}
	if len(names) != 2 || names[0] != "rh_u32" || names[1] != "robohub_spi_init" {
		t.Fatalf("expected the included type and the function, got %+v (%+v)", names, read.Diagnostics)
	}
}

func TestAHeaderThatIsNotThereIsReported(t *testing.T) {
	dir := t.TempDir()
	read := ReadHeader(filepath.Join(dir, "absent.h"), []string{dir}, map[string]bool{}, 0)
	if len(read.Decls) != 0 {
		t.Fatalf("expected nothing read, got %+v", read.Decls)
	}
	if len(read.Diagnostics) != 1 || !strings.Contains(read.Diagnostics[0].Message, "cannot read") {
		t.Fatalf("expected the missing header to be reported, got %+v", read.Diagnostics)
	}
}

func TestABitfieldIsReportedRatherThanRead(t *testing.T) {
	// A bitfield's width is a layout fact Rho cannot express, so a member read
	// without its width would be a member that is not what the C says.
	dir := t.TempDir()
	path := filepath.Join(dir, "flags.h")
	if err := os.WriteFile(path, []byte("typedef struct Flags {\n    rh_u32 a : 3;\n} Flags;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read := ReadHeader(path, []string{dir}, map[string]bool{}, 0)
	if len(read.Diagnostics) != 1 || !strings.Contains(read.Diagnostics[0].Message, "bitfield") {
		t.Fatalf("expected the bitfield to be reported, got %+v", read.Diagnostics)
	}
}

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func TestSplitMembersSplitsOnlyBetweenMembers(t *testing.T) {
	// A table of function pointers is a list of members separated by semicolons,
	// each of which may carry a parameter list with commas in it: splitting on
	// every comma halves every member that has one.
	parts := splitMembers("rh_name name; int32_t (*write)(struct X* self, rh_u32 pin); rh_u32 value;")
	if len(parts) != 3 {
		t.Fatalf("expected three members, got %d: %q", len(parts), parts)
	}
	if !strings.Contains(parts[1], "rh_u32 pin") {
		t.Fatalf("expected the parameter list kept whole, got %q", parts[1])
	}
	// An enumeration is the other shape: its members are comma-separated.
	members := splitTopLevel("A = 1, B = 2, C = 3")
	if len(members) != 3 {
		t.Fatalf("expected three enumeration members, got %d: %q", len(members), members)
	}
}

func TestEnumMembersDropThePrefixTheyShare(t *testing.T) {
	// A caller writes Enum.IO, not Enum.ROBOHUB_ERR_IO: the prefix says which
	// enumeration a member belongs to, and the Rho type already says that.
	members := []Field{
		{Name: "ROBOHUB_OK"},
		{Name: "ROBOHUB_ERR_FAIL"},
		{Name: "ROBOHUB_ERR_IO"},
	}
	got := EnumMembers(members)
	want := []string{"OK", "ERR_FAIL", "ERR_IO"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("expected %v, got %v", want, got)
		}
	}

	// A different spelling of the same idea, and a set that shares nothing.
	if got := EnumMembers([]Field{{Name: "RH_FALSE"}, {Name: "RH_TRUE"}}); got[0] != "FALSE" || got[1] != "TRUE" {
		t.Errorf("expected the shared prefix dropped, got %v", got)
	}
	if got := EnumMembers([]Field{{Name: "ALPHA"}}); got[0] != "ALPHA" {
		t.Errorf("expected a member with no shared prefix kept, got %v", got)
	}
}
