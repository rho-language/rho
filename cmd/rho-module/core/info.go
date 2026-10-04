//
// @file info.go
// @brief Rho Module Info - Enterprise v3.
//
// Reads and returns module metadata from module.yml.
// Includes plugin hooks if present.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"path/filepath"

	"rho/src/deps"
)

/* -------------------------------------------------------------------------- */
/*  Types                                                                      */
/* -------------------------------------------------------------------------- */

// ModuleInfo represents complete module metadata including hooks
type ModuleInfo struct {
	ModuleYAML
	// Extends is the component a plugin plugs into, or "" for a module that
	// extends nothing.
	Extends string
	// Hooks are the hooks of that component the plugin answers.
	Hooks []string
	// Commands are the executables answering them.
	Commands []string
}

/* -------------------------------------------------------------------------- */
/*  Public API                                                                 */
/* -------------------------------------------------------------------------- */

// ReadModuleInfo reads module.yml, and for a plugin the component it extends
// and the hooks of that component it answers.
func ReadModuleInfo(dir string) (ModuleInfo, error) {
	modPath := filepath.Join(dir, "module.yml")

	// Delegate to the validated loader so identity metadata cannot drift
	// from what build/pack stages see. This also keeps the command field.
	mod, err := LoadModuleYAML(modPath)
	if err != nil {
		return ModuleInfo{}, err
	}

	info := ModuleInfo{ModuleYAML: mod}
	if mod.Module.Type != "plugin" {
		return info, nil
	}
	relation, err := deps.LoadManifest(modPath)
	if err != nil || relation == nil || relation.Extends == nil {
		// A module that does not declare the relation is still described; it
		// simply extends nothing, which is what a build reports rather than
		// what a reader should be blocked by.
		return info, nil
	}
	info.Extends = relation.Extends.Component
	info.Hooks = relation.Extends.Hooks
	for _, command := range relation.Extends.Commands {
		info.Commands = append(info.Commands, command.Name+": "+command.Run+" ("+command.Stage()+")")
	}
	return info, nil
}
