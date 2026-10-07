//
// @file robohub.go
// @brief The RoboHub platform, as the toolchain's `platform` contracts.
//
// A platform is a mapping. Which family a part belongs to, what macro selects
// that family in the vendor's own driver sources, which header the vendor's
// umbrella includes by name — none of that is Rho's, and all of it used to be
// written into this toolchain, which is what made Rho a language for one
// ecosystem rather than a language with a platform attached.
//
// It is here instead. One plugin fills the `platform` contract of rho-target,
// rho-kconfig, rho-cimport and rho-flash, and runs only for a project whose
// module graph names `platform: robohub`.
//
// The protocol is the toolchain's, one JSON request in and one JSON response
// out, so the plugin is a program a person can run by hand:
//
//	echo '{"version":1,"hook":"spec","command":"target-spec",
//	       "component":"rho-target","payload":{"details":[]}}' | rho-robohub
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhorobohub

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProtocolVersion is the version of the request and response shapes this plugin
// was written against. A component speaking another version is reported rather
// than answered with something it would misread.
const ProtocolVersion = 1

/* -------------------------------------------------------------------------- */
/*  The envelope                                                              */
/* -------------------------------------------------------------------------- */

// Request is the toolchain's envelope. The payload is each component's own, so
// only the envelope is spelled out here; the component is what says which one.
type Request struct {
	Version   int             `json:"version"`
	Hook      string          `json:"hook"`
	Command   string          `json:"command"`
	Component string          `json:"component"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Response is what a plugin answers. Only the fields a component reads are here.
type Response struct {
	Version     int             `json:"version"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Diagnostics []Diagnostic    `json:"diagnostics,omitempty"`
}

// Diagnostic is a problem a plugin reports about what it was given.
type Diagnostic struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// The components this plugin answers, by the component they name.
const (
	ComponentTarget  = "rho-target"
	ComponentKconfig = "rho-kconfig"
	ComponentCimport = "rho-cimport"
	ComponentFlash   = "rho-flash"
)

/* -------------------------------------------------------------------------- */
/*  rho-target — the spec hook                                                */
/* -------------------------------------------------------------------------- */

// SpecRequest is rho-target's payload: the targets it carries, and enough
// detail about each that this plugin can map one without opening the spec
// files itself.
type SpecRequest struct {
	Targets  []string          `json:"targets,omitempty"`
	Families map[string]string `json:"families,omitempty"`
	Details  []TargetDetail    `json:"details,omitempty"`
}

// TargetDetail is what a target already declares.
type TargetDetail struct {
	Name      string   `json:"name"`
	Family    string   `json:"family,omitempty"`
	MCU       string   `json:"mcu,omitempty"`
	HALHeader string   `json:"hal_header,omitempty"`
	Defines   []string `json:"defines,omitempty"`
}

// SpecResponse is what this plugin answers rho-target's spec hook with. It
// carries no specs — RoboHub's boards are declared by the platform module, and
// this plugin does not restate a board — and only the platform facts a spec
// cannot derive on its own.
type SpecResponse struct {
	Augment []SpecAugment `json:"augment,omitempty"`
}

// SpecAugment is a fact this plugin adds to a target it does not own.
type SpecAugment struct {
	Target    string   `json:"target"`
	Defines   []string `json:"defines,omitempty"`
	HALHeader string   `json:"hal_header,omitempty"`
}

// The RoboHub conventions this plugin contributes to a target spec.
const (
	// familyPrefix is what the vendor's own driver sources test for to select
	// a family: `ROBOHUB_STM32_FAMILY_H7`. The suffix is the family with the
	// vendor's `STM32` dropped, so `STM32H7` reads as `H7`.
	familyPrefix = "ROBOHUB_STM32_FAMILY_"
	// familyStem is the vendor's own prefix for a part family, which the
	// macro's suffix is taken from.
	familyStem = "STM32"
	// halHeaderSuffix is how the vendor names a family's umbrella header, from
	// which the configuration header `*_conf.h` is derived by rho-target.
	halHeaderSuffix = "_hal.h"
)

// specAugment maps each STM32-family target to the macro that selects its
// family and the vendor header that target's build has to produce a
// configuration for.
//
// A target that already declares either is left alone: the platform's module
// and this plugin are two descriptions of one platform, and a fact stated in
// both is a fact that will eventually disagree. A target that is not an STM32
// part — the host, a Pico — is left entirely alone, which is what lets the same
// component build for every platform once this plugin is installed.
func specAugment(details []TargetDetail) []SpecAugment {
	augment := []SpecAugment{}
	for _, detail := range details {
		suffix, ok := familySuffix(detail.Family)
		if !ok {
			continue
		}
		entry := SpecAugment{Target: detail.Name}

		// The macro, unless the target already selects its family.
		macro := familyPrefix + suffix + "=1"
		if !declaresFamily(detail.Defines) {
			entry.Defines = append(entry.Defines, macro)
		}

		// The vendor header, unless the target already names one. ST's
		// umbrella includes `stm32<suffix>xx_hal.h` by its bare name, and the
		// configuration header beside it is derived from that.
		if detail.HALHeader == "" {
			entry.HALHeader = "stm32" + strings.ToLower(suffix) + "xx" + halHeaderSuffix
		}

		if len(entry.Defines) > 0 || entry.HALHeader != "" {
			augment = append(augment, entry)
		}
	}
	sort.Slice(augment, func(i, j int) bool { return augment[i].Target < augment[j].Target })
	return augment
}

// familySuffix is the macro suffix for a family name: `STM32H7` reads as `H7`.
// A family this naming does not cover — anything not an STM32 family — is
// reported as such, and the target is left to its own spec.
func familySuffix(family string) (string, bool) {
	family = strings.ToUpper(strings.TrimSpace(family))
	if !strings.HasPrefix(family, familyStem) {
		return "", false
	}
	suffix := strings.TrimPrefix(family, familyStem)
	if suffix == "" {
		return "", false
	}
	return suffix, true
}

// declaresFamily reports whether a target's own defines already select a family,
// under any suffix. It is what keeps this plugin from stating a fact the spec
// already states.
func declaresFamily(defines []string) bool {
	for _, define := range defines {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(define)), familyPrefix) {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  rho-kconfig — the render hook                                             */
/* -------------------------------------------------------------------------- */

// RenderRequest is rho-kconfig's payload: the dialect the build asked for and
// the resolved symbols.
type RenderRequest struct {
	Dialect string        `json:"dialect,omitempty"`
	Values  []RenderValue `json:"values,omitempty"`
}

// RenderValue is one resolved symbol as a plugin sees it.
type RenderValue struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
	From  string `json:"from,omitempty"`
}

// RenderResponse is what this plugin answers the render hook with.
//
// Empty, and deliberately. RoboHub's configuration is written by the dialect
// itself — the `linux` dialect writes the `autoconf.h` that the platform's own
// sources include, from the fragments the platform declared — so a plugin that
// also rendered files would be a second description of a configuration that is
// already described once. The contract is implemented because the platform may
// come to need something the dialects do not write, and this is where it goes.
type RenderResponse struct {
	Files []RenderFile `json:"files"`
}

// RenderFile is one file a plugin contributes.
type RenderFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

/* -------------------------------------------------------------------------- */
/*  rho-cimport — the map hook                                                 */
/* -------------------------------------------------------------------------- */

// MapRequest is rho-cimport's payload: the headers that were read and what they
// declared.
type MapRequest struct {
	Headers      []string         `json:"headers,omitempty"`
	Declarations []MapDeclaration `json:"declarations,omitempty"`
}

// MapDeclaration is one declaration as a plugin sees it.
type MapDeclaration struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Type   string `json:"type,omitempty"`
	Result string `json:"result,omitempty"`
}

// MapResponse is what this plugin answers the map hook with.
//
// Empty of overrides, and deliberately. The RoboHub interfaces are ordinary C
// the generator already reads — opaque structs, integer error codes, function
// pointers — so every declaration maps correctly without anything being said
// here. The contract is implemented because a platform whose C the generator
// cannot read needs a mapping, and this is where it would live.
type MapResponse struct {
	Types map[string]string `json:"types"`
}

/* -------------------------------------------------------------------------- */
/*  rho-flash — the flash hook                                                 */
/* -------------------------------------------------------------------------- */

// FlashRequest is rho-flash's payload: which board is being reached, what its
// target spec already says, and where the platform's own tree is.
type FlashRequest struct {
	Target       string `json:"target,omitempty"`
	Project      string `json:"project,omitempty"`
	Binary       string `json:"binary,omitempty"`
	PlatformRoot string `json:"platform_root,omitempty"`
	Flash        struct {
		Tool      string   `json:"tool,omitempty"`
		Command   string   `json:"command,omitempty"`
		Arguments []string `json:"arguments,omitempty"`
		Offset    string   `json:"offset,omitempty"`
		PostArgs  []string `json:"post_args,omitempty"`
		Comment   string   `json:"comment,omitempty"`
	} `json:"flash"`
}

// FlashResponse is what this plugin answers the flash hook with.
//
// A board names the tool that programs it. When the platform ships that tool —
// an RP2350's UF2 converter is the platform's own, because what reaches an
// RP2350 is a file on a mounted drive and that is a fact about the board rather
// than about a chip vendor's program — the command is rewritten to the tool's
// path inside the platform.
//
// The path is derived from the platform's own directory, so it is the same
// answer on every machine and every run: a tool found on PATH could be another
// build's, and a board programmed by the wrong converter is a board programmed
// with the wrong bytes.
//
// Empty otherwise, and deliberately: a board whose flashing is a system tool's
// job names that tool and command in its spec, and a hook that supplied a second
// command would be a board described twice.
type FlashResponse struct {
	Flash map[string]any `json:"flash,omitempty"`
}

// flashAnswer bridges a RoboHub board onto rho-flash's command.
//
// A tool the platform ships is a directory of its own under `tools/`, holding a
// `flash` entry point. The tool's name is the board's, so the two cannot drift:
// the spec says `elf2uf2` and the platform's `tools/elf2uf2/flash` is what runs.
func flashAnswer(request FlashRequest) FlashResponse {
	tool := strings.TrimSpace(request.Flash.Tool)
	if tool == "" || strings.TrimSpace(request.PlatformRoot) == "" {
		return FlashResponse{}
	}
	// A tool name with a path or a space in it is not one of the platform's.
	if strings.ContainsAny(tool, `/\ `) {
		return FlashResponse{}
	}
	entry := filepath.Join(request.PlatformRoot, "tools", tool, "flash")
	info, err := os.Stat(entry)
	if err != nil || info.IsDir() {
		return FlashResponse{}
	}
	// Only the program is replaced. What the board declared after it — the
	// family ID the bootrom checks, a mount the caller pinned — belongs to the
	// board and is carried through unchanged. Replacing the whole command would
	// drop those, and a UF2 that names the wrong part is one the bootrom
	// refuses.
	rest := ""
	switch command := strings.TrimSpace(request.Flash.Command); {
	case command == tool:
	case strings.HasPrefix(command, tool+" "):
		rest = command[len(tool):]
	default:
		// A command that does not begin with the tool is not one this can
		// rewrite without guessing what it means.
		return FlashResponse{}
	}
	return FlashResponse{Flash: map[string]any{
		"tool":    tool,
		"command": entry + rest,
	}}
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                    */
/* -------------------------------------------------------------------------- */

// Main is the program. It reads one request, answers it, and exits.
func Main(args []string) int {
	request, err := readRequest(os.Stdin)
	if err != nil {
		return writeResponse(Response{
			Version:     ProtocolVersion,
			Diagnostics: []Diagnostic{{Level: "error", Message: err.Error()}},
		})
	}
	if request.Version != ProtocolVersion {
		return writeResponse(Response{
			Version: ProtocolVersion,
			Diagnostics: []Diagnostic{{Level: "warning", Message: fmt.Sprintf(
				"this plugin speaks version %d of the protocol; the run asked for %d",
				ProtocolVersion, request.Version)}},
		})
	}
	return writeResponse(answer(request))
}

// answer dispatches a request to the component it names, and reports a component
// this plugin does not fill rather than answering something it was not asked.
func answer(request Request) Response {
	response := Response{Version: ProtocolVersion}
	var payload any
	switch request.Component {
	case ComponentTarget:
		if request.Hook != "spec" {
			return unsupported(request)
		}
		spec := SpecRequest{}
		if err := decode(request.Payload, &spec); err != nil {
			return badRequest(request, err)
		}
		payload = SpecResponse{Augment: specAugment(spec.Details)}
	case ComponentKconfig:
		if request.Hook != "render" {
			return unsupported(request)
		}
		render := RenderRequest{}
		if err := decode(request.Payload, &render); err != nil {
			return badRequest(request, err)
		}
		payload = RenderResponse{Files: []RenderFile{}}
	case ComponentCimport:
		if request.Hook != "map" {
			return unsupported(request)
		}
		mapping := MapRequest{}
		if err := decode(request.Payload, &mapping); err != nil {
			return badRequest(request, err)
		}
		payload = MapResponse{Types: map[string]string{}}
	case ComponentFlash:
		if request.Hook != "flash" {
			return unsupported(request)
		}
		flash := FlashRequest{}
		if err := decode(request.Payload, &flash); err != nil {
			return badRequest(request, err)
		}
		payload = flashAnswer(flash)
	default:
		return unsupported(request)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return badRequest(request, err)
	}
	response.Payload = encoded
	return response
}

// unsupported reports a hook or a component this plugin does not fill.
func unsupported(request Request) Response {
	return Response{
		Version: ProtocolVersion,
		Diagnostics: []Diagnostic{{Level: "warning", Message: fmt.Sprintf(
			"this plugin fills the platform contract of %s and does not answer %s/%s",
			ComponentTarget, request.Component, request.Hook)}},
	}
}

// badRequest reports a payload that could not be read.
func badRequest(request Request, err error) Response {
	return Response{
		Version:     ProtocolVersion,
		Diagnostics: []Diagnostic{{Level: "warning", Message: fmt.Sprintf("the %s payload could not be read: %v", request.Component, err)}},
	}
}

func readRequest(in io.Reader) (Request, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return Request{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return Request{}, fmt.Errorf("no request on standard input")
	}
	request := Request{}
	if err := json.Unmarshal(data, &request); err != nil {
		return Request{}, fmt.Errorf("cannot read the request: %w", err)
	}
	return request, nil
}

func decode(payload json.RawMessage, into any) error {
	if len(payload) == 0 {
		return nil
	}
	return json.Unmarshal(payload, into)
}

func writeResponse(response Response) int {
	encoded, err := json.Marshal(response)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rho-robohub:", err)
		return 1
	}
	os.Stdout.Write(encoded)
	return 0
}
