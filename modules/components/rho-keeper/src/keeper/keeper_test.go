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
// SPDX-License-Identifier: MIT
//

package keeper

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
