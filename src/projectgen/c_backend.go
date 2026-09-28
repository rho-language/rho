//
// @file c_backend.go
// @brief C artifacts for the generated-project descriptor.
//
// Produces the four C roles (implementation, interface, runtime, entrypoint)
// plus the CMake build file, matching the layout a Rho C project has always
// had. The generator writes these under the file paths the descriptor gives
// them, so renaming or relocating app.c is a descriptor change, not a code
// change.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package projectgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/src/codegen"
	"rho/src/ir"
)

// EmitCProject renders the C artifacts for a generated project.
func EmitCProject(ctx Context) (Artifacts, error) {
	artifacts := Artifacts{}

	appC := codegen.EmitC(ctx.IR)
	appC = strings.ReplaceAll(appC, "int main(", "int app_main(")
	appC = cBanner(ctx.ModuleName, "app.c", "Generated Rho application implementation.") +
		"#define _POSIX_C_SOURCE 200809L\n#include \"app.h\"\n\n" + appC

	// A project with a config/ directory gets a generated config.h that the
	// public interface includes.
	configInclude := ""
	// A project with a Kconfig bridge includes the header its dialect writes, so
	// a generated source can ask whether a symbol is set the way a C program
	// asks. The dialect decides which header that is; this is the generator's
	// half of the same convention, and the two are named on each side.
	if ctx.Config.Configured() {
		_, header := configHeader(ctx.Config.Dialect)
		configInclude = "#include \"" + header + "\"\n"
	}
	if info, err := os.Stat(filepath.Join(ctx.ProjectRoot, "config")); err == nil && info.IsDir() {
		config := cBanner(ctx.ModuleName, "config.h", "Generated Rho application configuration.") +
			"#pragma once\n\n#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n" +
			"#define RHO_APP_NAME \"" + ctx.ModuleName + "\"\n\n" +
			"#ifdef __cplusplus\n}\n#endif\n"
		if err := os.WriteFile(filepath.Join(ctx.OutputDir, "config.h"), []byte(config), 0644); err != nil {
			return artifacts, fmt.Errorf("write config.h: %w", err)
		}
		configInclude = "#include \"config.h\"\n"
	}

	initCall := ""
	if hasRhoInit(ctx.IR) {
		initCall = "    init();\n"
	}
	appC += "\nvoid app_init(void)\n{\n" + initCall + "}\n\n"

	// Split the generated runtime helpers into their own translation unit so
	// app.c stays focused on the application.
	runtimeStart := strings.Index(appC, "static char* rho_concat(const char* a, const char* b) {")
	initStart := strings.Index(appC, "\nvoid app_init(void)")
	if runtimeStart < 0 || initStart < runtimeStart {
		return artifacts, fmt.Errorf("generated runtime section is missing")
	}
	runtimeBody := appC[runtimeStart:initStart]
	appC = appC[:runtimeStart] + appC[initStart:]
	appC = strings.ReplaceAll(appC, "static char* rho_", "char* rho_")
	runtimeBody = strings.ReplaceAll(runtimeBody, "static char* rho_", "char* rho_")

	artifacts.Implementation = appC
	artifacts.Runtime = cBanner(ctx.ModuleName, "runtime.c", "Generated Rho C runtime support.") +
		"#define _POSIX_C_SOURCE 200809L\n#include <stdint.h>\n#include <stdbool.h>\n" +
		"#include <stdio.h>\n#include <stdlib.h>\n#include <string.h>\n#include <stddef.h>\n\n" +
		runtimeBody
	// A connected project compiles against its library's headers, so they are
	// included here: the types a foreign declaration names are the library's, and
	// without its headers every `LibType*` in the interface is an unknown type
	// to the C compiler.
	connectionIncludes := ""
	if ctx.Connection != nil {
		for _, header := range ctx.Connection.Headers {
			connectionIncludes += "#include <" + filepath.ToSlash(header) + ">\n"
		}
	}
	artifacts.Interface = cBanner(ctx.ModuleName, "app.h", "Generated Rho application interface.") +
		"#pragma once\n\n#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n" +
		"#include <stdint.h>\n#include <stdbool.h>\n#include <stddef.h>\n" + connectionIncludes + configInclude +
		"\nint app_main(void);\nvoid app_init(void);\n\n#ifdef __cplusplus\n}\n#endif\n"
	artifacts.Entrypoint = cBanner(ctx.ModuleName, "main.c", "Generated native system entry point for a Rho application.") +
		"#include \"app.h\"\n\nint main(void)\n{\n    app_init();\n    return app_main();\n}\n"
	artifacts.Extra = map[string]string{}
	artifacts.Build = emitCMakeLists(ctx)
	// A project that declared a Kconfig bridge gets the configuration it
	// declared: the merge runs as part of the build, and the sources see what it
	// wrote. The build is the single place the configuration is resolved, which
	// is what lets a person re-run it and change a value without a rebuild of
	// the toolchain.
	sources, err := copyProjectSources(ctx)
	if err != nil {
		return artifacts, err
	}
	for name, content := range sources {
		artifacts.Extra[name] = content
	}

	if ctx.Config.Configured() {
		extra, err := emitConfigArtifacts(ctx)
		if err != nil {
			return artifacts, err
		}
		artifacts.Extra = extra
	}

	return artifacts, nil
}

// emitConfigArtifacts writes the CMake glue that resolves a project's
// configuration, and nothing else: the merge itself is the rho-kconfig
// component's, run as a process so a project is not tied to a generator that
// knows what a dialect is.
func emitConfigArtifacts(ctx Context) (map[string]string, error) {
	config := ctx.Config
	out := map[string]string{}

	// The merge is one custom command, run before anything is compiled, and
	// every source depends on its output so a changed symbol rebuilds.
	var b strings.Builder
	b.WriteString(cBanner(ctx.ModuleName, "rho-config.cmake", "Kconfig bridge for a Rho project."))
	b.WriteString("# The configuration a project declared is resolved by the rho-kconfig\n")
	b.WriteString("# component, which is run here rather than at generation time: a value may\n")
	b.WriteString("# change without regenerating the project.\n\n")
	b.WriteString("# The tool is the rho that invoked this build when there is one, and\n")
	b.WriteString("# whatever rho is on PATH otherwise.\n")
	b.WriteString("if(DEFINED ENV{RHO_BIN} AND NOT \"$ENV{RHO_BIN}\" STREQUAL \"\")\n")
	b.WriteString("  set(RHO_CONFIG_TOOL \"$ENV{RHO_BIN}\")\n")
	b.WriteString("else()\n")
	b.WriteString("  set(RHO_CONFIG_TOOL rho)\n")
	b.WriteString("endif()\n\n")
	b.WriteString("set(RHO_CONFIG_DIALECT " + cmakeString(config.Dialect) + ")\n")
	b.WriteString("set(RHO_CONFIG_DIR ${CMAKE_CURRENT_SOURCE_DIR})\n")
	b.WriteString("set(RHO_CONFIG_OUT ${CMAKE_CURRENT_SOURCE_DIR})\n")

	var fragments strings.Builder
	for _, fragment := range config.Fragments {
		fragments.WriteString("  " + cmakeString(fragment) + "\n")
	}
	b.WriteString("set(RHO_CONFIG_FRAGMENTS\n" + fragments.String() + ")\n")
	if config.Defconfig != "" {
		b.WriteString("set(RHO_CONFIG_DEFCONFIG " + cmakeString(config.Defconfig) + ")\n")
	}
	b.WriteString("set(RHO_CONFIG_VALUES " + cmakeString(config.ValuesFile) + ")\n\n")

	b.WriteString("add_custom_command(\n")
	b.WriteString("  OUTPUT ${RHO_CONFIG_VALUES}\n")
	b.WriteString("  COMMAND ${RHO_CONFIG_TOOL} kconfig merge\n")
	b.WriteString("          --force\n")
	b.WriteString("          --dialect ${RHO_CONFIG_DIALECT}\n")
	b.WriteString("          --root ${RHO_CONFIG_DIR}\n")
	b.WriteString("          --out ${RHO_CONFIG_OUT}\n")
	if config.Defconfig != "" {
		b.WriteString("          --defconfig ${RHO_CONFIG_DEFCONFIG}\n")
	}
	b.WriteString("          ${RHO_CONFIG_FRAGMENTS}\n")
	b.WriteString("  COMMENT \"Resolving the Kconfig bridge with rho-kconfig\"\n")
	b.WriteString("  VERBATIM\n")
	b.WriteString(")\n\n")
	out["rho-config.cmake"] = b.String()
	return out, nil
}

// configHeader is the header a dialect writes and the directory it goes in,
// relative to the generated project.
//
// This is the generator's half of a convention the rho-kconfig component
// implements: a dialect writes one header, a source includes it by name, and the
// build puts the directory it was written in on the include path. A dialect that
// wrote its header somewhere else would fail to compile, which is the right way
// for a disagreement between the two halves to show up.
func configHeader(dialect string) (directory, name string) {
	switch dialect {
	case "espidf":
		return filepath.ToSlash(filepath.Join("build", "config")), "sdkconfig.h"
	case "freertos":
		return "", "FreeRTOSConfig.h"
	case "", "linux":
		return filepath.ToSlash(filepath.Join("include", "generated")), "autoconf.h"
	}
	// A dialect this generator does not know is treated as the Linux one, and
	// the merge says so in a diagnostic of its own.
	return filepath.ToSlash(filepath.Join("include", "generated")), "autoconf.h"
}

// cmakeString quotes a value for CMake, which is the one place a generated
// build file and a shell-quoted string can differ.
func cmakeString(value string) string {
	if value == "" {
		return "\"\""
	}
	return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
}

func emitCMakeLists(ctx Context) string {
	name := cmakeProjectName(ctx.ModuleName)
	var b strings.Builder
	b.WriteString("cmake_minimum_required(VERSION 3.16)\n\n")

	// A target a project generated for writes the files a cross-compile needs:
	// which compiler, which memory the program is laid out in, and how it is
	// written to the board. They are included when they are there, so a project
	// that targets the host builds as it did.
	//
	// The toolchain file comes before project(), because that is when CMake reads
	// it to learn which compiler it is configuring for; the target file comes
	// after, because its flags are added to a target that has to exist.
	b.WriteString("# The target this project builds for, when one was generated.\n")
	b.WriteString("if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/cmake/toolchain.cmake)\n")
	b.WriteString("  include(${CMAKE_CURRENT_SOURCE_DIR}/cmake/toolchain.cmake)\n")
	b.WriteString("endif()\n\n")
	fmt.Fprintf(&b, "project(%s C)\n\n", name)
	b.WriteString("if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/cmake/target.cmake)\n")
	b.WriteString("  include(${CMAKE_CURRENT_SOURCE_DIR}/cmake/target.cmake)\n")
	b.WriteString("endif()\n\n")
	b.WriteString("set(CMAKE_C_STANDARD 11)\nset(CMAKE_C_STANDARD_REQUIRED ON)\n\n")

	// A project with a Kconfig bridge resolves it before anything is compiled,
	// and every source sees the header the merge wrote. The bridge is included
	// rather than inlined so a person can read what a build does before it does
	// it, and change it if they meant something else.
	configured := ctx.Config.Configured()
	if configured {
		b.WriteString("include(${CMAKE_CURRENT_SOURCE_DIR}/rho-config.cmake)\n\n")
	}

	fmt.Fprintf(&b, "add_executable(%s main.c app.c\n    runtime.c\n", name)
	for _, source := range ctx.SourceFiles {
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(source))
	}
	b.WriteString(")\n\n")

	if configured {
		// The generated configuration is a source every file depends on, so a
		// changed symbol rebuilds rather than being compiled against the header
		// that was there when the project was generated.
		fmt.Fprintf(&b, "set_source_files_properties(${RHO_CONFIG_VALUES} PROPERTIES GENERATED TRUE)\n")
		fmt.Fprintf(&b, "add_dependencies(%s ${CMAKE_CURRENT_SOURCE_DIR}/${RHO_CONFIG_VALUES})\n", name)
		b.WriteString("set_property(SOURCE ${CMAKE_CURRENT_SOURCE_DIR}/main.c ${CMAKE_CURRENT_SOURCE_DIR}/app.c\n")
		b.WriteString("             ${CMAKE_CURRENT_SOURCE_DIR}/runtime.c\n")
		b.WriteString("             APPEND PROPERTY OBJECT_DEPENDS ${RHO_CONFIG_VALUES})\n\n")
	}

	// A connected project compiles against its library's headers as well as
	// its own, so the connection's include paths join the target's.
	link, includePaths := connectionCMake(ctx, name)

	fmt.Fprintf(&b, "target_include_directories(%s PRIVATE\n    ${CMAKE_CURRENT_SOURCE_DIR}\n", name)
	for _, include := range ctx.IncludePaths {
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(include))
	}
	for _, include := range includePaths {
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(include))
	}
	b.WriteString(")\n")

	if configured {
		// The header the merge wrote is found by including the directory it went
		// into, which is where autoconf.h and sdkconfig.h are read from.
		directory, _ := configHeader(ctx.Config.Dialect)
		if directory == "" {
			b.WriteString("target_include_directories(" + name + " PRIVATE ${RHO_CONFIG_OUT})\n")
		} else {
			b.WriteString("target_include_directories(" + name + " PRIVATE ${RHO_CONFIG_OUT}/" + directory + ")\n")
		}
	}

	b.WriteString(link)
	return b.String()
}

// copyProjectSources brings the C a project declares into the generated project.
//
// A declared source is written as the project wrote it, because that is the path
// a person reads in their manifest. A generated project is a directory someone
// can build on its own, so the file travels with it rather than being referenced
// from where it happened to be generated.
func copyProjectSources(ctx Context) (map[string]string, error) {
	files := map[string]string{}
	for _, source := range ctx.SourceFiles {
		// A source is named the way the project named it: relative to the
		// project, or already absolute when it comes from a dependency. Joining
		// blindly would make an absolute path a path inside the project.
		from := filepath.FromSlash(source)
		if !filepath.IsAbs(from) {
			from = filepath.Join(ctx.ProjectRoot, from)
		}
		data, err := os.ReadFile(from)
		if err != nil {
			// A source a project declares but has not written yet is left to the
			// build to report: another step may produce it, and a generator that
			// stopped over it would be one step in front of the wrong problem.
			continue
		}
		files[source] = string(data)
	}
	return files, nil
}

// connectionCMake is what a build of a connected project needs: the headers it
// compiles against, the macros that select an implementation, and the library it
// links.
//
// A build target a person names and a library a build links are different
// things, and this is where they meet: `--target stm32` is resolved by the
// connection to the vendor target robohub_stm32, and the generated project links
// that. Without a variant the project links the library it declared, which is
// what a host build of the same project does.
func connectionCMake(ctx Context, name string) (link string, includes []string) {
	if ctx.Connection == nil {
		return "", nil
	}
	connection := ctx.Connection
	variant, _ := connection.Variant(ctx.Variant)

	includes = append(includes, connection.IncludeDirs...)
	includes = append(includes, variant.IncludeDirs...)
	defines := append([]string{}, connection.Defines...)
	defines = append(defines, variant.Defines...)
	// A variant is selected at compile time, and the macros say which: a vendor
	// library compiles its own implementation behind them, and a project that
	// only linked it would compile the wrong one.
	if ctx.Variant != "" {
		defines = append(defines, "ROBOHUB_PLATFORM="+strings.ToUpper(ctx.Variant))
	}

	// A library is linked when it is named, and a build with nothing to link
	// gets no link line rather than an empty one.
	libraries := []string{}
	if connection.Library != "" {
		libraries = append(libraries, connection.Library)
	}
	libraries = append(libraries, variant.LinkLibraries()...)

	var b strings.Builder
	for _, define := range defines {
		fmt.Fprintf(&b, "add_compile_definitions(%s=%s)\n", define, cMakeValue(define))
	}
	if len(libraries) > 0 {
		fmt.Fprintf(&b, "target_link_libraries(%s PRIVATE %s)\n", name, strings.Join(libraries, " "))
	}
	// The include paths are returned rather than written here, because they
	// belong with the other include directories the target already carries.
	return b.String(), includes
}

// cMakeValue is a define's value as CMake writes it: a bare word is a word, a
// string is quoted.
func cMakeValue(value string) string {
	if value == "" {
		return "\"\""
	}
	if strings.ContainsAny(value, " \"") {
		return "\"" + strings.ReplaceAll(value, "\"", "\\\"") + "\""
	}
	return value
}

// cmakeProjectName keeps module names usable as CMake targets.
func cmakeProjectName(name string) string {
	if name == "" {
		return "rhoapp"
	}
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			b.WriteByte(c)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "rhoapp"
	}
	return out
}

func hasRhoInit(m ir.ModuleIR) bool {
	for _, fn := range m.Funcs {
		if fn.Name == "init" && !fn.Extern && len(fn.Params) == 0 {
			return true
		}
	}
	return false
}

func cBanner(project, file, brief string) string {
	return fmt.Sprintf("/*\n * @file %s\n * @brief %s\n *\n * @project %s\n * @version 0.1\n * @date 2026\n *\n * SPDX-License-Identifier: MIT\n */\n\n",
		file, brief, project)
}
