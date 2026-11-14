//
// @file production_test.go
// @brief Rho Flash production-mode tests.
//
// How a production plan is found, what it prints, and that nothing runs
// without the board's name typed exactly. No hardware is involved and no step
// is executed: the commands the plans name are never on the test's PATH, and
// the tests that reach the run stop at the confirmation or use a dry run.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPlan = `{
  "name": "rp2350 secure boot",
  "summary": "Signed images only.",
  "build": "CONFIG_RH_PRODUCTION=y",
  "steps": [
    {"description": "burn the boot key hash into OTP", "irreversible": true, "when": "before-flash",
     "command": "rh-test-no-such-picotool otp load {env:RH_TEST_OTP_JSON}", "show": "{env:RH_TEST_OTP_JSON}"},
    {"description": "reboot", "irreversible": false, "when": "after-flash",
     "command": "{platform}/tools/rh-test-no-such-picotool reboot"},
    {"description": "the bootloader burns eFuses", "irreversible": true, "when": "first-boot"}
  ],
  "notes": ["level 2 is not offered"]
}`

func writePlan(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, "production")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The board's own plan wins over its preset's, which wins over the family's and
// the platform's.
func TestProductionPlanLookupOrder(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "production.json", `{"name":"platform","steps":[{"description":"d","when":"first-boot"}]}`)
	writePlan(t, root, "stm32h7.json", `{"name":"family","steps":[{"description":"d","when":"first-boot"}]}`)

	plan, err := loadProductionPlan(root, "nucleo-h753zi", "", "STM32H7")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Name != "family" {
		t.Errorf("expected the family plan, got %q", plan.Name)
	}
	writePlan(t, root, "nucleo-h753zi.json", `{"name":"board","steps":[{"description":"d","when":"first-boot"}]}`)
	plan, _ = loadProductionPlan(root, "nucleo-h753zi", "stm32h7", "STM32H7")
	if plan.Name != "board" {
		t.Errorf("expected the board plan, got %q", plan.Name)
	}
	plan, _ = loadProductionPlan(root, "nucleo-f446ze", "stm32f4", "STM32F4")
	if plan.Name != "platform" {
		t.Errorf("expected the platform plan, got %q", plan.Name)
	}
}

// A platform with no plan has no production mode, and says so.
func TestProductionPlanMissingIsAnError(t *testing.T) {
	_, err := loadProductionPlan(t.TempDir(), "uno-r3", "atmega328p", "AVR")
	if err == nil || !strings.Contains(err.Error(), "no production mode") {
		t.Fatalf("expected a missing-plan error, got %v", err)
	}
	if _, err := loadProductionPlan("", "uno-r3", "", ""); err == nil {
		t.Fatal("expected an error with no platform")
	}
}

// A plan that could not be run as written is refused when it is read.
func TestProductionPlanValidation(t *testing.T) {
	cases := map[string]string{
		"no steps":            `{"steps":[]}`,
		"command missing":     `{"steps":[{"description":"d","when":"before-flash"}]}`,
		"first boot with cmd": `{"steps":[{"description":"d","when":"first-boot","command":"x"}]}`,
		"unknown when":        `{"steps":[{"description":"d","when":"later","command":"x"}]}`,
		"no description":      `{"steps":[{"when":"after-flash","command":"x"}]}`,
	}
	for name, body := range cases {
		root := t.TempDir()
		writePlan(t, root, "production.json", body)
		if _, err := loadProductionPlan(root, "b", "", ""); err == nil {
			t.Errorf("%s: expected the plan to be refused", name)
		}
	}
}

// A target name cannot reach outside the production directory.
func TestProductionCandidatesStayInTheDirectory(t *testing.T) {
	for _, name := range productionCandidates("../../etc/x", "a/b", "..") {
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			t.Errorf("candidate %q leaves the directory", name)
		}
	}
}

// The printout names every step with its exact command, marks the irreversible
// ones, puts them in the order they run, and a dry run stops there.
func TestProductionDryRunPrintsThePlanAndRunsNothing(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "rp2350.json", testPlan)
	otpFile := filepath.Join(root, "otp.json")
	if err := os.WriteFile(otpFile, []byte("{\n  \"crit1\": {\"secure_boot_enable\": 1}\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RH_TEST_OTP_JSON", otpFile)
	plan, err := loadProductionPlan(root, "pico2w", "rp2350", "")
	if err != nil {
		t.Fatal(err)
	}
	out := &bytes.Buffer{}
	err = flashProduction("pico2w", "build/app.elf", plan, "rh-test-no-such-flasher", []string{"build/app.elf"},
		productionOptions{Production: true, DryRun: true, In: strings.NewReader("pico2w\n"), Out: out})
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{
		"[IRREVERSIBLE] burn the boot key hash into OTP",
		"runs: rh-test-no-such-picotool otp load " + otpFile,
		"burns the contents of " + otpFile,
		`|   "crit1": {"secure_boot_enable": 1}`,
		"runs: rh-test-no-such-flasher build/app.elf",
		"[reversible] reboot",
		"runs: " + root + "/tools/rh-test-no-such-picotool reboot",
		"the firmware itself will",
		"[IRREVERSIBLE] the bootloader burns eFuses",
		"CONFIG_RH_PRODUCTION=y",
		"note: level 2 is not offered",
		"dry run: nothing was run",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the printout lacks %q:\n%s", want, text)
		}
	}
	otp := strings.Index(text, "otp load")
	flash := strings.Index(text, "write the image")
	reboot := strings.Index(text, "reboot")
	if !(otp < flash && flash < reboot) {
		t.Errorf("steps are not in run order:\n%s", text)
	}
	if strings.Contains(text, "Type the board name") {
		t.Error("a dry run asked for confirmation")
	}
}

// Only the exact board name continues.
func TestConfirmBoardName(t *testing.T) {
	for input, ok := range map[string]bool{
		"pico2w\n":    true,
		"  pico2w \n": true,
		"pico2w":      true, // the end of the input after the name
		"Pico2W\n":    false,
		"yes\n":       false,
		"y\n":         false,
		"\n":          false,
		"":            false,
		"pico2\n":     false,
	} {
		err := confirmBoardName(strings.NewReader(input), &bytes.Buffer{}, "pico2w")
		if (err == nil) != ok {
			t.Errorf("input %q: expected confirmed=%v, got err=%v", input, ok, err)
		}
	}
}

// A wrong name stops before the first step: the before-flash step names a
// program that does not exist, so reaching it would be an error of another kind.
func TestProductionRefusedRunsNothing(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "rp2350.json", testPlan)
	otpFile := filepath.Join(root, "otp.json")
	if err := os.WriteFile(otpFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RH_TEST_OTP_JSON", otpFile)
	plan, _ := loadProductionPlan(root, "pico2w", "rp2350", "")
	out := &bytes.Buffer{}
	err := flashProduction("pico2w", "build/app.elf", plan, "rh-test-no-such-flasher", nil,
		productionOptions{Production: true, In: strings.NewReader("yes\n"), Out: out})
	if err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("expected a refusal, got %v", err)
	}
}

// A plan that needs an environment variable nobody set fails before printing a
// plan that would not be what runs.
func TestProductionMissingEnvironmentIsAnError(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "rp2350.json", testPlan)
	os.Unsetenv("RH_TEST_OTP_JSON")
	plan, _ := loadProductionPlan(root, "pico2w", "rp2350", "")
	err := flashProduction("pico2w", "build/app.elf", plan, "x", nil,
		productionOptions{Production: true, DryRun: true, In: strings.NewReader(""), Out: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "RH_TEST_OTP_JSON") {
		t.Fatalf("expected the missing variable to be named, got %v", err)
	}
}

// A file a step burns that is not there stops the flash before it asks.
func TestProductionMissingShownFileIsAnError(t *testing.T) {
	root := t.TempDir()
	writePlan(t, root, "rp2350.json", testPlan)
	t.Setenv("RH_TEST_OTP_JSON", filepath.Join(root, "absent.json"))
	plan, _ := loadProductionPlan(root, "pico2w", "rp2350", "")
	out := &bytes.Buffer{}
	err := flashProduction("pico2w", "build/app.elf", plan, "x", nil,
		productionOptions{Production: true, In: strings.NewReader("pico2w\n"), Out: out})
	if err == nil || !strings.Contains(err.Error(), "cannot be read") {
		t.Fatalf("expected the missing file to stop the flash, got %v", err)
	}
	if strings.Contains(out.String(), "Type the board name") {
		t.Error("asked for confirmation with a file it could not show")
	}
}
