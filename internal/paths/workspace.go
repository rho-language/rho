package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WorkspaceEnvVar names the directory that holds a project and the modules it is
// built beside. It is the toolchain's own variable: a module that has to say
// where a sibling lives says it relative to the workspace, never as a path on the
// machine the manifest was written on.
const WorkspaceEnvVar = "RHO_WORKSPACE"

// workspaceSuffix marks the workspace variable of an ecosystem. A platform names
// its own (`ROBOHUB_WORKSPACE`), and when that one is not set it means the same
// directory the toolchain's workspace is, so a manifest can be written in the
// ecosystem's terms without the toolchain naming any ecosystem.
const workspaceSuffix = "_WORKSPACE"

// Workspace resolves the workspace for a directory and says where the answer
// came from: the environment, or the layout.
//
// With nothing set, the workspace is the directory that holds the repository the
// directory is in, which is where sibling checkouts sit.
func Workspace(base string) (dir, source string) {
	if named := os.Getenv(WorkspaceEnvVar); named != "" {
		return filepath.Clean(named), "$" + WorkspaceEnvVar
	}
	if base == "" {
		base, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}
	for current := base; ; {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return filepath.Dir(current), "the parent of the repository at " + current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Dir(base), "the parent of " + base
		}
		current = parent
	}
}

// lookup answers a variable for Expand: the environment first, then the
// workspace for a name that ends in _WORKSPACE.
func lookup(name, base string) (string, bool) {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value, true
	}
	// The installation is where it is whether or not RHO_HOME says so.
	if name == RhoHomeEnvVar {
		if home, err := GetRhoHome(); err == nil {
			return home, true
		}
	}
	if strings.HasSuffix(name, workspaceSuffix) {
		dir, _ := Workspace(base)
		return dir, true
	}
	return "", false
}

// Expand replaces `${NAME}` and `${NAME:-default}` in a path a manifest
// declares. `base` is the directory the manifest is in, which is what the
// workspace is derived from when nothing sets it.
//
// A variable that is not set and has no default is an error naming it: a path
// quietly left half-expanded is a build that fails somewhere further from the
// cause.
func Expand(value, base string) (string, error) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	var out strings.Builder
	for rest := value; rest != ""; {
		start := strings.Index(rest, "${")
		if start < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:start])
		end := strings.Index(rest[start:], "}")
		if end < 0 {
			return "", fmt.Errorf("path %q has an unterminated ${", value)
		}
		name, fallback, hasDefault := strings.Cut(rest[start+2:start+end], ":-")
		name = strings.TrimSpace(name)
		if name == "" {
			return "", fmt.Errorf("path %q has an empty variable name", value)
		}
		resolved, ok := lookup(name, base)
		switch {
		case ok:
			out.WriteString(resolved)
		case hasDefault:
			out.WriteString(fallback)
		default:
			return "", fmt.Errorf("path %q uses ${%s}, which is not set; set it, or write ${%s:-default}", value, name, name)
		}
		rest = rest[start+end+1:]
	}
	return out.String(), nil
}

// WorkspaceVariables lists the ecosystem workspace variables the environment
// sets, sorted, so `rho env` can show what overrides what.
func WorkspaceVariables() map[string]string {
	found := map[string]string{}
	for _, entry := range os.Environ() {
		name, value, _ := strings.Cut(entry, "=")
		if strings.HasSuffix(name, workspaceSuffix) && value != "" {
			found[name] = value
		}
	}
	return found
}

// SortedKeys returns a map's keys in order.
func SortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
