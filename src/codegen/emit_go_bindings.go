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
// so a setting changed on a running installation does not need a rebuild. The
// application's name is where its settings live when it is started without a
// workspace, as a standalone program is.
func EmitGoBindings(app string, config map[string]string) string {
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
	fmt.Fprintf(&b, "// rhoBindApp names the application's own directory when it has no workspace.\nconst rhoBindApp = %q\n\n", bindAppName(app))
	b.WriteString(goBindConfigSource)
	return b.String()
}

// bindAppName is an application name as one path segment.
func bindAppName(app string) string {
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < ' ' {
			return '-'
		}
		return r
	}, strings.TrimSpace(app))
	if name == "" || name == "." || name == ".." {
		return "default"
	}
	return name
}

const goBindConfigSource = `// rhoBindWorkspace is the directory the application's bind.conf is in: the
// workspace a supervisor gives it, otherwise a directory of its own under the
// user's ~/.rho, so a program started by hand keeps what it is told too.
func rhoBindWorkspace() string {
	if workspace := os.Getenv("RHO_APP_WORKSPACE"); workspace != "" {
		return workspace
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".rho", "apps", rhoBindApp)
}

// rhoBindConfig is one provider's setting: the value in the application's
// bind.conf (one ` + "`provider.key=value`" + ` per line) when it has one,
// otherwise the value the application's bind: gave it, otherwise "".
func rhoBindConfig(provider string, key string) string {
	name := provider + "." + key
	if workspace := rhoBindWorkspace(); workspace != "" {
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

// rhoBindConfigSet records one provider's setting in the application
// workspace's bind.conf, replacing the line it had, so the next
// rhoBindConfig answers it. It reports false when there is no workspace or the
// file cannot be written. The file is the owner's only: settings may be secrets.
func rhoBindConfigSet(provider string, key string, value string) bool {
	workspace := rhoBindWorkspace()
	if workspace == "" || strings.ContainsAny(provider+key+value, "\n\r") {
		return false
	}
	path := filepath.Join(workspace, "bind.conf")
	name := provider + "." + key + "="
	lines := []string{}
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if line != "" && !strings.HasPrefix(strings.TrimSpace(line), name) {
				lines = append(lines, line)
			}
		}
	}
	lines = append(lines, name+value)
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		return false
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600) == nil
}
`
