//
// @file contracts_test.go
// @brief What a plugin says it is, and what a component says it can be.
//
// A hook says how a plugin is called. It does not say what the plugin is for:
// `rho-doc` offers `index` and `page`, and a table of contents and an API
// reference both answer `index` while being nothing alike. These are what a
// contract is for, and these are the rules around one.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package plugins

import "testing"

// A plugin that names no contract is offered to every run of its component,
// because a component whose plugins are all of one kind has no reason to name
// them and a rule with no failure behind it is only an obstacle.
func TestAPluginThatNamesNoContractIsOfferedEverywhere(t *testing.T) {
	untyped := Plugin{Name: "rho-doc-toc", Extends: "rho-doc"}
	if !untyped.Fills("contents") {
		t.Error("a plugin that names no contract was excluded from a contract")
	}
	typed := Plugin{Name: "rho-doc-api", Extends: "rho-doc", Implements: []string{"contents"}}
	if !typed.Fills("contents") {
		t.Error("a plugin that names the contract was excluded from it")
	}
	other := Plugin{Name: "rho-sql-sqlite", Extends: "rho-doc", Implements: []string{"engine"}}
	if other.Fills("contents") {
		t.Error("a plugin was offered to a contract it does not fill")
	}
}

// A component asks which of its plugins it could run, which is what it needs
// in order to report them or to complain that one is missing.
func TestAPluginIsOfferedToAtLeastOneContractOrToAll(t *testing.T) {
	contents := Contract{Name: "contents", Component: "rho-doc"}
	api := Contract{Name: "api", Component: "rho-doc"}

	fitting := Plugin{Name: "rho-doc-api", Extends: "rho-doc", Implements: []string{"api"}}
	if !fitting.FillsAny([]Contract{contents, api}) {
		t.Error("a plugin that fills one of a component's contracts was offered to none")
	}
	missing := Plugin{Name: "stranger", Extends: "rho-doc", Implements: []string{"engine"}}
	if missing.FillsAny([]Contract{contents, api}) {
		t.Error("a plugin that fills none of a component's contracts was offered to it")
	}
	// A component with no contracts is not a component that turns plugins
	// away: it has not said what it wants, so everything is offered.
	untied := Plugin{Name: "rho-doc-toc", Extends: "rho-doc"}
	if !untied.FillsAny(nil) {
		t.Error("a plugin was excluded from a component that declares no contracts")
	}
}

// A component with no contracts offers every plugin to every run. That is the
// behaviour a component whose one plugin is the only kind there is wants, and it
// is what a plugin written before contracts existed gets.
func TestAContractIsHowAPluginSaysWhatItIsFor(t *testing.T) {
	// The contract carries its own cardinality, so a slot is a contract plus
	// `one`, and a component does not have to describe how many in a second
	// place that could disagree.
	engine := Contract{Name: "engine", One: true}
	contents := Contract{Name: "contents", One: false}

	if !engine.One {
		t.Error("an engine contract should be filled by exactly one plugin")
	}
	if contents.One {
		t.Error("a contents contract should take every enabled plugin")
	}
}
