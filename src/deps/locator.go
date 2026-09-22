package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Locator interface {
	Locate(dep DepSpec) (string, error)
}

type DefaultLocator struct {
	ProjectRoot string
	GlobalCache string // e.g. ~/.rho/pkg/mod
	LocalCache  string // e.g. .rho-deps
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func gitClone(url, dest string) error {
	if info, err := os.Stat(dest); err == nil {
		if info.IsDir() {
			return nil
		}
		return fmt.Errorf("destination exists and is not a directory: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	if dep.Path != "" {
		p := dep.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(l.ProjectRoot, p)
		}
		if exists(p) {
			return p, nil
		}
	}

	// Dependencies declared by a local dependency are resolved relative to
	// the project root when no explicit source is provided.
	if dep.GitURL == "" && dep.Path == "" {
		p := filepath.Join(l.ProjectRoot, dep.Name)
		if exists(p) {
			return p, nil
		}
	}

	// 3. Global cache
	g := filepath.Join(l.GlobalCache, dep.Name)
	if exists(g) {
		return g, nil
	}

	// 4. Local cache
	loc := filepath.Join(l.LocalCache, dep.Name)
	if exists(loc) {
		return loc, nil
	}

	// 5. Git clone → local cache
	if dep.GitURL == "" {
		return "", fmt.Errorf("dependency %s has no source", dep.Name)
	}

	if err := gitClone(dep.GitURL, loc); err != nil {
		return "", fmt.Errorf("git clone %s: %w", dep.Name, err)
	}

	return loc, nil
}
