package deps

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

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
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create lockfile: %w", err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(l); err != nil {
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
	return &l, nil
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
