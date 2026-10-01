//
// @file go.mod
// @brief The local dashboard and registry manager.
//
// It reads the toolchain's own state — what is installed, which version is
// enabled, what a manifest says — and shows it, so that the answers to "what
// have I got, and is it being used?" are somewhere to look rather than
// something to reconstruct from a directory listing.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

module github.com/rho/rho-keeper

go 1.23

require (
	// The documentation is rendered by the generator's own reader rather than a
	// second one: a markdown parser written for a dashboard would be a third
	// opinion about what the project's documentation says, and the one a person
	// reads on a generated site is the one that should be read here too.
	github.com/rho/rho-doc v0.0.0
	rho v0.0.0
)
