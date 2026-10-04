//
// @file auth_test.go
// @brief Tests for Rho authentication: the pluggable core, the signed-token
// mechanism, and credential storage.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("a-test-signing-secret-value-32b")

/* -------------------------------------------------------------------------- */
/*  Helpers                                                                    */
/* -------------------------------------------------------------------------- */

func issue(t *testing.T, request TokenRequest) (string, TokenClaims) {
	t.Helper()
	token, claims, err := IssueToken(testSecret, request)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	return token, claims
}

/* -------------------------------------------------------------------------- */
/*  Test: Issuing                                                              */
/* -------------------------------------------------------------------------- */

func TestIssueTokenShape(t *testing.T) {
	token, claims := issue(t, TokenRequest{
		Subject:  "miraur",
		Audience: "http://localhost:18080",
		Host:     "workstation",
		Scopes:   []string{ScopePull, ScopePush},
	})
	if parts := strings.Split(token, "."); len(parts) != 3 {
		t.Fatalf("expected three segments, got %d", len(parts))
	}
	if claims.Subject != "miraur" {
		t.Errorf("subject: got %q", claims.Subject)
	}
	if claims.TokenID == "" {
		t.Error("expected a token id so the token can be revoked")
	}
	if claims.Issuer != TokenIssuer {
		t.Errorf("issuer: got %q", claims.Issuer)
	}
	if claims.ExpiresAt <= claims.IssuedAt {
		t.Errorf("expiry %d should be after issue %d", claims.ExpiresAt, claims.IssuedAt)
	}
}

func TestIssueTokenRequiresSubject(t *testing.T) {
	if _, _, err := IssueToken(testSecret, TokenRequest{}); err == nil {
		t.Fatal("expected an error for a token with no subject")
	}
}

func TestIssueTokenRequiresSecret(t *testing.T) {
	if _, _, err := IssueToken(nil, TokenRequest{Subject: "x"}); err == nil {
		t.Fatal("expected an error without a signing secret")
	}
}

// A token may never outlive the maximum, whatever the caller asks for.
func TestIssueTokenClampsLifetime(t *testing.T) {
	_, claims := issue(t, TokenRequest{
		Subject:  "miraur",
		Lifetime: 10 * 365 * 24 * time.Hour,
	})
	granted := time.Duration(claims.ExpiresAt-claims.IssuedAt) * time.Second
	if granted > MaxTokenLifetime {
		t.Fatalf("granted %v, which exceeds the %v maximum", granted, MaxTokenLifetime)
	}
	if granted != MaxTokenLifetime {
		t.Fatalf("expected the lifetime to be clamped to %v, got %v", MaxTokenLifetime, granted)
	}
}

// A caller may ask for a shorter lifetime than the maximum.
func TestIssueTokenRespectsShorterLifetime(t *testing.T) {
	_, claims := issue(t, TokenRequest{Subject: "miraur", Lifetime: 24 * time.Hour})
	granted := time.Duration(claims.ExpiresAt-claims.IssuedAt) * time.Second
	if granted != 24*time.Hour {
		t.Fatalf("expected 24h, got %v", granted)
	}
}

func TestIssueTokenDefaultsLifetime(t *testing.T) {
	_, claims := issue(t, TokenRequest{Subject: "miraur"})
	granted := time.Duration(claims.ExpiresAt-claims.IssuedAt) * time.Second
	if granted != DefaultTokenLifetime {
		t.Fatalf("expected the default %v, got %v", DefaultTokenLifetime, granted)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Verification                                                         */
/* -------------------------------------------------------------------------- */

func TestVerifyAcceptsAValidToken(t *testing.T) {
	token, _ := issue(t, TokenRequest{
		Subject:  "miraur",
		Audience: "reg",
		Scopes:   []string{ScopePull, ScopePush},
	})
	verifier := TokenVerifier{Secret: testSecret, Audience: "reg"}
	principal, err := verifier.Verify(token)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if principal.Subject != "miraur" {
		t.Errorf("subject: got %q", principal.Subject)
	}
	if principal.Method != TokenMethod {
		t.Errorf("method: got %q", principal.Method)
	}
	if !principal.HasScope(ScopePush) {
		t.Error("expected the push scope to carry over")
	}
}

func TestVerifyRejectsAnotherSecret(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur"})
	other := TokenVerifier{Secret: []byte("a-different-secret-value-here!!")}
	if _, err := other.VerifyToken(token); !errors.Is(err, ErrRejected) {
		t.Fatalf("expected rejection, got %v", err)
	}
}

func TestVerifyRejectsATamperedPayload(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur"})
	parts := strings.Split(token, ".")
	// Rewrite the payload to claim a different subject, keeping the signature.
	parts[1] = encodeSegment([]byte(`{"sub":"admin","iat":1,"exp":9999999999}`))
	forged := strings.Join(parts, ".")

	verifier := TokenVerifier{Secret: testSecret}
	if _, err := verifier.VerifyToken(forged); !errors.Is(err, ErrRejected) {
		t.Fatalf("a tampered token must be rejected, got %v", err)
	}
}

func TestVerifyRejectsAnExpiredToken(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur", Lifetime: time.Hour})
	// Verify as though it were well past the expiry.
	verifier := TokenVerifier{
		Secret: testSecret,
		Now:    func() time.Time { return time.Now().Add(48 * time.Hour) },
	}
	if _, err := verifier.VerifyToken(token); !errors.Is(err, ErrRejected) {
		t.Fatalf("an expired token must be rejected, got %v", err)
	}
}

func TestVerifyRejectsAnotherAudience(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur", Audience: "other-registry"})
	verifier := TokenVerifier{Secret: testSecret, Audience: "my-registry"}
	if _, err := verifier.VerifyToken(token); !errors.Is(err, ErrRejected) {
		t.Fatalf("a token for another registry must be rejected, got %v", err)
	}
}

func TestVerifyRejectsMalformedTokens(t *testing.T) {
	verifier := TokenVerifier{Secret: testSecret}
	for _, bad := range []string{"", "not-a-token", "a.b", "a.b.c.d", "!!.??.##"} {
		if _, err := verifier.VerifyToken(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

// A token whose algorithm is not ours must not be accepted, which is what
// stops a token being presented as unsigned.
func TestVerifyRejectsADowngradedAlgorithm(t *testing.T) {
	header := encodeSegment([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := encodeSegment([]byte(`{"sub":"admin","exp":9999999999}`))
	forged := header + "." + payload + "."

	verifier := TokenVerifier{Secret: testSecret}
	if _, err := verifier.VerifyToken(forged); err == nil {
		t.Fatal("a token signed with no algorithm must be rejected")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: The Chain                                                           */
/* -------------------------------------------------------------------------- */

// The chain is what makes another mechanism possible later, so it has to hand
// a credential it does not recognise to the next verifier.
func TestChainPicksTheRecognisingMechanism(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur"})

	// A mechanism that never recognises anything stands in for a future one.
	other := VerifierFunc{
		Method: "apikey",
		Authenticate: func(string) (Principal, error) {
			return Anonymous(), errors.New("not an api key")
		},
	}
	chain := NewChain(other, TokenVerifier{Secret: testSecret})

	principal, err := chain.Authenticate(token)
	if err != nil {
		t.Fatalf("chain failed: %v", err)
	}
	if principal.Subject != "miraur" {
		t.Fatalf("subject: got %q", principal.Subject)
	}
	if principal.Method != TokenMethod {
		t.Fatalf("expected the token mechanism, got %q", principal.Method)
	}
}

func TestChainReportsMethods(t *testing.T) {
	chain := NewChain(
		TokenVerifier{Secret: testSecret},
		VerifierFunc{Method: "apikey", Authenticate: func(string) (Principal, error) { return Anonymous(), nil }},
	)
	methods := chain.Methods()
	if len(methods) != 2 || methods[0] != TokenMethod || methods[1] != "apikey" {
		t.Fatalf("unexpected methods: %v", methods)
	}
}

// A mechanism that recognises a credential and refuses it ends the chain, so
// the next one cannot accept what this one rejected.
func TestChainStopsOnARefusal(t *testing.T) {
	strict := VerifierFunc{
		Method: "strict",
		Authenticate: func(string) (Principal, error) {
			return Anonymous(), ErrRejected
		},
	}
	permissive := VerifierFunc{
		Method: "permissive",
		Authenticate: func(string) (Principal, error) {
			return Principal{Subject: "anyone", Method: "permissive"}, nil
		},
	}
	if _, err := NewChain(strict, permissive).Authenticate("whatever"); !errors.Is(err, ErrRejected) {
		t.Fatal("a refusal must end the chain")
	}
}

func TestChainReportsNoCredential(t *testing.T) {
	chain := NewChain(TokenVerifier{Secret: testSecret})
	principal, err := chain.Authenticate("")
	if !errors.Is(err, ErrNoCredential) {
		t.Fatalf("expected ErrNoCredential, got %v", err)
	}
	if !principal.IsAnonymous() {
		t.Fatal("an absent credential should yield the anonymous principal")
	}
}

func TestChainRefusesAnUnknownCredential(t *testing.T) {
	chain := NewChain(TokenVerifier{Secret: testSecret})
	if _, err := chain.Authenticate("this-is-not-a-token"); !errors.Is(err, ErrRejected) {
		t.Fatalf("expected a refusal, got %v", err)
	}
}

func TestChainCanGrowAfterConstruction(t *testing.T) {
	chain := NewChain()
	chain.Add(TokenVerifier{Secret: testSecret})
	token, _ := issue(t, TokenRequest{Subject: "miraur"})
	if _, err := chain.Authenticate(token); err != nil {
		t.Fatalf("a mechanism added after construction should work: %v", err)
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Principal                                                           */
/* -------------------------------------------------------------------------- */

func TestAnonymousPrincipal(t *testing.T) {
	principal := Anonymous()
	if !principal.IsAnonymous() {
		t.Fatal("expected the anonymous principal")
	}
	if principal.String() != "anonymous" {
		t.Errorf("display: got %q", principal.String())
	}
}

func TestPrincipalScopes(t *testing.T) {
	principal := Principal{Subject: "miraur", Method: TokenMethod, Scopes: []string{ScopePull}}
	if !principal.HasScope(ScopePull) {
		t.Error("expected the pull scope")
	}
	if principal.HasScope(ScopePush) {
		t.Error("did not expect a push scope")
	}
	// A credential with no scopes has none, rather than all of them.
	if (Principal{Subject: "x", Method: TokenMethod}).HasScope(ScopePull) {
		t.Error("a principal with no scopes should have none")
	}
	// A wildcard covers everything.
	if !(Principal{Scopes: []string{"*"}}).HasScope(ScopePush) {
		t.Error("a wildcard scope should cover everything")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Authorization header                                                */
/* -------------------------------------------------------------------------- */

func TestFromAuthorization(t *testing.T) {
	cases := map[string]string{
		"Bearer abc123": "abc123",
		"bearer abc123": "abc123",
		"BEARER abc123": "abc123",
		"abc123":        "abc123",
		"":              "",
		"  Bearer x  ":  "x",
	}
	for header, want := range cases {
		if got := FromAuthorization(header); got != want {
			t.Errorf("FromAuthorization(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestAuthorizationRoundTrip(t *testing.T) {
	header := Authorization("abc123")
	if header != "Bearer abc123" {
		t.Fatalf("got %q", header)
	}
	if got := FromAuthorization(header); got != "abc123" {
		t.Fatalf("round trip: got %q", got)
	}
	if Authorization("") != "" {
		t.Error("an empty credential should render as an empty header")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: Secret storage                                                       */
/* -------------------------------------------------------------------------- */

func TestLoadOrCreateSecretGeneratesThenReuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "signing.key")
	first, err := LoadOrCreateSecret(path)
	if err != nil {
		t.Fatalf("LoadOrCreateSecret failed: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("expected a generated secret")
	}
	second, err := LoadOrCreateSecret(path)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("the secret must be stable across calls")
	}
}

// The secret can mint credentials, so it must not be world readable.
func TestSecretIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "signing.key")
	if _, err := LoadOrCreateSecret(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		t.Fatalf("secret is readable beyond its owner: mode %v", mode)
	}
}

func TestLoadOrCreateSecretRejectsAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing.key")
	if err := os.WriteFile(path, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret, err := LoadOrCreateSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) == 0 {
		t.Fatal("an empty secret file should be replaced")
	}
}

/* -------------------------------------------------------------------------- */
/*  Test: The Security Model                                                  */
/* -------------------------------------------------------------------------- */

// A token is signed, not encrypted: the claims are readable by anyone holding
// it. That is a property of the scheme, and it is what makes a stolen token
// readable to whoever intercepts it, so it is pinned here rather than left to
// be assumed.
func TestTokenPayloadIsReadableNotHidden(t *testing.T) {
	token, claims := issue(t, TokenRequest{Subject: "miraur", Host: "workstation"})

	parts := strings.Split(token, ".")
	payload, err := decodeSegment(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `"sub":"miraur"`) {
		t.Fatalf("expected readable claims, got %s", payload)
	}
	if !strings.Contains(string(payload), `"host":"workstation"`) {
		t.Fatalf("expected the host claim, got %s", payload)
	}

	// Reading the claims does not let anyone change them: a rewritten payload
	// no longer matches the signature.
	forged := parts[0] + "." + encodeSegment([]byte(`{"sub":"admin","exp":9999999999}`)) + "." + parts[2]
	if _, err := (TokenVerifier{Secret: testSecret}).VerifyToken(forged); !errors.Is(err, ErrRejected) {
		t.Fatalf("rewriting the claims must break the signature, got %v", err)
	}
	_ = claims
}

// The signing secret is what makes a signature worth anything, and it never
// leaves the registry: a client asks for a token rather than minting one.
func TestSecretNeverTravelsInAToken(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur"})

	parts := strings.Split(token, ".")
	secretText := string(testSecret)
	for _, part := range parts {
		if strings.Contains(part, secretText) {
			t.Fatal("the signing secret must not appear in a token")
		}
	}
	// Nor may any encoding of it.
	if decoded, err := decodeSegment(parts[2]); err == nil {
		if strings.Contains(string(decoded), secretText) {
			t.Fatal("the signature must not reveal the secret")
		}
	}
}

// Every claim a holder might rely on is covered by the signature, so none of
// them can be altered in transit.
func TestEveryClaimIsCoveredByTheSignature(t *testing.T) {
	token, _ := issue(t, TokenRequest{Subject: "miraur", Audience: "reg"})

	for _, forged := range []string{
		// a longer lifetime
		`{"sub":"miraur","aud":"reg","exp":9999999999}`,
		// a different audience
		`{"sub":"miraur","aud":"other","exp":1}`,
		// a different subject
		`{"sub":"admin","aud":"reg","exp":1}`,
	} {
		parts := strings.Split(token, ".")
		altered := parts[0] + "." + encodeSegment([]byte(forged)) + "." + parts[2]
		if _, err := (TokenVerifier{Secret: testSecret, Audience: "reg"}).VerifyToken(altered); !errors.Is(err, ErrRejected) {
			t.Errorf("altering claims must be rejected: %s", forged)
		}
	}
}

// A token stops being usable when it runs out, and the check needs no clock
// from the caller: the verifier's own time decides.
func TestExpiryIsEnforcedWithoutCallerInput(t *testing.T) {
	token, claims := issue(t, TokenRequest{Subject: "miraur", Lifetime: time.Hour})
	verifier := TokenVerifier{Secret: testSecret}
	if _, err := verifier.VerifyToken(token); err != nil {
		t.Fatalf("a fresh token should verify: %v", err)
	}
	if claims.ExpiresAt <= time.Now().Unix() {
		t.Fatal("expected a future expiry")
	}
}
