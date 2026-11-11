//
// @file emit_go_middleware.go
// @brief The middleware the routing port supplies.
//
// Every wrapper here is a `func(http.Handler) http.Handler`, which is the shape
// the Go standard library composes and the shape any other wrapper can take. A
// project that wants something the port does not supply writes a function with
// that signature and names it in the same list; there is nothing else to learn
// and no other registration to make.
//
// Each wrapper is emitted whole rather than configured at run time, because a
// value a compiler can see is a value a reader can follow. The settings come
// from the target's descriptor and are written into the source, so a generated
// project shows which origin it allows and which limit it enforces without
// running anything to find out.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package codegen

import (
	"fmt"
	"strings"
)

/* -------------------------------------------------------------------------- */
/*  Request identity                                                           */
/* -------------------------------------------------------------------------- */

// emitRequestIDMiddleware renders the request-id wrapper.
//
// A request that cannot be named is a request nobody can correlate. The
// wrapper takes the identifier the client sent, mints one when there is none,
// echoes it back, and puts it where a log line and a client report can both
// reach it.
func emitRequestIDMiddleware(name string, spec MiddlewareSpec) string {
	header := option(spec, "header", "X-Request-Id")
	var b strings.Builder
	fmt.Fprintf(&b, `// requestIDHeader is the header a request's identity travels in, on the way
// in so a client can pin it and on the way out so the client can quote it.
const requestIDHeader = %q

func %[2]s(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDContextKey is where a request's identity lives for the handlers
// behind it, so a log line written deeper in the pipeline names the same
// request the client quoted.
type requestIDContextKey struct{}

// newRequestID mints an identifier for a request that arrived without one.
// 128 random bits, hex encoded, which is short enough to read in a log and long
// enough that two requests will not collide.
func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(raw[:])
}
`, header, name)
	return b.String()
}

// RequestIDContextKey is the key a request's identity travels under, for a
// wrapper a project writes that wants to log it.
const RequestIDContextKey = "rho-router/request-id"

/* -------------------------------------------------------------------------- */
/*  Cross-origin                                                                */
/* -------------------------------------------------------------------------- */

// emitCORSMiddleware renders the cross-origin wrapper.
//
// A browser will not hand a response to a page on another origin unless the
// server says so, and it will not send the real request at all until the
// preflight is answered. Both answers come from the same settings, so what a
// developer allows in one place is what is allowed in the other.
func emitCORSMiddleware(name string, spec MiddlewareSpec) string {
	origin := option(spec, "origin", "*")
	methods := option(spec, "methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
	headers := option(spec, "headers", "Authorization, Content-Type, X-Request-Id")
	maxAge := option(spec, "max_age", "600")

	var b strings.Builder
	fmt.Fprintf(&b, `// The cross-origin answers, written from the target's descriptor so a
// generated project states what it allows rather than implying it.
const (
	corsOrigin  = %q
	corsMethods = %q
	corsHeaders = %q
	corsMaxAge  = %q
)

func %[5]s(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("Access-Control-Allow-Origin", corsOrigin)
		header.Set("Vary", "Origin")
		if corsOrigin != "*" {
			// A specific origin is echoed back rather than answered with a
			// wildcard, because a browser refuses a credentialed request
			// answered with one.
			if request := r.Header.Get("Origin"); request != "" {
				header.Set("Access-Control-Allow-Origin", request)
				header.Add("Vary", "Access-Control-Request-Headers")
				header.Add("Vary", "Access-Control-Request-Method")
			}
		}
		header.Set("Access-Control-Expose-Headers", "X-Request-Id")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			header.Set("Access-Control-Allow-Methods", corsMethods)
			header.Set("Access-Control-Allow-Headers", corsHeaders)
			header.Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
`, origin, methods, headers, maxAge, name)
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Rate limiting                                                               */
/* -------------------------------------------------------------------------- */

// emitRateLimitMiddleware renders the rate-limit wrapper.
//
// The limit is a fixed window per client, counted in a map rather than by a
// token bucket: a fixed window is a counter, and a counter is something a
// reader can check by hand. The cost is the window's edge, where a client can
// spend two windows' worth of calls a second apart, which is the trade a fixed
// window makes in exchange for being obvious.
//
// Clients are told apart by address behind a proxy, so the wrapper answers 429
// with the header that says when to come back.
func emitRateLimitMiddleware(name string, spec MiddlewareSpec) string {
	// The settings as the descriptor wrote them, and the defaults they fall
	// back to. Both are named, so a reader of the generated project can see
	// what the service enforces without running it.
	requests := option(spec, "requests", "100")
	window := option(spec, "window", "1m")
	header := option(spec, "header", "X-RateLimit-Limit")

	var b strings.Builder
	fmt.Fprintf(&b, `// The settings, written from the target's descriptor as the strings it
// was given. They are resolved below rather than written straight into the code
// as literals: a settings map is free text, and "requests: yes" is a
// descriptor a person can write and a Go source a compiler must not be handed.
const (
	rateLimitSetting       = %[1]q
	rateLimitWindowSetting = %[2]q
	rateLimitHeader        = %[3]q
)

// The resolved limit. A setting that is not a count or a duration falls back to
// the default rather than to a limit the service cannot honour, so a mistake in
// a descriptor costs a default rather than a service that refuses every
// request.
var (
	rateLimitRequests = rhoCount(rateLimitSetting, %[5]d)
	rateLimitWindow   = rhoWindow(rateLimitWindowSetting, %[6]q)
)

// rhoCount resolves a count a descriptor wrote as text.
func rhoCount(setting string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(setting))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// rhoWindow resolves a duration, written as text on both sides so the fallback
// and the setting are read the same way.
func rhoWindow(setting, fallback string) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(setting))
	if err != nil || value <= 0 {
		if minutes, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(fallback), "m")); err == nil {
			return time.Duration(minutes) * time.Minute
		}
		return time.Minute
	}
	return value
}

// rateLimitCounters is one window's tally per client, held under a lock
// because a service handles requests concurrently.
var (
	rateLimitCounters = map[string]*rateLimitCounter{}
	rateLimitLock     sync.Mutex
)

// rateLimitCounter is one client's tally in the current window.
type rateLimitCounter struct {
	count   int
	expires time.Time
}

// rateLimitAllow reports whether a client may make one more call, and how long
// it should wait when it may not.
func rateLimitAllow(client string) (bool, time.Duration) {
	now := time.Now()
	rateLimitLock.Lock()
	defer rateLimitLock.Unlock()

	// A map that only grows is a map a flood fills. The expired windows are
	// dropped once the map is large enough to be worth walking, which bounds
	// what a stream of distinct clients can cost the service.
	if len(rateLimitCounters) > 4096 {
		for key, counter := range rateLimitCounters {
			if now.After(counter.expires) {
				delete(rateLimitCounters, key)
			}
		}
	}

	counter, seen := rateLimitCounters[client]
	if !seen || now.After(counter.expires) {
		rateLimitCounters[client] = &rateLimitCounter{count: 1, expires: now.Add(rateLimitWindow)}
		return true, 0
	}
	if counter.count >= rateLimitRequests {
		return false, time.Until(counter.expires)
	}
	counter.count++
	return true, 0
}

func %[4]s(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, retry := rateLimitAllow(rateLimitClient(r))
		if !allowed {
			seconds := int(retry.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		w.Header().Set(rateLimitHeader, strconv.Itoa(rateLimitRequests))
		next.ServeHTTP(w, r)
	})
}

// rateLimitClient names the caller the limit is counted against.
//
// Behind a proxy the connection address is the proxy's, so it is the last
// resort; the proxy's own account of the client is used first. X-Real-IP is
// preferred because a proxy sets it to the connection's address and overwrites
// whatever the client sent. X-Forwarded-For is read at its *rightmost* entry,
// which is the one the nearest proxy appended: a caller can put anything in the
// left of that header, and counting by a value the caller chooses counts every
// request as a new client.
func rateLimitClient(r *http.Request) string {
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		return real
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if index := strings.LastIndexByte(forwarded, ','); index >= 0 {
			return strings.TrimSpace(forwarded[index+1:])
		}
		return strings.TrimSpace(forwarded)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
`, requests, window, header, name, defaultRateLimitRequests, defaultRateLimitWindow)
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Bearer tokens                                                               */
/* -------------------------------------------------------------------------- */

// emitJWTMiddleware renders the authentication wrapper.
//
// A token is checked with the standard library: HMAC over SHA-256, base64url
// for the segments, and a signature compared in constant time. Nothing here
// decodes the claims into a Go value, because decoding into a value is what
// reflection is for; the claims are read from the bytes by the same scanner
// the parameter binding uses, and a claim the wrapper does not understand is a
// claim it simply does not read.
//
// The secret is named, not written: a signing key in a source file is a signing
// key in every clone of the file.
func emitJWTMiddleware(name string, spec MiddlewareSpec) string {
	secretEnv := option(spec, "secret_env", "RHO_JWT_SECRET")
	issuer := option(spec, "issuer", "")
	audience := option(spec, "audience", "")
	header := option(spec, "header", "Authorization")
	scheme := option(spec, "scheme", "Bearer")
	skip := option(spec, "skip_paths", "")

	var b strings.Builder
	fmt.Fprintf(&b, `// How a token is checked, written from the target's descriptor.
const (
	jwtSecretEnv = %q
	jwtIssuer    = %q
	jwtAudience  = %q
	jwtHeader    = %q
	jwtScheme    = %q
)

// jwtSkipPaths are the paths served without a token, which is how a health
// probe and a sign-in endpoint stay reachable.
var jwtSkipPaths = splitPaths(%q)

// jwtClockSkew is how far a token's clock may differ before it is refused. A
// small allowance covers the drift between two honest machines; anything larger
// is a window an attacker would use.
const jwtClockSkew = 60 * time.Second

func %[7]s(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isJWTExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		token := bearerToken(r.Header.Get(jwtHeader), jwtScheme)
		if token == "" {
			unauthorized(w, "a token is required")
			return
		}
		claims, ok := verifyJWT(token, jwtSecret())
		if !ok {
			unauthorized(w, "the token is not valid")
			return
		}
		if !jwtClaimsAccept(claims) {
			unauthorized(w, "the token is not valid")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), jwtClaimsKey{}, claims)))
	})
}

// jwtClaimsKey is where the verified claims live for the handlers behind the
// wrapper, so a handler reads the subject rather than reparsing the token.
type jwtClaimsKey struct{}

// jwtSecret returns the signing key, or "" when none is configured. A service
// with no key refuses every token rather than accepting every token, which is
// the failure that cannot be discovered in production.
func jwtSecret() string { return os.Getenv(jwtSecretEnv) }

// isJWTExempt reports whether a path is served without a token.
func isJWTExempt(path string) bool {
	for _, exempt := range jwtSkipPaths {
		if exempt == path {
			return true
		}
	}
	return false
}

// bearerToken returns the token of an Authorization-style header.
func bearerToken(header, scheme string) string {
	if header == "" {
		return ""
	}
	value, _, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(value, scheme) {
		return ""
	}
	return strings.TrimSpace(header[len(value)+1:])
}

// unauthorized answers a request the wrapper refused.
func unauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", jwtScheme)
	http.Error(w, message, http.StatusUnauthorized)
}

// verifyJWT checks a token's signature and reads its claims.
//
// The signature is the part that decides whether the token is genuine; the
// claims are only read once it is. A malformed token is refused rather than
// parsed leniently, because a parser that guesses is a parser with a bug in
// it.
func verifyJWT(token, secret string) (map[string]string, bool) {
	if secret == "" {
		return nil, false
	}
	first, rest, found := strings.Cut(token, ".")
	if !found {
		return nil, false
	}
	claimPart, signaturePart, found := strings.Cut(rest, ".")
	if !found {
		return nil, false
	}
	if first != "HS256" {
		// One algorithm, named in the header and compared as a string. A token
		// asking for another algorithm, including none, is refused before its
		// signature is read at all.
		return nil, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(first + "." + claimPart))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(claimPart)
	if err != nil {
		return nil, false
	}
	claims, ok := rhoJSONObject(string(raw))
	return claims, ok
}

// jwtClaimsAccept reports whether verified claims satisfy what the target
// declared: an issuer when one was named, an audience when one was named, and
// an expiry whenever the token carries one.
func jwtClaimsAccept(claims map[string]string) bool {
	if jwtIssuer != "" && claims["iss"] != jwtIssuer {
		return false
	}
	if jwtAudience != "" && claims["aud"] != jwtAudience {
		return false
	}
	if expiry, present := claims["exp"]; present {
		seconds, err := strconv.ParseInt(expiry, 10, 64)
		if err != nil {
			return false
		}
		if time.Now().After(time.Unix(seconds, 0).Add(jwtClockSkew)) {
			return false
		}
	}
	return true
}

// splitPaths reads a comma-separated path list, dropping the empty entries a
// trailing separator leaves behind.
func splitPaths(list string) []string {
	out := []string{}
	for _, path := range strings.Split(list, ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}
`, secretEnv, issuer, audience, header, scheme, skip, name)
	return b.String()
}

/* -------------------------------------------------------------------------- */
/*  Cookie sessions                                                            */
/* -------------------------------------------------------------------------- */

// emitAuthMiddleware renders the SSO session wrapper.
//
// It verifies an ES256 session token from a cookie against a pinned public
// key. A request with no valid session is sent to the provider's login URL for
// a browser, and answered with 401 for an API caller. The wrapper knows both
// deployments from the start: a direct WAN address, where the origin is the
// request's own host, and a reverse proxy, where the public origin and base
// path are configured and forwarded headers are trusted only when asked for.
func emitAuthMiddleware(name string, spec MiddlewareSpec) string {
	issuer := option(spec, "issuer", "")
	audience := option(spec, "audience", "")
	// A cookie is scoped by host, not by port, so two services on one host that
	// shared a cookie name would overwrite each other's session. The default is
	// named for the audience, which is what makes each service's session its own.
	cookie := option(spec, "cookie", "")
	if cookie == "" {
		cookie = "rho_session"
		if audience != "" {
			cookie += "_" + cookieToken(audience)
		}
	}
	publicKey := option(spec, "public_key", "")
	mode := option(spec, "mode", "direct")
	publicURL := option(spec, "public_url", "")
	basePath := option(spec, "base_path", "")
	loginURL := option(spec, "login_url", "/auth/login")
	keeperPort := option(spec, "keeper_port", "19090")
	returnParam := option(spec, "return_param", "return_to")
	skip := option(spec, "skip_paths", "/healthz,/auth/login,/auth/callback,/static")
	trustForwarded := option(spec, "trust_forwarded", "false") == "true"
	scheme := option(spec, "scheme", "auto")
	sessionFile := option(spec, "session_file", "")
	scopes := option(spec, "scopes", "")

	var b strings.Builder
	fmt.Fprintf(&b, `// The session this service accepts, written from the target's descriptor.
// The public key is the only secret material here: the signing key never
// leaves the authorization server.
const (
	authSsoIssuer       = %q
	authSsoAudience     = %q
	authSsoCookie       = %q
	authSsoPublicKeyPath = %q
	authSsoMode         = %q
	authSsoPublicURL    = %q
	authSsoBasePath     = %q
	authSsoLoginURL     = %q
	authSsoKeeperPort   = %q
	authSsoReturnParam  = %q
	authSsoTrustForwarded = %t
	authSsoScheme       = %q
	authSsoSessionFile  = %q
)

var authSsoSkipPaths = authSsoSplitPaths(%q)
var authSsoRequireScopes = authSsoSplitPaths(%q)

var (
	authSsoKeyOnce sync.Once
	authSsoKey     *ecdsa.PublicKey
	authSsoKeyErr  error
)

type authSsoSessionClaims struct {
	Subject   string   `+"`json:\"sub\"`"+`
	Issuer    string   `+"`json:\"iss\"`"+`
	Audience  string   `+"`json:\"aud\"`"+`
	TokenID   string   `+"`json:\"jti\"`"+`
	Scopes    []string `+"`json:\"scp,omitempty\"`"+`
	IssuedAt  int64    `+"`json:\"iat\"`"+`
	NotBefore int64    `+"`json:\"nbf,omitempty\"`"+`
	ExpiresAt int64    `+"`json:\"exp\"`"+`
}

type authSsoSessionKey struct{}

func authSsoLoadKey() (*ecdsa.PublicKey, error) {
	authSsoKeyOnce.Do(func() {
		path := os.Getenv("RHO_AUTH_PUBLIC_KEY")
		if path == "" {
			path = authSsoPublicKeyPath
		}
		if path == "" {
			authSsoKeyErr = errors.New("no auth public key configured")
			return
		}
		if strings.HasPrefix(path, "~/") {
			path = filepath.Join(os.Getenv("HOME"), path[2:])
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			authSsoKeyErr = err
			return
		}
		block, _ := pem.Decode(raw)
		if block == nil {
			authSsoKeyErr = errors.New("auth public key is not PEM")
			return
		}
		var parsed any
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				authSsoKeyErr = err
				return
			}
			parsed = cert.PublicKey
		} else {
			parsed, err = x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				authSsoKeyErr = err
				return
			}
		}
		key, ok := parsed.(*ecdsa.PublicKey)
		if !ok {
			authSsoKeyErr = errors.New("auth public key is not ECDSA")
			return
		}
		authSsoKey = key
	})
	return authSsoKey, authSsoKeyErr
}

func authDecodeSegment(segment string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(segment)
}

func authSsoVerifySession(token string) (authSsoSessionClaims, bool) {
	var claims authSsoSessionClaims
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return claims, false
	}
	headerRaw, err := authDecodeSegment(parts[0])
	if err != nil {
		return claims, false
	}
	var header struct {
		Algorithm string `+"`json:\"alg\"`"+`
		KeyID     string `+"`json:\"kid,omitempty\"`"+`
	}
	if err := jsonenc.Unmarshal(headerRaw, &header); err != nil || header.Algorithm != "ES256" {
		return claims, false
	}
	key, err := authSsoLoadKey()
	if err != nil || key == nil {
		return claims, false
	}
	signature, err := authDecodeSegment(parts[2])
	if err != nil || len(signature) != 64 {
		return claims, false
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return claims, false
	}
	payload, err := authDecodeSegment(parts[1])
	if err != nil || jsonenc.Unmarshal(payload, &claims) != nil {
		return claims, false
	}
	if authSsoRevoked(claims.TokenID) {
		return claims, false
	}
	now := time.Now().Unix()
	if claims.ExpiresAt > 0 && now >= claims.ExpiresAt+60 {
		return claims, false
	}
	if claims.NotBefore > 0 && now+60 < claims.NotBefore {
		return claims, false
	}
	if authSsoIssuer != "" && claims.Issuer != authSsoIssuer {
		return claims, false
	}
	if authSsoAudience != "" && claims.Audience != authSsoAudience {
		return claims, false
	}
	return claims, true
}

// authSsoRevokedPath is the revocation list beside the sessions, the same file
// the keeper's rho auth revoke --jti appends to.
func authSsoRevokedPath() string {
	return filepath.Join(authSsoStateDir(), "revoked.txt")
}

// authSsoRevoked reports whether a token id was revoked. A missing list is an
// empty list, so a service that has never revoked anything stays open.
func authSsoRevoked(jti string) bool {
	if jti == "" {
		return false
	}
	raw, err := os.ReadFile(authSsoRevokedPath())
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == jti {
			return true
		}
	}
	return false
}

func authSsoSplitPaths(list string) []string {
	out := []string{}
	for _, path := range strings.Split(list, ",") {
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return out
}

func authSsoExempt(path string) bool {
	for _, exempt := range authSsoSkipPaths {
		if exempt == path || strings.HasPrefix(path, exempt+"/") {
			return true
		}
	}
	return false
}

func authSsoWantsJSON(r *http.Request) bool {
	if strings.Contains(r.URL.Path, "/api/") {
		return true
	}
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func authSsoReturnTo(r *http.Request) string {
	path := r.URL.RequestURI()
	if authSsoBasePath != "" && !strings.HasPrefix(path, authSsoBasePath+"/") && path != authSsoBasePath {
		path = authSsoBasePath + path
	}
	if authSsoMode == "proxy" && authSsoPublicURL != "" {
		return strings.TrimRight(authSsoPublicURL, "/") + path
	}
	host := r.Host
	proto := "http"
	if r.TLS != nil || authSsoScheme == "https" {
		proto = "https"
	}
	if authSsoTrustForwarded {
		if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
			host = forwarded
		}
		if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
			proto = forwarded
		}
	}
	if authSsoScheme == "http" {
		proto = "http"
	}
	return proto + "://" + host + path
}

func authSsoStateDir() string {
	// RHO_APP_WORKSPACE is the toolchain's name; the name an ecosystem's
	// deployments already set is honoured so none has to change.
	dir := os.Getenv("RHO_APP_WORKSPACE")
	if dir == "" {
		dir = os.Getenv("ROBOHUB_APP_WORKSPACE")
	}
	if dir != "" {
		dir = dir + "/auth"
	} else {
		// Without a workspace each service still keeps its own sessions: a
		// directory shared by every service would be a session one service
		// opened and another accepted.
		name := authSsoAudience
		if name == "" {
			name = "default"
		}
		dir = filepath.Join(os.Getenv("HOME"), ".rho", "auth-app", name)
	}
	_ = os.MkdirAll(dir, 0o700)
	return dir
}

func authSsoSessionPath() string {
	if authSsoSessionFile != "" {
		path := authSsoSessionFile
		if strings.HasPrefix(path, "~/") {
			path = filepath.Join(os.Getenv("HOME"), path[2:])
		}
		return path
	}
	return filepath.Join(authSsoStateDir(), "sessions.txt")
}
func authSsoJtiPath() string { return filepath.Join(authSsoStateDir(), "used-tickets.txt") }

func authSsoNewID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}

// authSsoLookup finds a session this service opened, and the identity it was
// opened for: the subject and the scopes of the ticket that opened it. The
// audience is this service's own, because only this service writes the file.
func authSsoLookup(sid string) (authSsoSessionClaims, bool) {
	var identity authSsoSessionClaims
	if sid == "" {
		return identity, false
	}
	raw, err := os.ReadFile(authSsoSessionPath())
	if err != nil {
		return identity, false
	}
	now := time.Now().Unix()
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) < 2 || parts[0] != sid {
			continue
		}
		exp, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || now >= exp {
			return identity, false
		}
		identity.Subject = "session"
		if len(parts) > 2 && parts[2] != "" {
			identity.Subject = parts[2]
		}
		if len(parts) > 3 && parts[3] != "" {
			identity.Scopes = strings.Split(parts[3], ",")
		}
		identity.Audience = authSsoAudience
		identity.Issuer = authSsoIssuer
		identity.ExpiresAt = exp
		return identity, true
	}
	return identity, false
}

func authSsoCallback(w http.ResponseWriter, r *http.Request) bool {
	path := authSsoBasePath + "/auth/callback"
	if authSsoBasePath == "" {
		path = "/auth/callback"
	}
	if r.URL.Path != path {
		return false
	}
	ticket := r.URL.Query().Get("ticket")
	claims, ok := authSsoVerifySession(ticket)
	if !ok {
		http.Error(w, "invalid ticket", http.StatusUnauthorized)
		return true
	}
	if claims.TokenID != "" {
		used, _ := os.ReadFile(authSsoJtiPath())
		if strings.Contains(string(used), claims.TokenID+"|") {
			http.Error(w, "ticket already used", http.StatusUnauthorized)
			return true
		}
		f, err := os.OpenFile(authSsoJtiPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(claims.TokenID + "|" + claims.Subject + "\n")
			_ = f.Close()
		}
	}
	sid := authSsoNewID()
	if sid == "" {
		http.Error(w, "could not open a session", http.StatusInternalServerError)
		return true
	}
	expires := time.Now().Add(8 * time.Hour).Unix()
	f, err := os.OpenFile(authSsoSessionPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		http.Error(w, "could not open a session", http.StatusInternalServerError)
		return true
	}
	_, _ = f.WriteString(sid + "|" + strconv.FormatInt(expires, 10) + "|" +
		authSsoField(claims.Subject) + "|" + authSsoField(strings.Join(claims.Scopes, ",")) + "\n")
	_ = f.Close()
	cookiePath := authSsoBasePath
	if cookiePath == "" {
		cookiePath = "/"
	}
	http.SetCookie(w, &http.Cookie{Name: authSsoCookie, Value: sid, Path: cookiePath,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: authSsoScheme == "https" || (authSsoScheme == "auto" && r.TLS != nil)})
	returnTo := r.URL.Query().Get("next")
	if parsed, err := url.Parse(returnTo); err == nil && parsed.Host == r.Host {
		returnTo = parsed.RequestURI()
	}
	if !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = authSsoBasePath
		if returnTo == "" {
			returnTo = "/"
		}
	}
	http.Redirect(w, r, returnTo, http.StatusFound)
	return true
}

// authSsoHoldsScopes reports whether the presented scopes satisfy every scope
// the service requires. A principal with no scopes has none, and "*" is a
// wildcard that satisfies any requirement.
func authSsoHoldsScopes(held []string) bool {
	for _, required := range authSsoRequireScopes {
		satisfied := false
		for _, present := range held {
			if present == required || present == "*" {
				satisfied = true
				break
			}
		}
		if !satisfied {
			return false
		}
	}
	return true
}

// authSsoDenyScope answers a request whose verified token did not carry the
// scopes the service requires. An API caller gets JSON; a browser gets a plain
// forbidden page, which is not the login a missing session would cause.
func authSsoDenyScope(w http.ResponseWriter, r *http.Request) {
	if authSsoWantsJSON(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient scope"})
		return
	}
	http.Error(w, "insufficient scope", http.StatusForbidden)
}

// authSsoField keeps a value from breaking the session file's line format.
func authSsoField(value string) string {
	return strings.NewReplacer("|", "", "\n", "", "\r", "").Replace(value)
}

// authSsoIdentityHeaders are set by this middleware from a token or session it
// verified, and by nothing else.
var authSsoIdentityHeaders = []string{"X-Rho-Subject", "X-Rho-Scopes", "X-Rho-Aud"}

func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A client that sends the identity headers itself is claiming to be
		// someone. They are removed before anything reads them, on every path
		// including the exempt ones, so the only values a handler sees are the
		// ones verified here.
		for _, name := range authSsoIdentityHeaders {
			r.Header.Del(name)
		}
		if authSsoCallback(w, r) {
			return
		}
		if authSsoExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			if claims, ok := authSsoVerifySession(strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))); ok {
				r.Header.Set("X-Rho-Subject", claims.Subject)
				r.Header.Set("X-Rho-Scopes", strings.Join(claims.Scopes, ","))
				r.Header.Set("X-Rho-Aud", claims.Audience)
				if !authSsoHoldsScopes(claims.Scopes) {
					authSsoDenyScope(w, r)
					return
				}
				ctx := context.WithValue(r.Context(), authSsoSessionKey{}, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		cookie, err := r.Cookie(authSsoCookie)
		if err == nil && cookie.Value != "" {
			if claims, ok := authSsoVerifySession(cookie.Value); ok {
				r.Header.Set("X-Rho-Subject", claims.Subject)
				r.Header.Set("X-Rho-Scopes", strings.Join(claims.Scopes, ","))
				r.Header.Set("X-Rho-Aud", claims.Audience)
				ctx := context.WithValue(r.Context(), authSsoSessionKey{}, claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if identity, ok := authSsoLookup(cookie.Value); ok {
				r.Header.Set("X-Rho-Subject", identity.Subject)
				r.Header.Set("X-Rho-Scopes", strings.Join(identity.Scopes, ","))
				r.Header.Set("X-Rho-Aud", identity.Audience)
				ctx := context.WithValue(r.Context(), authSsoSessionKey{}, identity)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		if authSsoWantsJSON(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		target := authSsoLoginURL
		if strings.HasPrefix(target, "/") {
			target = authSsoAbsolute(r, target)
		}
		separator := "?"
		if strings.Contains(target, "?") {
			separator = "&"
		}
		callback := authSsoCallbackURL(r)
		http.Redirect(w, r, target+separator+authSsoReturnParam+"="+
			url.QueryEscape(callback)+"&next="+url.QueryEscape(authSsoReturnTo(r)),
			http.StatusFound)
	})
}

func authSsoCallbackURL(r *http.Request) string {
	base := authSsoReturnTo(r)
	if at := strings.Index(base, "://"); at >= 0 {
		if slash := strings.Index(base[at+3:], "/"); slash >= 0 {
			base = base[:at+3+slash]
		}
	}
	return base + strings.TrimRight(authSsoBasePath, "/") + "/auth/callback"
}

func authSsoAbsolute(r *http.Request, path string) string {
	base := authSsoReturnTo(r)
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return path
	}
	host := parsed.Hostname()
	keeperPort := os.Getenv("RHO_KEEPER_PORT")
	if keeperPort == "" {
		keeperPort = authSsoKeeperPort
	}
	if keeperPort != "" {
		host = net.JoinHostPort(host, keeperPort)
	}
	return parsed.Scheme + "://" + host + path
}

func authSsoSession(r *http.Request) (authSsoSessionClaims, bool) {
	claims, ok := r.Context().Value(authSsoSessionKey{}).(authSsoSessionClaims)
	return claims, ok
}
`, issuer, audience, cookie, publicKey, mode, publicURL, basePath, loginURL,
		keeperPort, returnParam, trustForwarded, scheme, sessionFile, skip, scopes)
	return b.String()
}

// cookieToken turns an audience into the part of a cookie name it can be: letters,
// digits and underscores, so `gateway:edge-1` becomes `gateway_edge_1`.
func cookieToken(audience string) string {
	var b strings.Builder
	for _, r := range audience {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
