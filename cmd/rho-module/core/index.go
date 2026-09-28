//
// @file index.go
// @brief The local registry: what is installed, what is selected, where it came from.
//
// `~/.rho` is the local registry, and it holds two different things that are
// easy to confuse:
//
//   - What is installed, which lives in `modules/installed/<kind>/<name>@<version>`
//     and is the only thing the toolchain dispatches to, the dependency resolver
//     resolves against, and a Rho import finds. The tree is the truth about
//     availability: if a module is not in the tree it is not installed, however
//     many places say it is.
//
//   - What is known about it, which lives in `registry/local/registry.yaml`:
//     which kind it is, which version is selected, and which registry it came
//     from. The tree cannot say any of those, so the index is where they live.
//
// The index is therefore never asked what is installed, only which of the
// installed versions is selected and where it came from. That division is why
// the two cannot disagree about availability: the tree answers it, and the index
// is reconciled against the tree whenever it is read, so a directory that was
// removed by hand is dropped from the index and a module that was installed by
// something other than `rho module publish` is added to it.
//
// A registry query is a view over both: the installed set, and the cached view
// of what the remote has. They are reported separately and never merged, because
// a module in the remote view is a module this machine does not have.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/internal/paths"
)

// The kinds the local registry tracks. A kind is the directory a module is
// installed under, and the toolchain resolves against all of them, so the index
// has to know all of them: a kind the index does not track is a module that is
// installed and invisible.
const (
	KindComponent = "component"
	KindPlugin    = "plugin"
	KindModule    = "module"
	KindLibrary   = "library"
)

// IndexKinds are the kinds a local registry records, in the order a listing
// shows them.
var IndexKinds = []string{KindComponent, KindPlugin, KindModule, KindLibrary}

// originLocal is a module that came from this machine: published from a
// directory, rather than fetched from a registry.
const originLocal = "local"

// originRemote is a module that came from a configured remote registry.
const originRemote = "remote"

// readIndex reads the index without reconciling it, for the reads that only
// need to know what the index says.
func readIndex() (*Registry, error) {
	path, err := registryFilePath()
	if err != nil {
		return nil, err
	}
	return readRegistry(path)
}

// originOf returns where the index says a module came from.
func originOf(registry *Registry, kind, name, version string) (string, bool) {
	for _, entry := range *versionBucketOf(registry, kind) {
		if entry.Name == name && entry.Version == version && entry.Origin != "" {
			return entry.Origin, true
		}
	}
	return "", false
}

// InstalledEntry is one installed module, read from the tree and reconciled into
// the index.
type InstalledEntry struct {
	// Name and Version are what the directory is called, which is the only place
	// they are written down.
	Name    string
	Version string
	// Kind is the directory under modules/installed this module was found in.
	Kind string
	// Selected is true when this is the version the toolchain will resolve. It
	// is the index that says so, and the tree that has to contain it.
	Selected bool
	// Origin is where it came from: this machine, or a remote registry.
	Origin string
	// Path is where it is installed.
	Path string

	// selected is whether the index names this version, which is a fact about
	// the index rather than about the directory, so it is not a field the
	// listing writes.
	selected bool
}

// IsRemote reports whether a module came from a configured remote registry.
func (e InstalledEntry) IsRemote() bool { return e.Origin == originRemote }

// IsSelected reports whether this is the version the toolchain will resolve.
func (e InstalledEntry) IsSelected() bool { return e.selected }

// installedRoot is the directory the installed tree hangs from.
func installedRoot() (string, error) {
	home, err := paths.GetRhoHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "modules", "installed"), nil
}

// ReadInstalled reads the installed tree, annotated with what the index says
// about each entry.
//
// The tree is the truth about what this machine has: a directory is what the
// toolchain dispatches to and a Rho import resolves against, so an entry in the
// index whose directory is gone describes nothing. What a directory cannot say
// is which of two installed versions is selected, or whether the module was
// published from this machine or fetched from a registry, and that is what the
// index contributes here.
func ReadInstalled() ([]InstalledEntry, error) {
	root, err := installedRoot()
	if err != nil {
		return nil, err
	}
	out := []InstalledEntry{}
	for _, kind := range IndexKinds {
		entries, err := os.ReadDir(filepath.Join(root, kind))
		if err != nil {
			// A kind nothing is installed under is not an error: a fresh
			// installation has no modules at all.
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name, version, cut := strings.Cut(entry.Name(), "@")
			if !cut || name == "" || version == "" {
				continue
			}
			out = append(out, InstalledEntry{
				Name:    name,
				Version: version,
				Kind:    kind,
				// The origin is left empty until the index says otherwise: a
				// directory cannot tell whether a module was published from
				// this machine or fetched, and guessing local would rewrite a
				// fetched one on every reconciliation.
				Origin:   "",
				Path:     filepath.Join(root, kind, entry.Name()),
				selected: false,
			})
		}
	}
	// The index is read, not required: a home whose index is unreadable still
	// has an installed tree, and a tree is enough to say what is installed.
	if registry, err := readIndex(); err == nil {
		for i := range out {
			version, selected := SelectedVersion(registry, out[i].Kind, out[i].Name)
			out[i].selected = selected && version == out[i].Version
			if origin, found := originOf(registry, out[i].Kind, out[i].Name, out[i].Version); found {
				out[i].Origin = origin
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		left, right := kindOrder(out[i].Kind), kindOrder(out[j].Kind)
		if left != right {
			return left < right
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return compareModuleVersions(out[i].Version, out[j].Version) < 0
	})
	return out, nil
}

// kindOrder is where a kind sits in a listing, so the groups come out in the
// order a reader expects rather than in the order the filesystem happened to
// return them.
func kindOrder(kind string) int {
	for i, candidate := range IndexKinds {
		if candidate == kind {
			return i
		}
	}
	return len(IndexKinds)
}

// InstalledVersions returns the versions installed for one module of one kind.
func InstalledVersions(kind, name string) []string {
	entries, err := ReadInstalled()
	if err != nil {
		return nil
	}
	out := []string{}
	for _, entry := range entries {
		if entry.Kind == kind && entry.Name == name {
			out = append(out, entry.Version)
		}
	}
	return out
}

// IsInstalled reports whether a version of a module is installed, under a kind.
func IsInstalled(kind, name, version string) bool {
	for _, entry := range allInstalled() {
		if entry.Kind == kind && entry.Name == name && entry.Version == version {
			return true
		}
	}
	return false
}

// IsInstalledVersion reports whether a version of a module is installed under any
// kind, which is what a caller that only knows a name and a version can ask.
func IsInstalledVersion(name, version string) bool {
	for _, entry := range allInstalled() {
		if entry.Name == name && entry.Version == version {
			return true
		}
	}
	return false
}

// allInstalled is ReadInstalled without the error, for the questions that
// tolerate an unreadable tree by answering "nothing is there".
func allInstalled() []InstalledEntry {
	entries, err := ReadInstalled()
	if err != nil {
		return nil
	}
	return entries
}

// compareModuleVersions orders dotted numeric versions, so the newest installed
// version is the one a listing shows first and the one a resolution prefers.
func compareModuleVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		left, right := 0, 0
		if i < len(as) {
			left = atoi(as[i])
		}
		if i < len(bs) {
			right = atoi(bs[i])
		}
		if left != right {
			if left < right {
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
	default:
		return 0
	}
}

// atoi reads the leading digits of a version part, which is all a dotted numeric
// version has, and treats anything else as 0 so a pre-release suffix orders
// before the release it leads to rather than panicking on it.
func atoi(text string) int {
	result := 0
	for _, r := range text {
		if r < '0' || r > '9' {
			break
		}
		result = result*10 + int(r-'0')
	}
	return result
}

/* -------------------------------------------------------------------------- */
/*  The index                                                                  */
/* -------------------------------------------------------------------------- */

// Reconcile brings the index into step with the installed tree and writes it
// back.
//
// It is called before anything reads the index, because the index is the only
// record of which installed version is selected and where it came from, and
// neither survives a directory being moved or removed by hand. Reconciling on
// read means the index heals rather than being trusted.
func Reconcile() (*Registry, error) {
	registryFile, err := registryFilePath()
	if err != nil {
		return nil, err
	}
	registry, err := readRegistry(registryFile)
	if err != nil {
		return nil, err
	}

	installed := allInstalled()
	// What the index claimed, so a module that is gone is dropped.
	claimed := map[string]bool{}
	for _, kind := range IndexKinds {
		for _, entry := range *versionBucketOf(registry, kind) {
			claimed[claimKey(kind, entry.Name, entry.Version)] = true
		}
	}
	// Anything installed is recorded, and the index is told where it came from
	// when it does not yet know.
	for _, entry := range installed {
		claimed[claimKey(entry.Kind, entry.Name, entry.Version)] = true
		recordInstalled(registry, entry)
	}
	// Anything the index claimed that is not in the tree is dropped, because a
	// module that is not on disk is not available however well recorded it is.
	//
	// The test is against the tree, not the index: the loop above has just added
	// the tree to the index, so asking the index would find everything it was
	// asked about and nothing would ever be dropped.
	onDisk := map[string]bool{}
	for _, entry := range installed {
		onDisk[claimKey(entry.Kind, entry.Name, entry.Version)] = true
	}
	for key := range claimed {
		if onDisk[key] {
			continue
		}
		kind, name, version, ok := splitClaim(key)
		if !ok {
			continue
		}
		removeInstalled(registry, kind, name, version)
	}

	if err := writeRegistry(registryFile, registry); err != nil {
		return nil, err
	}
	return registry, nil
}

// recordInstalled adds an installed module to the index, and selects it when no
// version of it is selected yet.
//
// It records a name@version pair rather than a name. A module with two versions
// installed is two entries: recording the second as "already known" because the
// name is known would leave the index describing a version that is not there, and
// the selection would then name a version the tree does not have.
func recordInstalled(registry *Registry, entry InstalledEntry) {
	versions := versionBucketOf(registry, entry.Kind)
	known := false
	for _, existing := range *versions {
		if existing.Name == entry.Name && existing.Version == entry.Version {
			known = true
			break
		}
	}
	if !known {
		*versions = append(*versions, VersionEntry{
			Name: entry.Name, Version: entry.Version, Origin: entry.Origin,
		})
	}
	// A module with nothing selected resolves to nothing, so the newest
	// installed version is selected when it arrives. That is the same rule the
	// resolver would apply, stated once in the index.
	if !hasActiveName(registry, entry.Kind, entry.Name) {
		if newest := newestOf(*versionBucketOf(registry, entry.Kind), entry.Name); newest != "" {
			setActiveName(registry, entry.Kind, entry.Name, newest)
		}
	}
}

// removeInstalled drops a version from the index, and clears the selection when
// that version was the selected one.
func removeInstalled(registry *Registry, kind, name, version string) {
	versions := versionBucketOf(registry, kind)
	kept := []VersionEntry{}
	for _, entry := range *versions {
		if entry.Name == name && entry.Version == version {
			continue
		}
		kept = append(kept, entry)
	}
	*versions = kept

	active := activeBucketOf(registry, kind)
	selected := ""
	keptActive := []ActiveEntry{}
	for _, entry := range *active {
		if entry.Name == name {
			selected = entry.Version
			continue
		}
		keptActive = append(keptActive, entry)
	}
	*active = keptActive
	// A module that lost its selected version selects another one if it has
	// one, rather than resolving to nothing while a version sits installed.
	if selected != "" {
		if newest := newestOf(*versions, name); newest != "" {
			*active = append(*active, ActiveEntry{Name: name, Version: newest})
		}
	}
}

// writeIndex writes the index to disk.
//
// It is the same function the publisher and the index read share, so a list
// that is written and read by two different functions is a list whose two
// versions can drift. The writer goes through one loop over IndexKinds rather
// than eight blocks of the same three lines, because a kind that is written but
// not read is a kind that silently loses its entries.
func writeIndex(registryPath string, registry *Registry) error {
	var out strings.Builder
	for _, kind := range IndexKinds {
		out.WriteString(indexSectionKey(kind) + ":\n")
		for _, entry := range *versionBucketOf(registry, kind) {
			fmt.Fprintf(&out, "  - name: %s\n    version: %s\n", entry.Name, entry.Version)
			if entry.Origin != "" {
				fmt.Fprintf(&out, "    origin: %s\n", entry.Origin)
			}
		}
		out.WriteString("active_" + indexSectionKey(kind) + ":\n")
		for _, entry := range *activeBucketOf(registry, kind) {
			fmt.Fprintf(&out, "  - name: %s\n    version: %s\n", entry.Name, entry.Version)
		}
	}
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(registryPath, []byte(out.String()), 0o644)
}

// parseIndex reads the index.
//
// It is a line reader rather than a general YAML parser because the index is
// written by writeIndex and is not a document anyone edits: one list per kind,
// one entry per line, three fields. A parser for that shape cannot disagree
// with the writer for it, which is the property that matters.
func parseIndex(data string, registry *Registry) {
	lines := strings.Split(data, "\n")
	// The section being read, and the entry being accumulated. A name opens an
	// entry and a version closes it, which is the order writeIndex emits.
	section := ""
	name := ""
	version := ""
	origin := ""

	flush := func() {
		if name == "" {
			return
		}
		kind, selection, known := kindOfSection(section)
		if known && version != "" {
			if selection {
				*activeBucketOf(registry, kind) = append(*activeBucketOf(registry, kind),
					ActiveEntry{Name: name, Version: version})
			} else {
				bucket := versionBucketOf(registry, kind)
				known := false
				for _, existing := range *bucket {
					if existing.Name == name && existing.Version == version {
						known = true
						break
					}
				}
				if !known {
					*bucket = append(*bucket, VersionEntry{Name: name, Version: version, Origin: origin})
				}
			}
		}
		name, version, origin = "", "", ""
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
			flush()
			version = ""
			section = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if value, found := cutPrefix(trimmed, "- name:"); found {
			flush()
			name = strings.TrimSpace(value)
			continue
		}
		if value, found := cutPrefix(trimmed, "version:"); found {
			version = strings.TrimSpace(value)
			continue
		}
		if value, found := cutPrefix(trimmed, "origin:"); found {
			origin = strings.TrimSpace(value)
		}
	}
	flush()
}
func claimKey(kind, name, version string) string {
	return kind + "\x00" + name + "\x00" + version
}

// splitClaim splits the key a claim is recorded under.
func splitClaim(key string) (kind, name, version string, ok bool) {
	parts := strings.Split(key, "\x00")
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// newestOf returns the newest version of a module in a bucket, which is what a
// module resolves to when the index selects nothing.
func newestOf(versions []VersionEntry, name string) string {
	return newestOfExcept(versions, name, "")
}

// newestOfExcept returns the newest version of a module other than one, which is
// what a deactivation selects next: the version that is left rather than the one
// the user asked to stop using.
func newestOfExcept(versions []VersionEntry, name, except string) string {
	newest := ""
	for _, entry := range versions {
		if entry.Name != name || entry.Version == except {
			continue
		}
		if newest == "" || compareModuleVersions(entry.Version, newest) > 0 {
			newest = entry.Version
		}
	}
	return newest
}

// SelectedVersion returns the version the index selects for a module, and
// whether it names one.
func SelectedVersion(registry *Registry, kind, name string) (string, bool) {
	for _, entry := range *activeBucketOf(registry, kind) {
		if entry.Name == name {
			return entry.Version, true
		}
	}
	return "", false
}

// SelectedVersions lists the selections, which is what a listing reports as
// active.
func SelectedVersions(registry *Registry, kind string) map[string]string {
	out := map[string]string{}
	for _, entry := range *activeBucketOf(registry, kind) {
		out[entry.Name] = entry.Version
	}
	return out
}

// registryFilePath is where the index lives inside the local registry.
func registryFilePath() (string, error) {
	registryPath, err := paths.GetRegistryPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(registryPath, "registry.yaml"), nil
}

// cutPrefix is strings.CutPrefix, spelled here because the index is read by a
// toolchain that builds with Go 1.18, where that function arrived later.
func cutPrefix(text, prefix string) (string, bool) {
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	return text[len(prefix):], true
}

// indexSectionKey is the key a kind's versions are listed under. It is the key
// the kinds that predate the others have always used, so an existing index keeps
// its entries, and the key is written out rather than pluralised because the
// plural of "library" is not a word.
func indexSectionKey(kind string) string {
	switch kind {
	case KindLibrary:
		return "libraries"
	default:
		return kind + "s"
	}
}

// kindOfSection is the inverse: the kind a section key names, and whether the
// key is that kind's selection.
//
// The key and the kind are different words, and treating them as one is how a
// lookup ends up asking for a bucket that does not exist. So the mapping is
// made explicit here rather than assumed.
func kindOfSection(key string) (kind string, selection bool, known bool) {
	if withoutActive, found := cutPrefix(key, "active_"); found {
		for _, candidate := range IndexKinds {
			if withoutActive == indexSectionKey(candidate) {
				return candidate, true, true
			}
		}
		return "", false, false
	}
	for _, candidate := range IndexKinds {
		if key == indexSectionKey(candidate) {
			return candidate, false, true
		}
	}
	return "", false, false
}

/* -------------------------------------------------------------------------- */
/*  Removing what is installed                                                  */
/* -------------------------------------------------------------------------- */

// NewestInstalled returns the newest version of a module installed under any
// kind, which is the one a user who names no version means.
func NewestInstalled(name string) string {
	entries, err := ReadInstalled()
	if err != nil {
		return ""
	}
	// The kinds are read in the order a listing shows them, so a component and
	// a module of the same name resolve to the one a reader sees first rather
	// than to whichever the filesystem happened to return.
	newest := ""
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if newest == "" || compareModuleVersions(entry.Version, newest) > 0 {
			newest = entry.Version
		}
	}
	return newest
}

// countInstalledVersions returns how many versions of a module are installed.
func countInstalledVersions(name string) int {
	count := 0
	for _, entry := range allInstalled() {
		if entry.Name == name {
			count++
		}
	}
	return count
}

// UninstallLocal removes a module version from the local registry.
//
// It touches no remote: this is what a machine does with its own installation,
// and it is the counterpart of `install`, which is also local. A version that
// the index selects is refused, because that is the version a project is
// currently compiled and linked against; the user is told to deactivate it, and
// a deactivated version can be removed.
func UninstallLocal(name, version string) error {
	registry, err := Reconcile()
	if err != nil {
		return err
	}
	kind, found := findModuleType(registry, name, version)
	if !found {
		// The index is reconciled above, so not finding it means it is not
		// installed: there is nothing to remove and saying so is the answer.
		if IsInstalledVersion(name, version) {
			return fmt.Errorf("%s@%s is installed but is not recorded; reconcile first",
				name, version)
		}
		return fmt.Errorf("%s@%s is not installed here", name, version)
	}
	if selected, _ := SelectedVersion(registry, kind, name); selected == version {
		// Removing the last installed version is allowed even when it is the
		// selected one: there is nothing left to be resolved against, so
		// removing it is the point rather than a change a build would notice.
		// Any other version has to be deactivated first, because a project is
		// compiled and linked against the selected one and silently moving it to
		// a different version is a change nobody asked for.
		if countInstalledVersions(name) > 1 {
			return fmt.Errorf(
				"%s@%s is the selected version; deactivate it first "+
					"(rho module deactivate %s@%s)", name, version, name, version)
		}
	}
	// The index and the tree are both changed, and the tree is what the toolchain
	// uses, so the directory goes first: a directory removed with the index
	// still naming it is healed by the next read, but an index entry removed
	// with the directory still present would be re-added by the reconciler.
	for _, entry := range allInstalled() {
		if entry.Name == name && entry.Version == version && entry.Kind == kind {
			if err := os.RemoveAll(entry.Path); err != nil {
				return fmt.Errorf("cannot remove %s: %w", entry.Path, err)
			}
			break
		}
	}
	removeInstalled(registry, kind, name, version)
	path, err := registryFilePath()
	if err != nil {
		return err
	}
	return writeIndex(path, registry)
}

// Deactivate clears the selection of a module without removing it, which is the
// step `uninstall` asks for and the way a user steps back from a version a
// project depends on.
//
// With nothing selected the resolver picks the newest installed version, so
// deactivating moves the selection rather than leaving the module resolving to
// nothing. The version being deactivated is still installed — that is the point
// of deactivating rather than uninstalling — so it is skipped when choosing what
// to select next: a user who deactivated 1.0.0 and is then shown 1.0.0 as
// selected has not stepped back from anything.
func Deactivate(name, version string) error {
	registry, err := Reconcile()
	if err != nil {
		return err
	}
	kind, found := findModuleType(registry, name, version)
	if !found {
		return fmt.Errorf("%s@%s is not installed here", name, version)
	}
	if selected, _ := SelectedVersion(registry, kind, name); selected != version {
		return fmt.Errorf("%s@%s is not the selected version", name, version)
	}
	clearActiveVersion(registry, kind, name)
	if next := newestOfExcept(*versionBucketOf(registry, kind), name, version); next != "" {
		setActiveName(registry, kind, name, next)
	}
	path, err := registryFilePath()
	if err != nil {
		return err
	}
	return writeIndex(path, registry)
}
