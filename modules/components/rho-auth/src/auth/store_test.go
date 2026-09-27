//
// @file store_test.go
// @brief Tests for credential storage under the Rho home.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "auth", "credentials.json"))
}

func credential(registry, subject, method string) Credential {
	now := time.Now()
	return Credential{
		Method:    method,
		Registry:  registry,
		Secret:    "signed-value",
		Subject:   subject,
		Scopes:    []string{ScopePull},
		IssuedAt:  now,
		ExpiresAt: now.Add(24 * time.Hour),
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Put and Get                                                         */
/* -------------------------------------------------------------------------- */

func TestStorePutAndGet(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://reg:8080", "miraur", TokenMethod)); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	got, err := store.Get("http://reg:8080", "")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Subject != "miraur" || got.Secret != "signed-value" {
		t.Fatalf("unexpected credential: %+v", got)
	}
}

func TestStoreGetMissing(t *testing.T) {
	store := newStore(t)
	if _, err := store.Get("http://nowhere", ""); !errors.Is(err, ErrNoCredentialStored) {
		t.Fatalf("expected ErrNoCredentialStored, got %v", err)
	}
}

func TestStoreGetOnAnAbsentFile(t *testing.T) {
	store := newStore(t)
	if _, err := store.Get("http://reg", ""); !errors.Is(err, ErrNoCredentialStored) {
		t.Fatalf("an absent store should report nothing stored, got %v", err)
	}
}

// Logging in again replaces the credential rather than accumulating them.
func TestStorePutReplacesForTheSameRegistryAndMethod(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://reg", "first", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(credential("http://reg", "second", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	all, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected one credential, got %d", len(all))
	}
	if all[0].Subject != "second" {
		t.Fatalf("expected the newer credential, got %q", all[0].Subject)
	}
}

// Several registries can be configured at once.
func TestStoreKeepsRegistriesApart(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://one", "a", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(credential("http://two", "b", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	one, err := store.Get("http://one", "")
	if err != nil {
		t.Fatal(err)
	}
	two, err := store.Get("http://two", "")
	if err != nil {
		t.Fatal(err)
	}
	if one.Subject != "a" || two.Subject != "b" {
		t.Fatalf("registries were mixed up: %q and %q", one.Subject, two.Subject)
	}
}

// Credentials for one registry can carry more than one mechanism, which is
// what lets a future scheme sit alongside tokens.
func TestStoreKeepsMethodsApart(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://reg", "with-token", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	apiKey := credential("http://reg", "with-key", "apikey")
	apiKey.Secret = "key-value"
	if err := store.Put(apiKey); err != nil {
		t.Fatal(err)
	}

	token, err := store.Get("http://reg", TokenMethod)
	if err != nil {
		t.Fatal(err)
	}
	if token.Secret != "signed-value" {
		t.Errorf("token credential: got %q", token.Secret)
	}
	key, err := store.Get("http://reg", "apikey")
	if err != nil {
		t.Fatal(err)
	}
	if key.Secret != "key-value" {
		t.Errorf("api key credential: got %q", key.Secret)
	}
}

func TestStoreDefaultsTheMethod(t *testing.T) {
	store := newStore(t)
	entry := credential("http://reg", "miraur", "")
	if err := store.Put(entry); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get("http://reg", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != TokenMethod {
		t.Fatalf("expected the token method by default, got %q", got.Method)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: List                                                                */
/* -------------------------------------------------------------------------- */

func TestStoreListIsSorted(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://b", "second", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(credential("http://a", "first", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	all, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected two, got %d", len(all))
	}
	if all[0].Registry != "http://a" {
		t.Fatalf("expected the list sorted by registry, got %q first", all[0].Registry)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Revoke                                                              */
/* -------------------------------------------------------------------------- */

func TestStoreRevoke(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://reg", "miraur", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	removed, err := store.Revoke("http://reg")
	if err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}
	if !removed {
		t.Fatal("expected the revoke to report a removal")
	}
	if _, err := store.Get("http://reg", ""); !errors.Is(err, ErrNoCredentialStored) {
		t.Fatal("the credential should be gone after a revoke")
	}
}

func TestStoreRevokeMissing(t *testing.T) {
	store := newStore(t)
	removed, err := store.Revoke("http://nowhere")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("revoking nothing should report no removal")
	}
}

func TestStoreRevokeLeavesOtherRegistries(t *testing.T) {
	store := newStore(t)
	if err := store.Put(credential("http://one", "a", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(credential("http://two", "b", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Revoke("http://one"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("http://two", ""); err != nil {
		t.Fatalf("the other registry should survive: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Persistence and permissions                                         */
/* -------------------------------------------------------------------------- */

func TestStorePersistsAcrossOpenings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "credentials.json")
	if err := Open(path).Put(credential("http://reg", "miraur", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	got, err := Open(path).Get("http://reg", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "miraur" {
		t.Fatalf("expected the credential to persist, got %+v", got)
	}
}

// The file holds credentials, so it must not be readable beyond its owner.
func TestStoreIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "credentials.json")
	if err := Open(path).Put(credential("http://reg", "miraur", TokenMethod)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("store is readable beyond its owner: mode %v", mode)
	}
}

// A corrupt store is reported, not silently replaced, so a credential is never
// lost without notice.
func TestStoreReportsCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "credentials.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path).List(); err == nil {
		t.Fatal("expected a corrupt store to be reported")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Credential                                                          */
/* -------------------------------------------------------------------------- */

func TestCredentialExpired(t *testing.T) {
	now := time.Now()
	if !(Credential{ExpiresAt: now.Add(-time.Hour)}).Expired(now) {
		t.Error("a past expiry should read as expired")
	}
	if (Credential{ExpiresAt: now.Add(time.Hour)}).Expired(now) {
		t.Error("a future expiry should not read as expired")
	}
	if (Credential{}).Expired(now) {
		t.Error("a credential with no expiry does not expire")
	}
}

func TestDefaultPaths(t *testing.T) {
	home := "/home/miraur/.rho"
	if got := DefaultStorePath(home); got != filepath.Join(home, "auth", "credentials.json") {
		t.Errorf("store path: %q", got)
	}
	if got := DefaultSecretPath(home); got != filepath.Join(home, "auth", "signing.key") {
		t.Errorf("secret path: %q", got)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: End to end                                                          */
/* -------------------------------------------------------------------------- */

// A credential issued, stored and presented back must verify: this is the
// path a client actually takes.
func TestIssuedCredentialVerifiesWhenPresented(t *testing.T) {
	secret, err := LoadOrCreateSecret(filepath.Join(t.TempDir(), "auth", "signing.key"))
	if err != nil {
		t.Fatal(err)
	}
	token, claims, err := IssueToken(secret, TokenRequest{
		Subject:  "miraur",
		Audience: "http://reg:8080",
		Host:     "workstation",
		Scopes:   []string{ScopePull, ScopePush},
		Lifetime: 7 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	store := newStore(t)
	if err := store.Put(Credential{
		Method:    TokenMethod,
		Registry:  "http://reg:8080",
		Secret:    token,
		Subject:   claims.Subject,
		Host:      claims.Host,
		Scopes:    claims.Scopes,
		IssuedAt:  time.Unix(claims.IssuedAt, 0),
		ExpiresAt: time.Unix(claims.ExpiresAt, 0),
	}); err != nil {
		t.Fatal(err)
	}

	// The server side: a header arrives and a principal comes out.
	stored, err := store.Get("http://reg:8080", "")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := NewChain(TokenVerifier{Secret: secret, Audience: "http://reg:8080"}).
		Authenticate(FromAuthorization(Authorization(stored.Secret)))
	if err != nil {
		t.Fatalf("presented credential did not verify: %v", err)
	}
	if principal.Subject != "miraur" {
		t.Errorf("subject: got %q", principal.Subject)
	}
	if !principal.HasScope(ScopePush) {
		t.Error("expected the push scope")
	}
}
