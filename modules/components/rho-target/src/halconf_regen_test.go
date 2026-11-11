package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rho/rho-target/src/rhtarget"
)

// A configuration the generator wrote follows its platform: a driver the
// platform enables later is enabled the next time it is written. One a person
// wrote by hand is left as it is.
func TestHalConfFollowsThePlatformButNotAHandWrittenFile(t *testing.T) {
	root := t.TempDir()
	platform := filepath.Join(root, "platform")
	project := filepath.Join(root, "app")
	template := filepath.Join(platform, "vendor", "hal", "Inc", "fam_hal_conf_template.h")
	for _, dir := range []string{filepath.Dir(template), filepath.Join(project, "src")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(template, []byte("#ifndef FAM_HAL_CONF_TEMPLATE_H\n#define FAM_HAL_CONF_TEMPLATE_H\n#define HAL_GPIO_MODULE_ENABLED\n#define HAL_RNG_MODULE_ENABLED\n#endif\n"), 0o644)
	writePlatform := func(defines string) {
		manifest := "module:\n  name: plat\n  version: 0.1.0\n  type: module\n  description: x\n\nmain: src/plat.c\n\nbuild:\n  language: c\n  defines:\n" + defines +
			"\nvendor:\n  dir: vendor\n"
		if err := os.WriteFile(filepath.Join(platform, "module.yml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(project, "module.yml"), []byte("module:\n  name: app\n  version: 0.1.0\n  type: module\n  description: x\n\nmain: src/main.c\n\ndepends:\n  - name: plat\n    type: c\n    path: "+platform+"\n"), 0o644)
	spec := rhtarget.Spec{Name: "board", HALHeader: "fam_hal.h"}
	conf := filepath.Join(platform, "vendor", "includes", "fam_hal_conf.h")

	writePlatform("    - HAL_GPIO_MODULE_ENABLED")
	if err := writeHalConf(project, spec); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(conf)
	if strings.Contains(string(first), "\n#define HAL_RNG_MODULE_ENABLED") {
		t.Fatal("a driver the platform did not enable was enabled")
	}

	writePlatform("    - HAL_GPIO_MODULE_ENABLED\n    - HAL_RNG_MODULE_ENABLED")
	if err := writeHalConf(project, spec); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(conf)
	if !strings.Contains(string(second), "\n#define HAL_RNG_MODULE_ENABLED") {
		t.Fatalf("a driver the platform now enables is still off:\n%s", second)
	}

	os.WriteFile(conf, []byte("/* mine */\n"), 0o644)
	if err := writeHalConf(project, spec); err != nil {
		t.Fatal(err)
	}
	if mine, _ := os.ReadFile(conf); string(mine) != "/* mine */\n" {
		t.Fatal("a hand-written configuration was overwritten")
	}
}
