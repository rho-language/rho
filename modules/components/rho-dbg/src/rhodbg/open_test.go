//
// @file open_test.go
// @brief Tests for the probe, and for which engine a session runs on.
//
// Both are about the same thing from a person's point of view: what happens before a
// session can start. A board behind a probe that was never started reports a
// connection that was refused, and a plugin that offers a debugger for a board but
// loses to the built-in one would leave a person believing they had installed it
// wrongly.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package rhodbg

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"rho/src/sysdep"
	"strconv"
	"strings"
	"testing"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  The probe                                                                  */
/* -------------------------------------------------------------------------- */

// A free port, and a listener on it, so a probe can be started against something
// real rather than against nothing.
func listeningOn(t *testing.T) (int, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	stopped := make(chan struct{})
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				close(stopped)
				return
			}
			_ = connection.Close()
		}
	}()
	return port, func() {
		_ = listener.Close()
		<-stopped
	}
}

func TestAProbeIsStartedAndWaitedForBeforeAnythingConnectsToIt(t *testing.T) {
	port, stop := listeningOn(t)
	defer stop()

	probe, err := StartProbe(ProbeSpec{Program: writeProbe(t), Port: strconv.Itoa(port)})
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Stop()

	if probe.Port != strconv.Itoa(port) {
		t.Errorf("expected the port it was told to listen on, got %q", probe.Port)
	}
	if !strings.Contains(probe.Describe(), "-s "+strconv.Itoa(port)) {
		t.Errorf("expected the port to be handed to the server, got %q", probe.Describe())
	}
}

// A board that is debugged in place has no probe, and that is answered with a probe
// that is not there rather than an error: the debugger connects to the target
// itself in that case, which is how a program on this machine is debugged.
func TestABoardWithNoProbeServerHasNoProbeToStart(t *testing.T) {
	probe, err := StartProbe(ProbeSpec{})
	if err != nil {
		t.Fatalf("expected no error for a target with no probe server, got %v", err)
	}
	if probe.Port != DefaultPort {
		t.Errorf("expected the default port to be reported, got %q", probe.Port)
	}
	if describe := probe.Describe(); describe != "none" {
		t.Errorf("expected no probe, got %q", describe)
	}
	if err := probe.Stop(); err != nil {
		t.Errorf("expected stopping a probe that was not there to be nothing, got %v", err)
	}
}

// A probe that exits instead of listening is a different problem from one that is
// still starting, and the two are told apart — because waiting out the whole timeout
// on a process that has already exited turns a two-second failure into a twenty.
func TestAProbeThatExitsIsSaidToHaveExited(t *testing.T) {
	_, err := StartProbe(ProbeSpec{Program: writeExiting(t)})
	if err == nil {
		t.Fatal("expected the probe that exited to be reported")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("expected the failure to say the probe exited, got %v", err)
	}
	if !IsNoProbe(err) {
		t.Errorf("expected this to be told apart from a debugger that failed, got %v", err)
	}
}

// A probe that says why it cannot find a board is reporting the whole reason, and
// that reason belongs in the failure. It is not in any error raised elsewhere.
func TestWhatTheProbeSaidIsPartOfTheFailure(t *testing.T) {
	_, err := StartProbe(ProbeSpec{Program: writeComplaining(t)})
	if err == nil {
		t.Fatal("expected the failure")
	}
	if !strings.Contains(err.Error(), "no CMSIS-DAP device found") {
		t.Errorf("expected what the probe said in the failure, got %v", err)
	}
}

func TestAProbeThatIsNotInstalledSaysSoByName(t *testing.T) {
	_, err := StartProbe(ProbeSpec{Program: "openocd-that-is-not-installed"})
	if err == nil {
		t.Fatal("expected the missing server to be reported")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("expected the failure to name the program, got %v", err)
	}
}

func TestAPortThatIsNotAPortIsRefused(t *testing.T) {
	_, err := StartProbe(ProbeSpec{Program: writeExiting(t), Port: "not-a-port"})
	if err == nil {
		t.Fatal("expected a port that is not one to be refused")
	}
}

// A probe that ignores being told to quit is killed rather than waited for: a
// session that will not end is worse than one that ends with the port briefly busy.
func TestStoppingAProbeIsBoundedAndIdempotent(t *testing.T) {
	_, stop := listeningOn(t)
	defer stop()

	probe, err := StartProbe(ProbeSpec{Program: writeStubborn(t)})
	if err != nil {
		t.Fatal(err)
	}
	first := time.Now()
	if err := probe.Stop(); err != nil {
		t.Fatalf("expected it to stop, got %v", err)
	}
	if elapsed := time.Since(first); elapsed > 5*time.Second {
		t.Errorf("expected stopping to be bounded, took %s", elapsed)
	}
	// And stopping again is nothing, because the caller stops what it started without
	// having to remember whether that was a process or a fact.
	if err := probe.Stop(); err != nil {
		t.Errorf("expected the second stop to be nothing, got %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Choosing an engine                                                         */
/* -------------------------------------------------------------------------- */

// The built-in engine is always there and always works: a machine with no plugin
// enabled can still debug a board.
func TestTheBuiltinEngineIsAlwaysAvailable(t *testing.T) {
	available := Engines{Builtin: true}
	spec, err := available.Choose("pico", "")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != BuiltinGDB || spec.Kind != KindDriver {
		t.Errorf("expected the built-in GDB engine, got %+v", spec)
	}
	if names := available.Names(); len(names) != 1 || names[0] != BuiltinGDB {
		t.Errorf("expected only the built-in engine, got %v", names)
	}
}

// A plugin that offers an engine for a board must beat the built-in one for that
// board. Otherwise enabling it would change nothing, and there would be no way to
// tell whether that was a bug or a plugin that was not doing what it said.
func TestAPluginEngineForABoardBeatsTheBuiltInOne(t *testing.T) {
	available := Engines{
		Builtin: true,
		FromPlugins: []EngineSpec{
			{Name: "lldb-dap", Kind: KindAdapter, Targets: []string{"stm32f4"}, Program: "lldb-dap"},
			{Name: "probe-rs", Kind: KindAdapter, Program: "probe-rs"},
		},
	}

	spec, err := available.Choose("stm32f4", "")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "lldb-dap" {
		t.Errorf("expected the plugin's engine for that board, got %q", spec.Name)
	}

	// A fallback does not: it would then beat the built-in one for every board, and
	// enabling a plugin would silently change what every session does.
	spec, err = available.Choose("pico", "")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != BuiltinGDB {
		t.Errorf("expected the built-in engine for a board nothing claims, got %q", spec.Name)
	}
}

// Two engines for one board is a decision, not an accident, and it is resolved the
// same way every time rather than by the order the plugins happened to load in.
func TestTwoEnginesForOneBoardResolveTheSameWayEveryTime(t *testing.T) {
	first := Engines{Builtin: true, FromPlugins: []EngineSpec{
		{Name: "zeta", Kind: KindAdapter, Targets: []string{"board"}, Program: "zeta"},
		{Name: "alpha", Kind: KindAdapter, Targets: []string{"board"}, Program: "alpha"},
	}}
	second := Engines{Builtin: true, FromPlugins: []EngineSpec{
		{Name: "alpha", Kind: KindAdapter, Targets: []string{"board"}, Program: "alpha"},
		{Name: "zeta", Kind: KindAdapter, Targets: []string{"board"}, Program: "zeta"},
	}}
	chosen, err := first.Choose("board", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := second.Choose("board", "")
	if err != nil {
		t.Fatal(err)
	}
	if chosen.Name != other.Name {
		t.Errorf("expected the same engine both times, got %q and %q", chosen.Name, other.Name)
	}
}

// An engine named by a person is taken at their word. Getting GDB when they wrote
// `lldb` would leave them looking for a feature they had every reason to expect.
func TestAnEngineNamedInTheConfigurationIsUsedOrReported(t *testing.T) {
	available := Engines{Builtin: true, FromPlugins: []EngineSpec{
		{Name: "lldb-dap", Kind: KindAdapter, Program: "lldb-dap"},
	}}
	spec, err := available.Choose("pico", "lldb-dap")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Name != "lldb-dap" {
		t.Errorf("expected the named engine, got %q", spec.Name)
	}
	// And the built-in one by its own name, because that is a name a launch
	// configuration may carry too.
	spec, err = available.Choose("pico", BuiltinGDB)
	if err != nil || spec.Kind != KindDriver {
		t.Errorf("expected the built-in engine by name, got %+v (%v)", spec, err)
	}
	// One that is not there is reported rather than quietly replaced.
	_, err = available.Choose("pico", "not-installed")
	if err == nil {
		t.Fatal("expected an engine that is not there to be reported")
	}
	if !strings.Contains(err.Error(), "not-installed") {
		t.Errorf("expected the failure to name what was asked for, got %v", err)
	}
}

// A plugin's spec is a program, and a spec that is not usable is refused where it
// arrives rather than when a session tries to start it.
func TestAnEngineSpecIsCheckedWhereItArrives(t *testing.T) {
	for name, spec := range map[string]EngineSpec{
		"no name":    {Kind: KindAdapter, Program: "x"},
		"no program": {Name: "x", Kind: KindAdapter},
		"wrong kind": {Name: "x", Kind: "in-process", Program: "x"},
	} {
		if err := spec.Validate(); err == nil {
			t.Errorf("expected %s to be refused", name)
		}
	}
	good := EngineSpec{Name: "lldb-dap", Kind: KindAdapter, Program: "lldb-dap"}
	if err := good.Validate(); err != nil {
		t.Errorf("expected a usable spec, got %v", err)
	}
	// A spec with no kind is an adapter, since that is the only kind a plugin can
	// provide.
	if err := (EngineSpec{Name: "x", Program: "x"}).Validate(); err != nil {
		t.Errorf("expected a spec with no kind to be read as an adapter, got %v", err)
	}
}

// One spec can serve two boards, which is what makes a plugin a plugin rather than a
// board description: the substitution happens where the program is run.
func TestAnEngineSpecIsExpandedForTheSessionItServes(t *testing.T) {
	spec := EngineSpec{
		Name:    "probe-rs",
		Kind:    KindAdapter,
		Program: "${program}-symbols",
		Args:    []string{"attach", "--chip", "${target}"},
	}
	program, arguments := spec.Expansion("nrf52840", "build/app.elf")
	if program != "build/app.elf-symbols" {
		t.Errorf("expected the program to be substituted, got %q", program)
	}
	if strings.Join(arguments, " ") != "attach --chip nrf52840" {
		t.Errorf("expected the target to be substituted, got %v", arguments)
	}
}

/* -------------------------------------------------------------------------- */
/*  Finding a board's register map                                            */
/* -------------------------------------------------------------------------- */

// The register map is looked for in two places, in a fixed order, because a search
// that finds a file somewhere unexpected shows the registers of a chip this is not.
func TestARegisterMapIsFoundWhereItWasSaidToBe(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "build")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path string) {
		if err := os.WriteFile(path, []byte("<device/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	program := filepath.Join(nested, "app.elf")

	// A bare name beside the program, which is where a project that vendors one has
	// it.
	beside := filepath.Join(nested, "rp2040.svd")
	write(beside)
	found, ok := RegisterMapPath("rp2040.svd", root, program, nil)
	if !ok || found != beside {
		t.Errorf("expected the map beside the program, got %q (%v)", found, ok)
	}

	// A path with a separator in it is a path, read from the project.
	vendored := filepath.Join(root, "vendor", "chip.svd")
	if err := os.MkdirAll(filepath.Dir(vendored), 0o755); err != nil {
		t.Fatal(err)
	}
	write(vendored)
	found, ok = RegisterMapPath("vendor/chip.svd", root, program, nil)
	if !ok || found != vendored {
		t.Errorf("expected the path from the project, got %q (%v)", found, ok)
	}

	// A bare name nowhere near the program is looked for in the project too, so a
	// file at the root of a project still works.
	write(filepath.Join(root, "at-root.svd"))
	found, ok = RegisterMapPath("at-root.svd", root, program, nil)
	if !ok || found != filepath.Join(root, "at-root.svd") {
		t.Errorf("expected the map at the project's root, got %q (%v)", found, ok)
	}

	// And one that is not there is reported rather than guessed at.
	if _, ok := RegisterMapPath("absent.svd", root, program, nil); ok {
		t.Error("expected a register map that is not there to be reported")
	}
	if _, ok := RegisterMapPath("", root, program, nil); ok {
		t.Error("expected no map to be reported when none was named")
	}
	// A directory is not a register map.
	if _, ok := RegisterMapPath("build", root, program, nil); ok {
		t.Error("expected a directory not to count as a register map")
	}
}

/* -------------------------------------------------------------------------- */
/*  Programs                                                                   */
/* -------------------------------------------------------------------------- */

func TestAMissingProgramIsNamedInTheFailure(t *testing.T) {
	// "exit status 127" says nothing about which program was missing, and a session
	// that reports it sends a person to look at the wrong component.
	_, err := ResolveProgram("gdb-that-is-not-installed")
	if err == nil {
		t.Fatal("expected the missing program to be reported")
	}
	if !strings.Contains(err.Error(), "gdb-that-is-not-installed") {
		t.Errorf("expected the program to be named, got %v", err)
	}
	if _, err := ResolveProgram(""); err == nil {
		t.Error("expected an empty name to be refused")
	}
}

func TestAProgramIsFoundOnThePath(t *testing.T) {
	root := t.TempDir()
	name := "a-tool"
	if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := ResolveProgram(name, root)
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(root, name) {
		t.Errorf("expected the tool where it was put, got %q", found)
	}
	// And one that is not executable is not found: a file with the right name and no
	// permission is not the tool.
	if err := os.WriteFile(filepath.Join(root, "not-executable"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProgram("not-executable", root); err == nil {
		t.Error("expected a file that cannot be run not to count as installed")
	}
}

/* -------------------------------------------------------------------------- */
/*  Fake servers                                                               */
/* -------------------------------------------------------------------------- */

// writeProbe writes a program that listens on the port it is told to, the way a
// probe server does.
func writeProbe(t *testing.T) string {
	t.Helper()
	return writeScript(t, "probe", `#!/bin/sh
# -s <port> is the port to listen on; -f and -c are the configuration files.
while [ $# -gt 0 ]; do
  case "$1" in
    -s) shift; PORT="$1";;
  esac
  shift
done
echo "Open On-Chip Debugger 0.12.0"
exec nc -l 127.0.0.1 "$PORT"
`)
}

func writeExiting(t *testing.T) string {
	t.Helper()
	return writeScript(t, "exiting", `#!/bin/sh
echo "Error: init mode failed (unable to connect to the target)" >&2
exit 1
`)
}

func writeComplaining(t *testing.T) string {
	t.Helper()
	return writeScript(t, "complaining", `#!/bin/sh
echo "Error: libusb_open() failed with LIBUSB_ERROR_NOT_FOUND" >&2
echo "no CMSIS-DAP device found" >&2
exit 1
`)
}

func writeStubborn(t *testing.T) string {
	t.Helper()
	return writeScript(t, "stubborn", `#!/bin/sh
PORT=3333
while [ $# -gt 0 ]; do
  case "$1" in
    -s) shift; PORT="$1";;
  esac
  shift
done
nc -l 127.0.0.1 "$PORT" &
sleep 30
`)
}

// writeScript writes an executable shell script and returns its path.
func writeScript(t *testing.T, name string, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

/* -------------------------------------------------------------------------- */
/*  The GDB engine against a debugger that is not GDB                          */
/* -------------------------------------------------------------------------- */

// The engine speaks MI over pipes, so a shell script standing in for GDB exercises
// the whole path — the tokens, the answers, the notifications — without a debugger
// being installed.
func TestTheEngineSpeaksToAProgramThatIsNotGDB(t *testing.T) {
	engine := fakeGDB(t)
	defer engine.Close()

	if capabilities, err := engine.Initialize(InitializeArguments{}); err != nil {
		t.Fatal(err)
	} else if capabilities.SupportsConfigurationDone {
		t.Error("expected no configuration-done support on a program that runs here, not on a board")
	}

	set, err := engine.SetBreakpoints("main.c", []Wanted{{Line: 42}, {Line: 43}})
	if err != nil {
		t.Fatal(err)
	}
	if len(set) != 2 || !set[0].Verified || set[0].Line != 42 {
		t.Errorf("expected both breakpoints verified, got %+v", set)
	}

	threads, err := engine.Threads()
	if err != nil {
		t.Fatal(err)
	}
	if len(threads) != 1 || threads[0].Name != "main" {
		t.Errorf("expected the thread the debugger reported, got %+v", threads)
	}

	frames, err := engine.StackTrace(1, Range{Frames: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || frames[0].Name != "main" || frames[0].Line != 42 {
		t.Errorf("expected the frames, got %+v", frames)
	}
	if frames[0].ID < FirstReference {
		t.Errorf("expected a frame number of our own, got %d", frames[0].ID)
	}

	scopes, err := engine.Scopes(frames[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, scope := range scopes {
		names = append(names, scope.Name)
	}
	if strings.Join(names, ",") != "Registers,Arguments,Locals" {
		t.Errorf("expected registers, arguments and locals, got %v", names)
	}
	for _, scope := range scopes {
		if scope.VariablesReference < FirstReference {
			t.Errorf("%s: expected a reference of our own, got %d", scope.Name, scope.VariablesReference)
		}
		variables, err := engine.Variables(scope.VariablesReference)
		if err != nil {
			t.Errorf("%s: %v", scope.Name, err)
		}
		if scope.Name == "Registers" && (len(variables) == 0 || variables[0].Name == "") {
			t.Errorf("expected named registers, got %+v", variables)
		}
	}

	if _, err := engine.ReadMemory(0x40011000, 4); err != nil {
		t.Errorf("expected the memory to be read, got %v", err)
	}
}

// The console stream is what the program under test prints, and a line that is not a
// record has to reach the editor rather than ending the read: a program that prints
// something beginning with a bracket must not be able to end a session.
func TestWhatTheProgramPrintsReachesTheEditor(t *testing.T) {
	engine := fakeGDB(t)
	defer engine.Close()

	if _, err := engine.Threads(); err != nil {
		t.Fatal(err)
	}
	select {
	case notification := <-engine.Notifications():
		if notification.Kind != NotifyOutput {
			t.Errorf("expected the program's output, got %+v", notification)
		}
	case <-time.After(2 * time.Second):
		t.Error("expected the program's output to arrive")
	}
}

// fakeGDB is a program that answers MI the way GDB does, including the parts this
// engine depends on: the token on every answer, the frame list in the shape GDB
// writes it, and something on the console stream.
func fakeGDB(t *testing.T) *GDB {
	t.Helper()
	script := writeScript(t, "fake-gdb", `#!/bin/sh
# The arguments this engine starts a debugger with, so a test can see that they are
# the ones that make it speak MI at all.
case "$*" in
  *--interpreter=mi2*) ;;
  *) echo "not started as an MI interpreter" >&2; exit 2;;
esac

while IFS= read -r line; do
  # The token off the command being answered, which is the whole correlation the
  # engine has with this process.
  token=$(printf '%s' "$line" | sed 's/^\([0-9]*\).*/\1/')
  # What the program under test prints, on the stream the debugger prints it on. It
  # is deliberately not a record: a program that prints something beginning with a
  # bracket must not be able to end a session.
  echo "~[booted] ready\r\n"
  case "$line" in
    *-break-insert*)
      # The location is the last argument, written as file:line.
      number=$(echo "$line" | sed 's/.*:\\([0-9]*\)$/\\1/')
      if [ "$number" = "99" ]; then
        echo "${token}^error,msg=\"No line 99 in file\""
      else
        echo "${token}^done,bkpt={number=\"1\",file=\"main.c\",fullname=\"/src/main.c\",line=\"${number}\",times=\"0\"}"
      fi;;
    *-thread-info*)
      echo "${token}^done,threads=[{id=\"1\",name=\"main\",target-id=\"Thread 0x1\"}],current-thread-id=\"1\"";;
    *-stack-list-frames*)
      echo "${token}^done,stack=[frame={level=\"0\",addr=\"0x400116\",func=\"main\",file=\"main.c\",fullname=\"/src/main.c\",line=\"42\",arch=\"armv7\"},frame={level=\"1\",addr=\"0x4001d8\",func=\"__libc_init_array\",file=\"crt0.c\",fullname=\"/lib/crt0.c\",line=\"14\",arch=\"armv7\"}],totalFrames=2";;
    *-stack-list-arguments*)
      echo "${token}^done,variables=[{name=\"argc\",value=\"1\",type=\"int\"}]";;
    *-stack-list-locals*)
      echo "${token}^done,variables=[{name=\"counter\",value=\"3\",type=\"int\"}]";;
    *-data-list-register-names*)
      echo "${token}^done,register-names=[\"pc\",\"x\",\"sp\",\"cpsr\",\"0\"]";;
    *-data-list-register-values*)
      echo "${token}^done,register-values=[{number=\"0\",value=\"0x400116\"},{number=\"1\",value=\"0x0\"},{number=\"2\",value=\"0x20001000\"},{number=\"3\",value=\"0x60000010\"},{number=\"4\",value=\"0x0\"}]";;
    *-data-read-memory-bytes*)
      echo "${token}^done,memory=[{begin=\"0x40011000\",offset=\"0x40011000\",end=\"0x40011004\",contents=\"48000f20\"}]";;
    *-var-create*)
      echo "${token}^done,name=\"rho1\",value=\"3\",type=\"int\",numchild=\"0\"";;
    *-exec-run*|*-exec-continue*|*-exec-next*|*-exec-step*|*-exec-finish*|*-exec-interrupt*|*-gdb-exit*|*-break-delete*|*-target-select*|load*|*-interpreter-exec*)
      echo "${token}^done";;
    *)
      echo "${token}^done";;
  esac
done
`)
	engine, err := StartGDB(script, "app.elf", GDBOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

// A program running on this machine is a session with no board: no probe to start,
// no register map to read, and a debugger the machine has. Every one of those is a
// question with no board to answer it, and asking anyway is a crash rather than a
// configuration.
func TestASessionOnThisMachineHasNoBoard(t *testing.T) {
	session, err := Open(OpenRequest{
		Program:  "app",
		Root:     t.TempDir(),
		Debugger: writeScript(t, "idle-gdb", "#!/bin/sh\nwhile IFS= read -r line; do\n  token=$(printf '%s' \"$line\" | sed 's/^\\([0-9]*\\).*/\\1/')\n  echo \"${token}^done\"\ndone\n"),
	}, NewCodec(strings.NewReader(""), io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	opening := session.Opening()
	if len(opening) == 0 {
		t.Fatal("expected the session to say how it was assembled")
	}
	joined := strings.Join(opening, "\n")
	if !strings.Contains(joined, "board: this machine") {
		t.Errorf("expected the session to say it is on this machine, got:\n%s", joined)
	}
	// And no register map scope that can be expanded, because a host has no
	// peripherals and an empty tree would claim it had none at all.
	if scope := session.peripheralScope(); scope.VariablesReference != 0 {
		t.Errorf("expected no peripherals on this machine, got %+v", scope)
	}
}

// A target says which debugger it means, and a machine may have that one or may
// have the one its distribution ships for several architectures. Refusing the
// session in the second case would leave a machine with a perfectly good cross
// debugger unable to debug a board its own toolchain builds for.
func TestADebuggerIsResolvedToTheOneThisMachineHas(t *testing.T) {
	root := t.TempDir()
	// The search is confined to the directory this test fills, so that what it finds
	// does not depend on which debuggers the machine running it has installed — a
	// test that passes on one machine and fails on another is one that is testing
	// the machine.
	t.Setenv("PATH", root)
	// The Rho home is the other half of the search — a fetched toolchain is looked
	// in before the fallback — so it is confined too, or the test answers with whatever
	// this machine has fetched.
	t.Setenv("RHO_HOME", t.TempDir())
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("gdb")
	write("gdb-multiarch")

	// The one that was asked for, when it is there.
	write("arm-none-eabi-gdb")
	if found, err := ResolveDebugger("arm-none-eabi-gdb", root); err != nil {
		t.Fatal(err)
	} else if filepath.Base(found) != "arm-none-eabi-gdb" {
		t.Errorf("expected the named debugger, got %q", found)
	}

	// The distribution's one, when that is what there is.
	if err := os.Remove(filepath.Join(root, "arm-none-eabi-gdb")); err != nil {
		t.Fatal(err)
	}
	if found, err := ResolveDebugger("arm-none-eabi-gdb", root); err != nil {
		t.Fatal(err)
	} else if filepath.Base(found) != "gdb-multiarch" {
		t.Errorf("expected the multiarch debugger, got %q", found)
	}

	// And the host's own, when a machine has nothing cross at all — last, because a
	// machine with both uses the one that was named.
	if err := os.Remove(filepath.Join(root, "gdb-multiarch")); err != nil {
		t.Fatal(err)
	}
	if found, err := ResolveDebugger("arm-none-eabi-gdb", root); err != nil {
		t.Fatal(err)
	} else if filepath.Base(found) != "gdb" {
		t.Errorf("expected the host's debugger, got %q", found)
	}

	// And none of them is a failure that names all three, because "not installed"
	// for one program says nothing about the two the reader has not tried.
	if err := os.Remove(filepath.Join(root, "gdb")); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveDebugger("arm-none-eabi-gdb", root)
	if err == nil {
		t.Fatal("expected a machine with no debugger at all to be reported")
	}
	for _, wanted := range []string{"arm-none-eabi-gdb", "multiarch", "gdb"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("expected %q in the failure, got %v", wanted, err)
		}
	}
}

// The translation table, checked against the debugger it is for.
//
// Every other test in this package speaks to a stand-in, which proves the
// bookkeeping and proves nothing about whether the commands are ones gdb accepts.
// This one starts a real one when the machine has it, and is skipped when it does
// not: a machine without a cross toolchain is not a machine this should fail on,
// and a machine with one is a machine where a command written the way another
// debugger's protocol writes it would otherwise only be found out about by a
// session somebody was watching.
//
// Each command is sent on its own and answered: what is checked is that gdb does
// not answer with a usage message, which is how every one of the mistakes in this
// file's history first showed up.
func TestTheTranslationTableIsReadByADebuggerWhenThereIsOne(t *testing.T) {
	engine, err := StartGDB("gdb-multiarch", "/bin/true", GDBOptions{})
	if err != nil {
		if _, resolveErr := ResolveDebugger("gdb-multiarch"); resolveErr != nil {
			t.Skip("this machine has no gdb-multiarch, so there is nothing to check the commands against")
		}
		t.Fatalf("start the debugger: %v", err)
	}
	defer engine.Close()

	for _, command := range []Command{
		Of("-gdb-set").Arg("mi-async").Arg("on"),
		With("-file-exec-and-symbols", "/bin/true"),
		Of("-thread-select").Num(1),
		Of("-thread-info"),
		Of("-data-list-register-names"),
		With("-data-list-register-values", "x"),
		Of("-stack-list-frames"),
		Of("-stack-list-arguments").Num(0),
		Of("-stack-list-locals").Num(0),
		Of("-break-insert").Word(Location("main.c", 21)),
		Of("-break-insert").Opt("-c", "1 == 1").Opt("-i", "2").Word(Location("main.c", 21)),
		Of("-data-read-memory-bytes").Arg("0x0").Num(4),
		Of("-var-create").Arg("probe").Arg("*").Arg("1 + 1"),
		Of("-exec-interrupt"),
	} {
		answer, sendErr := engine.Send(command)
		if sendErr != nil {
			// A usage message is the failure this test exists for; a connection
			// refused is the environment, and is left to the tests that check what a
			// refused connection looks like.
			if strings.Contains(sendErr.Error(), "Usage") ||
				strings.Contains(sendErr.Error(), "Unknown option") ||
				strings.Contains(sendErr.Error(), "Undefined MI command") ||
				strings.Contains(sendErr.Error(), "expected") {
				t.Errorf("%s: %v", command.Arguments(), sendErr)
				continue
			}
			t.Logf("%s: %v", command.Arguments(), sendErr)
			continue
		}
		if answer.Class == "error" {
			t.Errorf("%s: %s", command.Arguments(), errorOf(answer))
		}
	}
}

// A toolchain fetched into the Rho home is the one the module asked for, so it is
// found before the fallback. The other way round would report a requirement as
// satisfied and then debug with something else — which is the exact mismatch this
// whole block exists to catch.
func TestAFetchedDebuggerIsFoundBeforeTheFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	cache := sysdep.ToolchainRoot(home)
	// A fetched one, at the path the layout says.
	fetched := filepath.Join(cache, "arm-none-eabi-gdb-1.0", "toolchain-1.0", "bin")
	if err := os.MkdirAll(fetched, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fetched, "arm-none-eabi-gdb"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// And a fallback on the machine's own path, so there is something to *not* choose.
	machine := t.TempDir()
	for _, name := range []string{"arm-none-eabi-gdb", "gdb-multiarch", "gdb"} {
		if err := os.WriteFile(filepath.Join(machine, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", machine)

	found, err := ResolveDebugger("arm-none-eabi-gdb")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(fetched, "arm-none-eabi-gdb") {
		t.Errorf("expected the fetched debugger, got %q", found)
	}

	// With nothing fetched, the fallback is what there is — and it is still chosen
	// before the host's own bare gdb.
	if err := os.RemoveAll(filepath.Join(cache, "arm-none-eabi-gdb-1.0")); err != nil {
		t.Fatal(err)
	}
	found, err = ResolveDebugger("arm-none-eabi-gdb")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(machine, "arm-none-eabi-gdb") {
		t.Errorf("expected the machine's own cross debugger, got %q", found)
	}
}
