//
// @file svdsource_test.go
// @brief Tests for where a register map comes from, and the cache it lands in.
//
// The repository is a real one and the file is four megabytes, so nothing here
// touches it: the fetch is exercised against a server this test starts, which is
// what makes "fetched once, then read" a fact these tests establish rather than a
// thing that happened to be true on the machine that wrote them.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rho/src/targetspec"
)

// A source is a vendor directory and a file, the way the CMSIS device data
// repository writes one.
func TestASourceIsAVendorAndAFile(t *testing.T) {
	source, ok := ParseSVDSource("STMicro/STM32H753x.svd")
	if !ok {
		t.Fatal("expected a source")
	}
	if source.Vendor != "STMicro" || source.Device != "STM32H753x.svd" {
		t.Errorf("expected the vendor and the file, got %+v", source)
	}
	// A person copying a name out of a browser has a URL, and one copying out of a
	// repository listing leaves the suffix off. Both mean the file the vendor
	// publishes, and neither may be refused.
	for _, given := range []string{
		"STMicro/STM32H753x.svd",
		"STMicro/STM32H753x",
		"/STMicro/STM32H753x.svd",
		"https://github.com/cmsis-svd/cmsis-svd-data/blob/main/data/STMicro/STM32H753x.svd",
	} {
		if _, ok := ParseSVDSource(given); !ok {
			t.Errorf("expected %q to be read as a source", given)
		}
	}
	for _, given := range []string{"", "   ", "STM32H753x.svd", "notes.txt"} {
		if _, ok := ParseSVDSource(given); ok {
			t.Errorf("expected %q not to be read as a source", given)
		}
	}
}

// The file's address is derived from the repository the machine is configured
// with, so a machine pointed at its own mirror gets its own mirror.
func TestTheFileAddressComesFromTheConfiguredRepository(t *testing.T) {
	source, _ := ParseSVDSource("STMicro/STM32H753x.svd")
	for _, testCase := range []struct{ repository, ref, want string }{
		{DefaultRepository, "main", DefaultRepository + "/raw/main/data/STMicro/STM32H753x.svd"},
		{"https://git.example.com/mirror/svd/", "", "https://git.example.com/mirror/svd/raw/main/data/STMicro/STM32H753x.svd"},
		{"https://git.example.com/mirror/svd.git", "v1.2", "https://git.example.com/mirror/svd/raw/v1.2/data/STMicro/STM32H753x.svd"},
	} {
		if got := source.rawURL(testCase.repository, testCase.ref); got != testCase.want {
			t.Errorf("%s at %q:\n  expected %q\n  got      %q", source.Device, testCase.repository, testCase.want, got)
		}
	}
}

// The cache is shared by every project on the machine and laid out the way the
// repository is, so a file in it says which file it is without anything else
// having to be read.
func TestTheCacheIsInTheRhoHomeAndKeepsTheRepositorysLayout(t *testing.T) {
	home := t.TempDir()
	settings := Settings{Repository: DefaultRepository, Ref: "main", Cache: DefaultCache, Fetch: true, CacheHome: home}
	source, _ := ParseSVDSource("STMicro/STM32H753x.svd")
	want := filepath.Join(home, "cache", "svd", "STMicro", "STM32H753x.svd")
	if got := source.CachePath(settings); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	// An absolute path is taken as it is given, because a cache is a thing a person
	// may have said where.
	elsewhere := Settings{Cache: "/var/cache/svd"}
	if got := source.CachePath(elsewhere); got != "/var/cache/svd/STMicro/STM32H753x.svd" {
		t.Errorf("expected the absolute cache to be used, got %q", got)
	}
	// And no Rho home means no cache, said rather than guessed at: a cache written
	// beside the working directory is one a project would find, and a project's
	// files are somewhere people commit from.
	if got := (Settings{Cache: "cache/svd"}).CacheDirectory(); got != "" {
		t.Errorf("expected no cache without a Rho home, got %q", got)
	}
}

// Fetched once, and read from the cache after that: the second session costs
// nothing and asks nothing of the network.
func TestARegisterMapIsFetchedOnceAndThenReadFromTheCache(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.URL.Path != "/raw/main/data/STMicro/STM32H753x.svd" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("<device><name>STM32H753x</name><peripherals/></device>"))
	}))
	defer server.Close()

	home := t.TempDir()
	settings := Settings{
		Repository: server.URL, Ref: "main", Cache: DefaultCache, Fetch: true, CacheHome: home,
	}
	source, _ := ParseSVDSource("STMicro/STM32H753x.svd")

	first, err := EnsureRegisterMap(source, settings)
	if err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("expected one request, got %d", asked)
	}
	cached, isCached := CachedRegisterMap(source, settings)
	if !isCached || cached != first {
		t.Fatalf("expected the fetched file in the cache, got %q (%v)", cached, isCached)
	}
	if strings.Contains(cached, ".partial") {
		t.Errorf("expected the file to be named after the device, got %q", cached)
	}
	if data, readErr := os.ReadFile(cached); readErr != nil || !strings.Contains(string(data), "STM32H753x") {
		t.Errorf("expected the fetched contents, got %q (%v)", data, readErr)
	}

	// A second time is answered from the cache, and the file has to exist for that
	// answer to be about this device.
	if again, err := EnsureRegisterMap(source, settings); err != nil || again != first {
		t.Errorf("expected the same file the second time, got %q (%v)", again, err)
	}
	if asked != 1 {
		t.Errorf("expected no second request, got %d", asked)
	}
}

// Two sessions on one machine at once must not leave half a register map where a
// whole one belongs: the fetch writes beside the cache and renames into it.
func TestAConcurrentFetchLeavesOneWholeFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<device><name>part</name></device>"))
	}))
	defer server.Close()

	home := t.TempDir()
	settings := Settings{Repository: server.URL, Cache: DefaultCache, Fetch: true, CacheHome: home}
	source, _ := ParseSVDSource("Vendor/part.svd")

	done := make(chan error, 4)
	for attempt := 0; attempt < 4; attempt++ {
		go func() {
			_, err := EnsureRegisterMap(source, settings)
			done <- err
		}()
	}
	for attempt := 0; attempt < 4; attempt++ {
		if err := <-done; err != nil {
			t.Fatalf("every fetch should end with the file in place: %v", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, "cache", "svd", "Vendor"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".partial") {
			t.Errorf("a partial file was left behind: %s", entry.Name())
		}
	}
}

// A repository that answers wrongly is said so, with what it said, because "the
// fetch failed" sends a person to their network stack and the answer is usually
// a file name.
func TestARepositoryThatAnswersWronglyIsSaidTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404: Not Found"))
	}))
	defer server.Close()

	home := t.TempDir()
	settings := Settings{Repository: server.URL, Cache: DefaultCache, Fetch: true, CacheHome: home}
	source, _ := ParseSVDSource("STMicro/NotThere.svd")
	_, err := EnsureRegisterMap(source, settings)
	if err == nil {
		t.Fatal("expected a missing file to be reported")
	}
	if !strings.Contains(err.Error(), "NotThere.svd") {
		t.Errorf("expected the file to be named, got %v", err)
	}
	// And nothing is left behind to be found next time.
	if _, cached := CachedRegisterMap(source, settings); cached {
		t.Error("expected a failed fetch to leave no file in the cache")
	}
}

// A machine that may not reach the network is told why, by the setting that says
// so, rather than finding out by waiting.
func TestNotFetchingIsSaidAndNamesTheSetting(t *testing.T) {
	home := t.TempDir()
	source, _ := ParseSVDSource("STMicro/Part.svd")
	for name, settings := range map[string]Settings{
		"fetching is off": {Repository: DefaultRepository, Cache: DefaultCache, Fetch: false, CacheHome: home},
		"no repository":   {Repository: "", Cache: DefaultCache, Fetch: true, CacheHome: home},
		"no cache":        {Repository: DefaultRepository, Cache: DefaultCache, Fetch: true, CacheHome: ""},
	} {
		_, err := EnsureRegisterMap(source, settings)
		if err == nil {
			t.Errorf("%s: expected the fetch to be refused", name)
			continue
		}
		if !strings.Contains(err.Error(), "not fetch") && !strings.Contains(err.Error(), "nowhere") {
			t.Errorf("%s: expected the refusal to explain itself, got %v", name, err)
		}
	}
}

// Forgetting a file is what makes a stale one be fetched again, which is the only
// thing that makes a cached answer wrong.
func TestARegisterMapCanBeForgottenAndFetchedAgain(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		_, _ = w.Write([]byte("<device><name>part</name></device>"))
	}))
	defer server.Close()

	settings := Settings{Repository: server.URL, Cache: DefaultCache, Fetch: true, CacheHome: t.TempDir()}
	source, _ := ParseSVDSource("Vendor/part.svd")

	first, err := EnsureRegisterMap(source, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := ForgetRegisterMap(source, settings); err != nil {
		t.Fatal(err)
	}
	if _, cached := CachedRegisterMap(source, settings); cached {
		t.Error("expected the file to be gone")
	}
	second, err := EnsureRegisterMap(source, settings)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || asked != 2 {
		t.Errorf("expected the file fetched again into the same place, got %q then %q after %d requests", first, second, asked)
	}
	// And forgetting one that is not there says so rather than succeeding.
	if err := ForgetRegisterMap(SVDSource{Path: "Vendor/absent.svd"}, settings); err == nil {
		t.Error("expected forgetting an absent file to be reported")
	}
}

// The component's own settings are the contract its manifest declares, and the
// values are the file it names. Read against a manifest written by this test, so
// that a change to either is a test failure rather than a surprise in the field.
func TestTheSettingsComeFromTheManifestAndItsFile(t *testing.T) {
	root := t.TempDir()
	writeSettings(t, root, `module:
  name: rho-dbg
  type: component
  version: 0.1.0

main: rho-dbg

command: dbg

settings:
  file: config/dbg.yml
  settings:
    - name: svd_repository
      default: https://example.com/default/svd
    - name: svd_branch
      default: main
    - name: svd_cache
      default: cache/svd
    - name: svd_fetch
      default: "true"
`)
	writeFile(t, root, "config/dbg.yml", `# what this machine uses
svd_repository: https://git.example.com/mirror/svd
svd_branch: v1.2
svd_cache: /var/cache/registers
svd_fetch: false
`)
	t.Setenv("RHO_MODULE_ROOT", root)

	settings := ReadSettings()
	if settings.Repository != "https://git.example.com/mirror/svd" {
		t.Errorf("expected the file's repository, got %q", settings.Repository)
	}
	if settings.Ref != "v1.2" {
		t.Errorf("expected the file's ref, got %q", settings.Ref)
	}
	if settings.Cache != "/var/cache/registers" {
		t.Errorf("expected the file's cache, got %q", settings.Cache)
	}
	if settings.Fetch {
		t.Error("expected the file's svd_fetch to be believed")
	}
	if settings.Fetchable() {
		t.Error("expected a component told not to fetch to say so")
	}

	// A file that is not there is a component nobody has changed, so the declared
	// defaults are what is in use.
	if err := os.Remove(filepath.Join(root, "config", "dbg.yml")); err != nil {
		t.Fatal(err)
	}
	settings = ReadSettings()
	if settings.Repository != "https://example.com/default/svd" || settings.Ref != "main" || !settings.Fetch {
		t.Errorf("expected the declared defaults, got %+v", settings)
	}
	if settings.Cache != DefaultCache {
		t.Errorf("expected the declared cache, got %q", settings.Cache)
	}
}

// A component with no manifest at all is running on what the built-in defaults
// say, which is the CMSIS device data repository on `main` — the values this
// component was written against.
func TestTheDefaultsAreTheCMSISDeviceDataRepository(t *testing.T) {
	t.Setenv("RHO_MODULE_ROOT", filepath.Join(t.TempDir(), "nowhere"))
	settings := ReadSettings()
	if settings.Repository != "https://github.com/cmsis-svd/cmsis-svd-data" {
		t.Errorf("expected the CMSIS device data repository, got %q", settings.Repository)
	}
	if !settings.Fetchable() {
		t.Error("expected a component with no manifest to be able to fetch")
	}
}

func writeFile(t *testing.T, root string, name string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSettings writes this component's manifest, which is where the contract the
// values follow is declared.
func writeSettings(t *testing.T, root string, content string) {
	t.Helper()
	writeFile(t, root, "module.yml", content)
}

var _ = fmt.Sprint

/* -------------------------------------------------------------------------- */
/*  The naming                                                                 */
/* -------------------------------------------------------------------------- */

// A part number is a device, a package and a temperature grade in one string; a
// vendor publishes its register map under the device. These are the spellings, in
// the order they may be tried — and the order is the design, because a less
// specific name is only reached when every more specific one published nothing.
func TestAPartNumberIsTriedUnderTheNamesAVendorPublishes(t *testing.T) {
	for _, testCase := range []struct {
		part string
		want []string
	}{
		{
			// The case this all exists for: a part with a package code, whose line the
			// vendor publishes under a wildcard.
			"STM32H753ZI",
			[]string{"stm32h753zi", "stm32h753", "stm32h753x", "stm32h7xx"},
		},
		{
			// A part with a package and a temperature grade behind it, which is the
			// shape that defeats a rule which only looks at the end of the string.
			"stm32h743zit6",
			[]string{"stm32h743zit6", "stm32h743", "stm32h743x", "stm32h7xx"},
		},
		{
			// A separator says where the device ends, which is worth more than any
			// amount of stripping: `nRF52840-QIAA` is the device and `QIAA` the
			// package, and only the hyphen says so.
			"nRF52840-QIAA",
			[]string{"nrf52840-qiaa", "nrf52840", "nrf52840qiaa"},
		},
		{
			// A vendor that publishes a device under its own bare name gets it on the
			// first try, which is the point of the order.
			"STM32F446",
			[]string{"stm32f446", "stm32f446x", "stm32f4xx"},
		},
		{"", nil},
	} {
		got := NormalizeTarget(testCase.part)
		if len(got) != len(testCase.want) {
			t.Errorf("%q:\n  expected %v\n  got      %v", testCase.part, testCase.want, got)
			continue
		}
		for index := range got {
			if got[index] != testCase.want[index] {
				t.Errorf("%q at %d: expected %q, got %q (all: %v)", testCase.part, index,
					testCase.want[index], got[index], got)
			}
		}
	}
}

// The search is against what the vendor publishes, and a name that matches nothing
// is reported rather than approximated by the nearest thing to it — a register map
// for a neighbouring part is a tree full of plausible values for a different chip.
func TestADeviceIsFoundByWhatTheVendorPublishesAndOnlyThat(t *testing.T) {
	index := Index{Devices: map[string][]string{
		"STM32H753x": {"data/STMicro/STM32H753x.svd"},
		"nRF52840":   {"data/Nordic/nRF52840.svd"},
	}}
	source, tried, err := FindDevice(index, "STM32H753ZI")
	if err != nil {
		t.Fatalf("expected the part to be found, got %v (tried %v)", err, tried)
	}
	if source.Path != "STMicro/STM32H753x.svd" {
		t.Errorf("expected the line's own file, got %q", source.Path)
	}

	// A part nothing publishes is reported, with every name it was looked under,
	// because a person reading the failure needs to know whether their part number
	// is the problem or the vendor has nothing.
	_, tried, err = FindDevice(index, "STM32H723ZG")
	if !IsUnknownDevice(err) {
		t.Fatalf("expected an unpublished part to be reported, got %v", err)
	}
	if len(tried) < 3 {
		t.Errorf("expected the names it was looked under, got %v", tried)
	}

	// A name two vendors publish is refused rather than decided, because choosing
	// between them would be a guess about which chip is on the desk.
	_, _, err = FindDevice(Index{Devices: map[string][]string{
		"shared": {"data/One/shared.svd", "data/Two/shared.svd"},
	}}, "shared")
	if !IsUnknownDevice(err) {
		t.Fatalf("expected an ambiguous name to be refused, got %v", err)
	}
	if !strings.Contains(err.Error(), "One/shared.svd") || !strings.Contains(err.Error(), "Two/shared.svd") {
		t.Errorf("expected both files to be named, got %v", err)
	}
}

// A more specific name that matches is never traded away for a general one: a part
// the vendor publishes on its own gets its own file, even when its line's file is
// also there.
func TestASpecificMatchIsNeverReplacedByAGeneralOne(t *testing.T) {
	index := Index{Devices: map[string][]string{
		"STM32H753":  {"data/STMicro/STM32H753.svd"},
		"STM32H753x": {"data/STMicro/STM32H753x.svd"},
		"STM32H7xx":  {"data/STMicro/STM32H7xx.svd"},
	}}
	source, _, err := FindDevice(index, "STM32H753ZI")
	if err != nil {
		t.Fatal(err)
	}
	if source.Device != "STM32H753.svd" {
		t.Errorf("expected the file published for this device, got %q", source.Device)
	}
}

// The repository's own list of what it holds, read once and cached — against which
// every lookup afterwards is a local question.
func TestTheIndexIsFetchedOnceAndThenReadFromTheCache(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		_, _ = w.Write([]byte(`{"truncated": false, "tree": [
		  {"path": "data/STMicro/STM32H753x.svd", "type": "blob"},
		  {"path": "data/Nordic/nRF52840.svd", "type": "blob"},
		  {"path": "README.md", "type": "blob"}
		]}`))
	}))
	defer server.Close()

	settings := Settings{Repository: server.URL, Ref: "main", Cache: DefaultCache, Fetch: true,
		CacheHome: t.TempDir()}

	index, err := loadIndexFrom(settings, server.URL+"/trees", false)
	if err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Fatalf("expected one request, got %d", asked)
	}
	if len(index.Devices) != 2 {
		t.Errorf("expected the two register maps and not the readme, got %v", index.DeviceNames())
	}

	if _, err := loadIndexFrom(settings, server.URL+"/trees", false); err != nil {
		t.Fatal(err)
	}
	if asked != 1 {
		t.Errorf("expected the cache to answer, got %d requests", asked)
	}
	// A refresh is how a person asks for the current list.
	if _, err := loadIndexFrom(settings, server.URL+"/trees", true); err != nil {
		t.Fatal(err)
	}
	if asked != 2 {
		t.Errorf("expected a refresh to ask again, got %d requests", asked)
	}
}

// A truncated list says a device does not exist because the response ran out, which
// is the one answer that must never be wrong.
func TestATruncatedListIsRefusedRatherThanUsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"truncated": true, "tree": []}`))
	}))
	defer server.Close()

	settings := Settings{Repository: server.URL, Cache: DefaultCache, Fetch: true, CacheHome: t.TempDir()}
	if _, err := loadIndexFrom(settings, server.URL+"/trees", false); err == nil {
		t.Fatal("expected a truncated list to be refused")
	}
}

// A project that names its chip's register map is believed and nothing else is
// consulted: it is the answer, written down by somebody who knows.
func TestAProjectsOwnMappingIsUsedAndNothingElseIsLookedFor(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, filepath.Join("vendor", "chip.svd"), "<device/>")

	found, err := FindRegisterMap(TargetFacts{
		Project:    &DebugMapping{SVD: map[string]string{"nucleo-h753zi": "vendor/chip.svd"}},
		TargetName: "nucleo-h753zi",
		// A board whose own spec names a different file, and whose part number would
		// resolve to a third: none of which may be consulted.
		Target: &targetspec.Target{
			MCU:   "STM32H753ZI",
			Debug: targetspec.Debug{SVD: "nowhere/at/all.svd"},
		},
		Root:     root,
		Settings: Settings{Repository: "https://example.invalid/nothing", Cache: "", Fetch: false},
	})
	if err != nil {
		t.Fatalf("expected the project's own mapping to be used, got %v", err)
	}
	if found.Path != filepath.Join(root, "vendor", "chip.svd") {
		t.Errorf("expected the file the project names, got %q", found.Path)
	}
	if !strings.Contains(found.From, "manifest") {
		t.Errorf("expected the source to say it was the project, got %q", found.From)
	}

	// And a project that names a map which is not there is said so rather than
	// worked around: carrying on would look for a different map and succeed at it.
	_, err = FindRegisterMap(TargetFacts{
		Project:    &DebugMapping{SVD: map[string]string{"nucleo-h753zi": "vendor/absent.svd"}},
		TargetName: "nucleo-h753zi",
		Target:     &targetspec.Target{MCU: "STM32H753ZI"},
		Root:       root,
	})
	if err == nil {
		t.Fatal("expected a named map that is absent to be reported")
	}
	if !strings.Contains(err.Error(), "vendor/absent.svd") {
		t.Errorf("expected the file to be named, got %v", err)
	}
}

// The refusal a session gives when the naming does not settle it: the part, every
// name it was looked under, and the place to write the answer.
func TestTheRefusalNamesThePartTheNamesAndWhereToWriteIt(t *testing.T) {
	_, tried, err := FindDevice(Index{Devices: map[string][]string{"other": {}}}, "STM32H723ZG")
	if !IsUnknownDevice(err) {
		t.Fatal("expected the part to be unpublished")
	}
	refusal := CannotIdentify("nucleo-h723zg", "STM32H723ZG", tried, err).Error()
	for _, wanted := range []string{
		"Cannot identify svd for target under debug",
		"nucleo-h723zg",
		"STM32H723ZG",
		"debug:",
		"nucleo-h723zg: vendor/cmsis_svd",
		"module.yml",
	} {
		if !strings.Contains(refusal, wanted) {
			t.Errorf("expected %q in the refusal:\n%s", wanted, refusal)
		}
	}
	for _, candidate := range tried {
		if !strings.Contains(refusal, candidate) {
			t.Errorf("expected %q to be named as tried:\n%s", candidate, refusal)
		}
	}
}
