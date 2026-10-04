//
// @file auth.go
// @brief Authentication core for Rho.
//
// The mechanism is deliberately pluggable. A Verifier turns a presented
// credential into a Principal, and a Chain tries several in turn, so a new
// scheme (an API key, OAuth, a client certificate) is added by implementing one
// interface and listing it, with nothing else changing.
//
// The signed-token scheme that exists today lives in token.go; it is the first
// Verifier, not the shape of the system.
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
	"strings"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  Errors                                                                    */
/* -------------------------------------------------------------------------- */

// Errors a caller distinguishes. They are deliberately coarse: a rejected
// credential should not reveal which check failed.
var (
	// ErrNoCredential means nothing was presented.
	ErrNoCredential = errors.New("auth: no credential presented")
	// ErrRejected means a credential was presented and refused.
	ErrRejected = errors.New("auth: credential rejected")
	// ErrMalformed means a credential is not shaped like this mechanism's.
	ErrMalformed = errors.New("auth: malformed credential")
	// ErrMethodUnknown names a mechanism that is not configured.
	ErrMethodUnknown = errors.New("auth: unknown authentication method")
)

/* -------------------------------------------------------------------------- */
/*  Principal                                                                 */
/* -------------------------------------------------------------------------- */

// Principal is an authenticated identity: who is acting, by which mechanism,
// and until when.
type Principal struct {
	// Subject is who the credential belongs to.
	Subject string
	// Method is which mechanism authenticated them, for example "token".
	Method string
	// CredentialID identifies the specific credential, so it can be revoked.
	CredentialID string
	// Host is where the credential was issued from, for display.
	Host string
	// Expires is when the credential stops being valid. Zero means it does
	// not expire, which only a mechanism that can revoke should allow.
	Expires time.Time
	// Scopes are what the credential may do, checked by the caller.
	Scopes []string
}

// Anonymous is the principal for an unauthenticated request. Public routes
// accept it; private ones do not.
func Anonymous() Principal {
	return Principal{Subject: "anonymous", Method: "none"}
}

// IsAnonymous reports whether a principal carries no verified identity.
func (p Principal) IsAnonymous() bool {
	return p.Subject == "" || p.Method == "" || p.Method == "none"
}

// HasScope reports whether a principal carries a scope. A principal with no
// scopes has none.
func (p Principal) HasScope(scope string) bool {
	for _, held := range p.Scopes {
		if held == scope || held == "*" {
			return true
		}
	}
	return false
}

// String renders a principal for display.
func (p Principal) String() string {
	if p.IsAnonymous() {
		return "anonymous"
	}
	out := p.Subject + " via " + p.Method
	if p.Expires.IsZero() {
		return out
	}
	return out + " until " + p.Expires.Format("2006-01-02")
}

/* -------------------------------------------------------------------------- */
/*  Verifier                                                                  */
/* -------------------------------------------------------------------------- */

// Verifier turns a presented credential into a Principal.
//
// A verifier returns ErrRejected for a credential it recognises but refuses,
// and any other error for a credential it does not handle, so a Chain can tell
// "wrong" from "not mine".
type Verifier interface {
	// Name identifies the mechanism, and is recorded on the Principal.
	Name() string
	// Verify authenticates a credential.
	Verify(credential string) (Principal, error)
}

// VerifierFunc adapts a function to the Verifier interface, so a mechanism does
// not need a type of its own.
type VerifierFunc struct {
	// Method is the mechanism's name.
	Method string
	// Authenticate performs the check.
	Authenticate func(credential string) (Principal, error)
}

func (v VerifierFunc) Name() string { return v.Method }

func (v VerifierFunc) Verify(credential string) (Principal, error) {
	return v.Authenticate(credential)
}

/* -------------------------------------------------------------------------- */
/*  Chain                                                                     */
/* -------------------------------------------------------------------------- */

// Chain tries several mechanisms, so a service can accept more than one kind
// of credential. The first verifier that recognises a credential decides.
type Chain struct {
	verifiers []Verifier
}

// NewChain builds a chain in priority order.
func NewChain(verifiers ...Verifier) *Chain {
	return &Chain{verifiers: verifiers}
}

// Add appends a mechanism, so a service can grow without touching the ones
// already configured.
func (c *Chain) Add(verifier Verifier) *Chain {
	c.verifiers = append(c.verifiers, verifier)
	return c
}

// Methods lists the configured mechanisms, for display and diagnostics.
func (c *Chain) Methods() []string {
	out := make([]string, 0, len(c.verifiers))
	for _, verifier := range c.verifiers {
		out = append(out, verifier.Name())
	}
	return out
}

// Authenticate returns the first principal that verifies.
//
// A credential is refused only when a mechanism recognised it and said no; an
// empty credential is reported separately so a caller can tell "anonymous" from
// "rejected".
func (c *Chain) Authenticate(credential string) (Principal, error) {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return Anonymous(), ErrNoCredential
	}
	for _, verifier := range c.verifiers {
		principal, err := verifier.Verify(credential)
		if err == nil {
			return principal, nil
		}
		if errors.Is(err, ErrRejected) {
			return Anonymous(), err
		}
	}
	return Anonymous(), ErrRejected
}
