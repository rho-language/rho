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
	artifacts.Interface = cBanner(ctx.ModuleName, "app.h", "Generated Rho application interface.") +
		"#pragma once\n\n#ifdef __cplusplus\nextern \"C\" {\n#endif\n\n" +
		"#include <stdint.h>\n#include <stdbool.h>\n#include <stddef.h>\n" + configInclude +
		"\nint app_main(void);\nvoid app_init(void);\n\n#ifdef __cplusplus\n}\n#endif\n"
	artifacts.Entrypoint = cBanner(ctx.ModuleName, "main.c", "Generated native system entry point for a Rho application.") +
		"#include \"app.h\"\n\nint main(void)\n{\n    app_init();\n    return app_main();\n}\n"
	artifacts.Build = emitCMakeLists(ctx)

	return artifacts, nil
}

func emitCMakeLists(ctx Context) string {
	name := cmakeProjectName(ctx.ModuleName)
	var b strings.Builder
	b.WriteString("cmake_minimum_required(VERSION 3.16)\n")
	fmt.Fprintf(&b, "project(%s C)\n\n", name)
	b.WriteString("set(CMAKE_C_STANDARD 11)\nset(CMAKE_C_STANDARD_REQUIRED ON)\n\n")
	fmt.Fprintf(&b, "add_executable(%s main.c app.c\n    runtime.c\n", name)
	for _, source := range ctx.SourceFiles {
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(source))
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "target_include_directories(%s PRIVATE\n    ${CMAKE_CURRENT_SOURCE_DIR}\n", name)
	for _, include := range ctx.IncludePaths {
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(include))
	}
	b.WriteString(")\n")
	return b.String()
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
