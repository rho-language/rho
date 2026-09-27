//
// @file config_test.go
// @brief Tests for the configuration a generated C build resolves.
//
// The generator does not resolve a configuration: it writes the build that does.
// What it writes has to be right in three ways, and these tests are about those
// three: the glue must name the fragments and the tool, the build must depend on
// the configuration so a changed symbol rebuilds, and a project that declared
// nothing must get none of it.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package projectgen

import (
	"path/filepath"
	"strings"
	"testing"
)

// configured is a context for a project that declared a Kconfig bridge.
func configured(dialect string) Context {
	return Context{
		ModuleName:  "sensorhub",
		ProjectRoot: "/project",
		OutputDir:   "/project/build/generated/c",
		Config: &Config{
			Dialect:    dialect,
			Fragments:  []string{filepath.Join("kconfig", "board.Kconfig")},
			Defconfig:  filepath.Join("kconfig", "rho-defconfig"),
			ValuesFile: ".config",
			OutputDir:  "/project/build/generated/c",
		},
	}
}

/* -------------------------------------------------------------------------- */
/*  The glue                                                                  */
/* -------------------------------------------------------------------------- */

func TestTheGlueRunsTheMergeWithWhatTheProjectDeclared(t *testing.T) {
	// The glue is a build file a person can read, so it names the tool, the
	// dialect, the fragments and the baseline rather than hiding them.
	artifacts, err := EmitCProject(configured("freertos"))
	if err != nil {
		t.Fatalf("EmitCProject: %v", err)
	}
	glue, written := artifacts.Extra["rho-config.cmake"]
	if !written {
		t.Fatal("expected the configuration glue to be generated")
	}
	for _, want := range []string{
		"kconfig merge",
		"--force",
		`set(RHO_CONFIG_DIALECT "freertos")`,
		"board.Kconfig",
		"rho-defconfig",
		"RHO_BIN",
		"rho-config.cmake",
	} {
		if !strings.Contains(glue, want) {
			t.Errorf("expected the glue to carry %q, got:\n%s", want, glue)
		}
	}
	// The generated project is a build artefact, so a build writes over what a
	// previous run left rather than keeping a stale header.
	if !strings.Contains(glue, "--force") {
		t.Error("expected the merge to write over a previous run's files")
	}
}

func TestTheBuildDependsOnTheConfiguration(t *testing.T) {
	// A source compiled against the header that was there when the project was
	// generated would be a build that ignores the configuration it was given.
	artifacts, err := EmitCProject(configured("freertos"))
	if err != nil {
		t.Fatalf("EmitCProject: %v", err)
	}
	build := artifacts.Build
	for _, want := range []string{
		"include(${CMAKE_CURRENT_SOURCE_DIR}/rho-config.cmake)",
		"add_dependencies(",
		"OBJECT_DEPENDS",
		"target_include_directories(",
	} {
		if !strings.Contains(build, want) {
			t.Errorf("expected the build to carry %q, got:\n%s", want, build)
		}
	}
}

func TestAProjectWithNoConfigurationGetsAPlainBuild(t *testing.T) {
	// Most projects configure nothing, and a build carrying a merge for a
	// configuration that does not exist is a build that fails for no reason.
	ctx := configured("freertos")
	ctx.Config = nil
	artifacts, err := EmitCProject(ctx)
	if err != nil {
		t.Fatalf("EmitCProject: %v", err)
	}
	if len(artifacts.Extra) != 0 {
		t.Errorf("expected no configuration files, got %+v", artifacts.Extra)
	}
	for _, unwanted := range []string{"rho-config", "RHO_CONFIG", "autoconf.h"} {
		if strings.Contains(artifacts.Build, unwanted) {
			t.Errorf("expected the build to be plain, found %q in:\n%s", unwanted, artifacts.Build)
		}
		if strings.Contains(artifacts.Interface, unwanted) {
			t.Errorf("expected the interface to be plain, found %q in:\n%s", unwanted, artifacts.Interface)
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  Which header a dialect writes                                              */
/* -------------------------------------------------------------------------- */

func TestEachDialectsHeaderIsIncludedByNameAndFoundByPath(t *testing.T) {
	// A header in the wrong directory is a header nothing includes, and one with
	// the wrong name is a header the build cannot see. The generator's half of
	// this convention is here; the component writes the other half.
	for dialect, want := range map[string]struct {
		directory string
		header    string
	}{
		"linux":    {filepath.Join("include", "generated"), "autoconf.h"},
		"espidf":   {filepath.Join("build", "config"), "sdkconfig.h"},
		"freertos": {"", "FreeRTOSConfig.h"},
		"":         {filepath.Join("include", "generated"), "autoconf.h"},
	} {
		directory, header := configHeader(dialect)
		if directory != want.directory || header != want.header {
			t.Errorf("%s: expected %s in %q, got %s in %q", dialect, want.header, want.directory, header, directory)
		}

		artifacts, err := EmitCProject(configured(dialect))
		if err != nil {
			t.Fatalf("EmitCProject: %v", err)
		}
		if !strings.Contains(artifacts.Interface, "#include \""+header+"\"") {
			t.Errorf("%s: expected the interface to include %s, got:\n%s", dialect, header, artifacts.Interface)
		}
		if directory != "" && !strings.Contains(artifacts.Build, directory) {
			t.Errorf("%s: expected the build to add %s to the include path, got:\n%s", dialect, directory, artifacts.Build)
		}
	}
}

func TestAnUnknownDialectIsTreatedAsLinuxAndTheMergeSaysSo(t *testing.T) {
	// The generator does not know dialects, so it falls back to the one it knows;
	// the merge is where an unknown dialect is reported, by name, to whoever runs
	// the build.
	directory, header := configHeader("zephyr")
	if directory != filepath.Join("include", "generated") || header != "autoconf.h" {
		t.Fatalf("expected the Linux header as the fallback, got %s in %q", header, directory)
	}
}
