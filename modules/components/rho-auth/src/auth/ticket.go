//
// @file ticket.go
// @brief Short-lived SSO tickets and the keys that sign them.
//
// A ticket is an ES256 JWT. The keeper holds the private key and mints one for
// an application; the application holds only the public key and verifies it
// locally. The two halves are here so the keeper and a resource server cannot
// disagree about what a ticket is.
//
// SPDX-License-Identifier: Apache-2.0
//

package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// KeyPair is one installation's signing key and the public half that is copied
// to every resource server.
type KeyPair struct {
	Private *ecdsa.PrivateKey
	Public  *ecdsa.PublicKey
	KeyID   string
}

// TicketClaims are what a login ticket states. The fields are the ones a
// resource server must check before it trusts the ticket: who, for which app,
// issued by whom, for how long, and the one-use identifier.
type TicketClaims struct {
	Subject     string `json:"sub"`
	Issuer      string `json:"iss"`
	Audience    string `json:"aud"`
	TokenID     string `json:"jti"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	Nonce       string `json:"nonce,omitempty"`
	// SessionID is the issuer's session the ticket was minted from. An
	// application keeps it with its own session and asks the issuer whether it
	// still stands, so signing out at the issuer signs out everywhere.
	SessionID string   `json:"sid,omitempty"`
	Scopes    []string `json:"scp,omitempty"`
	IssuedAt  int64    `json:"iat"`
	NotBefore int64    `json:"nbf,omitempty"`
	ExpiresAt int64    `json:"exp"`
}

// GenerateKeyPair creates a P-256 key. The curve is named rather than
// configurable: a short list is easier to audit than a menu.
func GenerateKeyPair() (*KeyPair, error) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	public := &private.PublicKey
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(der)
	return &KeyPair{
		Private: private,
		Public:  public,
		KeyID:   base64.RawURLEncoding.EncodeToString(sum[:12]),
	}, nil
}

// WriteKeyPair writes the private key owner-only and the public key readable.
// A private key that another user can read is not a private key.
func WriteKeyPair(dir string, pair *KeyPair, force bool) (string, string, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		dir = filepath.Join(home, ".rho", "auth")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	privatePath := filepath.Join(dir, "private.pem")
	publicPath := filepath.Join(dir, "public.pem")
	if !force {
		if _, err := os.Stat(privatePath); err == nil {
			return privatePath, publicPath, fmt.Errorf("%s already exists (use --force to replace it)", privatePath)
		}
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(pair.Private)
	if err != nil {
		return "", "", err
	}
	publicDER, err := x509.MarshalPKIXPublicKey(pair.Public)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(privatePath, pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY", Bytes: privateDER,
	}), 0o600); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{
		Type: "PUBLIC KEY", Bytes: publicDER,
	}), 0o644); err != nil {
		return "", "", err
	}
	return privatePath, publicPath, nil
}

// LoadPrivateKey reads a PKCS8 PEM private key.
func LoadPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not ECDSA")
	}
	return key, nil
}

// LoadPublicKey reads a PKIX PEM public key.
func LoadPublicKey(path string) (*ecdsa.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("public key is not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not ECDSA")
	}
	return key, nil
}

// SignTicket mints an ES256 JWT. The signature is the raw 64-byte R||S form
// JWT requires, not the ASN.1 form ecdsa.Sign returns.
func SignTicket(private *ecdsa.PrivateKey, claims TicketClaims) (string, error) {
	if private == nil {
		return "", errors.New("no signing key")
	}
	header := map[string]string{"alg": "ES256", "typ": "JWT"}
	der, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err == nil {
		sum := sha256.Sum256(der)
		header["kid"] = base64.RawURLEncoding.EncodeToString(sum[:12])
	}
	head, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := base64.RawURLEncoding.EncodeToString(head) + "." +
		base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, private, digest[:])
	if err != nil {
		return "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// VerifyTicket checks the signature and the claims a resource server cares
// about. It does not decide whether a ticket has been used; that is the
// application's one-use cache.
func VerifyTicket(public *ecdsa.PublicKey, token, issuer, audience string, now time.Time) (TicketClaims, error) {
	var claims TicketClaims
	parts := splitThree(token)
	if parts == nil {
		return claims, errors.New("ticket is not a JWT")
	}
	head, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return claims, err
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if json.Unmarshal(head, &header) != nil || header.Algorithm != "ES256" {
		return claims, errors.New("ticket is not ES256")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return claims, errors.New("ticket signature is malformed")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(public, digest[:], r, s) {
		return claims, errors.New("ticket signature is not valid")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(body, &claims) != nil {
		return claims, errors.New("ticket claims are malformed")
	}
	if issuer != "" && claims.Issuer != issuer {
		return claims, errors.New("ticket issuer is not the configured one")
	}
	if audience != "" && claims.Audience != audience {
		return claims, errors.New("ticket audience is not this application")
	}
	if claims.ExpiresAt == 0 || !now.Before(time.Unix(claims.ExpiresAt, 0)) {
		return claims, errors.New("ticket has expired")
	}
	if claims.NotBefore > 0 && now.Before(time.Unix(claims.NotBefore, 0)) {
		return claims, errors.New("ticket is not valid yet")
	}
	return claims, nil
}

func splitThree(token string) []string {
	first := -1
	second := -1
	for i := 0; i < len(token); i++ {
		if token[i] != '.' {
			continue
		}
		if first < 0 {
			first = i
		} else if second < 0 {
			second = i
		} else {
			return nil
		}
	}
	if first < 0 || second < 0 {
		return nil
	}
	return []string{token[:first], token[first+1 : second], token[second+1:]}
}
