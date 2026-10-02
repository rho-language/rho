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
	"sort"
	"strings"

	"rho/src/codegen"
	"rho/src/deps"
	"rho/src/ir"
)

// shimInclude is what the application's own source needs to see about the foreign
// interface it calls.
//
// The declarations are not in the application's interface header: those are the
// library's own headers, which describe the C the library exports, while the
// functions a Rho program calls are the generated wrappers rho-cimport wrote for
// the declarations Rho cannot make itself. Without this a call to one compiles as
// an implicit declaration, which the compiler accepts and the linker then resolves
// to whatever the symbol happens to be — so a signature that changed still builds,
// and fails at run time instead.
func shimInclude(ctx Context) string {
	if ctx.ShimHeader == "" {
		return ""
	}
	return "#include \"" + ctx.ShimHeader + "\"\n"
}

// EmitCProject renders the C artifacts for a generated project.
func EmitCProject(ctx Context) (Artifacts, error) {
	artifacts := Artifacts{}

	appC := codegen.EmitC(ctx.IR)
	appC = strings.ReplaceAll(appC, "int main(", "int app_main(")
	appC = cBanner(ctx.ModuleName, "app.c", "Generated Rho application implementation.") +
		"#define _POSIX_C_SOURCE 200809L\n#include \"app.h\"\n" + shimInclude(ctx) + "\n" + appC

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

	// `_init` and `_fini` belong to the runtime file, not to the application.
	//
	// A bare-metal build passes `-nostartfiles`, which excludes the two files that
	// define them, and the startup assembly reaches them through the C library's
	// initializer. Weak, because the same generated project is also what a hosted
	// target builds and there the library defines them properly; a strong
	// definition would collide, a weak one yields.
	//
	// Placed here rather than in the code generation that produces them, because
	// this is where the file is assembled: the section cut out of the application
	// source and written here as the runtime is not where they were emitted.
	runtimeBody = "// Weak: a bare-metal build excludes the C library's own definitions of these\n" +
		"// with -nostartfiles, and the startup reaches them through the library's\n" +
		"// initializer. A hosted library defines them itself, so these yield to it.\n" +
		"__attribute__((weak)) void _init(void);\n__attribute__((weak)) void _init(void) {}\n" +
		"__attribute__((weak)) void _fini(void);\n__attribute__((weak)) void _fini(void) {}\n\n" +
		runtimeBody

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
		// Merged rather than assigned: the configuration is added to what the
		// project declared, and a project that declares both gets both. Replacing
		// the map would leave a configured project without the C it wrote.
		for name, content := range extra {
			artifacts.Extra[name] = content
		}
	}

	if ctx.Native {
		// A C project brings its own entry point, so nothing is written that
		// would define a second one. Writing an empty file rather than omitting it
		// is deliberate: the descriptor lists these, and a target naming a file
		// that is not there fails at configure with a message about a missing
		// source rather than about the decision.
		artifacts.Implementation = cBanner(ctx.ModuleName, "app.c",
			"Empty: this project is written in C and brings its own entry point.")
		artifacts.Entrypoint = cBanner(ctx.ModuleName, "main.c",
			"Empty: this project is written in C and brings its own entry point.")
	}

	// Which symbols were set, for a dependency whose own build file compiles its
	// own sources.
	//
	// Those sources are listed in that module's CMakeLists, which this generator
	// does not write and cannot add to. So rather than rewriting a list it does not
	// own, it is told the answer and the module filters with it.
	if content, ok := emitEnabledConfig(ctx); ok {
		artifacts.Extra[filepath.ToSlash(filepath.Join("cmake", "robo-config.cmake"))] = content
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
	b.WriteString(cmakeBanner(ctx.ModuleName, "rho-config.cmake", "Kconfig bridge for a Rho project."))
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

	// Absolute paths, and the reason is that the merge resolves a relative one
	// against its own installed fragments rather than against the project it was
	// given. A project that copied a fragment in beside itself was reading it
	// from somewhere else entirely, and every symbol in it was undeclared — which
	// is what a project's own configuration looks like when nothing it set is
	// recognised.
	var fragments strings.Builder
	for _, fragment := range config.Fragments {
		absolute := fragment
		if !filepath.IsAbs(absolute) && config.OutputDir != "" {
			absolute = filepath.Join(config.OutputDir, filepath.FromSlash(fragment))
		}
		fragments.WriteString("  " + cmakeString(filepath.ToSlash(absolute)) + "\n")
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
		// The value is joined to the flag rather than following it.
		//
		// The merge is run as a separate process, and this program re-reads the
		// arguments before passing them on. A value that arrives as its own
		// argument does not survive that; one attached to the flag does. The
		// failure said only that the flag needed an argument, which named the
		// command line and not the reason.
		b.WriteString("          --defconfig=${RHO_CONFIG_DEFCONFIG}\n")
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
	// Assembly is enabled when the project has any, which is what a board build
	// does: the startup assembly is what the linker enters through.
	//
	// Declared rather than assumed because the presence of assembly is a fact
	// about the project, and CMake needs to be told before it classifies the file.
	// A `.s` file in a project with no assembler configured is not compiled and
	// not reported — it is skipped, and the failure arrives one step later as a
	// link with no entry symbol, which names the linker and not the file.
	if hasAssembly(ctx) {
		b.WriteString("enable_language(ASM)\n")
	}

	fmt.Fprintf(&b, "project(%s C)\n\n", name)
	b.WriteString("if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/cmake/target.cmake)\n")
	b.WriteString("  include(${CMAKE_CURRENT_SOURCE_DIR}/cmake/target.cmake)\n")
	b.WriteString("endif()\n\n")
	b.WriteString("if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/cmake/rho-build-flags.cmake)\n")
	b.WriteString("  include(${CMAKE_CURRENT_SOURCE_DIR}/cmake/rho-build-flags.cmake)\n")
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

	// What the dependencies exported is consumed before any of them is descended
	// into.
	//
	// It has to be here rather than inside a dependency's own build file: the
	// declarations are the top-level project's to include, because a dependency
	// cannot know which other dependencies the project that pulls it in also
	// has. The imported targets are declared global, and the variables are cache
	// entries, so both are visible to the subdirectories included below.
	b.WriteString("if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps.cmake)\n")
	b.WriteString("  include(${CMAKE_CURRENT_SOURCE_DIR}/deps.cmake)\n")
	b.WriteString("endif()\n\n")

	// A C dependency is linked, not absorbed. Its sources belong to the module
	// that declares them, and that module is where they are compiled — a CMake
	// target cannot even be given a source outside its own tree, so listing them
	// here would produce a project that quietly compiles less than it says.
	//
	// The dependency is found through the project's manifest, the same way every
	// other dependency is, and contributed by its own build definition. A
	// dependency that has no build definition has nothing to contribute and is
	// skipped rather than half-added.
	// The directories the selected preset declares, added to the directory scope
	// rather than to one target.
	//
	// The platform's own sources include the vendor's headers, and those are
	// compiled by the platform's library — a target this file does not build and
	// cannot attach include paths to after the fact. A path given only to the
	// program would leave the library unable to compile a line of itself, and the
	// error would name a header in a vendor repository that the build was told
	// about, one target too late.
	if len(ctx.IncludePaths) > 0 {
		b.WriteString("include_directories(\n")
		for _, include := range ctx.IncludePaths {
			fmt.Fprintf(&b, "    %s\n", cmakeString(include))
		}
		b.WriteString(")\n\n")
	}

	dependencyRoots := cDependencyRoots(ctx.ProjectRoot)
	for index, root := range dependencyRoots {
		// Where this build's resolved configuration is, for the dependencies whose own build
		// files compile their own sources and so cannot be given a source list here.
		//
		// Written down *before* they are added, because they are subdirectories: a variable
		// set after an `add_subdirectory` has not reached it, and a dependency looking for
		// this file beside its own CMakeLists would not find it either.
		b.WriteString("# Where this build's resolved configuration is. The dependencies below compile\n")
		b.WriteString("# their own sources, so a source list cannot be handed to them here; they read\n")
		b.WriteString("# this to leave out whatever a symbol says the program did not ask for.\n")
		b.WriteString("set(RHO_ROBO_CONFIG ${CMAKE_CURRENT_SOURCE_DIR}/cmake/robo-config.cmake)\n\n")
		fmt.Fprintf(&b, "add_subdirectory(%s %s-dep%d)\n",
			cmakePath(root), strings.ToLower(name), index)
	}
	// After them, because a dependency that builds itself defines its target only
	// once it has been added, and the file below bridges the target it defined to
	// the name its manifest says to link it by. An alias made before the target
	// exists is an error, which is the one thing CMake is clear about here.
	b.WriteString("if(EXISTS ${CMAKE_CURRENT_SOURCE_DIR}/deps-aliases.cmake)\n")
	b.WriteString("  include(${CMAKE_CURRENT_SOURCE_DIR}/deps-aliases.cmake)\n")
	b.WriteString("endif()\n\n")

	// A C project brought its own `main`, so the wrapper that calls into a Rho
	// program's `app_main` is left out: two entry points would be one too many,
	// and the one that lost would be reported as a missing symbol rather than as
	// a program that had already declared its entry point.
	entry := "main.c app.c\n    "
	if ctx.Native {
		entry = ""
	}
	fmt.Fprintf(&b, "add_executable(%s %s runtime.c\n", name, entry)
	if os.Getenv("RHO_DBG_SRC") != "" {
		fmt.Fprintf(os.Stderr, "[dbg] SourceFiles=%d Shim=%d native=%v\n", len(ctx.SourceFiles), len(ctx.ShimSources), ctx.Native)
		for _, one := range ctx.SourceFiles {
			fmt.Fprintf(os.Stderr, "[dbg]   %s\n", one)
		}
	}
	for _, source := range ctx.SourceFiles {
		// A source is named the way the project named it, which for the project's
		// own files is relative to the project. Emitted into a generated project
		// in another directory, that relative name would resolve there instead —
		// and a missing source is reported as such, or, when a file of that name
		// happens to exist in the generated tree, compiled instead of the
		// program's own.
		//
		// It is resolved against the project before anything else looks at it,
		// because the two questions below are both about where the file actually
		// is, and a relative name resolves against whichever directory the reader
		// happened to run the build from.
		resolved := resolveProjectSource(ctx, source)

		// What a dependency provides is compiled by that dependency's own build
		// file, so it is left out here. A file the project wrote is its own and is
		// always compiled by this target, which is why being inside a dependency's
		// directory is not enough to exclude it: a project may live inside the
		// platform repository it depends on — an example beside the platform it is
		// written for — and every one of its sources is then under a dependency
		// root. Asking the question of the resolved path rather than of the name
		// also keeps a project that is built from somewhere else entirely from
		// being read against the wrong directory.
		if within(ctx.ProjectRoot, resolved) {
			fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(resolved))
			continue
		}
		if ownedBy(resolved, dependencyRoots) {
			continue
		}
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(resolved))
	}
	// The generated shim is part of what this project compiles: it is the C the
	// foreign interface calls, and it belongs to the build that generated it.
	for _, source := range ctx.ShimSources {
		if ctx.excludes(source) {
			continue
		}
		fmt.Fprintf(&b, "    %s\n", filepath.ToSlash(source))
	}
	b.WriteString(")\n\n")

	if configured {
		// The generated configuration is built before the sources that include
		// it, and rebuilt when a symbol changes.
		//
		// It is arranged as a source of the target rather than as a dependency
		// of it: `add_dependencies` names a *target*, and the output of the
		// merge is a file. Naming the file as a source is what tells CMake to
		// run the command that produces it first, and the ordering comes from
		// the custom command's own OUTPUT rather than from anything stated here.
		fmt.Fprintf(&b, "add_custom_target(%s-config ALL DEPENDS ${RHO_CONFIG_VALUES})\n", name)
		fmt.Fprintf(&b, "add_dependencies(%s %s-config)\n", name, name)
		b.WriteString("set_source_files_properties(${RHO_CONFIG_VALUES} PROPERTIES GENERATED TRUE)\n")
		// Every source waits for the configuration, so a changed symbol rebuilds
		// rather than being compiled against the header that was there when the
		// project was generated.
		dependents := []string{"${CMAKE_CURRENT_SOURCE_DIR}/runtime.c"}
		if !ctx.Native {
			dependents = append([]string{
				"${CMAKE_CURRENT_SOURCE_DIR}/main.c",
				"${CMAKE_CURRENT_SOURCE_DIR}/app.c",
			}, dependents...)
		}
		shim := ctx.ShimSources
		if ctx.Native && len(shim) > 0 {
			dependents = append(dependents, "${CMAKE_CURRENT_SOURCE_DIR}/"+filepath.Base(shim[len(shim)-1]))
		}
		b.WriteString("set_property(SOURCE " + strings.Join(dependents, " ") + "\n")
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
		from := resolveProjectSource(ctx, source)
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
// cDependencyRoots returns the roots of the C dependencies a project declares,
// and of the ones those declare.
//
// It reads the project's own manifest rather than looking for CMakeLists.txt
// beside something that looks like a dependency. A dependency that declares a
// build definition is contributing it; one that does not is not, and the
// difference is a statement in a manifest rather than a directory listing.
func cDependencyRoots(projectRoot string) []string {
	if projectRoot == "" {
		return nil
	}
	roots := []string{}
	seen := map[string]bool{}

	var walk func(base string, manifest *deps.Manifest, depth int)
	walk = func(base string, manifest *deps.Manifest, depth int) {
		if depth > 16 || seen[base] {
			return
		}
		seen[base] = true
		for _, dep := range manifest.Deps {
			if dep.Type != deps.DepTypeC || dep.Path == "" {
				continue
			}
			root := dep.Path
			if !filepath.IsAbs(root) {
				root = filepath.Join(base, root)
			}
			if _, err := os.Stat(filepath.Join(root, "CMakeLists.txt")); err != nil {
				continue
			}
			roots = append(roots, root)
			if child, err := deps.LoadManifest(deps.ManifestPath(root)); err == nil {
				walk(root, child, depth+1)
			}
		}
	}

	if manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot)); err == nil && manifest != nil {
		walk(projectRoot, manifest, 0)
	}
	return roots
}

// resolveProjectSource is where a declared source actually is.
//
// A source is named relative to the project that declared it, or is already
// absolute when it came from a dependency. It is resolved against the project
// rather than against the working directory, because a person is free to run
// `rho build` from anywhere and the file a manifest names does not move with
// them.
func resolveProjectSource(ctx Context, source string) string {
	from := filepath.FromSlash(source)
	if filepath.IsAbs(from) {
		return filepath.Clean(from)
	}
	if ctx.ProjectRoot == "" {
		absolute, err := filepath.Abs(from)
		if err != nil {
			return from
		}
		return absolute
	}
	return filepath.Join(ctx.ProjectRoot, from)
}

// within reports whether a path is inside a root. A path that is the root
// itself counts, because a project is inside its own directory and so is every
// source it declares.
func within(root, path string) bool {
	if root == "" {
		return false
	}
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanRoot {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanRoot+string(filepath.Separator))
}

// ownedBy reports whether a source belongs to one of the roots — that is,
// whether a dependency declared it and the dependency's own build compiles it.
//
// It is asked of an already-resolved path. Resolving here as well would make
// the answer depend on the directory the build was started from, which is not a
// fact about the project.
func ownedBy(source string, roots []string) bool {
	absolute, err := filepath.Abs(source)
	if err != nil {
		absolute = source
	}
	for _, root := range roots {
		if within(root, absolute) {
			return true
		}
	}
	return false
}

// cmakePath writes an absolute path as a CMake string literal.
func cmakePath(path string) string {
	return `"` + filepath.ToSlash(path) + `"`
}

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
	// Where the resolved configuration is written, handed to the dependencies
	// that compile their own sources.
	//
	// They are added as subdirectories, so a path relative to this file means
	for _, define := range defines {
		fmt.Fprintf(&b, "add_compile_definitions(%s=%s)\n", define, cMakeValue(define))
	}
	if len(libraries) > 0 {
		// A C dependency is a static library, and an archive is loaded strictly on
		// demand: a member is pulled only when it resolves something still
		// undefined. A vendor startup assembly declares each handler weak and
		// assigns it the address of its own default — an instruction to spin — and
		// that is a *definition*. By the time the linker sweeps the archive there
		// is nothing undefined to resolve, so the object implementing the handler
		// is discarded whole.
		//
		// The symptom is silent and total: the board boots, runs, and stops a
		// millisecond later when the first tick arrives, with no fault recorded
		// anywhere, because none occurred. The console never opens, because opening
		// it happens after that tick.
		//
		// So the whole archive is linked. Its cost is a few kilobytes of driver
		// code the program never calls; the alternative is a board that stops.
		fmt.Fprintf(&b, "target_link_libraries(%s PRIVATE\n", name)
		b.WriteString("    -Wl,--whole-archive\n")
		for _, library := range libraries {
			fmt.Fprintf(&b, "    %s\n", library)
		}
		b.WriteString("    -Wl,--no-whole-archive\n)\n")
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

// cmakeBanner is the same header in a form CMake can read.
//
// A `.cmake` file is not C, and the comment that makes a C file readable is a
// syntax error in a CMake one: CMake has no block comment, so a file opened with
// `/*` fails to parse before it reaches anything it was saying. The header is
// the same either way, and it is written with the file's own comment syntax
// rather than a fixed one.
func cmakeBanner(project, file, brief string) string {
	return fmt.Sprintf("# @file %s\n# @brief %s\n#\n# @project %s\n# @version 0.1\n# @date 2026\n#\n# SPDX-License-Identifier: MIT\n\n",
		file, brief, project)
}

// hasAssembly reports whether this project compiles any assembly.
func hasAssembly(ctx Context) bool {
	all := append(append([]string{}, ctx.SourceFiles...), ctx.ShimSources...)
	for _, source := range all {
		switch strings.ToLower(filepath.Ext(source)) {
		case ".s":
			return true
		}
	}
	return false
}

// excludes reports whether a source is governed by a symbol that is not set.
//
// A driver belongs to a peripheral, and whether a program uses that peripheral is
// the program's answer. Honouring it is the difference between a driver that is
// switched off and one that is merely unused: a driver left in the image is code
// nobody asked for, in flash nobody is using.
func (ctx Context) excludes(source string) bool {
	if len(ctx.GuardedSources) == 0 || len(ctx.EnabledConfigs) == 0 {
		return false
	}
	symbol, guarded := ctx.GuardedSources[filepath.Base(source)]
	if !guarded {
		return false
	}
	return !ctx.EnabledConfigs[symbol]
}

// emitEnabledConfig writes which configuration symbols ended up set, for a
// dependency's own build to read.
//
// A module's sources are compiled by that module's build file, which this
// generator does not write and cannot add to. So rather than rewrite a list it
// does not own, it is told the answer and the module filters with it: which
// symbols were set, written once beside the target's other files.
func emitEnabledConfig(ctx Context) (string, bool) {
	if len(ctx.EnabledConfigs) == 0 || len(ctx.GuardedSources) == 0 {
		return "", false
	}

	var b strings.Builder
	b.WriteString(cmakeBanner(ctx.ModuleName, "cmake/robo-config.cmake", "The configuration this build resolved to."))
	b.WriteString("# Written by rho build from the symbols the project's configuration set.\n")
	b.WriteString("#\n")
	b.WriteString("# A module whose sources are compiled by its own build file includes this and\n")
	b.WriteString("# leaves out whatever a symbol here says was not asked for. Generated, so it\n")
	b.WriteString("# is written from the merge rather than from a list kept beside it.\n\n")

	names := make([]string, 0, len(ctx.EnabledConfigs))
	for name := range ctx.EnabledConfigs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if ctx.EnabledConfigs[name] {
			fmt.Fprintf(&b, "set(RHO_CONFIG_%s ON)\n", name)
		} else {
			fmt.Fprintf(&b, "set(RHO_CONFIG_%s OFF)\n", name)
		}
	}
	return b.String(), true
}
