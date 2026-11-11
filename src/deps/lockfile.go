package deps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"rho/internal/paths"
)

// workspaceToken is how a lockfile writes a path inside the workspace: the
// lockfile is committed, and a path on the machine that wrote it is true for one
// person. Read back, it is resolved against the reader's own workspace.
const workspaceToken = "${" + paths.WorkspaceEnvVar + "}"

type LockedPackage struct {
	Name      string
	Type      DepType
	Version   string
	GitURL    string
	GitTag    string
	GitBranch string
	Rev       string
	Path      string
	Hash      string
}

type Lockfile struct {
	Packages []LockedPackage
}

// Save writes the lockfile as structured JSON so field values cannot corrupt
// the record format.
func (l *Lockfile) Save(path string) error {
	if err := validatePackages(l.Packages); err != nil {
		return err
	}
	// The record is written portable; the lockfile in memory is left as it was.
	portable := &Lockfile{Packages: append([]LockedPackage{}, l.Packages...)}
	workspace, _ := paths.Workspace(filepath.Dir(path))
	home, _ := paths.GetRhoHome()
	for i := range portable.Packages {
		portable.Packages[i].Path = portablePath(portable.Packages[i].Path, workspace, home)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create lockfile: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(portable); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}
	return f.Close()
}

// LoadLockfile loads a lockfile from disk.
func LoadLockfile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lockfile: %w", err)
	}

	var l Lockfile
	if err := json.Unmarshal(data, &l); err != nil {
		l, err = parseLegacyLockfile(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse lockfile: %w", err)
		}
	}
	if err := validatePackages(l.Packages); err != nil {
		return nil, err
	}
	for i := range l.Packages {
		expanded, err := paths.Expand(l.Packages[i].Path, filepath.Dir(path))
		if err != nil {
			return nil, fmt.Errorf("lockfile %s: %w", l.Packages[i].Name, err)
		}
		l.Packages[i].Path = expanded
	}
	return &l, nil
}

// portablePath writes a path inside the installation as ${RHO_HOME}/relative and
// one inside the workspace as ${RHO_WORKSPACE}/relative, and leaves any other path
// as it was: a package outside both has no portable name, and pretending it did
// would point a reader somewhere false.
func portablePath(path, workspace, home string) string {
	if path == "" || !filepath.IsAbs(path) {
		return path
	}
	// The installation first: it can sit inside the workspace, and a module that
	// was installed is the installation's, wherever the installation is.
	for _, root := range []struct{ dir, token string }{
		{home, "${" + paths.RhoHomeEnvVar + "}"},
		{workspace, workspaceToken},
	} {
		if root.dir == "" {
			continue
		}
		relative, err := filepath.Rel(root.dir, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		return root.token + "/" + filepath.ToSlash(relative)
	}
	return path
}

func parseLegacyLockfile(data string) (Lockfile, error) {
	var l Lockfile
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 6)
		if len(parts) != 6 {
			return Lockfile{}, fmt.Errorf("invalid legacy lockfile line: %s", line)
		}
		l.Packages = append(l.Packages, LockedPackage{
			Name: parts[0], Version: parts[1], GitURL: parts[2], Rev: parts[3], Path: parts[4], Hash: parts[5],
		})
	}
	return l, nil
}

func validatePackages(packages []LockedPackage) error {
	seen := make(map[string]struct{}, len(packages))
	for _, p := range packages {
		if p.Name == "" {
			return fmt.Errorf("invalid lockfile package: empty name")
		}
		if _, ok := seen[p.Name]; ok {
			return fmt.Errorf("duplicate lockfile package: %s", p.Name)
		}
		seen[p.Name] = struct{}{}
	}
	return nil
}
