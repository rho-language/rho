//
// @file emit_go_auth.go
// @brief Authentication for generated Go server targets.
//
// A server target needs to authenticate a request, so the backend generates
// the mechanisms. The token scheme is generated rather than imported, because a
// generated project has no dependency on the Rho source tree.
//
// The structure is a chain, so a service can accept more than one kind of
// credential: a second mechanism is a case, and nothing else changes. The
// mechanism present today is the signed token.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: MIT
//

package codegen

import (
	"strings"
)

// AuthFileName is the generated file holding the authentication support.
const AuthFileName = "auth.go"

// EmitGoAuth renders the authentication support for a server target.
//
// Whether a credential is required is decided at run time by whether a signing
// secret exists, so a deployment that adds one is closed without a rebuild.
func EmitGoAuth() string {
	var b strings.Builder
	b.WriteString(goBanner(irModuleForBanner(), AuthFileName, "Generated Rho authentication support."))
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"crypto/hmac\"\n")
	b.WriteString("\t\"crypto/sha256\"\n")
	b.WriteString("\t\"encoding/base64\"\n")
	b.WriteString("\t\"encoding/hex\"\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"errors\"\n")
	b.WriteString("\t\"crypto/rand\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\t\"path/filepath\"\n")
	b.WriteString("\t\"strings\"\n")
	b.WriteString("\t\"time\"\n")
	b.WriteString(")\n\n")
	b.WriteString(goAuthTypes())
	b.WriteString(goAuthPrincipal())
	b.WriteString(goAuthVerifier())
	b.WriteString(goAuthSecret())
	b.WriteString(goAuthAPI())
	return b.String()
}

// goAuthTypes emits the claims a token carries.
func goAuthTypes() string {
	return `// authTokenAlgorithm is the only algorithm accepted, so a token cannot be
// presented as unsigned or downgraded to something weaker.
const authTokenAlgorithm = "HS256"

// authMaxTokenLifetime bounds how long an issued token may last.
const authMaxTokenLifetime = 90 * 24 * time.Hour

// authDefaultTokenLifetime is used when a caller does not choose.
const authDefaultTokenLifetime = 30 * 24 * time.Hour

// authTokenHeader is a token's first segment.
type authTokenHeader struct {
	Algorithm string ` + "`json:\"alg\"`" + `
	Type      string ` + "`json:\"typ\"`" + `
}

// authClaims are the statements a token carries.
type authClaims struct {
	Subject   string   ` + "`json:\"sub\"`" + `
	TokenID   string   ` + "`json:\"jti\"`" + `
	Issuer    string   ` + "`json:\"iss\"`" + `
	Audience  string   ` + "`json:\"aud\"`" + `
	Host      string   ` + "`json:\"host,omitempty\"`" + `
	Scopes    []string ` + "`json:\"scp,omitempty\"`" + `
	IssuedAt  int64    ` + "`json:\"iat\"`" + `
	ExpiresAt int64    ` + "`json:\"exp\"`" + `
}

func encodeAuthSegment(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeAuthSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}

func signAuthSegment(message string, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

`
}

// goAuthPrincipal emits the shared identity type.
func goAuthPrincipal() string {
	return `// authPrincipal is an authenticated identity: who is acting, by which
// mechanism, and until when.
type authPrincipal struct {
	Subject   string
	Method    string
	TokenID   string
	Host      string
	Expires   time.Time
	Scopes    []string
	Anonymous bool
}

// HasScope reports whether the principal may do something. A credential with
// no scopes has none rather than all of them.
func (p authPrincipal) HasScope(scope string) bool {
	for _, held := range p.Scopes {
		if held == scope || held == "*" {
			return true
		}
	}
	return false
}

// authAnonymous is the principal for an unauthenticated request.
var authAnonymous = authPrincipal{Anonymous: true, Subject: "anonymous", Method: "none"}

`
}

// goAuthVerifier emits the signed-token mechanism, shaped so another mechanism
// can be added beside it.
func goAuthVerifier() string {
	return `// authTokenVerifier authenticates signed tokens.
type authTokenVerifier struct {
	secret   []byte
	audience string
}

func (v authTokenVerifier) name() string { return "token" }

// verify reports whether a credential is one this mechanism handles, and
// whether it is accepted.
func (v authTokenVerifier) verify(credential string) (authPrincipal, bool, bool) {
	parts := strings.Split(strings.TrimSpace(credential), ".")
	if len(parts) != 3 {
		return authAnonymous, false, false
	}
	headerJSON, err := decodeAuthSegment(parts[0])
	if err != nil {
		return authAnonymous, false, false
	}
	var head authTokenHeader
	if err := json.Unmarshal(headerJSON, &head); err != nil {
		return authAnonymous, false, false
	}
	if head.Algorithm != authTokenAlgorithm {
		return authAnonymous, false, false
	}
	if len(v.secret) == 0 {
		return authAnonymous, true, false
	}
	provided, err := decodeAuthSegment(parts[2])
	if err != nil {
		return authAnonymous, false, false
	}
	if !hmac.Equal(signAuthSegment(parts[0]+"."+parts[1], v.secret), provided) {
		return authAnonymous, true, false
	}
	payloadJSON, err := decodeAuthSegment(parts[1])
	if err != nil {
		return authAnonymous, false, false
	}
	var claims authClaims
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return authAnonymous, false, false
	}
	if claims.ExpiresAt > 0 && time.Now().Unix() >= claims.ExpiresAt {
		return authAnonymous, true, false
	}
	if v.audience != "" && claims.Audience != "" && claims.Audience != v.audience {
		return authAnonymous, true, false
	}
	principal := authPrincipal{
		Subject: claims.Subject,
		Method:  v.name(),
		TokenID: claims.TokenID,
		Host:    claims.Host,
		Scopes:  claims.Scopes,
	}
	if claims.ExpiresAt > 0 {
		principal.Expires = time.Unix(claims.ExpiresAt, 0)
	}
	return principal, true, true
}

// authChain tries the configured mechanisms in turn. Adding one is a case here.
type authChain struct {
	verifiers []authTokenVerifier
}

func newAuthChain(secret []byte, audience string) *authChain {
	return &authChain{verifiers: []authTokenVerifier{{secret: secret, audience: audience}}}
}

// authenticate returns the first principal that verifies. handled reports
// whether any mechanism recognised the credential, which separates a wrong
// credential from one nothing recognised.
func (c *authChain) authenticate(credential string) (authPrincipal, bool) {
	if strings.TrimSpace(credential) == "" {
		return authAnonymous, true
	}
	for _, verifier := range c.verifiers {
		if principal, handled, ok := verifier.verify(credential); handled {
			if !ok {
				return authAnonymous, false
			}
			return principal, true
		}
	}
	return authAnonymous, false
}

`
}

// goAuthSecret emits secret loading and token issuing.
func goAuthSecret() string {
	return `// authSecretPath is where the signing secret lives. A registry creates one on
// first use, and a client never reads it.
func authSecretPath() string {
	if override := os.Getenv("RHO_AUTH_SECRET_FILE"); override != "" {
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".rho", "auth", "signing.key")
}

// authLoadSecret reads the signing secret, generating one on first use. The
// file is written owner-only because the secret can mint tokens.
func authLoadSecret() []byte {
	path := authSecretPath()
	if path == "" {
		return nil
	}
	if data, err := os.ReadFile(path); err == nil {
		if secret := []byte(strings.TrimSpace(string(data))); len(secret) > 0 {
			return secret
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil
	}
	encoded := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil
	}
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		return nil
	}
	return []byte(encoded)
}

// authIssueToken mints a token. The lifetime is clamped, so a caller cannot
// ask for one that never expires.
func authIssueToken(secret []byte, subject, audience, host string, scopes []string, lifetime time.Duration) (string, authClaims, error) {
	var claims authClaims
	if len(secret) == 0 {
		return "", claims, errors.New("no signing secret is configured")
	}
	if strings.TrimSpace(subject) == "" {
		return "", claims, errors.New("a token needs a subject")
	}
	if lifetime <= 0 {
		lifetime = authDefaultTokenLifetime
	}
	if lifetime > authMaxTokenLifetime {
		lifetime = authMaxTokenLifetime
	}
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", claims, err
	}
	now := time.Now()
	claims = authClaims{
		Subject:   subject,
		TokenID:   hex.EncodeToString(buffer),
		Issuer:    "rho-registry",
		Audience:  audience,
		Host:      host,
		Scopes:    scopes,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(lifetime).Unix(),
	}
	headerJSON, err := json.Marshal(authTokenHeader{Algorithm: authTokenAlgorithm, Type: "JWT"})
	if err != nil {
		return "", claims, err
	}
	payloadJSON, err := json.Marshal(claims)
	if err != nil {
		return "", claims, err
	}
	signing := encodeAuthSegment(headerJSON) + "." + encodeAuthSegment(payloadJSON)
	return signing + "." + encodeAuthSegment(signAuthSegment(signing, secret)), claims, nil
}

`
}

// goAuthAPI emits what a Rho handler calls.
func goAuthAPI() string {
	return `// authBearer is the Go spelling a Rho handler reaches.
func authBearer(header string) string { return auth_bearer(header) }

// authSubject is the Go spelling a Rho handler reaches.
func authSubject(header string) string { return auth_subject(header) }

// authIsAuthenticated is the Go spelling a Rho handler reaches.
func authIsAuthenticated(header string) bool { return auth_is_authenticated(header) }

// auth_bearer returns the credential from an Authorization header value.
func auth_bearer(header string) string {
	const prefix = "Bearer "
	trimmed := strings.TrimSpace(header)
	if len(trimmed) >= len(prefix) && strings.EqualFold(trimmed[:len(prefix)], prefix) {
		return strings.TrimSpace(trimmed[len(prefix):])
	}
	return trimmed
}

// authenticate resolves the credential presented with a request.
func authenticate(header string) (authPrincipal, bool) {
	chain := newAuthChain(authLoadSecret(), authAudience())
	return chain.authenticate(auth_bearer(header))
}

// authAudience is the registry a token must be for. It is derived from the
// address the service is reached at, so a token minted for one deployment is
// not accepted by another.
func authAudience() string {
	return os.Getenv("RHO_REGISTRY_AUDIENCE")
}

// auth_subject is the identity behind a request, or "" when anonymous.
func auth_subject(header string) string {
	principal, _ := authenticate(header)
	if principal.Anonymous {
		return ""
	}
	return principal.Subject
}

// auth_is_authenticated reports whether a request carried a valid credential.
func auth_is_authenticated(header string) bool {
	principal, _ := authenticate(header)
	return !principal.Anonymous
}

// auth_required reports whether the service refuses unauthenticated writes.
//
// This is configuration rather than a side effect of the secret existing,
// because the secret is created on first use: deriving it from the secret
// would close the registry before anyone could log in, and there would be no
// way to get the first token.
func auth_required() bool {
	return os.Getenv("RHO_REGISTRY_REQUIRE_AUTH") == "1"
}

// auth_open_tokens reports whether the service will issue a token to a caller
// that presents no credential. It is open by default so the first token can be
// obtained, and can be closed once tokens are in place.
func auth_open_tokens() bool {
	return os.Getenv("RHO_REGISTRY_OPEN_TOKENS") != "0"
}

// authRequired is the Go spelling a Rho handler reaches.
func authRequired() bool { return auth_required() }

// authOpenTokens is the Go spelling a Rho handler reaches.
func authOpenTokens() bool { return auth_open_tokens() }

// authIssue is the Go spelling a Rho handler reaches.
func authIssue(user, host string, days uint32, scopes string) string {
	return auth_issue(user, host, days, scopes)
}

// auth_issue mints a token and returns the JSON body a client expects, so the
// response shape is built once, in Go, rather than assembled in Rho.
func auth_issue(user, host string, days uint32, scopes string) string {
	lifetime := time.Duration(days) * 24 * time.Hour
	if lifetime <= 0 {
		lifetime = authDefaultTokenLifetime
	}
	if lifetime > authMaxTokenLifetime {
		lifetime = authMaxTokenLifetime
	}
	wanted := []string{}
	for _, scope := range strings.Fields(scopes) {
		wanted = append(wanted, scope)
	}
	if len(wanted) == 0 {
		wanted = []string{"pull"}
	}
	secret := authLoadSecret()
	token, claims, err := authIssueToken(secret, user, authAudience(), host, wanted, lifetime)
	if err != nil {
		errorBody, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(errorBody)
	}
	answer := map[string]any{
		"token": token,
		"sub":   claims.Subject,
		"jti":   claims.TokenID,
		"iss":   claims.Issuer,
		"aud":   claims.Audience,
		"host":  claims.Host,
		"scp":   claims.Scopes,
		"iat":   claims.IssuedAt,
		"exp":   claims.ExpiresAt,
	}
	body, _ := json.Marshal(answer)
	return string(body)
}

// authExpires is the Go spelling a Rho handler reaches.
func authExpires(header string) uint32 { return auth_expires(header) }

// auth_expires is the Unix time a presented credential runs out, or 0.
func auth_expires(header string) uint32 {
	principal, _ := authenticate(header)
	if principal.Expires.IsZero() {
		return 0
	}
	return uint32(principal.Expires.Unix())
}

`
}
