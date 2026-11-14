//
// @file wizard.go
// @brief Schema validation for the keeper's deployment wizard documents.
//
// The wizard is data, so its integrity has to be checked before an action reads
// a field that is not there. The documents are `wizard.yml` (the table of
// contents) and one file per step. A step names its fields and actions as maps,
// which is the part the generic schema validator cannot reach: it validates a
// declared field, not the values of a map whose keys the document chooses.
// ValidateWizardStep therefore validates the step's outer shape and each map
// value against the field and action schemas.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package schema

import (
	"fmt"
	"os"
	"path/filepath"

	"rho/src/yaml"
)

// ValidWizardFieldTypes are the controls a step may ask the keeper to render.
var ValidWizardFieldTypes = []string{"board", "driver", "select", "text", "password", "number", "nodes"}

// WizardSchema is the shape of wizard.yml.
func WizardSchema() Schema {
	return Schema{
		Name: "wizard.yml",
		Fields: []FieldSchema{
			{Name: "title", Required: true, RejectNull: true, Kind: KindString},
			{Name: "description", Required: true, RejectNull: true, Kind: KindString},
			{Name: "steps", Required: true, RejectNull: true, Kind: KindMap},
		},
	}
}

// WizardStepSchema is the outer shape of a step document.
func WizardStepSchema() Schema {
	return Schema{
		Name: "wizard step",
		Fields: []FieldSchema{
			{Name: "title", Required: true, RejectNull: true, Kind: KindString},
			{Name: "description", Required: true, RejectNull: true, Kind: KindString},
			{Name: "fields", Required: false, Kind: KindMap},
			{Name: "actions", Required: false, Kind: KindMap},
		},
	}
}

// WizardFieldSchema is the shape of one entry under `fields`.
func WizardFieldSchema() Schema {
	return Schema{
		Name: "wizard field",
		Fields: []FieldSchema{
			{Name: "label", Required: true, RejectNull: true, Kind: KindString},
			{Name: "type", Required: true, RejectNull: true, Kind: KindString, AllowedValues: ValidWizardFieldTypes},
			{Name: "role", Required: false, Kind: KindString},
			{Name: "options", Required: false, Kind: KindString},
			{Name: "default", Required: false, Kind: KindString},
			{Name: "required", Required: false, Kind: KindBool},
			{Name: "help", Required: false, Kind: KindString},
		},
	}
}

// WizardActionSchema is the shape of one entry under `actions`.
func WizardActionSchema() Schema {
	return Schema{
		Name: "wizard action",
		Fields: []FieldSchema{
			{Name: "label", Required: true, RejectNull: true, Kind: KindString},
			{Name: "handler", Required: false, Kind: KindString},
			{Name: "command", Required: false, Kind: KindString},
			{Name: "report", Required: false, Kind: KindString},
			{Name: "expect", Required: false, Kind: KindString},
			{Name: "progress", Required: false, Kind: KindBool},
			{Name: "help", Required: false, Kind: KindString},
		},
	}
}

// ValidateWizardDocument validates the table of contents.
func ValidateWizardDocument(data yaml.Value) ValidateResult {
	return Validate(data, WizardSchema())
}

// ValidateWizardStep validates one step document, including every field and
// action it declares.
func ValidateWizardStep(data yaml.Value) ValidateResult {
	result := Validate(data, WizardStepSchema())

	fields, hasFields := data.Map["fields"]
	if hasFields && fields.Kind == yaml.KindMap {
		for name, value := range fields.Map {
			fieldPath := "fields." + name
			fieldResult := Validate(value, WizardFieldSchema())
			for _, issue := range fieldResult.Errors {
				issue.Path = fieldPath + "." + issue.Path
				result.Errors = append(result.Errors, issue)
			}
			if value.Kind != yaml.KindMap {
				continue
			}
			kind := value.Map["type"].Str
			role := value.Map["role"].Str
			options := value.Map["options"].Str
			if kind == "board" && role == "" {
				result.Errors = append(result.Errors, ValidationError{
					Path:    fieldPath + ".role",
					Message: "a board field must name the capability role it offers",
				})
			}
			if kind == "select" && options == "" {
				result.Errors = append(result.Errors, ValidationError{
					Path:    fieldPath + ".options",
					Message: "a select field must name its options",
				})
			}
		}
	}

	actions, hasActions := data.Map["actions"]
	if hasActions && actions.Kind == yaml.KindMap {
		for name, value := range actions.Map {
			actionPath := "actions." + name
			actionResult := Validate(value, WizardActionSchema())
			for _, issue := range actionResult.Errors {
				issue.Path = actionPath + "." + issue.Path
				result.Errors = append(result.Errors, issue)
			}
			if value.Kind != yaml.KindMap {
				continue
			}
			handler := value.Map["handler"].Str
			command := value.Map["command"].Str
			if handler == "" && command == "" {
				result.Errors = append(result.Errors, ValidationError{
					Path:    actionPath,
					Message: "an action must name a handler or a command",
				})
			}
		}
	}

	return result
}

// ValidateWizardFiles validates a wizard directory: its table of contents and
// every step the table names.
func ValidateWizardFiles(root string) ValidateResult {
	result := ValidateResult{}
	main := filepath.Join(root, "wizard.yml")
	data, err := loadYAMLFile(main)
	if err != nil {
		result.Errors = append(result.Errors, ValidationError{Path: main, Message: err.Error()})
		return result
	}
	result = ValidateWizardDocument(data)

	steps, ok := data.Map["steps"]
	if !ok || steps.Kind != yaml.KindMap {
		return result
	}
	for name, value := range steps.Map {
		if value.Kind != yaml.KindString || value.Str == "" {
			result.Errors = append(result.Errors, ValidationError{
				Path:    "steps." + name,
				Message: "a step must name its file",
			})
			continue
		}
		path := filepath.Join(root, value.Str)
		step, err := loadYAMLFile(path)
		if err != nil {
			result.Errors = append(result.Errors, ValidationError{Path: path, Message: err.Error()})
			continue
		}
		stepResult := ValidateWizardStep(step)
		for _, issue := range stepResult.Errors {
			issue.Path = name + "." + issue.Path
			result.Errors = append(result.Errors, issue)
		}
	}
	return result
}

func loadYAMLFile(path string) (yaml.Value, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return yaml.Value{}, fmt.Errorf("read %s: %w", path, err)
	}
	value, err := yaml.Load(string(source))
	if err != nil {
		return yaml.Value{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return value, nil
}
