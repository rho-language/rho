package projectgen

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rho/src/deps"
	"rho/src/plugins"
)

// platformPortSuffix names a port file a platform's plugin ships: ports/NAME.go.txt.
// It is not `.go`, so the plugin's own module does not compile it as part of
// itself; it is Go that belongs to the project the port is generated into.
const platformPortSuffix = ".go.txt"

// platformPorts reads the ports shipped by the plugins that serve the platform
// a project is built for, by port name. A project that names no platform gets
// none, which is the standard build.
func platformPorts(projectRoot string) map[string]string {
	platform := deps.Platform(projectRoot)
	if platform == "" {
		return nil
	}
	source := plugins.Source{Tree: projectRoot, RhoHome: deps.RhoHome(), Platform: platform}
	found := map[string]string{}
	for _, plugin := range source.All() {
		if !source.Offered(plugin) || len(plugin.Platforms) == 0 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(plugin.Root, "ports"))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name, isPort := strings.CutSuffix(entry.Name(), platformPortSuffix)
			if !isPort || entry.IsDir() {
				continue
			}
			if data, readErr := os.ReadFile(filepath.Join(plugin.Root, "ports", entry.Name())); readErr == nil {
				found[name] = string(data)
			}
		}
	}
	return found
}

func sortedPortNames(ports map[string]string) []string {
	names := make([]string, 0, len(ports))
	for name := range ports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
