//
// @file config.go
// @brief What a project declared, and the config a run works from.
//
// The `monitor:` block is a project's own declaration, read through the manifest
// model every component shares — so what `rho monitor` opens is the same block a
// person reads in `module.yml`, parsed by the parser that validates it, rather than
// a second reading written next to it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package rhomonitor

import (
	"path/filepath"
	"strings"
	"time"

	"rho/src/deps"
)

// ReadConfig is a project's monitor declaration, as this component runs on it.
//
// Everything the block does not say is filled in, because every value in it has a
// default that works and a monitor that required the block to be complete before
// it would read anything would be a monitor nobody could try.
func ReadConfig(projectRoot string) Config {
	out := DefaultConfig()
	manifest, err := deps.LoadManifest(deps.ManifestPath(projectRoot))
	if err != nil || manifest == nil || manifest.Monitor == nil {
		return out
	}

	declared := manifest.Monitor
	switch declared.DefaultBackend {
	case deps.BackendRTT, deps.BackendUART, deps.BackendSWO, deps.BackendAuto:
		out.Backend = Kind(declared.DefaultBackend)
	}
	if declared.RTT.Host != "" {
		out.RTT.Host = declared.RTT.Host
	}
	if declared.RTT.Port > 0 {
		out.RTT.Port = declared.RTT.Port
	}
	out.RTT.Channel = declared.RTT.Channel
	out.RTT.BlockIfFull = declared.RTT.BlockIfFull
	if declared.UART.Port != "" {
		out.UART.Port = declared.UART.Port
	}
	out.UART.AutoDetect = declared.UART.AutoDetect
	if declared.UART.BaudRate > 0 {
		out.UART.BaudRate = declared.UART.BaudRate
	}
	out.Formatting.Timestamps = declared.Formatting.Timestamps
	out.Formatting.ColorLevels = declared.Formatting.ColorLevels
	out.Formatting.LogFile = declared.Formatting.LogFile
	return out
}

// LogFilePath is where the log file goes, resolved against the project.
//
// A path in the manifest is relative to the project, so a project that declares
// `logs/target.log` gets it in its own tree rather than in whatever directory a
// monitor happened to be run from.
func LogFilePath(projectRoot, declared string) string {
	if strings.TrimSpace(declared) == "" {
		return ""
	}
	if filepath.IsAbs(declared) {
		return declared
	}
	return filepath.Join(projectRoot, filepath.FromSlash(declared))
}

// Backends is every transport this component can read, for a `--list-backends`
// and for a usage message.
func Backends() []Kind { return []Kind{KindRTT, KindUART, KindSWO} }

// KindNames is every transport's name, as a person would type it.
func KindNames() []string {
	names := make([]string, 0, len(Backends()))
	for _, kind := range Backends() {
		names = append(names, string(kind))
	}
	return names
}

// ParseBackend is a transport by name, and whether it is one this component reads.
//
// Refused rather than resolved to something nearby: a backend nobody reads would
// otherwise sit in the configuration looking like a decision, and the run would
// quietly read something other than what was asked for.
func ParseBackend(name string) (Kind, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case string(KindRTT):
		return KindRTT, true
	case string(KindUART):
		return KindUART, true
	case string(KindSWO):
		return KindSWO, true
	case "auto", "":
		return Kind("auto"), true
	}
	return "", false
}

// DefaultTimeout is how long detection waits for a transport to answer.
const DefaultTimeout = 750 * time.Millisecond
