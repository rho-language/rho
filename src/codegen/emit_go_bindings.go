package codegen

import (
	"fmt"
	"sort"
	"strings"
)

// BindingsFileName is the generated file that carries an application's provider
// configuration.
const BindingsFileName = "rho_bindings.go"

// EmitGoBindings renders the configuration an application's `bind:` gives its
// providers, keyed `provider.key`, and rho_bind_config, which a provider reads
// it through. A value the application's workspace overrides in bind.conf wins,
// so a setting changed on a running installation does not need a rebuild.
func EmitGoBindings(config map[string]string) string {
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(goBanner(irModuleForBanner(), BindingsFileName, "Generated provider configuration."))
	b.WriteString("package main\n\nimport (\n\t\"os\"\n\t\"path/filepath\"\n\t\"strings\"\n)\n\n")
	b.WriteString("// The configuration the application's bind: gives each provider, by\n// provider and key.\n")
	b.WriteString("var rhoBindDefaults = map[string]string{\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "\t%q: %s,\n", key, goQuoted(config[key]))
	}
	b.WriteString("}\n\n")
	b.WriteString(goBindConfigSource)
	return b.String()
}

const goBindConfigSource = `// rhoBindConfig is one provider's setting: the value in the application's
// workspace bind.conf (one ` + "`provider.key=value`" + ` per line) when it has one,
// otherwise the value the application's bind: gave it, otherwise "".
func rhoBindConfig(provider string, key string) string {
	name := provider + "." + key
	if workspace := os.Getenv("RHO_APP_WORKSPACE"); workspace != "" {
		if data, err := os.ReadFile(filepath.Join(workspace, "bind.conf")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if value, found := strings.CutPrefix(strings.TrimSpace(line), name+"="); found {
					return value
				}
			}
		}
	}
	return rhoBindDefaults[name]
}
`
