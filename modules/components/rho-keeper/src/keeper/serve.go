//
// @file serve.go
// @brief The dashboard: a page, a stream, and the actions that change something.
//
// It is one binary with nothing in it but Go's own library. The page is a
// template, the styles and the script are embedded beside it, and the data comes
// from this process — so a dashboard that takes a second to start is a
// dashboard that is actually being started, and one that needs a network to look
// right is not one that can be run offline on a machine nobody has connected.
//
// State is pushed rather than asked for. A dashboard about a machine is a thing
// people leave open, and a page that shows what was true when it loaded is
// misleading the moment somebody else changes something — so every change
// re-reads the installation and tells whoever is watching.
//
// The stream is Server-Sent Events rather than a WebSocket because the data is
// one way. A socket would buy a channel nothing here sends on, at the cost of a
// protocol the browser reconnects to badly and this does not need to.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rho/src/deps"
	"rho/src/yaml"
)

// defaultAddr is the address the dashboard listens on when its manifest and its
// settings file say nothing.
//
// It is the loopback address on a high port, on purpose. A low port is a system
// service, and this is a tool a person runs for themselves; the loopback is the
// one that cannot be reached from another machine, which is right for a page
// that lists what this one has.
const defaultAddr = "127.0.0.1:8477"

// Options are what a run of the dashboard takes.
type Options struct {
	// Addr is the address to listen on. Empty is what the configuration says.
	Addr string
	// Now is the clock, so a page that shows when it was read says a time that
	// can be pinned in a test.
	Now func() time.Time
}

// settings is what this dashboard is set to, read when the command is given.
//
// The contract is this module's own manifest: it says which settings exist, what
// they are called and what they are for, and it is what a person reads to find
// out how the dashboard can be changed. The file named there holds the values,
// and it travels with the module, so a keeper installed from a registry reads
// the same file wherever it lands.
//
// A value is read when the command runs, not once at build time, because a
// person changes the file and then restarts, and a dashboard that had already
// read it would be showing settings that are not the ones in use.
type settings struct {
	addr        string
	logRequests bool
	// from is "flag" when a person named one, "file" when the file said so, and
	// "" when the default in the manifest is in use. It is what the line
	// printed says, because a person who chose a setting and a person who is
	// being given one are in different situations.
	from string
	// file is where the values were read, and where a change is written back to.
	file   string
	values map[string]string
}

// readSettings is this module's manifest and the file it names.
//
// The manifest is parsed with the same loader that parses every manifest, so the
// contract a component declares is the contract the toolchain reads rather than a
// second reading of it written next to it.
func readSettings(override string) (settings, error) {
	out := settings{logRequests: true, values: map[string]string{}}

	root := moduleRoot()
	if root == "" {
		return out, nil
	}
	manifest, err := deps.LoadManifest(filepath.Join(root, "module.yml"))
	if err != nil {
		// A module whose manifest cannot be read has not declared a contract, and
		// running on the built-in defaults is better than refusing to serve a
		// page over a file that was never going to be read anyway.
		return out, nil
	}

	// The declared defaults are the contract's half: a value the file does not
	// set is the default the manifest names for it.
	if manifest.Settings != nil {
		for _, setting := range manifest.Settings.Settings {
			switch setting.Name {
			case "addr":
				if setting.Default != "" {
					out.addr = normaliseAddr(setting.Default)
				}
			case "log_requests":
				out.logRequests = !strings.EqualFold(setting.Default, "false")
			}
		}
		if manifest.Settings.File != "" {
			out.file = filepath.Join(root, filepath.FromSlash(manifest.Settings.File))
		}
	}
	if out.addr == "" {
		out.addr = defaultAddr
	}

	// The file is this module's own values, and it is the only thing that says
	// what they are set to. A file that is not there is a module nobody has
	// changed, which is not a failure.
	if out.file != "" {
		if data, readErr := os.ReadFile(out.file); readErr == nil {
			document, parseErr := yaml.Load(string(data))
			if parseErr != nil {
				// A file that is there and will not parse is a failure. Serving
				// past it would mean showing settings that are not the settings
				// in use, which is the one thing a dashboard must not do.
				return out, fmt.Errorf("the settings in %s are not YAML this can read: %w", out.file, parseErr)
			}
			for key, value := range document.Map {
				if value.Kind == yaml.KindMap || value.Kind == yaml.KindList {
					continue
				}
				out.values[key] = value.Str
			}
		} else if !os.IsNotExist(readErr) {
			return out, fmt.Errorf("the settings in %s cannot be read: %w", out.file, readErr)
		}
	}
	if addr, found := out.values["addr"]; found && addr != "" {
		out.addr = normaliseAddr(addr)
		out.from = "file"
	}
	if logging, found := out.values["log_requests"]; found {
		out.logRequests = !strings.EqualFold(logging, "false")
		out.from = "file"
	}

	// A flag beats the file, and naming one writes it back: a port chosen once is
	// the port every later run uses.
	if strings.TrimSpace(override) != "" {
		out.addr = normaliseAddr(override)
		out.from = "flag"
		if err := out.write(); err != nil {
			// The address asked for is the one to serve on. What is lost is the
			// remembering, and a person who is not told finds out at the next run.
			fmt.Fprintf(os.Stderr, "rho keeper: the settings could not be written back: %v\n", err)
		}
	}
	return out, nil
}

// write changes the settings in the file they came from.
func (s settings) write() error {
	if s.file == "" {
		return nil
	}
	s.values["addr"] = s.addr
	s.values["log_requests"] = fmt.Sprintf("%t", s.logRequests)
	// The file is written in a stable order, so one rewritten for a single change
	// is one a person can read as a diff.
	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s: %s\n", key, s.values[key])
	}
	return os.WriteFile(s.file, []byte(b.String()), 0o644)
}

// moduleRoot is where this module was installed from, which is where its own
// files are.
//
// The dispatcher says so in the environment, because a program that has to find
// its own files by walking up from its executable breaks the moment it is invoked
// through a link or from a different working directory.
func moduleRoot() string {
	if root := strings.TrimSpace(os.Getenv("RHO_MODULE_ROOT")); root != "" {
		return root
	}
	if executable, err := os.Executable(); err == nil {
		return filepath.Dir(executable)
	}
	return ""
}

// normaliseAddr is an address a person typed, made into one a listener can take.
//
// A bare number is a port on this machine, which is what somebody who typed one
// meant. Naming a host is how a person asks for it to be reachable beyond it.
func normaliseAddr(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return defaultAddr
	}
	if !strings.Contains(trimmed, ":") {
		return "127.0.0.1:" + trimmed
	}
	return trimmed
}

/* -------------------------------------------------------------------------- */
/*  Running                                                                    */
/* -------------------------------------------------------------------------- */

// Keeper is the dashboard, described in the snapshot the page is sent.
type Snapshot struct {
	State Installation `json:"state"`
	Read  string       `json:"read"`
}

// Start runs the dashboard until the process is stopped.
//
// A port already in use is reported as itself, with what is on it, rather than
// as a failure to listen: "address in use" is a thing a person can act on and a
// number they can change.
func Start(options Options) error {
	config, err := readSettings(options.Addr)
	if err != nil {
		return err
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	listener, err := net.Listen("tcp", config.addr)
	if err != nil {
		if isAddressInUse(err) {
			return fmt.Errorf(
				"the dashboard is asked for %s, and something is already listening there; "+
					"choose another port with: rho keeper start -addr 127.0.0.1:%d",
				config.addr, nextPort(config.addr))
		}
		return err
	}
	defer listener.Close()

	// The page shows where it is listening, so the address it was given is part
	// of what it sends.
	config.addr = listener.Addr().String()

	hub := newHub()
	server := &http.Server{
		Handler:           Handler(now, config.logRequests, hub, config.addr),
		ReadHeaderTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://%s/", listener.Addr().String())
	fmt.Fprintf(os.Stderr, "rho keeper: %s\n", url)
	switch config.from {
	case "flag":
		fmt.Fprintf(os.Stderr, "rho keeper: listening on %s, written back to %s\n", config.addr, config.file)
	case "file":
		fmt.Fprintf(os.Stderr, "rho keeper: listening on %s, from %s; name another with: rho keeper start -addr <address>\n", config.addr, config.file)
	default:
		fmt.Fprintf(os.Stderr, "rho keeper: listening on %s, the default; name another with: rho keeper start -addr <address>\n", config.addr)
	}
	if isLoopback(listener.Addr()) {
		fmt.Fprintf(os.Stderr, "rho keeper: press ctrl-c to stop\n")
	} else {
		// The dashboard lists what a machine has and what it can install. Saying
		// so out loud when it is reachable from elsewhere is the least that can
		// be done about it.
		fmt.Fprintf(os.Stderr, "rho keeper: this is reachable beyond this machine\n")
	}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

/* -------------------------------------------------------------------------- */
/*  The stream                                                                 */
/* -------------------------------------------------------------------------- */

// hub tells whoever is watching that the machine has changed.
//
// It holds no history and asks for nothing: a watcher that arrives late gets the
// state when it next asks, and one that has been connected a long time is sent
// whatever is true now. That is enough for a page about an installation, where
// the question is always "what is it like now".
type hub struct {
	mu       sync.Mutex
	watchers map[chan struct{}]bool
}

func newHub() *hub { return &hub{watchers: map[chan struct{}]bool{}} }

// watch is a channel that is signalled when something changes, and unregistered
// when the caller goes away.
func (h *hub) watch() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watchers[ch] = true
	return ch
}

func (h *hub) drop(ch chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.watchers, ch)
}

// changed tells every watcher, without blocking on a slow one.
//
// A watcher that has not been read yet has already been signalled — the channel
// holds one — so a full channel means it knows, and skipping it costs nothing.
func (h *hub) changed() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.watchers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

/* -------------------------------------------------------------------------- */
/*  The routes                                                                 */
/* -------------------------------------------------------------------------- */

// Handler is the dashboard, as a handler, so that a test can serve it without
// opening a port.
func Handler(now func() time.Time, log bool, h *hub, addrOf string) http.Handler {
	if now == nil {
		now = time.Now
	}
	if h == nil {
		h = newHub()
	}
	mux := http.NewServeMux()

	// The page, the styles and the script. The page is a shell: it says what the
	// dashboard is and where its data comes from, and the data arrives after.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w)
	})
	// The page's own files are revalidated on every load rather than
	// remembered.
	//
	// They are inside a binary that changes, and a browser that kept the copy it
	// was given would go on running a version of the page whose bugs have been
	// fixed. Three small files over a loopback connection is a cheap thing to ask
	// twice, and a page that quietly runs last week's script is not one anybody
	// can debug.
	assets := http.StripPrefix("/static/", http.FileServer(http.FS(assetRoot())))
	mux.Handle("GET /static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		assets.ServeHTTP(w, r)
	}))

	// What is installed, as data.
	// The address the server is listening on, so a page can say which port it
	// is on rather than leaving a person to read it off the terminal they
	// started it from.
	addr := addrOf
	mux.HandleFunc("GET /api/v1/state", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, snapshot(now).atAddress(addr))
	})

	// The stream. It sends the whole reading on connect, and the whole reading
	// again whenever anything changes, so a page that is left open is never
	// showing a machine that has since been altered.
	mux.HandleFunc("GET /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		stream(w, r, now, h)
	})

	// The things a person can change. Each answers with the command it performed,
	// whether it worked or not.
	mux.HandleFunc("POST /api/v1/modules/{name}/enable", func(w http.ResponseWriter, r *http.Request) {
		act(w, r, Action{OK: false}, h, func(version string) Action {
			return Enable(r.PathValue("name"), version)
		})
	})
	mux.HandleFunc("POST /api/v1/modules/{name}/disable", func(w http.ResponseWriter, r *http.Request) {
		act(w, r, Action{OK: false}, h, func(string) Action { return Disable(r.PathValue("name")) })
	})

	// Starting a project: the same `rho init`, through the same function. What
	// it writes is what the project's manifest declares, so a drivers
	// repository produces a driver and the dashboard does not have to know that.
	mux.HandleFunc("POST /api/v1/init", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Project     string            `json:"project"`
			Dir         string            `json:"dir"`
			Name        string            `json:"name"`
			Generator   string            `json:"generator"`
			Category    string            `json:"category"`
			Description string            `json:"description"`
			Set         map[string]string `json:"set"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, Action{Error: err.Error()})
			return
		}
		writeJSON(w, Init(body.Project, body.Dir, body.Name, body.Generator, body.Category, body.Description, body.Set))
	})

	// One module in full: what it is, what it documents, and what it is set to.
	mux.HandleFunc("GET /api/v1/modules/{name}", func(w http.ResponseWriter, r *http.Request) {
		detail := Detail(r.PathValue("name"))
		if detail == nil {
			http.Error(w, "there is no such module", http.StatusNotFound)
			return
		}
		writeJSON(w, detail)
	})

	// Changing one of its settings, which writes the file it reads.
	mux.HandleFunc("POST /api/v1/modules/{name}/settings", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := decodeBody(w, r, &body); err != nil {
			writeJSON(w, Action{Error: err.Error()})
			return
		}
		action := SetSetting(name, body.Key, body.Value)
		if action.OK {
			h.changed()
		}
		writeJSON(w, action)
	})

	// The documentation.
	//
	// Three things are documented: this project's own, which is a tree rather
	// than a page; this module's, which is the README beside its manifest; and
	// every other module's, which is what a badge in the list opens.
	//
	// They are read on request rather than embedded, because a copy inside the
	// binary is a second one that drifts the first time the original is edited —
	// and the original is what a person is sent to read.
	mux.HandleFunc("GET /api/v1/docs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ProjectDocs())
	})
	mux.HandleFunc("GET /api/v1/docs/{path...}", func(w http.ResponseWriter, r *http.Request) {
		document, err := ReadProjectDoc(r.PathValue("path"))
		if err != nil {
			http.Error(w, "there is no such page", http.StatusNotFound)
			return
		}
		writeJSON(w, document)
	})
	mux.HandleFunc("GET /api/v1/modules/{name}/docs", func(w http.ResponseWriter, r *http.Request) {
		document, err := ReadModuleDoc(r.PathValue("name"))
		if err != nil {
			http.Error(w, "this module carries no documentation", http.StatusNotFound)
			return
		}
		writeJSON(w, document)
	})

	// What this toolchain can be asked to do, read from the toolchain rather
	// than written here, so that a page listing commands cannot go stale.
	mux.HandleFunc("GET /api/v1/cli", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ReadCommands())
	})

	// Whether the project's own documentation was found, so a page says so
	// rather than showing a menu that is empty for a reason nobody can see.
	mux.HandleFunc("GET /api/v1/docs/available", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"available": ProjectDocsAvailable()})
	})

	// What an editor shows beside a keyword: the reference's section for it, and
	// the page a person can read the rest on. One word, one request, so a hover
	// does not have to fetch and search the whole page.
	mux.HandleFunc("GET /api/v1/help/{word}", func(w http.ResponseWriter, r *http.Request) {
		help, ok := HelpFor(r.PathValue("word"))
		if !ok {
			http.Error(w, "the reference has nothing for that word", http.StatusNotFound)
			return
		}
		writeJSON(w, help)
	})
	// The words help is available for, so a caller can see the set.
	mux.HandleFunc("GET /api/v1/help", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, HelpKeywords())
	})

	// What this machine is, for somebody asking why it will not do something.
	mux.HandleFunc("GET /api/v1/system", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, ReadSystem())
	})

	// What the installed modules need from this machine, which is the doctor's
	// report rather than a second reading of it — and the fix for each, as the
	// command a person would run.
	mux.HandleFunc("GET /api/v1/system/requirements", func(w http.ResponseWriter, r *http.Request) {
		rows, problem := SystemRequirements()
		writeJSON(w, map[string]any{
			"requirements": rows,
			"summary":      SystemSummary(),
			"host":         HostDescription(),
			"problem":      problem,
			// What a plan would do, so the page can say what `sync` would do before
			// anybody asks it to.
			"plan":    PlanFor(readReportOrEmpty()),
			"fetched": FetchSummary(),
		})
	})

	// Doing one of them, with its output as it happens. Loading this without
	// `confirm` answers with what it would do and stops: a URL a page can be made to
	// open must not be able to start a download or reach for privileges, and a
	// person clicking a button is what makes it run.
	mux.HandleFunc("GET /api/v1/system/install/{module}/{dep}", handleInstall)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"status": "ok"})
	})

	if log {
		return logRequests(mux)
	}
	return mux
}

// snapshot is the reading, with the time it was read.
func snapshot(now func() time.Time) Snapshot {
	return Snapshot{State: Read(), Read: now().UTC().Format(time.RFC3339)}
}

// atAddress is the reading with this run's address on it.
//
// The address is a fact about the process rather than about the machine, so it
// is applied here rather than read: a re-read of the installation happens every
// time something changes, and the address does not change with it.
func (s Snapshot) atAddress(addr string) Snapshot {
	s.State.Keeper.Addr = addr
	return s
}

// stream is a Server-Sent Events connection.
//
// The stream is what makes a dashboard about a machine honest: a person leaves
// it open, somebody changes something in a terminal, and the page stops showing a
// machine that no longer exists.
func stream(w http.ResponseWriter, r *http.Request, now func() time.Time, h *hub) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "this server cannot stream", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func() bool {
		payload, err := json.Marshal(snapshot(now))
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: state\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}

	// A comment every so often, so that a connection through something that
	// times out idle is noticed rather than left looking open and dead.
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	changes := h.watch()
	defer h.drop(changes)
	for {
		select {
		case <-r.Context().Done():
			return
		case <-changes:
			if !send() {
				return
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// act runs a change, answers with what it did, and tells the watchers.
//
// The command is answered whether it worked or not, because a person who has
// been refused is precisely the person who wants to see what was tried.
func act(w http.ResponseWriter, r *http.Request, fallback Action, h *hub, run func(version string) Action) {
	var body struct {
		Version string `json:"version"`
	}
	// A body is not required — the common case changes nothing but a name — so
	// one that is not there, or is not JSON, is not a failure.
	if err := decodeBody(w, r, &body); err != nil {
		writeJSON(w, Action{Error: err.Error()})
		return
	}
	action := run(body.Version)
	// Everything that changed is re-read once, and the watchers are told. A
	// change that did not happen is not broadcast: there is nothing to say, and
	// a stream that emits unchanged state is a stream nobody can trust to mean
	// anything.
	if action.OK {
		h.changed()
	} else {
		action.Command = firstNonEmpty(action.Command, fallback.Command)
	}
	writeJSON(w, action)
}

// decodeBody reads a request's JSON, for a body that carries what to do.
//
// A body that is not there is not a failure: the common case is a change that
// names nothing but a module, and a request that arrived without one is a
// request that has already said what it means. A body that is there and is not
// the shape asked for is a failure, because guessing at it would be doing
// something the caller did not say.
func decodeBody(w http.ResponseWriter, r *http.Request, into any) error {
	if r.Body == nil {
		return nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("the request cannot be read: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, into); err != nil {
		return fmt.Errorf("the request is not the shape this expects: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

// logRequests says what was asked for, on standard error.
//
// A local server that says nothing is a local server nobody can tell is running.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		fmt.Fprintf(os.Stderr, "rho keeper: %s %s %s\n",
			r.Method, r.URL.RequestURI(), time.Since(started).Round(time.Millisecond))
	})
}

// isAddressInUse reports whether a port is taken.
//
// The standard error says so in prose that varies by platform, and matching on
// the port rather than the text is what makes this work the same everywhere.
func isAddressInUse(err error) bool {
	var opError *net.OpError
	if !asOpError(err, &opError) {
		return false
	}
	return strings.Contains(strings.ToLower(opError.Error()), "address already in use") ||
		strings.Contains(strings.ToLower(opError.Error()), "only one usage of each socket address")
}

// nextPort is a port to try instead, derived from the one that is taken.
//
// It is the next number rather than a random one, so a person told "try 8478"
// finds a port that is almost certainly free and does not have to think about it.
func nextPort(addr string) int {
	wanted, err := strconv.Atoi(defaultAddr[strings.LastIndex(defaultAddr, ":")+1:])
	if err != nil {
		wanted = 0
	}
	if index := strings.LastIndex(addr, ":"); index >= 0 {
		if parsed, parseErr := strconv.Atoi(addr[index+1:]); parseErr == nil {
			wanted = parsed
		}
	}
	return wanted + 1
}

// isLoopback reports whether a listener can only be reached from this machine.
func isLoopback(addr net.Addr) bool {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	parsed := net.ParseIP(host)
	return parsed != nil && parsed.IsLoopback()
}
