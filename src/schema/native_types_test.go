//
// @file native_types_test.go
// @brief Tests for which module types are native, and which declare a main.
//
// The type of a module decides what a manifest has to say about it. A Rho
// module is entered through a main; a component's or a plugin's main is a
// binary built while packing it; a native module's is nothing at all, because
// it is linked into a project rather than run.
//
// @project Rho Programming Language
// @license Apache-2.0
//

package schema

import "testing"

/* -------------------------------------------------------------------------- */
/*  platform is a module type                                                  */
/* -------------------------------------------------------------------------- */

// A platform is a first-class module type, not an `interfaces` module wearing a
// different name. It is what implements the interfaces a `interfaces` module
// declares, and a manifest that says `platform` has to be accepted rather than
// refused for a value the schema has never heard of.
func TestPlatformIsAValidModuleType(t *testing.T) {
	for _, candidate := range ValidModuleTypes {
		if candidate == "platform" {
			return
		}
	}
	t.Errorf("platform is not a valid module type; got %v", ValidModuleTypes)
}

// A board's support package is a third thing again: it is not the platform,
// which implements a whole family, and it is not the interfaces, which declare
// what a platform implements. Collapsing it into `platform` would leave a
// reader unable to tell which of a board's files are the family's driver code
// and which are that one board's pin map.
func TestPlatformBoardIsAValidModuleType(t *testing.T) {
	for _, candidate := range ValidModuleTypes {
		if candidate == "platform-board" {
			return
		}
	}
	t.Errorf("platform-board is not a valid module type; got %v", ValidModuleTypes)
}

/* -------------------------------------------------------------------------- */
/*  native types declare no main                                               */
/* -------------------------------------------------------------------------- */

// A native module is linked into a project rather than run, so it declares no
// entry point. The distinction is what lets a hardware platform be published
// without inventing a file to call its main.
func TestNativeTypesAreInterfacesAndPlatform(t *testing.T) {
	for _, moduleType := range []string{"interfaces", "platform", "platform-board"} {
		if !IsNativeType(moduleType) {
			t.Errorf("%s is not a native type", moduleType)
		}
		if IsRhoSourceType(moduleType) {
			t.Errorf("%s is both native and a Rho source type", moduleType)
		}
	}
}

// The native types are exactly the ones that are not written in Rho. A type
// that were both would have two contradictory rules about its main, and which
// one applied would depend on which check ran first.
func TestNoTypeIsBothNativeAndRhoSource(t *testing.T) {
	for _, moduleType := range ValidModuleTypes {
		if IsNativeType(moduleType) && IsRhoSourceType(moduleType) {
			t.Errorf("%s is classified as both native and Rho source", moduleType)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  a Rho library's main is a file                                             */
/* -------------------------------------------------------------------------- */

// Only a Rho library names a source file that has to exist. A component's and a
// plugin's main is a binary, so requiring the file for those refused every
// plugin whose binary had not been built yet.
func TestOnlyRhoLibrariesHaveASourceFileAsMain(t *testing.T) {
	for _, moduleType := range []string{"rho", "module"} {
		if !IsRhoLibraryType(moduleType) {
			t.Errorf("%s should have a source file as its main", moduleType)
		}
	}
	for _, moduleType := range []string{"component", "plugin", "interfaces", "platform"} {
		if IsRhoLibraryType(moduleType) {
			t.Errorf("%s should not have a source file as its main", moduleType)
		}
	}
}
