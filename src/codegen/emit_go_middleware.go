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
