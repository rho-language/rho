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
	// A port is Go source compiled into the project, as its own sources are, so
	// the plugins the project's own tree carries provide ports too: an
	// application can keep a port of its own beside its code and version it
	// with it, rather than in the platform's plugin.
	return integration.Ports(plugins.Source{Tree: projectRoot, RhoHome: deps.RhoHome(), Platform: platform, TrustTree: true})
}

func sortedPortNames(ports map[string]string) []string {
	names := make([]string, 0, len(ports))
	for name := range ports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
