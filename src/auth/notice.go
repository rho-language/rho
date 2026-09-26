//
// @file notice.go
// @brief Reporting on credentials that are about to stop working.
//
// A credential that quietly lapses turns into a confusing failure in an
// unrelated command, so the things that use a credential check it first and
// say something useful. The thresholds are deliberately generous: a warning
// that fires constantly stops being read.
//
// @project Rho
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package auth

import (
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ExpiryWarning is how long ahead a credential is called out.
const ExpiryWarning = 7 * 24 * time.Hour

// Notice describes something worth telling the user about a credential.
type Notice struct {
	// Registry is which registry the credential is for.
	Registry string
	// Subject is who it belongs to.
	Subject string
	// Method is which mechanism issued it.
	Method string
	// Level is whether it is advice or a problem.
	Level Level
	// Reason is a short explanation.
	Reason string
}

// Level says how much a notice matters.
type Level int

const (
	// Info is background, such as when a credential was issued.
	Info Level = iota
	// Warn is something that will need attention soon.
	Warn
	// Fail is something already broken.
	Fail
)

// String renders a level for display.
func (l Level) String() string {
	switch l {
	case Warn:
		return "warning"
	case Fail:
		return "error"
	default:
		return "info"
	}
}

// Describe checks every stored credential and returns what is worth saying,
// soonest first.
func (s *Store) Describe(now time.Time) ([]Notice, error) {
	credentials, err := s.List()
	if err != nil {
		return nil, err
	}

	notices := make([]Notice, 0, len(credentials))
	for _, credential := range credentials {
		switch {
		case credential.Expired(now):
			notices = append(notices, Notice{
				Registry: credential.Registry,
				Subject:  credential.Subject,
				Method:   credential.Method,
				Level:    Fail,
				Reason: fmt.Sprintf("expired on %s; run: rho auth login --registry %s",
					credential.ExpiresAt.Format("2006-01-02"), credential.Registry),
			})
		case credential.ExpiresAt.Sub(now) <= ExpiryWarning:
			notices = append(notices, Notice{
				Registry: credential.Registry,
				Subject:  credential.Subject,
				Method:   credential.Method,
				Level:    Warn,
				Reason: fmt.Sprintf("expires on %s, in %d days; renew with: rho auth login --registry %s",
					credential.ExpiresAt.Format("2006-01-02"),
					int(credential.ExpiresAt.Sub(now).Hours()/24),
					credential.Registry),
			})
		}
	}

	// Soonest first, so the most pressing is the first thing read.
	sort.Slice(notices, func(i, j int) bool {
		left, right := notices[i], notices[j]
		if left.Level != right.Level {
			return left.Level > right.Level
		}
		return left.Registry < right.Registry
	})
	return notices, nil
}

// FormatNotices renders notices as lines a command can print.
func FormatNotices(notices []Notice) string {
	lines := make([]string, 0, len(notices))
	for _, notice := range notices {
		who := notice.Subject
		if who == "" {
			who = notice.Method
		}
		lines = append(lines, fmt.Sprintf("%s: %s credential for %s: %s",
			notice.Level, who, notice.Registry, notice.Reason))
	}
	return strings.Join(lines, "\n")
}

/* -------------------------------------------------------------------------- */
/*  Acquisition                                                               */
/* -------------------------------------------------------------------------- */

// Policy decides whether a token is created for the user or by them.
//
// A registry that issues tokens to anyone who asks provides no authentication
// at all, so obtaining one is something a person does deliberately. The
// automatic path exists for the case where the operator has said a registry is
// trusted, and it is off unless asked for.
type Policy struct {
	// AutoLogin lets the client request a token when one is missing, for a
	// registry the operator has marked as trusted.
	AutoLogin bool
	// Issuer obtains a token for a registry. It is the login endpoint, and it
	// is absent when the user has to sign in themselves.
	Issuer func(registry string) (string, TokenClaims, error)
}

// TokenForRegistry returns the credential to present to a registry, obtaining
// one first when the policy allows it.
//
// The automatic path is only taken when the user has both enabled it and the
// registry can issue a token without a login, so nothing happens silently.
func (s *Store) TokenForRegistry(registry string, policy Policy, now time.Time) (Credential, error) {
	stored, err := s.Get(registry, "")
	if err == nil && !stored.Expired(now) {
		return stored, nil
	}
	if err != nil && !isMissing(err) {
		return Credential{}, err
	}

	if !policy.AutoLogin || policy.Issuer == nil {
		return Credential{}, fmt.Errorf(
			"no valid credential for %s; run: rho auth login --registry %s", registry, registry)
	}

	token, claims, err := policy.Issuer(registry)
	if err != nil {
		return Credential{}, err
	}
	credential := Credential{
		Method:   TokenMethod,
		Registry: registry,
		Secret:   token,
		Subject:  claims.Subject,
		Host:     claims.Host,
		Scopes:   claims.Scopes,
		IssuedAt: time.Unix(claims.IssuedAt, 0),
	}
	if claims.ExpiresAt > 0 {
		credential.ExpiresAt = time.Unix(claims.ExpiresAt, 0)
	}
	if err := s.Put(credential); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func isMissing(err error) bool {
	return err == ErrNoCredentialStored || (err != nil && strings.Contains(err.Error(), "no credential stored"))
}

/* -------------------------------------------------------------------------- */
/*  Inspection                                                                */
/* -------------------------------------------------------------------------- */

// Inspect decodes a token's claims without verifying the signature.
//
// This is deliberately separate from Verify: a token's claims are readable by
// anyone holding it, so inspecting one is not a security question, while
// trusting one is. Callers that want a decision should use Verify.
func Inspect(token string) (TokenClaims, error) {
	var claims TokenClaims
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return claims, ErrMalformed
	}
	payload, err := decodeSegment(parts[1])
	if err != nil {
		return claims, ErrMalformed
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, ErrMalformed
	}
	return claims, nil
}

// CheckSignature reports whether a token's signature is intact, without
// checking whether it has expired. It answers "is this token genuine", which is
// a different question from "is this token usable now".
func CheckSignature(secret []byte, token string) bool {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return false
	}
	provided, err := decodeSegment(parts[2])
	if err != nil {
		return false
	}
	return hmac.Equal(signSegment(parts[0]+"."+parts[1], secret), provided)
}
