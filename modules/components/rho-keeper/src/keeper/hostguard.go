//
// @file hostguard.go
// @brief A dashboard that answers only to the names it is reachable by.
//
// The dashboard on loopback is protected by being on loopback, and that holds
// against other machines but not against a browser. DNS rebinding turns a page on
// an attacker's domain into one that reads 127.0.0.1: the name resolves first to
// the attacker's server and then, with a short TTL, to 127.0.0.1, so the browser
// believes the dashboard is the same origin as the page. Every other check here —
// `Origin`, `Sec-Fetch-Site` — then agrees, because both name the attacker's
// domain, and the page can read what the machine has and ask it to install things.
//
// What a rebound request cannot do is change its `Host` header: that is still the
// attacker's name. So the dashboard answers only requests whose Host is one it is
// actually listening as — an address, `localhost`, or a name the person gave it —
// and refuses any other with a message that says how to allow it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"net"
	"net/http"
	"os"
	"strings"
)

// allowedHosts is the set of host names a request may be addressed to.
//
// It is the address the dashboard listens on, the loopback names when it listens on
// loopback, and whatever RHO_KEEPER_ALLOWED_HOSTS adds — a comma-separated list for
// a dashboard reached through a name, such as one behind a proxy.
type allowedHosts map[string]bool

// hostsFor builds the set for a listening address.
func hostsFor(listening string) allowedHosts {
	allowed := allowedHosts{}
	host, _, err := net.SplitHostPort(listening)
	if err != nil {
		host = listening
	}
	host = strings.Trim(strings.ToLower(host), "[]")
	if host != "" && host != "0.0.0.0" && host != "::" {
		allowed[host] = true
	}
	// The loopback names, always: the dashboard's own page reaches it by
	// `localhost` as often as by the address it printed.
	if ip := net.ParseIP(host); host == "" || host == "localhost" || (ip != nil && ip.IsLoopback()) {
		for _, name := range []string{"localhost", "127.0.0.1", "::1"} {
			allowed[name] = true
		}
	}
	for _, name := range strings.Split(os.Getenv("RHO_KEEPER_ALLOWED_HOSTS"), ",") {
		if name = strings.Trim(strings.ToLower(strings.TrimSpace(name)), "[]"); name != "" {
			allowed[name] = true
		}
	}
	return allowed
}

// permits reports whether a Host header names somewhere the dashboard is.
func (a allowedHosts) permits(hostHeader string) bool {
	host := strings.ToLower(strings.TrimSpace(hostHeader))
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}
	host = strings.Trim(host, "[]")
	return a[host]
}

// hostGuard refuses a request whose Host is not one the dashboard answers to.
//
// It is outermost, before the token and before the same-site check, because a
// request that fails it is not one of the dashboard's at all and should learn
// nothing — not even whether a token would have been wanted.
func hostGuard(listening string, next http.Handler) http.Handler {
	allowed := hostsFor(listening)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed.permits(r.Host) {
			http.Error(w, "this dashboard does not answer to that host name; "+
				"if it is yours, allow it with RHO_KEEPER_ALLOWED_HOSTS", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
