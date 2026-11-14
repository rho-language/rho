//
// @file native_types_test.go
// @brief What a type is, when the type belongs to an ecosystem.
//
// The toolchain defines a small set of types and knows nothing about the rest.
// An ecosystem adds its own in a module contract, and the toolchain validates
// them without knowing what they mean. These tests are about that boundary: a
// type the contract adds is accepted, the sections it requires are enforced,
// and a native type declares no entry point.
//
// @project Rho Programming Language
// @license Apache-2.0 WITH LLVM-exception
//

package schema

import (
	"testing"

	"rho/src/yaml"
)

/* -------------------------------------------------------------------------- */
/*  The toolchain's own types                                                 */
/* -------------------------------------------------------------------------- */

func TestCoreModuleTypesAreTheToolchainsOwn(t *testing.T) {
	for _, moduleType := range []string{"rho", "module", "component", "plugin", "provider"} {
		found := false
		for _, candidate := range ValidModuleTypes {
			if candidate == moduleType {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is not a core module type; got %v", moduleType, ValidModuleTypes)
		}
	}
}

// An ecosystem's words are not the toolchain's. A contract adds them, and the
// toolchain does not carry them when no contract is present.
func TestAnEcosystemTypeIsNotACoreType(t *testing.T) {
	for _, moduleType := range []string{"driver", "device", "platform", "collection"} {
		if IsRhoSourceType(moduleType) {
			t.Errorf("%s should not be a Rho source type", moduleType)
		}
		if ModuleAllows(moduleType) {
			t.Errorf("%s should not be a core type", moduleType)
		}
	}
}

// ModuleAllows reports whether the toolchain's own schema names a type.
func ModuleAllows(moduleType string) bool {
	for _, candidate := range ValidModuleTypes {
		if candidate == moduleType {
			return true
		}
	}
	return false
}

/* -------------------------------------------------------------------------- */
/*  An ecosystem adds a type                                                  */
/* -------------------------------------------------------------------------- */

const gadgetContract = `{
  "version": 1,
  "types": {
    "gadget": {
      "native": true,
      "required": ["gadget", "build"],
      "fields": [
        {"name": "gadget", "required": true, "kind": "map", "fields": [
          {"name": "kind", "required": true, "kind": "string", "allowed": ["sensor", "switch"]},
          {"name": "transports", "required": true, "kind": "list", "item": {"kind": "string"}}
        ]}
      ]
    }
  }
}`

func loadGadgetContract(t *testing.T) *ModuleContract {
	t.Helper()
	contract, err := ParseModuleContract([]byte(gadgetContract))
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestAnEcosystemTypeIsAcceptedWithAContract(t *testing.T) {
	src := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
gadget:
  kind: sensor
  transports: [gpio]
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	result := ValidateModuleManifestWithContract(data, "gadget", loadGadgetContract(t))
	if result.HasErrors() {
		t.Fatalf("expected the contract's type to validate, got: %s", result.String())
	}
}

func TestAnEcosystemTypeIsRefusedWithoutAContract(t *testing.T) {
	src := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifest(data, "gadget").HasErrors() {
		t.Fatal("expected a type the toolchain does not define to be refused")
	}
}

func TestAContractRequiresItsSections(t *testing.T) {
	src := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifestWithContract(data, "gadget", loadGadgetContract(t)).HasErrors() {
		t.Fatal("expected a gadget without its gadget section to be refused")
	}
}

func TestAContractValidatesItsFields(t *testing.T) {
	src := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
gadget:
  kind: spaceship
  transports: [gpio]
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifestWithContract(data, "gadget", loadGadgetContract(t)).HasErrors() {
		t.Fatal("expected a gadget kind outside the contract's enum to be refused")
	}
}

// A native type is linked, not run, so a manifest that gives it a main says
// two things at once.
func TestANativeContractTypeDeclaresNoMain(t *testing.T) {
	src := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
main: src/main.c
build:
  language: c
gadget:
  kind: sensor
  transports: [gpio]
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifestWithContract(data, "gadget", loadGadgetContract(t)).HasErrors() {
		t.Fatal("expected a native type with a main to be refused")
	}
}

func TestContractMergeAddsTypes(t *testing.T) {
	first := loadGadgetContract(t)
	second := &ModuleContract{
		Version: 1,
		Types: map[string]TypeContract{
			"widget": {Native: true, Required: []string{"build"}},
		},
	}
	first.Merge(second)
	if !first.Allows("gadget") || !first.Allows("widget") {
		t.Fatalf("expected both types after merge, got %v", first.TypeNames())
	}
}

func TestContractRejectsAnUnknownVersion(t *testing.T) {
	if _, err := ParseModuleContract([]byte(`{"version":2,"types":{"gadget":{}}}`)); err == nil {
		t.Fatal("expected an unsupported contract version to be refused")
	}
}

// A package read from an archive is not re-litigated: the type it names belongs
// to an ecosystem whose contract the installing machine may not carry. The
// package was validated when it was published.
func TestAPackageManifestAcceptsAnEcosystemType(t *testing.T) {
	src := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
`
	data, err := yaml.Load(src)
	if err != nil {
		t.Fatal(err)
	}
	if result := ValidateModuleManifestPackage(data); result.HasErrors() {
		t.Fatalf("expected a package manifest to accept its ecosystem type, got: %s", result.String())
	}
}

// A contract can make the identity a fact rather than a convention. A module
// whose name can be shared by another repository is not identified by its name.
func TestAContractCanRequireAQualifiedModuleID(t *testing.T) {
	contract, err := ParseModuleContract([]byte(`{"version":1,"types":{"gadget":{"native":true,"module_id":true,"required":["build"]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	without := `module:
  name: a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
`
	data, err := yaml.Load(without)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateModuleManifestWithContract(data, "gadget", contract).HasErrors() {
		t.Fatal("expected a gadget without module.id to be refused")
	}

	with := `module:
  name: a-gadget
  id: examples.gadgets.a-gadget
  type: gadget
  version: 0.1.0
build:
  language: c
`
	data, err = yaml.Load(with)
	if err != nil {
		t.Fatal(err)
	}
	if result := ValidateModuleManifestWithContract(data, "gadget", contract); result.HasErrors() {
		t.Fatalf("expected a qualified module.id to validate, got: %s", result.String())
	}
}
