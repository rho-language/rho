//
// @file descriptor_merge.go
// @brief A project's project.yml says what differs from the built-in targets.
//
// The built-in descriptor already says what a target is: its files, its kind,
// its emitter. A project's project.yml is read on top of it, so a project writes
// what is its own — its templates, its locales, its public paths — and not the
// layout every project shares. A target with a built-in id takes the built-in
// target's settings for every key it does not write; a key it writes replaces
// that setting whole. A target with an id of its own is complete on its own.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package projectgen

import (
	"fmt"
	"os"

	"rho/src/yaml"
)

// ParseDescriptorFileOverDefault reads a project's project.yml on top of the
// built-in descriptor.
func ParseDescriptorFileOverDefault(path string) (*Descriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	descriptor, err := ParseDescriptorOverDefault(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return descriptor, nil
}

// ParseDescriptorOverDefault reads descriptor YAML on top of the built-in one.
func ParseDescriptorOverDefault(content []byte) (*Descriptor, error) {
	base, err := yaml.Load(string(DefaultDescriptorYAML))
	if err != nil {
		return nil, fmt.Errorf("the built-in project descriptor: %w", err)
	}
	project, err := yaml.Load(string(content))
	if err != nil {
		return nil, fmt.Errorf("invalid YAML in project descriptor: %w", err)
	}
	return parseDescriptorValue(mergeDescriptor(base, project))
}

// mergeDescriptor is base with project's settings over it.
func mergeDescriptor(base, project yaml.Value) yaml.Value {
	if project.Kind != yaml.KindMap {
		return base
	}
	out := yaml.Value{Kind: yaml.KindMap, Map: map[string]yaml.Value{}}
	for key, value := range base.Map {
		out.Map[key] = value
	}
	for key, value := range project.Map {
		if key != "targets" {
			out.Map[key] = value
		}
	}
	targets := append([]yaml.Value(nil), base.Map["targets"].List...)
	for _, target := range project.Map["targets"].List {
		id := target.Map["id"].Str
		merged := false
		for i, existing := range targets {
			if id != "" && existing.Map["id"].Str == id {
				targets[i] = overlay(existing, target)
				merged = true
				break
			}
		}
		if !merged {
			targets = append(targets, target)
		}
	}
	out.Map["targets"] = yaml.Value{Kind: yaml.KindList, List: targets}
	return out
}

// overlay is a target with another's keys written over it.
func overlay(base, over yaml.Value) yaml.Value {
	if over.Kind != yaml.KindMap {
		return base
	}
	out := yaml.Value{Kind: yaml.KindMap, Map: map[string]yaml.Value{}}
	for key, value := range base.Map {
		out.Map[key] = value
	}
	for key, value := range over.Map {
		out.Map[key] = value
	}
	return out
}
