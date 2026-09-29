package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"rho/internal/paths"
)

type Locator interface {
	Locate(dep DepSpec) (string, error)
}

type DefaultLocator struct {
	ProjectRoot string
	GlobalCache string // e.g. ~/.rho/pkg/mod
	LocalCache  string // e.g. .rho-deps
}

// RhoHome returns the directory a Rho installation keeps its state in, or ""
// when there is no home to speak of. It is the one place the compiler and the
// module commands ask, so they cannot resolve different installations.
func RhoHome() string {
	home, err := paths.GetRhoHome()
	if err != nil {
		return ""
	}
	return home
}

// installedModuleRoot returns the directory of the highest installed version
// of a module, or "" when the module is not installed.
//
// Both the component and the plugin trees are searched, so a dependency does not
// have to care how the module was published. This is what lets a project depend
// on a component and import its Rho sources.
func installedModuleRoot(name string) string {
	return highestInstalledRoot(name, "component", "plugin", "library", "module")
}

// InstalledComponentRoot returns the directory of the highest installed version
// of a component, or "" when no component of that name is installed.
//
// Only the component tree counts here, and only the newest version of it: a Rho
// import resolves against a component, and it has to resolve against the same
// copy the dependency resolver picked, or the sources compiled and the library
// linked come from different versions.
func InstalledComponentRoot(name string) string {
	return highestInstalledRoot(name, "component")
}

// InstalledPluginRoot returns the directory of the highest installed version of
// a plugin, or "" when no plugin of that name is installed.
//
// A tool that runs plugins picks the newest one for the same reason a
// dependency resolver does: two versions of a plugin are not two plugins, and
// whichever answered last would otherwise decide which one a build runs.
func InstalledPluginRoot(name string) string {
	return highestInstalledRoot(name, "plugin")
}

// InstalledPluginNames are the plugins installed, one entry per name.
func InstalledPluginNames() []string {
	home := RhoHome()
	if home == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, "modules", "installed", "plugin"))
	if err != nil {
		return nil
	}
	names := []string{}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, _, cut := strings.Cut(entry.Name(), "@")
		if !cut || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// highestInstalledRoot returns the newest installed directory named `name`
// across the given publication kinds.
// highestInstalledRoot is the root of the version of a module that runs.
//
// It is the active version where one was chosen, and the highest installed
// where one was not. The distinction is the point of the record: a machine may
// hold several versions of a module, and a build that picked the largest number
// rather than the chosen one would run whatever was installed last rather than
// what anyone decided to use.
//
// A module that is installed and not active has no root here, and the caller is
// told so rather than being handed a version nobody enabled.
func highestInstalledRoot(name string, kinds ...string) string {
	home := RhoHome()
	if home == "" {
		return ""
	}
	base := filepath.Join(home, "modules", "installed")

	state, err := LoadEnabled()
	if err != nil {
		// A record that cannot be read is a fact about the installation, and
		// guessing past it would run a module whose state nobody can see. The
		// highest installed is still the best answer available, which is what
		// this installation did before there was a record.
		state = &Enabled{versions: map[string]string{}}
	}

	best := ""
	bestVersion := ""
	for _, kind := range kinds {
		entries, err := os.ReadDir(filepath.Join(base, kind))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			installed, version, cut := strings.Cut(entry.Name(), "@")
			if !cut || installed != name || version == "" {
				continue
			}
			if !state.IsEnabled(kind, name, version) {
				continue
			}
			if best == "" || compareModuleVersions(version, bestVersion) > 0 {
				best = filepath.Join(base, kind, entry.Name())
				bestVersion = version
			}
		}
	}
	return best
}

// compareModuleVersions orders dotted numeric versions, so the newest
// installed version wins.
func compareModuleVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aerr := strconv.Atoi(as[i])
		bn, berr := strconv.Atoi(bs[i])
		switch {
		case aerr != nil || berr != nil:
			if as[i] != bs[i] {
				if as[i] < bs[i] {
					return -1
				}
				return 1
			}
		case an != bn:
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

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// VendoredComponentRoot returns the directory of a component that ships in the
// repository the project lives in, or "" when the project is not inside such a
// repository or the component is not there.
//
// A component keeps its own sources under modules/components of the repository
// that holds it, so a project in that repository can build against the
// component in the working tree. That is what lets a checkout build without an
// installation having been made first.
//
// The walk stops at the repository the project belongs to. Above that boundary
// a components directory belongs to some other project, so honouring one would
// let a shared parent directory decide what a project compiles.
func VendoredComponentRoot(projectRoot, name string) string {
	if projectRoot == "" || name == "" || filepath.Base(filepath.Clean(name)) != name || name == "." {
		return ""
	}
	for current := projectRoot; ; {
		root := filepath.Join(current, "modules", "components", name)
		if isComponentDir(root) {
			return root
		}
		// A linked directory is not followed: the boundary is the repository,
		// and a link is a way out of it.
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return ""
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

// isComponentDir reports whether a path is a component directory: a real
// directory carrying the manifest that makes it one.
func isComponentDir(root string) bool {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	return exists(filepath.Join(root, "module.yml"))
}

func gitClone(url, dest, tag, branch string) error {
	if info, err := os.Stat(dest); err == nil {
		if info.IsDir() {
			return nil
		}
		return fmt.Errorf("destination exists and is not a directory: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	args := gitCloneArgs(url, dest, tag, branch)
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitCloneArgs(url, dest, tag, branch string) []string {
	args := []string{"clone"}
	ref := tag
	if ref == "" {
		ref = branch
	}
	if ref != "" {
		args = append(args, "--branch", ref, "--single-branch")
	}
	args = append(args, url, dest)
	return args
}

func (l *DefaultLocator) Locate(dep DepSpec) (string, error) {
	if dep.Name == "" || filepath.Base(filepath.Clean(dep.Name)) != dep.Name || dep.Name == "." {
		return "", fmt.Errorf("invalid dependency name %q", dep.Name)
	}
	// 1. ENV override
	if dep.EnvVar != "" {
		if p := os.Getenv(dep.EnvVar); p != "" {
			return p, nil
		}
	}

	// 2. Path override
	cloneDest := filepath.Join(l.LocalCache, dep.Name)
	if dep.Path != "" {
		p := dep.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(l.ProjectRoot, p)
		}
		if exists(p) {
			return p, nil
		}
		cloneDest = p
	}

	// Dependencies declared by a local dependency are resolved relative to
	// the project root when no explicit source is provided.
	if dep.GitURL == "" && dep.Path == "" {
		p := filepath.Join(l.ProjectRoot, dep.Name)
		if exists(p) {
			return p, nil
		}
	}

	// 3. Installed modules.
	//
	// A component or library installed with `rho module install` lives in the
	// Rho home rather than in a cache, so it is found here. This is what lets
	// a project depend on a component and import its Rho sources.
	if root := installedModuleRoot(dep.Name); root != "" {
		return root, nil
	}

	// 3b. Component in the repository the project lives in.
	//
	// An installation answers first: it is what was published, and it is what a
	// project outside the repository depends on. A project inside the repository
	// falls back to the component in the working tree, so a build from a
	// checkout resolves the dependency without an install step having been run
	// on that machine.
	//
	// Only a dependency with no source of its own is answered this way. A
	// dependency that names a path, a repository or an environment variable is
	// resolved from that source, because its lockfile entry has to say where
	// the code came from.
	if dep.GitURL == "" && dep.Path == "" && dep.EnvVar == "" {
		if root := VendoredComponentRoot(l.ProjectRoot, dep.Name); root != "" {
			return root, nil
		}
	}

	// 4. Global cache
	g := filepath.Join(l.GlobalCache, dep.Name)
	if exists(g) {
		return g, nil
	}

	// 5. Local cache
	loc := filepath.Join(l.LocalCache, dep.Name)
	if exists(loc) {
		return loc, nil
	}

	// 6. Git clone → local cache
	if dep.GitURL == "" {
		return "", fmt.Errorf("dependency %s has no source", dep.Name)
	}

	if err := gitClone(dep.GitURL, cloneDest, dep.GitTag, dep.GitBranch); err != nil {
		return "", fmt.Errorf("git clone %s: %w", dep.Name, err)
	}

	return cloneDest, nil
}
