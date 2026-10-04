//
// @file notice_test.go
// @brief Tests for credential notices and the token acquisition policy.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package auth

import (
	"strings"
	"testing"
	"time"
)

// putExpiring writes a credential that expires at the given moment, into a
// store the caller keeps, so the notice and the read see the same file.
func putExpiring(t *testing.T, store *Store, expires time.Time) Credential {
	t.Helper()
	entry := Credential{
		Method:    TokenMethod,
		Registry:  "http://reg:8080",
		Secret:    "signed",
		Subject:   "miraur",
		IssuedAt:  time.Now().Add(-24 * time.Hour),
		ExpiresAt: expires,
	}
	if err := store.Put(entry); err != nil {
		t.Fatal(err)
	}
	return entry
}

/* -------------------------------------------------------------------------- */
/*  Test: Notices                                                             */
/* -------------------------------------------------------------------------- */

func TestDescribeWarnsAboutSoonExpiry(t *testing.T) {
	store := newStore(t)
	putExpiring(t, store, time.Now().Add(48*time.Hour))

	notices, err := store.Describe(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 {
		t.Fatalf("expected one notice, got %v", notices)
	}
	if notices[0].Level != Warn {
		t.Errorf("level: got %v, want warning", notices[0].Level)
	}
	if !strings.Contains(notices[0].Reason, "rho auth login") {
		t.Errorf("the notice should say how to renew: %q", notices[0].Reason)
	}
}

func TestDescribeReportsExpiry(t *testing.T) {
	store := newStore(t)
	putExpiring(t, store, time.Now().Add(-time.Hour))

	notices, err := store.Describe(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 1 || notices[0].Level != Fail {
		t.Fatalf("expected an error notice, got %v", notices)
	}
}

// A healthy credential is not worth a message.
func TestDescribeIsQuietWhenHealthy(t *testing.T) {
	store := newStore(t)
	putExpiring(t, store, time.Now().Add(30*24*time.Hour))

	notices, err := store.Describe(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(notices) != 0 {
		t.Fatalf("expected no notices, got %v", notices)
	}
}

func TestFormatNotices(t *testing.T) {
	out := FormatNotices([]Notice{
		{Registry: "http://reg", Subject: "miraur", Method: TokenMethod, Level: Warn, Reason: "expires soon"},
		{Registry: "http://other", Method: TokenMethod, Level: Fail, Reason: "expired"},
	})
	if !strings.Contains(out, "warning: miraur") {
		t.Errorf("expected a warning line, got %q", out)
	}
	if !strings.Contains(out, "error:") {
		t.Errorf("expected an error line, got %q", out)
	}
}

func TestLevelString(t *testing.T) {
	if Info.String() != "info" || Warn.String() != "warning" || Fail.String() != "error" {
		t.Fatal("unexpected level names")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Token acquisition policy                                            */
/* -------------------------------------------------------------------------- */

// Without automatic login, a missing credential reports how to get one rather
// than doing it silently.
func TestTokenForRegistryReportsWhenManual(t *testing.T) {
	store := newStore(t)
	_, err := store.TokenForRegistry("http://reg:8080", Policy{}, time.Now())
	if err == nil {
		t.Fatal("expected an error when no credential and no automatic login")
	}
	if !strings.Contains(err.Error(), "rho auth login") {
		t.Fatalf("the error should say how to log in, got %v", err)
	}
}

// A stored, valid credential is used as is, without contacting the registry.
func TestTokenForRegistryReusesAStoredCredential(t *testing.T) {
	store := newStore(t)
	entry := Credential{
		Method:    TokenMethod,
		Registry:  "http://reg:8080",
		Secret:    "already-have-this",
		Subject:   "miraur",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := store.Put(entry); err != nil {
		t.Fatal(err)
	}

	called := false
	policy := Policy{
		AutoLogin: true,
		Issuer: func(string) (string, TokenClaims, error) {
			called = true
			return "", TokenClaims{}, nil
		},
	}
	got, err := store.TokenForRegistry("http://reg:8080", policy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("a valid stored credential should not trigger a login")
	}
	if got.Secret != "already-have-this" {
		t.Errorf("got %q", got.Secret)
	}
}

// With automatic login enabled, a missing credential is obtained and stored.
func TestTokenForRegistryObtainsWhenAutomatic(t *testing.T) {
	store := newStore(t)
	issued, claims, err := IssueToken(testSecret, TokenRequest{
		Subject:  "miraur",
		Audience: "http://reg:8080",
	})
	if err != nil {
		t.Fatal(err)
	}

	policy := Policy{
		AutoLogin: true,
		Issuer: func(registry string) (string, TokenClaims, error) {
			return issued, claims, nil
		},
	}
	got, err := store.TokenForRegistry("http://reg:8080", policy, time.Now())
	if err != nil {
		t.Fatalf("expected the credential to be obtained, got %v", err)
	}
	if got.Secret != issued {
		t.Error("the obtained token was not returned")
	}
	// And it was persisted, so the next command does not log in again.
	stored, err := store.Get("http://reg:8080", "")
	if err != nil {
		t.Fatalf("the obtained credential was not stored: %v", err)
	}
	if stored.Secret != issued {
		t.Error("the stored credential differs from the issued one")
	}
}

// An expired credential is replaced, which is what makes renewal work.
func TestTokenForRegistryReplacesAnExpiredCredential(t *testing.T) {
	store := newStore(t)
	stale := Credential{
		Method:    TokenMethod,
		Registry:  "http://reg:8080",
		Secret:    "stale",
		Subject:   "miraur",
		IssuedAt:  time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := store.Put(stale); err != nil {
		t.Fatal(err)
	}

	fresh, claims, err := IssueToken(testSecret, TokenRequest{Subject: "miraur", Audience: "http://reg:8080"})
	if err != nil {
		t.Fatal(err)
	}
	policy := Policy{
		AutoLogin: true,
		Issuer:    func(string) (string, TokenClaims, error) { return fresh, claims, nil },
	}
	got, err := store.TokenForRegistry("http://reg:8080", policy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got.Secret != fresh {
		t.Error("an expired credential should have been replaced")
	}
}

// A registry that cannot issue a token surfaces its own failure.
func TestTokenForRegistrySurfacesIssuerFailure(t *testing.T) {
	store := newStore(t)
	policy := Policy{
		AutoLogin: true,
		Issuer:    func(string) (string, TokenClaims, error) { return "", TokenClaims{}, ErrRejected },
	}
	if _, err := store.TokenForRegistry("http://reg", policy, time.Now()); err == nil {
		t.Fatal("expected the issuer's failure to surface")
	}
}
