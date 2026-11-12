package projectgen

import (
	"sort"

	"rho/src/deps"
	"rho/src/integration"
	"rho/src/plugins"
)

// platformPorts are the Go ports the integration plugin of the project's
// platform provides, by port name. A project that names no platform gets none,
// which is the standard build.
func platformPorts(projectRoot string) map[string]string {
	platform := deps.Platform(projectRoot)
	if platform == "" {
		return nil
	}
	return integration.Ports(plugins.Source{Tree: projectRoot, RhoHome: deps.RhoHome(), Platform: platform})
}

func sortedPortNames(ports map[string]string) []string {
	names := make([]string, 0, len(ports))
	for name := range ports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
