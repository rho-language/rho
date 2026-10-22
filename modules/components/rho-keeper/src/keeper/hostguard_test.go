//
// @file hostguard_test.go
// @brief A page on another domain cannot reach the dashboard by rebinding to it.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package keeper

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// request sends one request addressed to a Host and answers the status.
func requestTo(t *testing.T, handler http.Handler, host string, headers map[string]string) int {
	t.Helper()
	request := httptest.NewRequest("GET", "/healthz", nil)
	request.Host = host
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Code
}

// The rebinding attack: the page's domain resolves to 127.0.0.1, so the browser
// sends the request there — with the attacker's name in Host, and the same name in
// Origin and a Sec-Fetch-Site of same-origin, so every other check agrees. Only
// the Host check can tell.
func TestARebindToLoopbackIsRefused(t *testing.T) {
	handler := Handler(nil, false, nil, "127.0.0.1:8477")
	rebound := map[string]string{
		"Origin":         "http://evil.example:8477",
		"Sec-Fetch-Site": "same-origin",
	}
	if got := requestTo(t, handler, "evil.example:8477", rebound); got != http.StatusForbidden {
		t.Fatalf("a rebound request was answered with %d, want 403", got)
	}
}

// The names a person actually uses still work.
func TestLoopbackNamesAreAnswered(t *testing.T) {
	handler := Handler(nil, false, nil, "127.0.0.1:8477")
	for _, host := range []string{"127.0.0.1:8477", "localhost:8477", "LOCALHOST:8477", "[::1]:8477", "localhost"} {
		if got := requestTo(t, handler, host, nil); got != http.StatusOK {
			t.Errorf("Host %q was answered with %d, want 200", host, got)
		}
	}
}

// A dashboard reached through a name says so, and only that name is added.
func TestAnAllowedHostIsAnswered(t *testing.T) {
	t.Setenv("RHO_KEEPER_ALLOWED_HOSTS", "keeper.internal, other.internal")
	handler := Handler(nil, false, nil, "127.0.0.1:8477")
	if got := requestTo(t, handler, "keeper.internal:8477", nil); got != http.StatusOK {
		t.Errorf("an allowed host was answered with %d", got)
	}
	if got := requestTo(t, handler, "evil.example:8477", nil); got != http.StatusForbidden {
		t.Errorf("a host that was not allowed was answered with %d", got)
	}
}

// A dashboard listening on one address is not reachable as another's name: bound
// to a LAN address it answers to that address, not to the loopback names.
func TestAListeningAddressIsAnsweredAsItself(t *testing.T) {
	handler := Handler(nil, false, nil, "192.168.1.20:8477")
	if got := requestTo(t, handler, "192.168.1.20:8477", nil); got != http.StatusOK {
		t.Errorf("its own address was answered with %d", got)
	}
	if got := requestTo(t, handler, "127.0.0.1:8477", nil); got != http.StatusForbidden {
		t.Errorf("a loopback name was answered with %d on a dashboard that is not on loopback", got)
	}
}

// The check comes before the token, so a refused host learns nothing about
// whether a token would have been wanted.
func TestTheHostIsCheckedBeforeTheToken(t *testing.T) {
	t.Setenv("RHO_KEEPER_TOKEN", "s3cret")
	handler := Handler(nil, false, nil, "127.0.0.1:8477")
	if got := requestTo(t, handler, "evil.example:8477", nil); got != http.StatusForbidden {
		t.Errorf("a refused host got %d, want 403 and not the token's 401", got)
	}
	if got := requestTo(t, handler, "127.0.0.1:8477", nil); got != http.StatusUnauthorized {
		t.Errorf("a good host without a token got %d, want 401", got)
	}
}

// A wildcard listen address names no host, so it adds none: the person says which
// names reach it.
func TestAWildcardListenAddressAllowsNoNameByItself(t *testing.T) {
	allowed := hostsFor("0.0.0.0:8477")
	if allowed.permits("0.0.0.0") {
		t.Error("the wildcard address was treated as a host name")
	}
	if allowed.permits("evil.example") {
		t.Error("an arbitrary name was allowed")
	}
}
