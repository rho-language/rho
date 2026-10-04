//
// @file keeper_test.go
// @brief What the dashboard shows, and that it shows it honestly.
//
// A dashboard is only worth the trust a person puts in it if it says what it
// found, including the parts it could not read. These are the claims it makes
// and what backs each of them.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"rho/src/sysdep"
	"testing"

	"rho/src/deps"
)

// installed writes an installed module of a kind and version, with a manifest
// that says what it is.
func installed(t *testing.T, kind, name, version, moduleType, description string) string {
	t.Helper()
	root := filepath.Join(deps.RhoHome(), "modules", "installed", kind, name+"@"+version)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "module:\n  name: " + name + "\n  type: " + moduleType +
		"\n  version: " + version + "\n"
	if description != "" {
		manifest += "  description: " + description + "\n"
	}
	manifest += "\nmain: " + name + "\n"
	if moduleType == "plugin" {
		manifest += "\nextends: rho-doc\n\nimplements:\n  - contents\n\nhooks:\n  - index\n"
	}
	if err := os.WriteFile(filepath.Join(root, "module.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// The page says what is installed and which of it is used, because those are the
// two questions a person opens it for.
func TestTheDashboardReportsWhatIsInstalledAndWhatIsUsed(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")
	installed(t, deps.KindPlugin, "rho-doc-toc", "0.1.0", "plugin", "Adds a table of contents")

	state := Read()
	if state.Empty {
		t.Fatal("an installation with modules in it was read as empty")
	}
	if len(state.Modules) != 2 {
		t.Fatalf("read %d modules, want 2", len(state.Modules))
	}
	for _, module := range state.Modules {
		if module.Enabled {
			t.Errorf("%s is reported as enabled, and nothing enabled it", module.Name)
		}
	}
	if err := deps.Enable(deps.KindComponent, "rho-doc"); err != nil {
		t.Fatal(err)
	}
	state = Read()
	for _, module := range state.Modules {
		want := module.Name == "rho-doc"
		if module.Enabled != want {
			t.Errorf("%s enabled=%v, want %v", module.Name, module.Enabled, want)
		}
	}
}

// A plugin says what it is, and a page that shows a plugin without that is
// showing a name and a hook list — which is the question contracts exist to
// answer.
func TestAPluginIsShownWithWhatItProvides(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindPlugin, "rho-doc-toc", "0.1.0", "plugin", "Adds a table of contents")

	state := Read()
	if len(state.Modules) != 1 {
		t.Fatalf("read %d modules, want 1", len(state.Modules))
	}
	module := state.Modules[0]
	if len(module.Implements) != 1 || module.Implements[0] != "contents" {
		t.Errorf("the plugin is shown as providing %v, want [contents]", module.Implements)
	}
	if module.Extends != "rho-doc" {
		t.Errorf("the plugin is shown as extending %q", module.Extends)
	}
}

// A machine with no Rho home is a real state — someone who has not installed —
// and the page says so rather than showing an empty table that looks like an
// installation with nothing in it.
func TestAMachineWithNoInstallationIsSaidToHaveNone(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	state := Read()
	if !state.Empty {
		t.Fatalf("an empty home was read as having %d modules", len(state.Modules))
	}
	if len(state.Errors) == 0 {
		t.Error("an empty home was read without saying anything about it")
	}
}

// Several versions may be installed and the page shows the one that is used, and
// that the others are there — showing one version makes it look like the only
// one there is.
func TestThePageDoesNotHideTheOtherVersionsInstalled(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-log", "0.1.0", "component", "Logger")
	installed(t, deps.KindComponent, "rho-log", "0.2.0", "component", "Logger")
	if err := deps.SetVersionEnabled(deps.KindComponent, "rho-log", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	state := Read()
	if len(state.Modules) != 1 {
		t.Fatalf("read %d entries for one module, want 1", len(state.Modules))
	}
	module := state.Modules[0]
	if module.Version != "0.1.0" {
		t.Errorf("the page shows %q, want the version that is used", module.Version)
	}
	if len(module.InstalledVersions) != 2 {
		t.Errorf("the page knows of %d versions, want both", len(module.InstalledVersions))
	}
}

// A record that cannot be read is reported. Every answer below it would
// otherwise come from a state nobody can see.
func TestAStateThatCannotBeReadIsReported(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	installed(t, deps.KindComponent, "rho-log", "0.1.0", "component", "Logger")
	if err := os.MkdirAll(filepath.Join(home, "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deps.EnabledPath(), []byte("component:\n  - not a map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := Read()
	if len(state.Errors) == 0 {
		t.Error("a record that could not be read was reported as nothing")
	}
}

// A module installed but not describable is still listed, with the reason. A list
// that quietly leaves things out is a list nobody can trust to be complete.
func TestAModuleThatCannotBeDescribedIsStillListed(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	root := installed(t, deps.KindComponent, "rho-broken", "0.1.0", "component", "")
	if err := os.Remove(filepath.Join(root, "module.yml")); err != nil {
		t.Fatal(err)
	}
	state := Read()
	if len(state.Modules) != 1 {
		t.Fatalf("a module with no manifest was not listed: %+v", state.Modules)
	}
	if state.Modules[0].Readable == "" {
		t.Error("a module that could not be read is listed without saying why")
	}
}

// The page and the API read the same state, so a script and a person cannot be
// told different things.
func TestThePageAndTheAPIAgree(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")
	installed(t, deps.KindPlugin, "rho-doc-toc", "0.1.0", "plugin", "Adds a table of contents")

	server := httptest.NewServer(Handler(nil, false, nil, ""))
	defer server.Close()

	// The page is a shell: the data arrives over the stream after it loads, so
	// what the server renders is the frame and not a table that would be
	// correct only until somebody changed the machine.
	page := readBody(t, server.URL+"/")
	// The page is a shell with sections, so what it must carry is the frame and
	// where its data comes from — not the data, which arrives over the stream.
	for _, want := range []string{
		"/static/keeper.css", "/static/keeper.js",
		"/api/v1/events", "/api/v1/docs",
		`data-view="overview"`, `data-view="modules"`, `data-nav="contracts"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not have %s", want)
		}
	}
	// A file it says it needs and cannot read would be a page that is missing
	// half of itself, and the test would not see it.
	for _, want := range []string{"/static/keeper.css", "/static/keeper.js"} {
		body := readBody(t, server.URL+want)
		if len(body) == 0 {
			t.Errorf("%s is empty", want)
		}
	}

	raw := readBody(t, server.URL+"/api/v1/state")
	var answer Snapshot
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatalf("the API did not answer with the state: %v\n%s", err, raw)
	}
	if len(answer.State.Modules) != 2 {
		t.Errorf("the API reports %d modules, the test installed 2", len(answer.State.Modules))
	}
	// The stream carries the same reading as the request does, so a page left
	// open and a page just loaded cannot be told apart from the data.
	if answer.Read == "" {
		t.Error("the API does not say when it was read")
	}
}

// The status command is the same reading without the page, so a person who has
// not opened a browser is told the same thing.
func TestStatusSaysTheSameThingAsThePage(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")
	if err := deps.Enable(deps.KindComponent, "rho-doc"); err != nil {
		t.Fatal(err)
	}
	summary := Summary(Read())
	for _, want := range []string{"rho-doc@0.1.0", "enabled", "1 installed, 1 enabled"} {
		if !strings.Contains(summary, want) {
			t.Errorf("the summary does not say %q:\n%s", want, summary)
		}
	}
}

// readBody is a request whose body a test reads, failing the test rather than
// returning an error a caller has to remember to check.
func readBody(t *testing.T, url string) string {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s answered %d", url, response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

/* -------------------------------------------------------------------------- */
/*  The stream                                                                 */
/* -------------------------------------------------------------------------- */

// A person leaves this page open, somebody changes something in a terminal, and
// the page has to stop showing a machine that no longer exists. That is what the
// stream is for, and it is the only way the page is ever kept honest: every
// render comes from a state the server sent.
func TestTheStreamCarriesTheStateOnConnect(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")

	server := httptest.NewServer(Handler(nil, false, nil, ""))
	defer server.Close()

	response, err := http.Get(server.URL + "/api/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("the stream answered %q, want an event stream", got)
	}

	// The first thing sent is the state as it is, so a page that connects shows
	// something without waiting for something to change.
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "event: state") {
		t.Fatalf("the stream sent %q first, want a state event", line)
	}
	data, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(data, "data: ") {
		t.Fatalf("the event carried %q, want data", data)
	}
	var answer Snapshot
	if err := json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(data), "data: ")), &answer); err != nil {
		t.Fatalf("the event did not carry the state: %v", err)
	}
	if len(answer.State.Modules) != 1 || answer.State.Modules[0].Name != "rho-doc" {
		t.Errorf("the event carried %+v, want the one module that is installed", answer.State.Modules)
	}
}

// A change made anywhere reaches whoever is watching. It is the difference
// between a dashboard about a machine and a screenshot of one.
func TestAChangeReachesTheStream(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")

	hub := newHub()
	server := httptest.NewServer(Handler(nil, false, hub, ""))
	defer server.Close()

	response, err := http.Get(server.URL + "/api/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if _, err := reader.ReadString('\n'); err != nil { // the first event
		t.Fatal(err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// Something changed, the way a person in a terminal would. The change is
	// made before the read rather than beside it, because a read that blocks
	// waiting for a push that is racing it would test the scheduler.
	if action := Enable("rho-doc", ""); !action.OK {
		t.Fatalf("the change did not happen: %s", action.Error)
	}
	hub.changed()

	// The next event carries the new state, and it says the module is on.
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var answer Snapshot
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &answer); err != nil {
			t.Fatal(err)
		}
		if len(answer.State.Modules) == 1 && answer.State.Modules[0].Enabled {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Error("a change was not pushed to the stream")
}

/* -------------------------------------------------------------------------- */
/*  Changing something                                                          */
/* -------------------------------------------------------------------------- */

// An action is the same operation the CLI performs, through the same functions.
// A dashboard that turned a module on by its own means would be a second
// implementation of a rule the toolchain already has, and the two would disagree
// about what enabling a version means.
func TestAnActionIsWhatTheCommandWouldHaveDone(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")

	action := Enable("rho-doc", "")
	if !action.OK {
		t.Fatalf("enabling did not happen: %s", action.Error)
	}
	// The command is the one a person would have typed, and it is filled in
	// whether it worked or not.
	if action.Command != "rho module enable rho-doc@0.1.0" {
		t.Errorf("the action said %q, want the command it ran", action.Command)
	}
	// And the machine is what that command would have done to it.
	state, err := deps.LoadEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !state.IsEnabled(deps.KindComponent, "rho-doc", "0.1.0") {
		t.Error("the action did not enable the module")
	}

	off := Disable("rho-doc")
	if !off.OK {
		t.Fatalf("disabling did not happen: %s", off.Error)
	}
	if off.Command != "rho module disable rho-doc" {
		t.Errorf("the action said %q, want the command it ran", off.Command)
	}
	// It stays installed: turning something off is not the same as removing it,
	// and turning it back on should not mean finding it again.
	if len(deps.InstalledVersions(deps.KindComponent, "rho-doc")) != 1 {
		t.Error("disabling removed the module")
	}
}

// A change that did not happen is still answered with the command, because a
// person who was refused is precisely the one who wants to see what was tried.
func TestARefusedActionStillSaysWhatItTried(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())

	action := Enable("rho-absent", "")
	if action.OK {
		t.Fatal("a module that is not installed was enabled")
	}
	if action.Command == "" {
		t.Error("a refused action did not say what it tried")
	}
	if action.Error == "" {
		t.Error("a refused action did not say why")
	}
}

// An action over HTTP is the same action, and the page is told what ran.
func TestTheActionRouteRunsAndAnswersWithTheCommand(t *testing.T) {
	t.Setenv("RHO_HOME", t.TempDir())
	installed(t, deps.KindComponent, "rho-doc", "0.1.0", "component", "Documentation generator")

	server := httptest.NewServer(Handler(nil, false, nil, ""))
	defer server.Close()

	response, err := http.Post(server.URL+"/api/v1/modules/rho-doc/enable", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the action answered %d", response.StatusCode)
	}
	var action Action
	if err := json.NewDecoder(response.Body).Decode(&action); err != nil {
		t.Fatal(err)
	}
	if !action.OK || action.Command == "" {
		t.Errorf("the route answered %+v, want the action that ran", action)
	}
	// The state the page would then be shown is the state on disk.
	if got := len(Read().Modules); got != 1 {
		t.Errorf("after the action the page would show %d modules, want 1", got)
	}
}

// The system's requirements are read from the doctor rather than worked out here,
// so a dashboard and a terminal cannot disagree about what the machine has. What is
// tested is the *acting*: what a plan would do, and which recipe is chosen for this
// host.
func TestAPlanUsesThisHostsOwnManagerAndSkipsTheRest(t *testing.T) {
	current := detectHost()
	if current.Manager == "" {
		t.Skip("this machine has no package manager, so there is nothing to plan")
	}
	report := sysdep.Report{Results: []sysdep.CheckResult{
		{
			Module: "rho-dbg", Dep: "openocd", Kind: sysdep.KindExecutable, Required: true,
			Status: sysdep.StatusMissing, Message: "not installed",
			// Both this host's manager and another host's: only one may be used.
			Install: map[string]string{current.Manager: "openocd-tool", "a-manager-that-does-not-exist": "x"},
		},
	}}
	plan := PlanFor(report)
	if len(plan.Steps) != 1 {
		t.Fatalf("expected one step, got %d: %s", len(plan.Steps), plan.Text())
	}
	if plan.Steps[0].Manager != current.Manager {
		t.Errorf("expected this host's manager, got %q", plan.Steps[0].Manager)
	}
	if strings.Contains(plan.Steps[0].Line(), "a-manager-that-does-not-exist") {
		t.Errorf("expected another host's manager left out: %s", plan.Steps[0].Line())
	}
	if current.Distro != "" && !strings.Contains(plan.Text(), current.Distro) {
		t.Errorf("expected the plan to say which machine it was made for:\n%s", plan.Text())
	}
}

// A fetch is preferred over a package manager, always: it needs no privileges, it
// belongs to the person rather than to the machine, and it does not change when the
// machine's package manager does.
func TestAFetchIsPreferredOverAPackageManager(t *testing.T) {
	current := detectHost()
	report := sysdep.Report{Results: []sysdep.CheckResult{
		{
			Module: "rho-dbg", Dep: "arm-none-eabi-gdb", Kind: sysdep.KindExecutable, Required: true,
			Status: sysdep.StatusMissing, Message: "not installed",
			Install: map[string]string{current.Manager: "gdb-multiarch"},
			Fetch:   &sysdep.FetchRecipe{URL: "https://example.com/gdb.tar.gz", Version: "14.2"},
		},
	}}
	plan := PlanFor(report)
	if len(plan.Steps) != 1 || plan.Steps[0].Manager != "fetch" {
		t.Fatalf("expected the fetch to be preferred, got:\n%s", plan.Text())
	}
	if plan.NeedsRoot {
		t.Error("expected a fetch to need no privileges")
	}
}

// What a plan skips is reported with the reason. A plan that quietly omits what it
// cannot do reads as a plan with nothing left to do.
func TestWhatAPlanCannotDoIsSaidWithItsReason(t *testing.T) {
	report := sysdep.Report{Results: []sysdep.CheckResult{
		{
			Module: "rho-dbg", Dep: "a-program", Kind: sysdep.KindExecutable, Required: true,
			Status: sysdep.StatusMissing, Message: "not installed",
			Install: map[string]string{"manual": "https://example.com/downloads"},
		},
		{
			Module: "rho-dbg", Dep: "A_VARIABLE", Kind: sysdep.KindEnv, Required: true,
			Status: sysdep.StatusMissing, Message: "not set",
		},
		{
			Module: "rho-dbg", Dep: "a-rule", Kind: sysdep.KindUdev, Required: true,
			Status: sysdep.StatusMissing, Message: "no rule",
			FoundPath: "/etc/udev/rules.d/x.rules", RuleSource: "/opt/x.rules",
		},
	}}
	text := PlanFor(report).Text()
	for _, wanted := range []string{"not in a package manager", "environment variable", "host permission"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("expected %q in the plan:\n%s", wanted, text)
		}
	}
}

// A rule is the one thing here that needs root, and it is fixable only when the
// module carries the file — because installing it is a copy of something a person can
// read first.
func TestARuleIsFixableOnlyWithItsOwnFile(t *testing.T) {
	withSource := sysdep.CheckResult{
		Kind: sysdep.KindUdev, Status: sysdep.StatusMissing, Required: true,
		FoundPath: "/etc/udev/rules.d/x.rules", RuleSource: "/opt/x.rules",
	}
	if !Fixable(withSource) {
		t.Error("expected a rule whose file is known to be fixable")
	}
	withoutSource := withSource
	withoutSource.RuleSource = ""
	if Fixable(withoutSource) {
		t.Error("expected a rule with no file not to be fixable: there is nothing to copy")
	}
	satisfied := withSource
	satisfied.Status = sysdep.StatusOK
	if Fixable(satisfied) {
		t.Error("expected a satisfied requirement not to be offered a fix")
	}
	if FixCommand(withSource) == "" {
		t.Error("expected a command for a fixable requirement")
	}
}

// The root notice is the same one the CLI gives, and it can only ever be a notice:
// a container is root and a pipeline is root, and a warning that reads like a
// refusal is one people learn to scroll past.
func TestTheRootNoticeIsANoticeAndNotARefusal(t *testing.T) {
	out := &bytes.Buffer{}
	CheckNotRoot(out)
	if strings.Contains(strings.ToLower(out.String()), "refus") ||
		strings.Contains(strings.ToLower(out.String()), "cannot run") {
		t.Errorf("expected a notice and not a refusal: %q", out.String())
	}
}

// The install stream is where a page turns into something that changes a machine, so
// two things about it are tested: that loading it without being asked to does
// nothing, and that a stream carries a plan before it carries any output.
//
// The first is the safety property and the reason the question travels in the URL:
// an `EventSource` cannot send anything but a GET, so a page that loads this must
// be unable to start a download or reach for privileges by being made to open it.
func TestTheInstallStreamDoesNothingUntilItIsAskedTo(t *testing.T) {
	withInstallReport(sysdep.Report{Results: []sysdep.CheckResult{{
		Module: "rho-dbg", Dep: "arm-none-eabi-gdb", Kind: sysdep.KindExecutable,
		Required: true, Status: sysdep.StatusMissing, Message: "not installed",
		Install: map[string]string{"apt": "gdb-multiarch"},
	}}}, "", func() {
		server := httptest.NewServer(Handler(nil, false, nil, ""))
		defer server.Close()

		// No `confirm`: what it would do, and nothing of it.
		response, err := http.Get(server.URL + "/api/v1/system/install/rho-dbg/arm-none-eabi-gdb")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body := readAll(t, response)

		if got := response.Header.Get("Content-Type"); got != "text/event-stream" {
			t.Errorf("expected a stream, got %q", got)
		}
		if !strings.Contains(body, "event: plan") {
			t.Errorf("expected the plan even when nothing is asked for:\n%s", body)
		}
		if !strings.Contains(body, "not asked to") {
			t.Errorf("expected it to say it did nothing:\n%s", body)
		}
		// And no `confirm`, no plan is turned into a command that runs.
		if strings.Contains(body, "\"ok\":true") {
			t.Errorf("expected nothing installed:\n%s", body)
		}
	})
}

// With the question asked, the stream answers the same way — and the person watching
// sees the plan before anything is done, which is the whole reason for sending one.
func TestTheInstallStreamAnswersWithThePlanFirst(t *testing.T) {
	withInstallReport(sysdep.Report{Results: []sysdep.CheckResult{{
		Module: "rho-dbg", Dep: "arm-none-eabi-gdb", Kind: sysdep.KindExecutable,
		Required: true, Status: sysdep.StatusMissing, Message: "not installed",
	}}}, "", func() {
		server := httptest.NewServer(Handler(nil, false, nil, ""))
		defer server.Close()

		response, err := http.Get(server.URL +
			"/api/v1/system/install/rho-dbg/arm-none-eabi-gdb?confirm=yes")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body := readAll(t, response)

		planAt := strings.Index(body, "event: plan")
		doneAt := strings.Index(body, "event: done")
		if planAt < 0 || doneAt < 0 {
			t.Fatalf("expected a plan and an ending, got:\n%s", body)
		}
		if planAt > doneAt {
			t.Errorf("expected the plan before the ending:\n%s", body)
		}
	})
}

// A requirement that is already satisfied says so and finishes, rather than
// installing a second copy of something the machine has.
func TestTheStreamLeavesASatisfiedRequirementAlone(t *testing.T) {
	withInstallReport(sysdep.Report{Results: []sysdep.CheckResult{{
		Module: "rho-target", Dep: "cmake", Kind: sysdep.KindExecutable,
		Required: true, Status: sysdep.StatusOK, FoundPath: "/usr/bin/cmake",
		Message: "4.4.3 at /usr/bin/cmake",
	}}}, "", func() {
		server := httptest.NewServer(Handler(nil, false, nil, ""))
		defer server.Close()

		response, err := http.Get(server.URL +
			"/api/v1/system/install/rho-target/cmake?confirm=yes")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body := readAll(t, response)
		if !strings.Contains(body, "already satisfied") {
			t.Errorf("expected it to say so:\n%s", body)
		}
		if strings.Contains(body, "event: plan") {
			t.Errorf("expected no plan for something already satisfied:\n%s", body)
		}
	})
}

// A name that is not declared says what is, because "not found" on its own sends a
// person through the report looking for a spelling that is not there.
func TestTheStreamSaysWhatAModuleDoesNeed(t *testing.T) {
	withInstallReport(sysdep.Report{Results: []sysdep.CheckResult{{
		Module: "rho-dbg", Dep: "openocd", Kind: sysdep.KindExecutable,
		Required: true, Status: sysdep.StatusOK, Message: "installed",
	}}}, "", func() {
		server := httptest.NewServer(Handler(nil, false, nil, ""))
		defer server.Close()

		response, err := http.Get(server.URL + "/api/v1/system/install/rho-dbg/no-such-thing?confirm=yes")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body := readAll(t, response)
		if !strings.Contains(body, "no such requirement") {
			t.Errorf("expected the failure to say so:\n%s", body)
		}
	})
}

func readAll(t *testing.T, response *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The format a recipe names is compared by its words and not by a suffix that never
// matched: a spec writes `tar.xz`, and the suffix of that is `.xz`. Getting this
// wrong sends every vendor's xz release to the branch that treats the archive as
// the program, which looks like a broken download rather than a string comparison
// that was never true.
func TestTheArchiveFormatIsReadFromWhatASpecWrites(t *testing.T) {
	for _, given := range []string{"tar.gz", ".tar.gz", "tgz", "gz"} {
		if got := archiveKind(given); got != "tar.gz" {
			t.Errorf("%q should be a gzip tar, got %q", given, got)
		}
	}
	for _, given := range []string{"tar.xz", ".tar.xz", "txz", "xz"} {
		if got := archiveKind(given); got != "tar.xz" {
			t.Errorf("%q should be an xz tar, got %q", given, got)
		}
	}
	for _, given := range []string{"tar.bz2", ".tar.bz2", "bz2"} {
		if got := archiveKind(given); got != "tar.bz2" {
			t.Errorf("%q should be a bzip2 tar, got %q", given, got)
		}
	}
	if got := archiveKind(".zip"); got != "zip" {
		t.Errorf(".zip should be a zip, got %q", got)
	}
	// Nothing said at all is a gzip tar, because that is what a plain `.tar.gz`
	// is and what an unset archive is most often taken to mean.
	if got := archiveKind(""); got != "tar.gz" {
		t.Errorf("expected nothing to mean a gzip tar, got %q", got)
	}
}

// A recipe names both the directory an archive unpacks into and the binary inside
// it, because a vendor's archive has a top-level directory — and a lookup that
// looked for only the binary would read a fetched toolchain as missing, and offer
// to fetch it again.
func TestARecipeNamesTheDirectoryAndTheBinaryWithinIt(t *testing.T) {
	dep := sysdep.ExecutableDep{
		Name: "arm-none-eabi-gdb",
		Fetch: &sysdep.FetchRecipe{
			Dir:    "arm-gnu-toolchain-14.2.rel1-x86_64-arm-none-eabi",
			Binary: "bin/arm-none-eabi-gdb",
		},
	}
	want := "arm-gnu-toolchain-14.2.rel1-x86_64-arm-none-eabi/bin/arm-none-eabi-gdb"
	if got := sysdep.FetchedRelative(dep); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
	// A recipe that names a directory and no binary still looks in the bin the
	// vendor's own layout puts there.
	dirOnly := sysdep.ExecutableDep{
		Name:  "thing",
		Fetch: &sysdep.FetchRecipe{Dir: "thing-1.0"},
	}
	if got := sysdep.FetchedRelative(dirOnly); got != "thing-1.0/bin/thing" {
		t.Errorf("expected the default bin inside the directory, got %q", got)
	}
	// And one that names neither is the plain shape.
	plain := sysdep.ExecutableDep{Name: "thing"}
	if got := sysdep.FetchedRelative(plain); got != "bin/thing" {
		t.Errorf("expected bin/thing, got %q", got)
	}
}
