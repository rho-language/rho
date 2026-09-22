package deps

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

func hashPath(path string) string {
	data, err := os.ReadFile(filepath.Join(path, "rho.mod"))
	if err == nil {
		return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	}
	data, err = os.ReadFile(path)
	if err == nil {
		return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	}
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		h := sha256.New()
		walkErr := filepath.Walk(path, func(filePath string, fileInfo os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, relErr := filepath.Rel(path, filePath)
			if relErr != nil {
				return relErr
			}
			if fileInfo.IsDir() {
				if rel == ".git" || filepath.Base(rel) == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			contents, readErr := os.ReadFile(filePath)
			if readErr != nil {
				return readErr
			}
			_, _ = h.Write([]byte(rel))
			_, _ = h.Write(contents)
			return nil
		})
		if walkErr == nil {
			return fmt.Sprintf("sha256:%x", h.Sum(nil))
		}
	}
	return ""
}

// HashPath returns the content hash used for a lockfile entry.
func HashPath(path string) string {
	return hashPath(path)
}

type PackageNode struct {
	Name         string
	Version      Semver
	Path         string
	Type         DepType
	Deps         []DepSpec
	ResolvedDeps []*PackageNode
}

type DependencyGraph struct {
	Nodes map[string]*PackageNode
}

type Resolver struct {
	Locator Locator
}

// Resolve builds the full dependency graph and lockfile from a root manifest.
func (r *Resolver) Resolve(manifest *Manifest, projectRoot string) (*DependencyGraph, *Lockfile, error) {
	graph := &DependencyGraph{Nodes: make(map[string]*PackageNode)}
	lock := &Lockfile{}
	resolving := make(map[string]bool)

	for _, dep := range manifest.Deps {
		if err := r.resolveDep(graph, lock, dep, resolving); err != nil {
			return nil, nil, err
		}
	}

	return graph, lock, nil
}

func (r *Resolver) resolveDep(graph *DependencyGraph, lock *Lockfile, dep DepSpec, resolving map[string]bool) error {
	// Already resolved?
	if node, ok := graph.Nodes[dep.Name]; ok {
		cs, err := ParseVersionRange(dep.Version.Raw)
		if err != nil || !VersionInRange(node.Version, cs) {
			return fmt.Errorf("dependency %s has incompatible version constraint %s", dep.Name, dep.Version.Raw)
		}
		return nil
	}
	if resolving[dep.Name] {
		return fmt.Errorf("dependency cycle detected at %s", dep.Name)
	}
	resolving[dep.Name] = true
	defer delete(resolving, dep.Name)

	// 1. Locate dependency
	path, err := r.Locator.Locate(dep)
	if err != nil {
		return fmt.Errorf("locate %s: %w", dep.Name, err)
	}

	var dmanifest *Manifest
	dependencyType := dep.Type
	if dependencyType == "" {
		dependencyType = DepTypeRho
	}
	version := dep.Version.Raw
	var v Semver
	hasResolvedVersion := false
	if dependencyType == DepTypeRho {
		dmanifest, err = LoadDepManifest(path)
		if err != nil {
			return fmt.Errorf("load manifest %s: %w", dep.Name, err)
		}
		if dmanifest.ModuleName != "" && dmanifest.ModuleName != dep.Name {
			return fmt.Errorf("dependency %s declares module %s", dep.Name, dmanifest.ModuleName)
		}
		version = dmanifest.Version
		v, err = ParseSemver(version)
		if err != nil {
			return fmt.Errorf("parse version %s: %w", version, err)
		}
		hasResolvedVersion = true
	} else {
		if parsed, parseErr := ParseSemver(version); parseErr == nil {
			v = parsed
			hasResolvedVersion = true
		}
		if candidate, loadErr := LoadDepManifest(path); loadErr == nil {
			// C repositories may include a rho.mod for version metadata or
			// transitive Rho dependencies, but it is not required.
			dmanifest = candidate
			if candidate.Version != "" {
				version = candidate.Version
				v, err = ParseSemver(version)
				if err != nil {
					return fmt.Errorf("parse version %s: %w", version, err)
				}
				hasResolvedVersion = true
			}
		}
	}

	// Parse and validate a Rho version range. C repositories do not expose
	// a required manifest, so their declared version is retained as metadata.
	cs, err := ParseVersionRange(dep.Version.Raw)
	if err != nil {
		return fmt.Errorf("parse range %s: %w", dep.Version.Raw, err)
	}

	// 5. Check version compatibility
	if hasResolvedVersion && !VersionInRange(v, cs) {
		return fmt.Errorf("dependency %s version %s not in range %s",
			dep.Name, dmanifest.Version, dep.Version.Raw)
	}

	// 6. Create node
	node := &PackageNode{
		Name:    dep.Name,
		Version: v,
		Path:    path,
		Type:    dependencyType,
		Deps: func() []DepSpec {
			if dmanifest == nil {
				return nil
			}
			return dmanifest.Deps
		}(),
	}
	graph.Nodes[dep.Name] = node

	// 7. Resolve transitive dependencies
	for _, child := range node.Deps {
		if err := r.resolveDep(graph, lock, child, resolving); err != nil {
			return err
		}
	}

	// 8. Add to lockfile
	lock.Packages = append(lock.Packages, LockedPackage{
		Name:    dep.Name,
		Type:    dependencyType,
		Version: version,
		GitURL:  dep.GitURL,
		Rev:     version,
		Path:    path,
		Hash:    hashPath(path),
	})

	return nil
}
