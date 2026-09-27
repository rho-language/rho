//
// @file token.go
// @brief The signed-token mechanism.
//
// A token is three base64url segments:
//
//	header.payload.signature
//
// The header names the algorithm, the payload carries the claims, and the
// signature is an HMAC-SHA256 over the first two segments. The shape follows
// JWT, so the tokens are recognisable to anyone who has worked with a web API.
//
// The signing secret never leaves the registry: a client asks for a token
// through `rho auth login` and receives a signed one, so a stolen client
// configuration cannot be used to mint new credentials.
//
// A token is valid for a bounded period. A caller may ask for less, never for
// more than MaxTokenLifetime, so a token cannot become a permanent credential
// by asking nicely.
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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/* -------------------------------------------------------------------------- */
/*  Limits                                                                    */
/* -------------------------------------------------------------------------- */

// MaxTokenLifetime bounds how long any token may be valid, so a caller cannot
// ask for a credential that never expires.
const MaxTokenLifetime = 90 * 24 * time.Hour

// DefaultTokenLifetime is used when a caller does not choose.
const DefaultTokenLifetime = 30 * 24 * time.Hour

// TokenIssuer is the claim value identifying a Rho registry.
const TokenIssuer = "rho-registry"

// TokenAlgorithm is the only signing algorithm accepted, which keeps a token
// from being reinterpreted as something weaker.
const TokenAlgorithm = "HS256"

// TokenMethod names this mechanism in a Principal and in configuration.
const TokenMethod = "token"

// Token scopes. Reading a public module needs nothing; publishing needs push.
const (
	ScopePull = "pull"
	ScopePush = "push"
)

/* -------------------------------------------------------------------------- */
/*  Claims                                                                    */
/* -------------------------------------------------------------------------- */

// TokenClaims are the statements a token carries.
type TokenClaims struct {
	// Subject is who the token was issued to.
	Subject string `json:"sub"`
	// TokenID uniquely identifies this token, so it can be revoked.
	TokenID string `json:"jti"`
	// Issuer names who signed it.
	Issuer string `json:"iss"`
	// Audience is the registry the token is for.
	Audience string `json:"aud"`
	// Host is where the token was issued from, for display and auditing.
	Host string `json:"host,omitempty"`
	// Scopes are what the token may do.
	Scopes []string `json:"scp,omitempty"`
	// IssuedAt is when the token was created, in Unix seconds.
	IssuedAt int64 `json:"iat"`
	// ExpiresAt is when the token stops being valid, in Unix seconds.
	ExpiresAt int64 `json:"exp"`
}

// Principal converts verified claims into the shared identity type.
func (c TokenClaims) Principal() Principal {
	principal := Principal{
		Subject:      c.Subject,
		Method:       TokenMethod,
		CredentialID: c.TokenID,
		Host:         c.Host,
		Scopes:       c.Scopes,
	}
	if c.ExpiresAt > 0 {
		principal.Expires = time.Unix(c.ExpiresAt, 0)
	}
	return principal
}

// tokenHeader is the token's first segment.
type tokenHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

/* -------------------------------------------------------------------------- */
/*  Encoding                                                                  */
/* -------------------------------------------------------------------------- */

// encodeSegment renders bytes as unpadded base64url, the form JWT uses.
func encodeSegment(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}

func signSegment(message string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

/* -------------------------------------------------------------------------- */
/*  Issuing                                                                   */
/* -------------------------------------------------------------------------- */

// TokenRequest describes the token a caller is asking for.
type TokenRequest struct {
	// Subject is who the token is for.
	Subject string
	// Audience is the registry it is for.
	Audience string
	// Host is where the caller is issuing from.
	Host string
	// Scopes are what the token may do.
	Scopes []string
	// Lifetime is how long it should last, capped at MaxTokenLifetime.
	Lifetime time.Duration
}

// IssueToken mints a signed token.
//
// A requested lifetime is clamped to MaxTokenLifetime; a caller may ask for an
// earlier expiry but not a later one.
func IssueToken(secret []byte, request TokenRequest) (string, TokenClaims, error) {
	var claims TokenClaims
	if len(secret) == 0 {
		return "", claims, errors.New("auth: a signing secret is required")
	}
	if strings.TrimSpace(request.Subject) == "" {
		return "", claims, errors.New("auth: a token needs a subject")
	}

	lifetime := request.Lifetime
	if lifetime <= 0 {
		lifetime = DefaultTokenLifetime
	}
	if lifetime > MaxTokenLifetime {
		lifetime = MaxTokenLifetime
	}

	id, err := NewTokenID()
	if err != nil {
		return "", claims, err
	}
	now := time.Now()
	claims = TokenClaims{
		Subject:   request.Subject,
		TokenID:   id,
		Issuer:    TokenIssuer,
		Audience:  request.Audience,
		Host:      request.Host,
		Scopes:    request.Scopes,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(lifetime).Unix(),
	}

	headerJSON, err := json.Marshal(tokenHeader{Algorithm: TokenAlgorithm, Type: "JWT"})
	if err != nil {
		return "", claims, err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", claims, err
	}

	signing := encodeSegment(headerJSON) + "." + encodeSegment(payloadJSON)
	token := signing + "." + encodeSegment(signSegment(signing, secret))
	return token, claims, nil
}

// NewTokenID returns a random identifier, so two tokens issued in the same
// second are still distinguishable and separately revocable.
func NewTokenID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

/* -------------------------------------------------------------------------- */
/*  Verification                                                              */
/* -------------------------------------------------------------------------- */

// TokenVerifier authenticates signed tokens. It implements Verifier, so a
// service can add it to a Chain alongside other mechanisms.
type TokenVerifier struct {
	// Secret signs and verifies tokens.
	Secret []byte
	// Audience, when set, is the only registry a token may be for.
	Audience string
	// Now supplies the current time, for testing.
	Now func() time.Time
}

func (v TokenVerifier) Name() string { return TokenMethod }

func (v TokenVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// Verify authenticates a token, reporting a recognised-but-refused credential
// differently from one this mechanism does not handle.
func (v TokenVerifier) Verify(token string) (Principal, error) {
	claims, err := v.VerifyToken(token)
	if err != nil {
		// A malformed or unsigned token is not ours to judge, so the chain can
		// try the next mechanism.
		if errors.Is(err, ErrRejected) {
			return Anonymous(), ErrRejected
		}
		return Anonymous(), err
	}
	return claims.Principal(), nil
}

// VerifyToken checks a token and returns its claims.
//
// The errors are deliberately coarse: a caller should not learn which check a
// token failed.
func (v TokenVerifier) VerifyToken(token string) (TokenClaims, error) {
	var claims TokenClaims
	if len(v.Secret) == 0 {
		return claims, errors.New("auth: a signing secret is required")
	}

	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return claims, errNotOurToken
	}

	headerJSON, err := decodeSegment(parts[0])
	if err != nil {
		return claims, errNotOurToken
	}
	var head tokenHeader
	if err := json.Unmarshal(headerJSON, &head); err != nil {
		return claims, errNotOurToken
	}
	// Refusing any algorithm other than the one we issue stops a token being
	// downgraded to "none" or to an unsigned form.
	if head.Algorithm != TokenAlgorithm {
		return claims, errNotOurToken
	}

	provided, err := decodeSegment(parts[2])
	if err != nil {
		return claims, errNotOurToken
	}
	if !hmac.Equal(signSegment(parts[0]+"."+parts[1], v.Secret), provided) {
		return claims, ErrRejected
	}

	payloadJSON, err := decodeSegment(parts[1])
	if err != nil {
		return claims, errNotOurToken
	}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return claims, errNotOurToken
	}

	if claims.ExpiresAt > 0 && v.now().Unix() >= claims.ExpiresAt {
		return claims, ErrRejected
	}
	if v.Audience != "" && claims.Audience != "" && claims.Audience != v.Audience {
		return claims, ErrRejected
	}
	return claims, nil
}

// errNotOurToken marks a credential this mechanism does not handle, so a chain
// moves on rather than refusing outright.
var errNotOurToken = errors.New("auth: not a signed token")

// TokenValid reports whether a token verifies right now.
func TokenValid(secret []byte, token string, audience string) bool {
	_, err := (TokenVerifier{Secret: secret, Audience: audience}).VerifyToken(token)
	return err == nil
}

/* -------------------------------------------------------------------------- */
/*  Secret                                                                    */
/* -------------------------------------------------------------------------- */

// LoadOrCreateSecret reads the signing secret, generating one on first use.
//
// The secret is hex encoded on disk and the encoded text is what signs and
// verifies, so a restart reads back exactly the value that was used before.
// The file is written owner-only because the secret can mint tokens.
func LoadOrCreateSecret(path string) ([]byte, error) {
	secret, err := readSecret(path)
	if err == nil {
		return secret, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	encoded := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return nil, err
	}
	return []byte(encoded), nil
}

// readSecret reads an existing secret, treating an empty or blank file as
// absent so it is regenerated rather than signing with nothing.
func readSecret(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	secret := []byte(strings.TrimSpace(string(data)))
	if len(secret) == 0 {
		return nil, os.ErrNotExist
	}
	return secret, nil
}

/* -------------------------------------------------------------------------- */
/*  Authorization header                                                      */
/* -------------------------------------------------------------------------- */

// BearerPrefix is the Authorization scheme a credential travels under.
const BearerPrefix = "Bearer "

// FromAuthorization extracts a credential from an Authorization header value.
func FromAuthorization(header string) string {
	trimmed := strings.TrimSpace(header)
	if len(trimmed) >= len(BearerPrefix) && strings.EqualFold(trimmed[:len(BearerPrefix)], BearerPrefix) {
		return strings.TrimSpace(trimmed[len(BearerPrefix):])
	}
	return trimmed
}

// Authorization renders a header value for a credential.
func Authorization(credential string) string {
	if credential == "" {
		return ""
	}
	return BearerPrefix + credential
}

// String renders a claim set for display.
func (c TokenClaims) String() string {
	return fmt.Sprintf("%s (id %s, expires %s)",
		c.Subject, shortID(c.TokenID), time.Unix(c.ExpiresAt, 0).Format("2006-01-02"))
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
