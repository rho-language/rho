//
// @file module_manifest.go
// @brief Schema definitions for Rho module.yml manifests.
//
// Provides type-aware schema validation for module.yml files.
// Different module types have different valid fields:
//
//   - rho/app:     main is required (source file path)
//   - module:      main is required (source file path), command not allowed
//   - component:   main is required (binary name), command optional
//   - plugin:      main is required (a source file or a command); a plugin plugs
//                  into a component, which it names in extends:
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package schema

import (
	"fmt"
	"os"
	"strings"

	"rho/src/yaml"
)

// ValidModuleTypes lists all allowed module.type values.
var ValidModuleTypes = []string{
	"rho", "module", "component", "plugin",
	"interfaces", "platform", "platform-board",
	"driver", "driver-board",
	"runtime", "middleware",
}

// ValidVisibilities lists the visibilities a module may be published under.
//
// Public is the default rather than a choice a manifest has to make: a module
// that says nothing is a module anybody may install, which is what publishing
// to a registry means unless it is said otherwise.
var ValidVisibilities = []string{"public", "private"}

// RhoSourceModuleTypes are the module types written in Rho. They declare a
// main entry point.
var RhoSourceModuleTypes = []string{"rho", "module", "component", "plugin"}

// NativeModuleTypes are the module types that are native code rather than Rho:
// they are compiled and linked through a project's build section, and they
// declare no Rho entry point.
//
// The three of them are the layers of a hardware platform, and each says
// something the other two do not:
//
//   - `interfaces` declares the abstractions. A module that only declares
//     interfaces has nothing to compile; it is headers, error codes and
//     contracts, and it is shared by every platform.
//   - `platform` implements those abstractions against one family of hardware.
//     It is the code that drives a peripheral, and it knows nothing about any
//     particular board.
//   - `platform-board` is one board's support package: the pin map, the clock
//     configuration and the startup that make a platform usable on the piece of
//     hardware it is fitted to.
//
// Two more are the layers of a device rather than of a board, and they sit
// between a platform and an application:
//
//   - `driver` is a device driver: code for one part — a temperature sensor, a
//     display, a radio — written against the HAL interfaces a platform
//     implements, and against the driver interface its own kind declares. It is
//     platform-agnostic, which is the point: the same DS18B20 driver runs on an
//     RP2350 or an STM32 because it reaches the bus through a contract and not
//     through a vendor's SDK.
//   - `driver-board` is one eval or breakout module's support package: which
//     driver it carries and how the module's pins are named, the way a
//     platform-board is one MCU board's pin map. It is not a platform-board
//     because it describes an IC on a carrier rather than the carrier's MCU.
//
// One more sits above both, and it is what a program is built on rather than
// what it is built from:
//
//   - `runtime` is the application runtime: the event bus, the scheduler, the
//     application lifecycle and the diagnostics a program uses to run several
//     modules together. It is native and platform-agnostic, and it declares no
//     entry point because it is linked into a program, not run as one.
//   - `middleware` plugs an outside system into the ecosystem — an RTOS, a
//     filesystem, a network stack. It implements a contract the interfaces
//     declare (the OSAL, say) and is linked in place of the default that ships
//     with the runtime, so the program above it is unchanged whether the
//     middleware is there or not.
//
// They are kept apart because they are not the same thing, and a module that
// implements a platform while claiming to be its interfaces — or a board that
// claims to be a platform — describes itself as something it is not, in the
// one place a reader looks to find out what a module is.
//
// A board carries no `depends`, and that is the whole of what it says about
// where it sits. It cannot be built on its own: a board is a support package,
// and a support package is only ever compiled into something that has a platform,
// an interface and a set of dependencies to compile it against. All three belong
// to the application, so the application declares them and the board is found
// through the platform it belongs to.
//
// The alternative is what twenty-seven board manifests did, each declaring the
// interfaces and the platform it is part of — the second of them being the
// repository the board lives in. Every one of those was a dependency resolved by a
// path relative to itself, which works on a checkout and on nothing else, and a
// board that cannot resolve its own dependencies cannot be the thing that
// decides them.
//
// None of them declares `main`: each is native code a project links rather than
// something it runs.
var NativeModuleTypes = []string{"interfaces", "platform", "platform-board", "driver", "driver-board", "runtime", "middleware"}

// IsRhoSourceType reports whether a module type is written in Rho.
func IsRhoSourceType(moduleType string) bool {
	for _, candidate := range RhoSourceModuleTypes {
		if candidate == moduleType {
			return true
		}
	}
	return false
}

// IsNativeType reports whether a module type is native code consumed through a
// project's build section rather than a Rho entry point.
func IsNativeType(moduleType string) bool {
	for _, candidate := range NativeModuleTypes {
		if candidate == moduleType {
			return true
		}
	}
	return false
}

// RhoLibraryModuleTypes are the module types whose main is a Rho source file
// that is part of the module, as opposed to a binary built while packing it
// (`component`, `plugin`) or to nothing at all (the native types).
//
// It is the set whose main has to be on disk before the module means anything,
// which is why it is not the same set as RhoSourceModuleTypes: a component is
// written in Rho too, and its entry point is a program rather than a file.
var RhoLibraryModuleTypes = []string{"rho", "module"}

// IsRhoLibraryType reports whether a module type's main is a Rho source file
// belonging to the module.
func IsRhoLibraryType(moduleType string) bool {
	for _, candidate := range RhoLibraryModuleTypes {
		if candidate == moduleType {
			return true
		}
	}
	return false
}

// ValidBuildLanguages lists the source languages a build section may declare.
var ValidBuildLanguages = []string{"c", "go", "cgo"}

// ValidBuildTargets lists the generated-project backends an application may
// request. The set is intentionally open-ended: a descriptor may define new
// targets, and validation of unknown ids happens against that descriptor.
var ValidBuildTargets = []string{"c", "go"}

// ModuleSchema returns a schema for validating module.yml content.
// It returns a base schema that requires module.name, module.type,
// module.version, and main at minimum.
func ModuleSchema() Schema {
	return Schema{
		Name: "module.yml",
		Fields: []FieldSchema{
			{
				Name:     "module",
				Required: true,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "name", Required: true, RejectNull: true, Kind: KindString},
					{Name: "type", Required: true, RejectNull: true, Kind: KindString, AllowedValues: ValidModuleTypes},
					{Name: "version", Required: true, RejectNull: true, Kind: KindString},
					{Name: "command", Required: false, Kind: KindString},
					// import_path is the dotted name Rho code uses to reach this
					// component's Rho sources, which cannot be guessed from the
					// module name: a component published as ext-probe may expose
					// its sources as ext.probe.
					{Name: "import_path", Required: false, Kind: KindString},
					// Descriptive metadata carried by published modules.
					{Name: "description", Required: false, Kind: KindString},
					// author is who published the module, and the only identity a
					// remote registry will let withdraw it. A registry records
					// the author against the module's name the first time it is
					// published and refuses an unpublish for anyone else, so
					// this is the claim the registry checks rather than one it
					// trusts: the caller's credential has to say the same thing.
					//
					// It is optional in the schema so that a module being read,
					// built or validated locally is not required to claim a
					// publisher; a registry refuses a publish without one.
					{Name: "author", Required: false, Kind: KindString},
					// visibility is whether a published module is anybody's to
					// install or its owner's alone. It is declared here rather
					// than inferred, because who may install a module is a
					// statement about the module and not a fact about the
					// registry it happens to be on.
					{Name: "visibility", Required: false, Kind: KindString,
						AllowedValues: ValidVisibilities},
					{Name: "maintainer", Required: false, Kind: KindString},
					{Name: "license", Required: false, Kind: KindString},
					{Name: "created", Required: false, Kind: KindString},
				},
			},
			{
				Name:     "main",
				Required: false,
				Kind:     KindString,
			},
			{
				// requires lists the runtime packages the built project needs,
				// which is a different thing from `deps`: a dependency is
				// resolved and written into the project at build time, while a
				// requirement is a package the generated code links against and
				// that the project's own manifest has to name for anyone who
				// builds it later.
				//
				// A database driver is the first thing that needs one: the code
				// a `sql:` block generates calls into it, and a project whose
				// generated code names a driver nobody declared has a build that
				// works on the machine that generated it and nowhere else.
				Name:     "requires",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindMap,
					Fields: []FieldSchema{
						{Name: "name", Required: true, RejectNull: true, Kind: KindString},
						{Name: "version", Required: false, Kind: KindString},
					},
				},
			},
			{
				// The sql section is what a project writes to have its SQL
				// turned into Rho code. It is a declaration rather than a
				// setting, because everything in it says where something is
				// rather than how it should behave.
				//
				// Nothing here names an engine's settings. The driver names the
				// engine plugin, which decides what its own settings are, and
				// the engine is handed what it needs through the environment.
				Name:     "sql",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					// driver is the engine plugin the generated code runs
					// against, as it is named in the manifest. An engine that is
					// not installed is a build that cannot run its own
					// statements, so a missing one is reported as a build error
					// rather than as an absent page.
					{Name: "driver", Required: true, RejectNull: true, Kind: KindString},
					// schema is the file a project's tables are declared in,
					// relative to the project. It is required, because a
					// database whose tables nothing states is a database whose
					// generated code is typed from guesses.
					//
					// A schema may be a file of create statements or a file of
					// declarations, and which one it is is the file's extension:
					// a project that declared tables gets the create statements
					// written from them.
					{Name: "schema", Required: true, RejectNull: true, Kind: KindString},
					// queries is where a project's own statements live, relative
					// to the project. It is optional: the common operations over
					// a declared table — read one by its key, read them all, add
					// one, change one, remove one — are generated from the table
					// itself, so a project only writes this file for the
					// statements that are about its domain rather than about its
					// tables.
					{Name: "queries", Required: false, Kind: KindString},
					// out is where the generated Rho is written, relative to the
					// project. It defaults to the schema file's own directory, so
					// a project that keeps its SQL in one place gets its
					// generated code beside it.
					{Name: "out", Required: false, Kind: KindString},
					// package is the Rho module the generated file belongs to,
					// which defaults to the project's own name.
					{Name: "package", Required: false, Kind: KindString},
				},
			},
			{
				Name:     "deps",
				Required: false,
				Kind:     KindMap,
			},
			{
				Name:     "features",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindString,
				},
			},
			{
				// The docs section is doc-as-code: the module names the sources
				// its documentation is written in, and where the pages belong in
				// a site. The generator is the rho-doc component, so a module
				// does not have to know how a page is rendered.
				Name:     "docs",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					// title overrides the page title derived from the file.
					{Name: "title", Required: false, Kind: KindString},
					// section is the sidebar group the pages appear under.
					{Name: "section", Required: false, Kind: KindString},
					// order sorts a module against its peers in a section.
					{Name: "order", Required: false, Kind: KindString},
					// sources are the doc-as-code files, relative to the module
					// root. A glob is allowed, so a module can name a whole
					// tree without listing it.
					{Name: "sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// index is the page a section links to first.
					{Name: "index", Required: false, Kind: KindString},
				},
			},
			{
				// What a component offers a plugin: the hooks a run of the tool
				// goes through. A tool that offers no hooks offers no place for
				// a plugin to stand, and a plugin is checked against this list
				// rather than against whatever it happens to name.
				Name:     "provides",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "hooks", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// contracts say what a plugin has to be for this component,
					// which the hooks alone do not: two plugins may answer the
					// same hook and be unrelated things, and a person deciding
					// whether to run one needs to be able to see what it is.
					//
					// A contract names what is filled and says how many plugins
					// may fill it: one is a choice between engines, and not one
					// is a pipeline every enabled plugin joins.
					{Name: "contracts", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
						Kind: KindMap,
						Fields: []FieldSchema{
							{Name: "name", Required: true, RejectNull: true, Kind: KindString},
							{Name: "description", Required: false, Kind: KindString},
							{Name: "one", Required: false, Kind: KindString},
							{Name: "hooks", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
						},
					}},
				},
			},
			{
				// How a plugin says what it plugs into. A plugin is not
				// free-floating: it names the component it extends, and the
				// hooks of that component it answers, so the relation is
				// declared rather than assumed.
				//
				// Two shapes are accepted, so Kind is left open and the shape is
				// checked below: a single component name, which is what a plugin
				// that extends one component writes, or a list of
				// `{component, implements, hooks}` entries, which is what a
				// plugin spanning several components of the toolchain writes.
				Name:     "extends",
				Required: false,
				Kind:     "",
			},
			{
				// How this project's output should be read: which transport, and
				// how it is presented.
				//
				// Every value here has a default that works, so a project that says
				// nothing still gets its output read. That is the whole design of
				// this block: it exists for the cases where the defaults are wrong
				// — a console on another device, an RTT channel other than zero —
				// not for turning a feature on.
				Name:     "monitor",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "default_backend", Required: false, Kind: KindString,
						AllowedValues: []string{"auto", "rtt", "uart", "swo"}},
					{
						Name:     "rtt",
						Required: false,
						Kind:     KindMap,
						Fields: []FieldSchema{
							{Name: "host", Required: false, Kind: KindString},
							{Name: "port", Required: false, Kind: KindString},
							{Name: "channel", Required: false, Kind: KindString},
							{Name: "block_if_full", Required: false, Kind: KindBool},
						},
					},
					{
						Name:     "uart",
						Required: false,
						Kind:     KindMap,
						Fields: []FieldSchema{
							{Name: "auto_detect", Required: false, Kind: KindBool},
							{Name: "port", Required: false, Kind: KindString},
							{Name: "baud_rate", Required: false, Kind: KindString},
						},
					},
					{
						Name:     "formatting",
						Required: false,
						Kind:     KindMap,
						Fields: []FieldSchema{
							{Name: "timestamps", Required: false, Kind: KindBool},
							{Name: "color_levels", Required: false, Kind: KindBool},
							{Name: "log_file", Required: false, Kind: KindString},
						},
					},
				},
			},
			{
				// How this project is measured: how many iterations, how many
				// warm-up runs to discard, and the limits a check enforces.
				//
				// Everything here changes what a number means rather than only how
				// it is presented, which is why it is a project declaration and not
				// a flag: a benchmark measured a thousand times is not comparable
				// with one measured once, and which of those two a CI run compared
				// against a baseline is the difference between a regression and a
				// coincidence.
				Name:     "bench",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "default_iterations", Required: false, Kind: KindString},
					{Name: "warmup_runs", Required: false, Kind: KindString},
					{Name: "stack_watermark", Required: false, Kind: KindBool},
					{
						Name:     "budgets",
						Required: false,
						Kind:     KindMap,
						Fields: []FieldSchema{
							{Name: "max_stack_bytes", Required: false, Kind: KindString},
							{Name: "max_heap_bytes", Required: false, Kind: KindString},
							{Name: "max_cycles_per_iteration", Required: false, Kind: KindString},
							{Name: "fail_on_regression_percent", Required: false, Kind: KindString},
						},
					},
					{Name: "targets", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				// The platform identity this module implements, such as
				// `zenith`. A platform module declares it so a project can be
				// recognized as being built for that platform without a name, a
				// repository or a directory being assumed.
				Name:     "platform",
				Required: false,
				Kind:     KindString,
			},
			{
				// What this project's memory budget is: which build artifacts
				// to read, what each memory region is allowed to use, and what a
				// check is allowed to fail on.
				//
				// A top-level section rather than one under `build:`, because a
				// budget is not a description of how a build runs — it is a
				// promise about the result, and it outlives any single build's
				// flags. `build:` says how the image is produced; this says how
				// much of the board the image is allowed to take.
				Name:     "mem",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "map_file", Required: false, Kind: KindString},
					{Name: "elf_file", Required: false, Kind: KindString},
					{Name: "baseline", Required: false, Kind: KindString},
					{
						// A budget per region, keyed by the name the linker
						// script gives the region. A map rather than a list,
						// because the name is the key: a budget for DTCM is
						// about DTCM, and a list would have to carry the name
						// separately and could carry the wrong one.
						Name:     "regions",
						Required: false,
						Kind:     KindMap,
						Fields: []FieldSchema{
							{Name: "max_bytes", Required: false, Kind: KindString},
							{Name: "warn_threshold", Required: false, Kind: KindString},
						},
					},
					{
						Name:     "enforce",
						Required: false,
						Kind:     KindMap,
						Fields: []FieldSchema{
							{Name: "fail_on_overflow", Required: false, Kind: KindBool},
							// A number of bytes rather than a flag: "fail if it
							// grew by more than 10 kB" is the question a project
							// actually asks, and a boolean cannot answer it.
							{Name: "fail_on_growth_bytes", Required: false, Kind: KindString},
						},
					},
				},
			},
			{
				// The platform identities a plugin serves. Empty means the
				// plugin is platform-agnostic and runs for any project. Named
				// `serves` rather than `platforms` because the latter is already
				// the list of modules that implement an interfaces module.
				Name:     "serves",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindString,
				},
			},
			{
				// The contracts of the component this plugin fills, so a plugin
				// says what it is rather than leaving it to be read off the
				// hooks it happens to answer. A contract is checked against the
				// ones the component offers, which is what catches a plugin
				// written against a component that has since changed.
				Name:     "implements",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindString,
				},
			},
			{
				// The hooks of the component this plugin answers. Each has to
				// be one the component offers, which is what keeps a plugin
				// from claiming a stage the tool never runs.
				Name:     "hooks",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindString,
				},
			},
			{
				// The executables a plugin answers a hook with. Each names the
				// hook it answers and the program that answers it, which is a
				// path or a name found on PATH.
				Name:     "commands",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindMap,
					Fields: []FieldSchema{
						{Name: "name", Required: true, RejectNull: true, Kind: KindString},
						{Name: "run", Required: true, RejectNull: true, Kind: KindString},
						{Name: "hook", Required: false, Kind: KindString},
						// The component a command answers, when the plugin
						// extends more than one. Empty answers whichever
						// component is asking.
						{Name: "component", Required: false, Kind: KindString},
					},
				},
			},
			{
				Name:     "depends",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{
					Kind: KindMap,
					Fields: []FieldSchema{
						{Name: "name", Required: true, RejectNull: true, Kind: KindString},
						// type is optional: the resolver infers "c" or "rho"
						// from the dependency's own module.yml when absent.
						{Name: "type", Required: false, Kind: KindString, AllowedValues: []string{"c", "rho"}},
						{Name: "git", Required: false, Kind: KindString},
						{Name: "tag", Required: false, Kind: KindString},
						{Name: "branch", Required: false, Kind: KindString},
						{Name: "version", Required: false, Kind: KindString},
						{Name: "path", Required: false, Kind: KindString},
						{Name: "env", Required: false, Kind: KindString},
						{Name: "include", Required: false, Kind: KindString},
						{Name: "src", Required: false, Kind: KindString},
					},
				},
			},
			{
				Name:     "build",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					// `defines` are the compile definitions a build of this module needs.
					//
					// As much a contract as the include directories: a vendor HAL's
					// umbrella header includes a driver's header only when that driver is
					// enabled, so a module that supports a driver says which ones.
					{Name: "defines", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// Sources compiled only when a symbol is set.
					//
					// A driver belongs to a peripheral, and whether a program uses
					// that peripheral is the program's answer to set, not the
					// platform's to assume. Naming the pairing here is what lets a
					// platform support a driver and a program leave it out: the
					// symbol says whether, and the symbol is the same one that
					// decides whether the interface exists at all.
					//
					// Matched by file name, because the sources live in a vendor
					// repository the platform does not own, and a rule that had to
					// be written against a path would break when the repository moved.
					{Name: "guarded_sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
						Kind: KindMap,
						Fields: []FieldSchema{
							{Name: "config", Required: true, Kind: KindString},
							{Name: "sources", Required: true, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
						},
					}},
					// `output_dir` is where this project's built binaries are put, inside
					// the build directory: a binary is a build output, and a project with
					// several targets keeps them side by side.
					{Name: "output_dir", Required: false, Kind: KindString},
					{Name: "link_map", Required: false, Kind: KindBool},
					{Name: "language", Required: true, RejectNull: true, Kind: KindString, AllowedValues: ValidBuildLanguages},
					// targets lists the generated-project backends to emit for
					// this application (for example c and go).
					{Name: "targets", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "include_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "source_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "headers", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// `toolchain` names the target a project builds for, as a
					// rho-target spec: an architecture, an MCU or a board. It is
					// the one thing in a manifest that decides which compiler
					// runs, so a project that names it cannot be built for the
					// host by accident.
					//
					// It is not the compiler itself. The compiler, the memory
					// map, the linker script and how the board is programmed are
					// one another's business, and a manifest that spelled them out
					// would have to be edited for every board it was reused on.
					// Naming a spec keeps the facts in one place.
					//
					// The values are not checked here. A spec is a name the
					// rho-target component knows, and this toolchain does not
					// carry that list; a project naming one that does not exist
					// is told so by the build, which is the component that can
					// say what there is.
					{Name: "toolchain", Required: false, Kind: KindString},
				},
			},
			{
				// The package section describes what goes into a published
				// archive. Defaults come from the module type, so a component,
				// a plugin and an application can differ without the packager
				// carrying a hardcoded list per kind.
				Name:     "package",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "kind", Required: false, Kind: KindString, AllowedValues: []string{"component", "plugin", "application", "library"}},
					{Name: "binary", Required: false, Kind: KindString},
					{Name: "artifact_dir", Required: false, Kind: KindString},
					{Name: "include", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// interface lists the Rho declarations a caller compiles
					// against: the header, in C terms. The implementation is
					// supplied by a port, so a library package carries no host
					// source and nothing a user could edit.
					{Name: "interface", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "require", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				// Which chip's register map this project is built against, as a file.
				//
				// A path and nothing else, because this is the one thing a person
				// settles for themselves: a file on this machine, chosen by whoever
				// wired the board up. A name in a vendor's published set is a fact about
				// a chip rather than a choice, so it belongs in the board's own spec —
				// and a session that could not settle it asks for this, because a person
				// asked can answer it by pointing at the file.
				Name:     "debug",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					// Which board's register map is which file, keyed by the name the
					// target spec answers to.
					//
					// A map rather than one path because a project written for several
					// boards has several chips, and a single path would be one board's
					// registers standing in for all of them — on the boards it is wrong
					// for, silently. A board that is not named here has no claim from
					// the project, and the search goes on to what the board's own spec
					// and the vendor's naming say.
					{Name: "svd", Required: false, Kind: KindMap},
				},
			},
			{
				// Where a platform keeps the declarations of the hardware it supports.
				//
				// Declared rather than found. A directory called `targets` beside a
				// dependency is a guess that reads as a contract and is not one.
				Name:     "hardware",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "boards", Required: false, Kind: KindString},
					{Name: "presets", Required: false, Kind: KindString},
					{Name: "targets", Required: false, Kind: KindString},
					{Name: "target", Required: false, Kind: KindString},
					{Name: "kconfig", Required: false, Kind: KindString},
				},
			},
			{
				// What this module needs from the machine it runs on: the programs it
				// runs, the environment it expects, the host permissions it needs.
				//
				// A different thing from `deps`, and declared apart from it because it
				// is a different problem. `deps` is a graph this toolchain resolves —
				// every requirement is another module, and everything it needs is here.
				// A requirement on the machine is a program somebody has to install, and
				// no amount of resolving this toolchain's own graph will produce it.
				//
				// So `deps` is what `rho-deps` resolves and `system_deps` is what
				// `rho-doctor` reports on and `rho-keeper` acts for: a block a module
				// states its requirements in, and nothing this toolchain fetches.
				Name:     "system_deps",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "executables", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
						Kind: KindMap,
						Fields: []FieldSchema{
							{Name: "name", Required: true, Kind: KindString},
							// The constraint, in the grammar `deps` uses: `>=10.0`,
							// `=0.12.0`, `>=0.11 <0.13`, `~1.2`, `^2`, and
							// alternatives with `||`. A minimum, a maximum and a
							// range are one field written three ways, and this
							// toolchain already has a grammar for all three.
							{Name: "version", Required: false, Kind: KindString},
							// The flag that makes it print a version, for the programs that
							// do not use `--version`.
							{Name: "version_flag", Required: false, Kind: KindString},
							// How to read the version out of what it printed: a pattern whose
							// first group is the version. Needed because `gdb` prints four
							// version-shaped runs and means the last.
							{Name: "version_pattern", Required: false, Kind: KindString},
							{Name: "description", Required: false, Kind: KindString},
							// Required defaults to true: a system dependency a module does
							// not need is not one, and a block of optional programs nobody
							// checks for is a block that reports nothing.
							{Name: "required", Required: false, Kind: KindString},
							// How to get it, by package manager. Declared rather than
							// executed: a manifest is a statement about a requirement, and
							// somebody runs the recipe.
							{Name: "install", Required: false, Kind: KindMap},
							// How to get it without a package manager: a prebuilt archive
							// put under the Rho home. Preferred wherever it exists — a
							// toolchain in the user's own directory belongs to that user,
							// does not change when the machine's package manager does, and
							// comes away with the directory it was put in.
							{Name: "fetch", Required: false, Kind: KindMap},
						},
					}},
					{Name: "env_vars", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
						Kind: KindMap,
						Fields: []FieldSchema{
							{Name: "name", Required: true, Kind: KindString},
							{Name: "description", Required: false, Kind: KindString},
							{Name: "required", Required: false, Kind: KindString},
						},
					}},
					{Name: "udev_rules", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
						Kind: KindMap,
						Fields: []FieldSchema{
							{Name: "name", Required: true, Kind: KindString},
							// Where the rule has to be. A path outside the project,
							// because it is the host's file, and a missing one is a device
							// that is present and not permitted.
							{Name: "target_path", Required: true, Kind: KindString},
							{Name: "description", Required: false, Kind: KindString},
							// Where the rule file is in the repository that carries it,
							// which is what makes installing it a copy rather than a
							// written-out guess at the vendor's rules.
							{Name: "source", Required: false, Kind: KindString},
							{Name: "optional", Required: false, Kind: KindString},
						},
					}},
				},
			},
			{
				// Where this module keeps its vendor sources.
				//
				// A path that already holds a repository is used as it is; a path that holds
				// nothing is where the repository is cloned. Declared rather than always
				// fetched, because a repository somebody put there on purpose — to patch it,
				// to work offline, to avoid a second copy of a large tree — should be used
				// rather than fetched over.
				Name:     "vendor",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "dir", Required: false, Kind: KindString},
					// Each vendor repository this module builds against, and the
					// folder inside `dir` it lives in.
					//
					// `name` is how a preset's dependency is matched to this one, so
					// the two are joined by a name rather than by a path either of
					// them happened to write. `path` is relative to `dir`; a
					// repository already there is used as it is, and one that is
					// absent is fetched into it from `git` at `branch`.
					//
					// Declared per repository rather than one directory for all of
					// them, because "where the SDK is" is a question with a
					// per-answer: three repositories of different sizes and revisions
					// do not share a folder, and a person keeping a patched copy of
					// one of them needs to say which.
					{Name: "repos", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{
						Kind: KindMap,
						Fields: []FieldSchema{
							{Name: "name", Required: true, Kind: KindString},
							{Name: "path", Required: true, Kind: KindString},
							{Name: "git", Required: false, Kind: KindString},
							{Name: "branch", Required: false, Kind: KindString},
						},
					}},
				},
			},
			{
				// The modules that implement a set of interfaces.
				//
				// A project declares what it depends on and that is where a build looks
				// first. This is the backup for a program written against the interfaces
				// alone, which named the interfaces and not the platform.
				Name:     "platforms",
				Required: false,
				Kind:     KindList,
				ItemSchema: &ListItemSchema{Kind: KindMap, Fields: []FieldSchema{
					{Name: "name", Required: true, Kind: KindString},
					{Name: "path", Required: false, Kind: KindString},
					{Name: "git", Required: false, Kind: KindString},
					{Name: "branch", Required: false, Kind: KindString},
				}},
			},
			{
				// When this module's tests are eligible to be built.
				//
				// A unit test that runs on the host links a hosted C library, and a library
				// built for a microcontroller has no syscalls for it to resolve against.
				// So the module says which kind of target its tests are for, and a build for
				// another kind does not register them.
				Name:     "testing",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "host_tests", Required: false, Kind: KindMap, Fields: []FieldSchema{
						{Name: "enabled", Required: false, Kind: KindBool},
						{Name: "target_constraint", Required: false, Kind: KindString},
					}},
					{Name: "target_tests", Required: false, Kind: KindMap, Fields: []FieldSchema{
						{Name: "enabled", Required: false, Kind: KindBool},
						{Name: "target_constraint", Required: false, Kind: KindString},
					}},
				},
			},
			{
				Name:     "exports",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "headers", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "sources", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "dependencies",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "required", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "optional", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				// The Kconfig bridge: the fragments that define a project's
				// symbols, the values it sets, the module of Rho constants it
				// may set them in, and the dialect the generated headers are
				// written in. The rho-kconfig component does the merging and
				// the build runs it, so a value can change without the project
				// being regenerated.
				Name:     "config",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "kconfig_fragments", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					// `values_file` is the file a project writes the symbols it
					// sets in. Declared rather than found: a build that looked for a
					// file called `project.conf` would be guessing at a name, and a
					// rename would silently stop applying every value in it.
					{Name: "values_file", Required: false, Kind: KindString},
					{Name: "dialect", Required: false, Kind: KindString, AllowedValues: []string{"linux", "espidf", "freertos"}},
					{Name: "module", Required: false, Kind: KindString},
					{Name: "values", Required: false, Kind: KindMap},
				},
			},
			{
				Name:     "tests",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "enabled", Required: false, Kind: KindString},
					{Name: "test_dirs", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "test_files", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
				},
			},
			{
				Name:     "metadata",
				Required: false,
				Kind:     KindMap,
				Fields: []FieldSchema{
					{Name: "tags", Required: false, Kind: KindList, ItemSchema: &ListItemSchema{Kind: KindString}},
					{Name: "stability", Required: false, Kind: KindString},
					{Name: "visibility", Required: false, Kind: KindString},
				},
			},
		},
	}
}

// ValidateModuleManifest performs type-aware validation of a module.yml.
// It validates the base schema and adds type-specific rules:
//   - component: 'command' is recommended for dynamic dispatch
//   - plugin: 'extends:' should name a component, and 'hooks:' the stages of it
//     the plugin answers
//   - rho/module: 'main' should point to a .rho source file
//   - depends: each entry must specify 'git' or 'path'
func ValidateModuleManifest(data yaml.Value, moduleType string) ValidationErrors {
	result := Validate(data, ModuleSchema())
	errs := ValidationErrors(result.Errors)

	// Type-specific validations
	switch moduleType {
	case "component":
		modMap, ok := data.Map["module"]
		if ok && modMap.Kind == yaml.KindMap {
			if _, hasCmd := modMap.Map["command"]; !hasCmd {
				errs = append(errs, ValidationError{
					Path:     "module.command",
					Message:  "component modules should specify a 'command' for dynamic dispatch",
					Severity: SeverityWarning,
				})
			}
		}

	case "plugin":
		modMap, ok := data.Map["module"]
		if ok && modMap.Kind == yaml.KindMap {
			if cmd, exists := modMap.Map["command"]; exists && cmd.Str != "" {
				errs = append(errs, ValidationError{
					Path:     "module.command",
					Message:  "a plugin plugs into a component rather than publishing a command; it names the component in 'extends:'",
					Severity: SeverityError,
				})
			}
		}

	case "rho", "module":
		mainVal, hasMain := data.Map["main"]
		if hasMain && mainVal.Kind == yaml.KindString {
			// A Rho project is transpiled; a C project is used as it stands.
			//
			// The second is not a lesser kind of project — it is the same program
			// written a language earlier, and everything the build does for it is
			// the same except that there is nothing to translate. Refusing the
			// extension would mean the toolchain only works for programs written in
			// it, which is not the same as working for programs it builds.
			if !strings.HasSuffix(mainVal.Str, ".rho") && !strings.HasSuffix(mainVal.Str, ".c") {
				errs = append(errs, ValidationError{
					Path:     "main",
					Message:  fmt.Sprintf("main for type '%s' should reference a .rho or .c source file, got '%s'", moduleType, mainVal.Str),
					Severity: SeverityError,
				})
			}
		}
	}

	// A Rho-source module must declare an entry point. Native library
	// modules (for example type `interfaces`) legitimately have none.
	if IsRhoSourceType(moduleType) {
		mainVal, hasMain := data.Map["main"]
		if !hasMain {
			errs = append(errs, ValidationError{
				Path:     "main",
				Message:  fmt.Sprintf("module type '%s' must declare a main entry point", moduleType),
				Severity: SeverityError,
			})
		} else if mainVal.Kind == yaml.KindNull || mainVal.Str == "" {
			errs = append(errs, ValidationError{
				Path:     "main",
				Message:  "main is declared but has no value",
				Severity: SeverityError,
			})
		}
	}

	// Validate depends entries: each must have at least 'git' or 'path'
	if dependsNode, ok := data.Map["depends"]; ok && dependsNode.Kind == yaml.KindList {
		for i, item := range dependsNode.List {
			if item.Kind != yaml.KindMap {
				continue
			}
			hasGit, _ := item.Map["git"]
			hasPath, _ := item.Map["path"]
			if hasGit.Str == "" && hasPath.Str == "" {
				errs = append(errs, ValidationError{
					Path:     fmt.Sprintf("depends[%d]", i),
					Message:  "dependency must specify either 'git' or 'path'",
					Severity: SeverityError,
				})
			}
			// tag and branch are mutually exclusive
			if item.Map["tag"].Str != "" && item.Map["branch"].Str != "" {
				errs = append(errs, ValidationError{
					Path:     fmt.Sprintf("depends[%d]", i),
					Message:  "dependency cannot specify both 'tag' and 'branch'",
					Severity: SeverityError,
				})
			}
		}
	}

	// When targets are declared, the declared language should be covered by
	// one of them. A project generating only a Go server while declaring
	// language: c is misleading, but a mixed C and Go build is legitimate, so
	// this is a warning rather than a structural error.
	if build, ok := data.Map["build"]; ok && build.Kind == yaml.KindMap {
		language := build.Map["language"].Str
		targetsNode, hasTargets := build.Map["targets"]
		if language != "" && hasTargets && targetsNode.Kind == yaml.KindList {
			if len(targetsNode.List) > 0 && !targetCoversLanguage(targetsNode, language) {
				errs = append(errs, ValidationError{
					Path:     "build.language",
					Message:  fmt.Sprintf("language %q is not covered by build.targets", language),
					Severity: SeverityWarning,
				})
			}
		}
	}

	// `extends` is a component name or a list of components a plugin spans, so
	// the shape is checked here rather than by a single expected kind. A list
	// entry has to name its component, because that is the whole of what the
	// entry is for.
	if extensionNode, ok := data.Map["extends"]; ok && extensionNode.Kind != yaml.KindNull {
		switch extensionNode.Kind {
		case yaml.KindString:
			// The single-component form.
		case yaml.KindList:
			for i, item := range extensionNode.List {
				if item.Kind != yaml.KindMap {
					errs = append(errs, ValidationError{
						Path:     fmt.Sprintf("extends[%d]", i),
						Message:  "each extends entry must be a map naming its component",
						Severity: SeverityError,
					})
					continue
				}
				if component, hasComponent := item.Map["component"]; !hasComponent || component.Str == "" {
					errs = append(errs, ValidationError{
						Path:     fmt.Sprintf("extends[%d].component", i),
						Message:  "an extends entry must name the component it plugs into",
						Severity: SeverityError,
					})
				}
			}
		default:
			errs = append(errs, ValidationError{
				Path:     "extends",
				Message:  "extends must be a component name or a list of components",
				Severity: SeverityError,
			})
		}
	}

	return errs
}

// targetCoversLanguage reports whether any declared target builds the given
// language. A target id either names the language outright or is derived from
// it, as "go-server" is derived from "go".
func targetCoversLanguage(targets yaml.Value, language string) bool {
	for _, target := range targets.List {
		id := target.Str
		if id == language ||
			strings.HasPrefix(id, language+"-") ||
			strings.HasPrefix(id, language+"_") {
			return true
		}
	}
	return false
}

// ValidateModuleYAMLFile reads and validates a module.yml file.
func ValidateModuleYAMLFile(path string) (ValidationErrors, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	root, err := yaml.Load(string(data))
	if err != nil {
		return nil, fmt.Errorf("invalid YAML in %s: %w", path, err)
	}

	moduleType := ""
	if modNode, ok := root.Map["module"]; ok {
		moduleType = modNode.Map["type"].Str
	}

	return ValidateModuleManifest(root, moduleType), nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
