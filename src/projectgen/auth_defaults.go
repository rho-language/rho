//
// @file auth_defaults.go
// @brief What an application's auth wrapper is when the descriptor does not say.
//
// The auth wrapper accepts a session issued for one audience, verified against
// one public key, and sends a browser that has none to a login route. For an
// application all three follow from what it already says about itself: the
// audience is the service it deploys as, the key is where `rho auth keygen`
// puts it, and the login route names the audience. A descriptor writes only
// what is its own decision — who issues the session, which paths are public.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package projectgen

import (
	"os"
	"strconv"
	"strings"

	"rho/src/deps"
	"rho/src/yaml"
)

// DefaultAuthPublicKey is where `rho auth keygen` writes the public key a
// service verifies sessions with.
const DefaultAuthPublicKey = "~/.rho/auth/public.pem"

// withAuthDefaults fills the auth wrapper's audience, public key and login
// route from the application, where the descriptor left them out.
func withAuthDefaults(target Target, projectRoot string) Target {
	index := -1
	for i, item := range target.Middleware {
		if item.Name == "auth" {
			index = i
		}
	}
	if index < 0 {
		return target
	}
	options := map[string]string{}
	for key, value := range target.Middleware[index].Options {
		options[key] = value
	}
	if options["audience"] == "" {
		options["audience"] = applicationAudience(projectRoot)
	}
	if options["public_key"] == "" {
		options["public_key"] = DefaultAuthPublicKey
	}
	if options["login_url"] == "" && options["audience"] != "" {
		options["login_url"] = "/auth/login?audience=" + options["audience"]
	}
	middleware := append([]Middleware(nil), target.Middleware...)
	middleware[index] = Middleware{Name: "auth", Options: options}
	target.Middleware = middleware
	return target
}

// applicationAudience is the service an application deploys as, or its module
// name when it says none.
func applicationAudience(projectRoot string) string {
	path := deps.ManifestPath(projectRoot)
	if data, err := os.ReadFile(path); err == nil {
		if root, err := yaml.Load(string(data)); err == nil {
			if service := strings.TrimSpace(root.Map["deploy"].Map["service"].Str); service != "" {
				return service
			}
		}
	}
	if manifest, err := deps.LoadManifest(path); err == nil && manifest != nil {
		return manifest.ModuleName
	}
	return ""
}

// applicationPort is the port an application says it listens on — `module.port`,
// else `deploy.port` — or "" when it says none. A service started by hand then
// comes up where its manifest and its documentation say, on this machine only.
func applicationPort(projectRoot string) string {
	data, err := os.ReadFile(deps.ManifestPath(projectRoot))
	if err != nil {
		return ""
	}
	root, err := yaml.Load(string(data))
	if err != nil {
		return ""
	}
	for _, port := range []string{root.Map["module"].Map["port"].Str, root.Map["deploy"].Map["port"].Str} {
		port = strings.Trim(strings.TrimSpace(port), "\"'")
		if n, convErr := strconv.Atoi(port); convErr == nil && n > 0 && n < 65536 {
			return port
		}
	}
	return ""
}
