//
// @file emit_go_port.go
// @brief Built-in port implementations for the Go backend.
//
// A Rho module declares the storage functions it needs as `extern fn`. The
// backend can supply those implementations, so a project stays pure Rho and
// still gets persistence, addressing the fact that Rho has no heap, no globals
// and no file I/O yet.
//
// A new port is a new entry in portGenerators; the generator itself is
// unchanged.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

import (
	"fmt"
	"sort"
	"strings"

	"rho/src/ir"
)

// PortFileName is the generated file holding port implementations.
const PortFileName = "ports.go"

// portAliases pairs the names Rho calls with the implementations they reach and
// the result each returns.
//
// A Rho call site becomes lowerCamel in Go, so the spellings are generated
// from this one list rather than written by hand; that is how they drifted from
// the call sites before.
var portAliases = []struct {
	rho, implementation, result string
}{
	{"registry_data_dir", "dataDir", "string"},
	{"registry_access", "configGetAccess", "string"},
	{"registry_title", "configGetTitle", "string"},
	{"registry_site_url", "configGetSiteURL", "string"},
	{"registry_url", "configGetRegistryURL", "string"},
	{"registry_open_tokens", "OpenTokens", "bool"},
	{"registry_max_upload_mb", "MaxUploadMB", "uint32"},
}

// portAliasFuncs renders the Go spelling of every name in the alias list.
//
// Only names whose Go spelling differs from what Rho calls appear here. A name
// the emitted code already defines in the right shape is left alone, so there is
// one definition of each rather than two that must agree.
func portAliasFuncs() string {
	var b strings.Builder
	b.WriteString("// Go spellings for the names Rho handlers call.\n")
	for _, alias := range portAliases {
		fmt.Fprintf(&b, "func %s() %s { return %s() }\n",
			lowerCamel(alias.rho), alias.result, alias.implementation)
	}
	b.WriteString("\n")
	return b.String()
}

// portGenerator renders one built-in port, returning the imports it needs and
// the body to place in the generated file.
type portGenerator func(PortContext) (imports []string, body string)

// PortContext carries what a port needs beyond the compiled module. A port that
// only implements extern declarations ignores most of it; a port that also
// shapes the generated service reads the router and the pipeline the project
// declared, so the code it emits is the code the project asked for.
type PortContext struct {
	// IR is the compiled Rho module the port supplies implementations for.
	IR ir.ModuleIR
	// ModuleName is the Rho module name, for banners and messages.
	ModuleName string
	// ProjectRoot is the Rho project directory, for a port that reads files
	// the project carries.
	ProjectRoot string
	// Server is how the target routes and what it wraps the router in.
	Server ServerConfig
}

// portGenerators are the built-in ports a descriptor may request.
var portGenerators = map[string]portGenerator{
	"docemit":     emitDocPort,
	"fsstore":     emitFSStorePort,
	"logemit":     emitLogPort,
	"httproute":   emitHTTPRoutePort,
	"openapiemit": emitOpenAPIPort,
}

// RegisteredPorts lists the available port names.
func RegisteredPorts() []string {
	out := make([]string, 0, len(portGenerators))
	for name := range portGenerators {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// HasPort reports whether a port name is available.
func HasPort(name string) bool {
	_, ok := portGenerators[name]
	return ok
}

// EmitGoPorts renders the port implementations a target asked for.
func EmitGoPorts(ctx PortContext, ports []string) string {
	if len(ports) == 0 {
		return ""
	}
	imports := map[string]bool{}
	var body strings.Builder

	for _, name := range ports {
		generator, ok := portGenerators[name]
		if !ok {
			// An unknown port is reported by the generator, which can name the
			// available ones; nothing is emitted here.
			continue
		}
		needed, portBody := generator(ctx)
		for _, name := range needed {
			imports[name] = true
		}
		body.WriteString(portBody)
		body.WriteString("\n")
	}

	var b strings.Builder
	b.WriteString(goBanner(ctx.IR, PortFileName, "Generated Rho port implementations."))
	b.WriteString("package main\n")
	if len(imports) > 0 {
		names := make([]string, 0, len(imports))
		for name := range imports {
			names = append(names, name)
		}
		sort.Strings(names)
		b.WriteString("\nimport (\n")
		for _, name := range names {
			fmt.Fprintf(&b, "\t%s\n", name)
		}
		b.WriteString(")\n")
	}
	b.WriteString("\n")
	b.WriteString(body.String())
	return b.String()
}

// emitFSStorePort renders a filesystem-backed module store, which is what a
// registry service needs: a place to put uploaded packages, a way to read
// them back, and a listing to search.
func emitFSStorePort(ctx PortContext) ([]string, string) {
	m := ctx.IR
	version := m.Version
	if version == "" {
		version = "0.0.0"
	}
	imports := []string{
		`"archive/tar"`,
		`"bytes"`,
		`"compress/gzip"`,
		`"io"`,
		`jsonenc "encoding/json"`,
		`"fmt"`,
		// The store logs what it refuses, so a registry whose publish or
		// withdrawal failed has a reason in its log rather than only the status
		// the caller was given.
		`"log"`,
		`"os"`,
		`"path/filepath"`,
		`"sort"`,
		`"strconv"`,
		`"strings"`,
		`"sync"`,
		`"time"`,
	}
	identity := fmt.Sprintf(
		"// The module this project was generated from, so a page can report it.\n"+
			"const rhoModuleName = %q\nconst rhoModuleVersion = %q\n\n",
		m.Name, version)
	return imports, identity + portAliasFuncs() + `// Generated by the Rho fsstore port. A registry keeps each module
// version in its own directory, so publishing is a file write and searching
// is a directory walk.
//
//	<data>/modules/<name>/<version>/package.rho-pkg   uploaded package
//	<data>/modules/<name>/<version>/meta.json        module metadata
//	<data>/index/<name>.json                         version list per module

// moduleMeta is the metadata recorded for one module version.
type moduleMeta struct {
	Name        string            ` + "`json:\"name\"`" + `
	Version     string            ` + "`json:\"version\"`" + `
	Type        string            ` + "`json:\"type\"`" + `
	Description string            ` + "`json:\"description\"`" + `
	Main        string            ` + "`json:\"main\"`" + `
	Command     string            ` + "`json:\"command\"`" + `
	// Author and Maintainer are the module's own claim about who wrote and
	// looks after it, taken from its manifest. They are what a listing shows;
	// they are not what a registry checks, because a claim in a file is not a
	// credential.
	Author     string ` + "`json:\"author,omitempty\"`" + `
	Maintainer string ` + "`json:\"maintainer,omitempty\"`" + `
	// Ownership, Visibility and Mine are what the registry knows about who holds
	// the module, filled in when a listing is written and not stored with the
	// package: they belong to the name rather than to a version, and an owner
	// who renames themselves must not leave a stale copy behind on each version.
	Ownership  string ` + "`json:\"owner,omitempty\"`" + `
	Visibility string ` + "`json:\"visibility,omitempty\"`" + `
	Mine       bool   ` + "`json:\"mine,omitempty\"`" + `
	Features   []string          ` + "`json:\"features\"`" + `
	Deps       map[string]string ` + "`json:\"deps\"`" + `
}

// modulePublisher is who holds a module in this registry, and who may see it.
//
// It is recorded against the module's name rather than a version, because the
// right to publish and the right to withdraw belong to the name. A module that
// changed hands on every release could be taken over by whoever published it
// most recently.
//
// The owner is the identity whose credential published the name first. For now
// the owner is also the only maintainer, because they are the same identity;
// the maintainer list exists so that letting a second identity in later is a
// change to what is stored rather than a change to what is read.
//
//	{"name": "rho-router", "owner": "miraur", "maintainers": ["miraur"],
//	 "visibility": "public", "first_published": "2026-09-28T10:07:00Z"}
type modulePublisher struct {
	Name string ` + "`json:\"name\"`" + `
	// Owner is the identity that published the name first, and the only one
	// that may withdraw the module.
	Owner string ` + "`json:\"owner\"`" + `
	// Maintainers may publish further versions of the module but may not
	// withdraw it. Withdrawing is the destructive operation, so it is kept with
	// the owner rather than with everyone who can add to the module.
	Maintainers []string ` + "`json:\"maintainers,omitempty\"`" + `
	// Visibility is whether anyone may install the module, or only its owner.
	Visibility string ` + "`json:\"visibility\"`" + `
	// FirstPublished is when the name was claimed, which is what a reader
	// comparing two claims for the same name needs to see.
	FirstPublished time.Time ` + "`json:\"firstPublished\"`" + `
}

// The visibilities a module may be published under.
const (
	// VisibilityPublic is a module anybody may install.
	VisibilityPublic = "public"
	// VisibilityPrivate is a module only its owner may install.
	VisibilityPrivate = "private"
)

// MayPublish reports whether an identity may publish a further version of the
// module: the owner or one of the maintainers.
func (p *modulePublisher) MayPublish(identity string) bool {
	if identity == "" {
		return false
	}
	if p.Owner == identity {
		return true
	}
	for _, maintainer := range p.Maintainers {
		if maintainer == identity {
			return true
		}
	}
	return false
}

// MayUnpublish reports whether an identity may withdraw the module: only the
// owner.
func (p *modulePublisher) MayUnpublish(identity string) bool {
	return identity != "" && p.Owner == identity
}

// MayInstall reports whether an identity may install the module. A public module
// is anybody's; a private one is its owner's.
func (p *modulePublisher) MayInstall(identity string) bool {
	if p.Visibility != VisibilityPrivate {
		return true
	}
	return identity != "" && p.Owner == identity
}

// IsPrivate reports whether a module is published for its owner alone.
func (p *modulePublisher) IsPrivate() bool { return p.Visibility == VisibilityPrivate }

// holders names who may publish a further version, for a message that says so.
func (p *modulePublisher) holders() string {
	if len(p.Maintainers) == 0 {
		return p.Owner
	}
	names := append([]string{p.Owner}, p.Maintainers...)
	names = uniqueStrings(names)
	return strings.Join(names, ", ")
}

// uniqueStrings drops a repeat, so a list that carries the owner twice because
// they are also the only maintainer reads once.
func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// fsStore is a filesystem-backed module store rooted at a data directory.
type fsStore struct {
	root string
	mu   sync.RWMutex
}

// registryConfig is the settings file, read once at start-up.
//
// The data directory lives here rather than beside the application, so
// installing a new version over the old one cannot take the modules with it.
// An environment variable still wins, which is what a container uses.
var registryConfig = loadRegistryConfig()

// loadRegistryConfig reads key = value lines from the settings file. A file
// that is missing is not an error: the defaults stand.
func loadRegistryConfig() map[string]string {
	settings := map[string]string{}
	path := os.Getenv("RHO_REGISTRY_CONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return settings
		}
		path = filepath.Join(home, ".rho", "registry", "registry.conf")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return settings
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}
		settings[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return settings
}

// registrySettings is every setting the service reads, with its default.
//
// Keeping them in one table means a deployment can see what may be configured
// and what happens when it is not, without reading the code.
var registrySettings = []struct {
	key         string
	fallback    string
	description string
}{
	{"data_dir", "data", "where published modules are stored, outside the application"},
	{"access", "public", "public, or secured to require a token for publishing"},
	{"title", "Rho module registry", "the name shown on the pages"},
	{"site_url", "https://rho-lang.org", "where the footer links to"},
	// The address this service is published under, which is what the pages of a
	// module's documentation lead back to. It is a setting rather than something
	// read from the request because the documentation is generated once, at
	// publish, from outside any request.
	{"registry_url", "/", "the address this service is published under, used by generated documentation"},
	{"token_days", "30", "how long an issued token lasts"},
	{"token_max_days", "90", "the longest a token may ever last"},
	{"open_tokens", "true", "whether a token can be issued without holding one"},
	{"max_upload_mb", "256", "the largest package accepted, in megabytes"},
}

// SettingDefaults lists the settings and their defaults, so a service can
// report its own configuration.
func SettingDefaults() [][2]string {
	out := make([][2]string, 0, len(registrySettings))
	for _, setting := range registrySettings {
		out = append(out, [2]string{setting.key, configGet(setting.key, setting.fallback)})
	}
	return out
}

// AccessSecured reports whether publishing needs a credential. The default is
// an open registry, which suits a trusted network; setting access = secured
// turns rho-auth on for writes.
func AccessSecured() bool {
	return strings.EqualFold(configGet("access", "public"), "secured")
}

// MaxUploadMB is the largest package accepted, in megabytes.
func MaxUploadMB() uint32 { return configUint("max_upload_mb", 256) }

// The settings accessors the generated aliases reach. Each one names a single
// setting, so the defaults are stated in exactly one place.
func configGet(key, fallback string) string {
	if value, ok := registryConfig[key]; ok && value != "" {
		return value
	}
	return fallback
}

func configGetAccess() string  { return configGet("access", "public") }
func configGetTitle() string  { return configGet("title", "Rho module registry") }
func configGetSiteURL() string { return configGet("site_url", "https://rho-lang.org") }
func configGetRegistryURL() string { return configGet("registry_url", "/") }
func clockYear() uint32        { return uint32(time.Now().Year()) }

func registryTokenDays() uint32 {
	normal, _ := TokenDays()
	return normal
}

func registryTokenMaxDays() uint32 {
	_, maximum := TokenDays()
	return maximum
}



// OpenTokens reports whether a token can be issued without holding one.
func OpenTokens() bool {
	value := strings.ToLower(configGet("open_tokens", "true"))
	return value != "false" && value != "no" && value != "0"
}

// configUint reads a numeric setting, falling back when it is unusable.
func configUint(key string, fallback uint32) uint32 {
	value, err := strconv.ParseUint(strings.TrimSpace(configGet(key, "")), 10, 32)
	if err != nil {
		return fallback
	}
	return uint32(value)
}

// newFSStore returns a store rooted at the configured data directory.
func newFSStore() *fsStore {
	root := os.Getenv("RHO_REGISTRY_DATA")
	if root == "" {
		root = configGet("data_dir", "")
	}
	if root == "" {
		root = "data"
	}
	return &fsStore{root: root}
}

// dataDir is the configured data directory.
func dataDir() string { return newFSStore().root }

// storeDataDir is the Go spelling a Rho handler reaches.
func storeDataDir() string { return dataDir() }

// registryVersion is the manifest version, so a page can report which build of
// the registry is running.
func registryVersion() string { return rhoModuleVersion }





// TokenDays reports the normal and maximum token lifetimes in days.
func TokenDays() (uint32, uint32) {
	normal, maximum := configUint("token_days", 30), configUint("token_max_days", 90)
	if maximum < 1 {
		maximum = 90
	}
	if normal < 1 {
		normal = 30
	}
	// A token may never outlive the ceiling, whatever the settings say.
	if normal > maximum {
		normal = maximum
	}
	return normal, maximum
}







// versionDir is the directory holding one module version.
func (s *fsStore) versionDir(name, version string) string {
	return filepath.Join(s.root, "modules", name, version)
}

// safeSegment rejects a path element that could escape the data directory.
func safeSegment(value string) error {
	if value == "" {
		return fmt.Errorf("empty path segment")
	}
	if value == "." || value == ".." || strings.ContainsAny(value, ` + "`/\\`" + `) {
		return fmt.Errorf("invalid path segment %q", value)
	}
	return nil
}

// put stores an uploaded package and its metadata.
//
// The author is checked against what the registry already holds for the name
// before anything is written, because a module that is written and then refused
// is a module that exists and cannot be withdrawn.
// put stores an uploaded package and its metadata.
//
// The owner is the identity the store acts on behalf of, which the handler has
// taken from the caller's credential. A name that is already held is not a
// first publish, and the identities recorded against it are the only ones that
// may add a version, so a re-publish cannot take a module over.
func (s *fsStore) put(name, version, owner string, payload []byte, meta moduleMeta) (int, error) {
	if err := safeSegment(name); err != nil {
		return 0, err
	}
	if err := safeSegment(version); err != nil {
		return 0, err
	}
	if owner == "" {
		// A name needs an owner, or nobody can ever add a version to it or
		// withdraw it. On an open registry the owner is the author the manifest
		// names, which is at least a claim a reader can see.
		owner = meta.Author
	}
	if owner == "" {
		return 0, fmt.Errorf("a module must name its author to be published")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	held, err := s.publisherLocked(name)
	if err != nil {
		return 0, err
	}
	if held != nil {
		if !held.MayPublish(owner) {
			return 0, fmt.Errorf("%s is published by %s; only %s may publish it",
				name, held.Owner, held.holders())
		}
		if s.hasLocked(name, version) {
			// Same name, same version, is the same artifact. Two identities
			// claiming one version is either a mistake or a takeover, and
			// either way it is refused here rather than discovered later.
			return 0, fmt.Errorf("%s@%s is already published", name, version)
		}
	}

	dir := s.versionDir(name, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(dir, "package.rho-pkg"), payload, 0o644); err != nil {
		return 0, err
	}
	encoded, err := jsonenc.MarshalIndent(meta, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), encoded, 0o644); err != nil {
		return 0, err
	}
	if held == nil {
		if err := s.claimName(name, owner, meta.Maintainer, meta.Visibility); err != nil {
			return 0, err
		}
	}
	if err := s.writeIndex(name); err != nil {
		return 0, err
	}
	return len(payload), nil
}

// publisherFile is where a module's publisher is recorded.
func (s *fsStore) publisherFile(name string) string {
	return filepath.Join(s.root, "index", name+".publisher.json")
}

// publisher returns who may withdraw a module, or nil when the module is not
// held.
func (s *fsStore) publisher(name string) (*modulePublisher, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publisherLocked(name)
}

// publisherLocked is the reading half, for a caller that already holds the lock.
func (s *fsStore) publisherLocked(name string) (*modulePublisher, error) {
	if err := safeSegment(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.publisherFile(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	held := &modulePublisher{}
	if err := jsonenc.Unmarshal(data, held); err != nil {
		return nil, fmt.Errorf("the publisher record for %s cannot be read: %w", name, err)
	}
	return held, nil
}

// claimName records who published a module name, once, and how visible it is.
// It is written before the version is, so a name is never held by nobody.
//
// The owner is the only maintainer for now, because they are the same identity.
// The list is kept anyway so that letting a second identity in later is a change
// to what is stored rather than a change to what is read.
func (s *fsStore) claimName(name, owner, maintainer, visibility string) error {
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	if visibility != VisibilityPrivate {
		visibility = VisibilityPublic
	}
	held := &modulePublisher{
		Name:           name,
		Owner:          owner,
		Maintainers:    uniqueStrings([]string{owner, maintainer}),
		Visibility:     visibility,
		FirstPublished: time.Now().UTC(),
	}
	encoded, err := jsonenc.MarshalIndent(held, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.root, "index"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(s.publisherFile(name), encoded, 0o644)
}

// remove deletes a published version: its package, the documentation generated
// from it, the unpacked sources, and its entry in the index.
//
// All of it goes, because a version that is half removed is a version that
// still answers a search, still serves its documentation, and no longer holds
// the package those were made from. A registry that cannot withdraw a version is
// a registry a mistaken publish is permanent in.
func (s *fsStore) remove(name, version string) error {
	if err := safeSegment(name); err != nil {
		return err
	}
	if err := safeSegment(version); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// The version itself, which is where the package, the metadata, the
	// unpacked sources and the generated documentation all live.
	if err := os.RemoveAll(s.versionDir(name, version)); err != nil {
		return err
	}
	// The module's index, which lists every version it holds. It is rewritten
	// rather than removed so that the last version of a module leaving takes
	// the module's own entry with it, and a module that still has versions keeps
	// the ones it has.
	versions := s.versions(name)
	if len(versions) == 0 {
		if err := os.Remove(filepath.Join(s.root, "index", name+".json")); err != nil && !os.IsNotExist(err) {
			return err
		}
		// The name goes with its last version: a name nobody has published
		// again is free to be claimed by someone else, which is the behaviour a
		// name that has been fully withdrawn should have.
		if err := os.Remove(s.publisherFile(name)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return os.RemoveAll(filepath.Join(s.root, "modules", name))
	}
	return s.writeIndex(name)
}

// get returns the stored package for a module version.
func (s *fsStore) get(name, version string) ([]byte, error) {
	if err := safeSegment(name); err != nil {
		return nil, err
	}
	if err := safeSegment(version); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return os.ReadFile(filepath.Join(s.versionDir(name, version), "package.rho-pkg"))
}

// has reports whether a module version is stored.
func (s *fsStore) has(name, version string) bool {
	if safeSegment(name) != nil || safeSegment(version) != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hasLocked(name, version)
}

// hasLocked is has() for a caller that already holds the lock.
//
// It is separate because a sync.RWMutex is not reentrant: a writer that holds
// the lock and then calls has would block on its own read lock, forever. put is
// exactly that caller — it holds the write lock across the check, the write and
// the index update so that a concurrent publish cannot interleave.
func (s *fsStore) hasLocked(name, version string) bool {
	if safeSegment(name) != nil || safeSegment(version) != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(s.versionDir(name, version), "package.rho-pkg"))
	return err == nil
}

// versionsDescending lists a module's stored versions, newest first, which is
// the order a page shows them in.
func (s *fsStore) versionsDescending(name string) []string {
	versions := s.versions(name)
	out := make([]string, 0, len(versions))
	for i := len(versions) - 1; i >= 0; i-- {
		out = append(out, versions[i])
	}
	return out
}

// versions lists a module's stored versions, lowest first.
func (s *fsStore) versions(name string) []string {
	if safeSegment(name) != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "modules", name))
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool { return compareVersions(out[i], out[j]) < 0 })
	return out
}

// writeIndex rewrites a module's index entry from what is on disk.
func (s *fsStore) writeIndex(name string) error {
	if err := os.MkdirAll(filepath.Join(s.root, "index"), 0o755); err != nil {
		return err
	}
	document := map[string]any{"name": name, "versions": s.versions(name)}
	encoded, err := jsonenc.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.root, "index", name+".json"), encoded, 0o644)
}

// search returns the metadata of every stored module matching the query, case
// insensitively. An empty query lists everything.
// searchFor returns the modules matching a query, as seen by one identity.
//
// A private module is only visible to its owner. That is what private means: a
// module nobody else can install is a module nobody else should be told exists,
// because the name and the description leak whether the contents do not.
//
// mineOnly asks for the viewer's own modules, whatever their visibility, which
// is how an owner finds what they published and sees whether it is public.
func (s *fsStore) searchFor(query, viewer string, mineOnly bool) []moduleMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(filepath.Join(s.root, "modules"))
	if err != nil {
		return nil
	}
	needle := strings.ToLower(query)
	out := []moduleMeta{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		held, err := s.publisherLocked(entry.Name())
		if err != nil {
			continue
		}
		if held == nil {
			// A module with no record of who published it predates the record or
			// was written by hand. It is served, and it belongs to nobody, so no
			// one's private view includes it.
			held = &modulePublisher{Name: entry.Name(), Visibility: VisibilityPublic}
		}
		owns := viewer != "" && held.Owner == viewer
		if mineOnly && !owns {
			continue
		}
		if held.IsPrivate() && !owns {
			continue
		}
		versions, err := os.ReadDir(filepath.Join(s.root, "modules", entry.Name()))
		if err != nil {
			continue
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			meta, ok := readMeta(filepath.Join(s.root, "modules", entry.Name(), version.Name()))
			if !ok {
				continue
			}
			if needle != "" &&
				!strings.Contains(strings.ToLower(meta.Name), needle) &&
				!strings.Contains(strings.ToLower(meta.Description), needle) {
				continue
			}
			// What the caller is allowed to know travels with the entry. A public
			// module names who published it, because that is public information
			// and is who a reader would ask about it; a private module is not in
			// the answer at all unless the caller owns it, so its owner is not
			// disclosed by being asked about.
			meta.Ownership = held.Owner
			meta.Visibility = held.Visibility
			meta.Mine = owns
			out = append(out, meta)
		}
	}
	return out
}

// setVisibility changes whether a module is anybody's to install or its owner's
// alone. The caller has already established that the identity asking is allowed
// to; the store records the answer.
func (s *fsStore) setVisibility(name, visibility string) error {
	if err := safeSegment(name); err != nil {
		return err
	}
	visibility = strings.ToLower(strings.TrimSpace(visibility))
	if visibility != VisibilityPrivate {
		visibility = VisibilityPublic
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	held, err := s.publisherLocked(name)
	if err != nil {
		return err
	}
	if held == nil {
		return fmt.Errorf("%s is not published here", name)
	}
	held.Visibility = visibility
	encoded, err := jsonenc.MarshalIndent(held, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.publisherFile(name), encoded, 0o644)
}

// readMeta reads the metadata of one stored version.
func readMeta(dir string) (moduleMeta, bool) {
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return moduleMeta{}, false
	}
	var meta moduleMeta
	if err := jsonenc.Unmarshal(data, &meta); err != nil {
		return moduleMeta{}, false
	}
	return meta, true
}

// compareVersions orders dotted numeric versions.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := parseVersionNumber(as[i])
		bn, berr := parseVersionNumber(bs[i])
		if aerr != nil || berr != nil {
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
			continue
		}
		if an != bn {
			if an < bn {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

func parseVersionNumber(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("empty")
	}
	total := 0
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return 0, fmt.Errorf("not a number")
		}
		total = total*10 + int(value[i]-'0')
	}
	return total, nil
}

/* -------------------------------------------------------------------------- */
/*  Documentation                                                              */
/*                                                                             */
/*  A registry documents a module from what it received: the published package */
/*  is unpacked into a source tree, the documentation generator is run over that */
/*  tree, and the pages it wrote are served from here. The generator writes the */
/*  files itself, so the store's part is the directory they go in, unpacking,  */
/*  and reading a page back.                                                    */
/* -------------------------------------------------------------------------- */

// docsDir is where the generated pages of one version live. It sits beside the
// modules rather than inside them, because a page is not part of the package: it
// is derived from it, and a re-publish replaces it.
func (s *fsStore) docsDir(name, version string) string {
	return filepath.Join(s.root, "docs", name, version)
}

// sourceDir is where a published package is unpacked, which is the tree the
// generator is given.
func (s *fsStore) sourceDir(name, version string) string {
	return filepath.Join(s.versionDir(name, version), "src")
}

// unpack extracts a published package into the version's source directory and
// returns it, or "" when the package cannot be read.
//
// An entry that would leave the directory is refused rather than written, so a
// package cannot write outside the data directory by naming an entry "../../".
func (s *fsStore) unpack(name, version string) (string, error) {
	if err := safeSegment(name); err != nil {
		return "", err
	}
	if err := safeSegment(version); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := os.ReadFile(filepath.Join(s.versionDir(name, version), "package.rho-pkg"))
	if err != nil {
		return "", err
	}
	// The previous tree is replaced rather than merged: a package may carry a
	// different file list, and leaving the old one behind would document files
	// this version does not have.
	dest := s.sourceDir(name, version)
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	reader, err := gzip.NewReader(bytesReader(payload))
	if err != nil {
		return "", err
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		target := filepath.Join(dest, filepath.FromSlash(header.Name))
		// The check is on the cleaned path, so an entry that climbs out through
		// a subdirectory is refused as well as one that names ".." directly.
		if !withinDir(dest, target) {
			return "", fmt.Errorf("package entry %q leaves the package directory", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(file, archive); err != nil {
				file.Close()
				return "", err
			}
			if err := file.Close(); err != nil {
				return "", err
			}
		}
	}
	return dest, nil
}

// docPage reads one generated page, or "" when there is none.
func (s *fsStore) docPage(name, version, page string) ([]byte, error) {
	if err := safeSegment(name); err != nil {
		return nil, err
	}
	if err := safeSegment(version); err != nil {
		return nil, err
	}
	if page == "" {
		page = "index.html"
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return os.ReadFile(filepath.Join(s.docsDir(name, version), filepath.FromSlash(page)))
}

// hasDocs reports whether a version has documentation, which is what a page
// checks before it offers a link to it.
func (s *fsStore) hasDocs(name, version string) bool {
	if safeSegment(name) != nil || safeSegment(version) != nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, err := os.Stat(filepath.Join(s.docsDir(name, version), "index.html"))
	return err == nil
}

// bytesReader reads a payload without copying it.
func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }

// withinDir reports whether a path is inside a directory.
func withinDir(dir, path string) bool {
	relative, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

/* -------------------------------------------------------------------------- */
/*  Port entry points                                                          */
/*                                                                             */
/*  These satisfy the extern declarations a Rho module makes. The Go spelling  */
/*  is lowerCamel, which is how a Rho call site is emitted.                     */
/* -------------------------------------------------------------------------- */

// storeUnpack extracts a published package and returns the source tree, which
// is what a documentation generator is given. The empty string means the package
// could not be read.
func storeUnpack(name, version string) string {
	dir, err := newFSStore().unpack(name, version)
	if err != nil {
		return ""
	}
	return dir
}

// storeDocPage returns a generated documentation page, or "" when it is absent.
func storeDocPage(name, version, page string) string {
	data, err := newFSStore().docPage(name, version, page)
	if err != nil {
		return ""
	}
	return string(data)
}

// docHomeSet is called at start-up by the service that publishes documentation.
//
// The address a site is published under is a property of the service, not of the
// generator, so the service passes it in: a reader who followed a link out of a
// registry and into a module's documentation needs a way back, and a generator
// cannot invent the address it was published at.
func docHomeSet(home string, title string) {
	if home != "" {
		os.Setenv("RHO_DOC_HOME", home)
	}
	if title != "" {
		os.Setenv("RHO_DOC_HOME_TITLE", title)
	}
}

// storeDocHome is the Rho-facing name of the setter, so a service that publishes
// documentation declares it as an extern and sets the address at start-up
// without the generated file needing to know who calls it.
func storeDocHome(home string, title string) {
	docHomeSet(home, title)
}

// storeHasDocs reports whether a version has documentation published for it.
func storeHasDocs(name, version string) bool {
	return newFSStore().hasDocs(name, version)
}

// storePut stores an uploaded package and its metadata, returning 0 on success.
//
// The owner is the identity the caller presented, which the handler has taken
// from its credential; the store compares it against what the module already
// records rather than reading a token itself.
func storePut(name, version, owner string, payload string, metaJSON string) uint32 {
	meta := moduleMeta{}
	if metaJSON != "" {
		if err := jsonenc.Unmarshal([]byte(metaJSON), &meta); err != nil {
			return 1
		}
	}
	if meta.Name == "" {
		meta.Name = name
	}
	if meta.Version == "" {
		meta.Version = version
	}
	if meta.Features == nil {
		meta.Features = []string{}
	}
	if meta.Deps == nil {
		meta.Deps = map[string]string{}
	}
	if _, err := newFSStore().put(name, version, owner, []byte(payload), meta); err != nil {
		return 1
	}
	return 0
}

// storeGet returns the stored package, or "" when it is absent.
func storeGet(name, version string) string {
	data, err := newFSStore().get(name, version)
	if err != nil {
		return ""
	}
	return string(data)
}

// storeHas reports whether a version is already published.
func storeHas(name, version string) bool {
	return newFSStore().has(name, version)
}

// storeDelete withdraws a published version, and reports 0 when it is gone.
//
// The publisher is not checked here: the store holds what the registry told it
// to hold, and who is allowed to say so is a question about credentials, which
// the handler answers before it reaches this.
func storeDelete(name, version string) uint32 {
	store := newFSStore()
	if err := store.remove(name, version); err != nil {
		log.Printf("store: delete %s@%s: %v", name, version, err)
		return 1
	}
	return 0
}

// storeSetVisibility records whether a module is anybody's to install or its
// owner's alone. The caller has already established that the identity asking may
// change it; the store records the answer.
func storeSetVisibility(name, visibility string) uint32 {
	if err := newFSStore().setVisibility(name, visibility); err != nil {
		log.Printf("store: visibility %s: %v", name, err)
		return 1
	}
	return 0
}

// storePublisher returns a JSON document naming who may withdraw a module, or ""
// when the module is not held here. It is the registry's answer to "may this
// caller unpublish", and it is read rather than kept in a handler so that the
// answer is the same one the store would act on.
func storePublisher(name string) string {
	held, err := newFSStore().publisher(name)
	if err != nil || held == nil {
		return ""
	}
	encoded, err := jsonenc.Marshal(held)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// storeSearch returns a JSON array of matching module metadata, as seen by one
// identity. The viewer is the identity the caller presented, or "" for a caller
// who presented none; a private module is only in the answer for its owner.
func storeSearch(query, viewer string, mineOnly bool) string {
	results := newFSStore().searchFor(query, viewer, mineOnly)
	if results == nil {
		// An empty registry is an empty list, not a null, so a Rho handler
		// walking the response finds nothing rather than a missing value.
		return "[]"
	}
	encoded, err := jsonenc.Marshal(results)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// storeVersions returns a JSON document listing a module's versions.
func storeVersions(name string) string {
	s := newFSStore()
	versions := s.versions(name)
	if versions == nil {
		versions = []string{}
	}
	encoded, err := jsonenc.Marshal(map[string]any{
		"name":     name,
		"versions": versions,
	})
	if err != nil {
		return fmt.Sprintf("{\"name\":%q,\"versions\":[]}", name)
	}
	return string(encoded)
}






// storeDetail lists one module's versions, newest first, as tab-separated
// lines of name, version, type, description, main and command.
//
// The fields a Rho handler needs are provided directly, because a Rho handler
// cannot walk a nested document: it has no way to iterate a JSON array.
func storeDetail(name string) string {
	s := newFSStore()
	var out strings.Builder
	for _, version := range s.versionsDescending(name) {
		meta, ok := readMeta(filepath.Join(s.root, "modules", name, version))
		if !ok {
			continue
		}
		out.WriteString(detailLine(meta))
		out.WriteByte('\n')
	}
	return out.String()
}

// storeMeta lists the published modules, newest version first per module, in
// the same tab-separated shape. A listing for the index and for search.
//
// It is the store's own listing rather than a rendering of the JSON, because
// Rho cannot walk a nested document: a handler that wants to render a list gets
// text it can read line by line. What it may see is the viewer's, so a private
// module is absent for anyone but its owner, exactly as it is from a search.
func storeMeta(query, viewer string, mineOnly bool) string {
	results := newFSStore().searchFor(query, viewer, mineOnly)
	// The search returns versions oldest first, so the newest of each module is
	// the last one seen. It is kept in order of first appearance, which keeps
	// the listing sorted by name.
	order := []string{}
	newest := map[string]moduleMeta{}
	for _, meta := range results {
		if _, seen := newest[meta.Name]; !seen {
			order = append(order, meta.Name)
		}
		newest[meta.Name] = meta
	}
	var out strings.Builder
	for _, name := range order {
		out.WriteString(detailLine(newest[name]))
		out.WriteByte('\n')
	}
	return out.String()
}

// storeLatest returns the newest version of a module, or "" when it has none.
func storeLatest(name string) string {
	s := newFSStore()
	versions := s.versionsDescending(name)
	if len(versions) == 0 {
		return ""
	}
	return versions[0]
}


// detailLine renders one module version for a Rho handler to read.
func detailLine(meta moduleMeta) string {
	fields := []string{
		meta.Name,
		meta.Version,
		meta.Type,
		meta.Description,
		meta.Main,
		meta.Command,
	}
	// A field must not break the line format, so tabs and newlines are dropped.
	for i, field := range fields {
		fields[i] = strings.ReplaceAll(strings.ReplaceAll(field, "\t", " "), "\n", " ")
	}
	return strings.Join(fields, "\t")
}

// storeList returns the matching modules as "name<TAB>version" lines, one per
// module, sorted by name then version.
//
// A line-based listing exists so a Rho handler can render it without parsing
// JSON: Rho has no way to walk a nested document, and the store already holds
// the fields as strings.
func storeList(query, viewer string, mineOnly bool) string {
	results := newFSStore().searchFor(query, viewer, mineOnly)
	if results == nil {
		return ""
	}
	var out strings.Builder
	for _, meta := range results {
		out.WriteString(meta.Name)
		out.WriteByte('\t')
		out.WriteString(meta.Version)
		out.WriteByte('\n')
	}
	return out.String()
}
`
}
