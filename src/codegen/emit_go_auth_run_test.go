package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The generated auth middleware is run, not only read: a security property that
// holds in the text and not in the program is not a property. The test renders the
// middleware into a scratch module with a small harness and runs the harness's own
// tests against real requests.
func TestGeneratedAuthMiddlewareBehaves(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	dir := t.TempDir()
	body := emitAuthMiddleware("auth", MiddlewareSpec{Options: map[string]string{
		"issuer": "keeper", "audience": "mesh", "public_key": filepath.Join(dir, "missing.pem"),
		"skip_paths": "/open",
	}})
	source := `package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	jsonenc "encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	_ = context.Background
	_ = ecdsa.Verify
	_ = rand.Read
	_ = sha256.Sum256
	_ = x509.ParsePKIXPublicKey
	_ = base64.RawURLEncoding
	_ = hex.EncodeToString
	_ = pem.Decode
	_ = errors.New
	_ = big.NewInt
	_ = net.JoinHostPort
	_ = url.Parse
	_ = os.Getenv
	_ = filepath.Join
	_ = strconv.Itoa
	_ = strings.Split
	_ = sync.Once{}
	_ = time.Now
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonenc.NewEncoder(w).Encode(payload)
}

` + body
	harness := `package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"strconv"
)

func echo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("aud=" + r.Header.Get("X-Rho-Aud") + ";scopes=" + r.Header.Get("X-Rho-Scopes") + ";sub=" + r.Header.Get("X-Rho-Subject")))
	})
}

func TestForgedHeadersAreRemovedOnAnExemptPath(t *testing.T) {
	r := httptest.NewRequest("GET", "/open", nil)
	r.Header.Set("X-Rho-Aud", "mesh")
	r.Header.Set("X-Rho-Scopes", "*")
	r.Header.Set("X-Rho-Subject", "admin")
	w := httptest.NewRecorder()
	withAuth(echo()).ServeHTTP(w, r)
	if got := w.Body.String(); got != "aud=;scopes=;sub=" {
		t.Fatalf("a forged identity reached the handler: %q", got)
	}
}

func TestASessionCarriesItsIdentityAndAudience(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	exp := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	if err := os.WriteFile(authSsoSessionPath(), []byte("sid1|"+exp+"|alice|mesh:home\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("Accept", "application/json")
	r.AddCookie(&http.Cookie{Name: authSsoCookie, Value: "sid1"})
	w := httptest.NewRecorder()
	withAuth(echo()).ServeHTTP(w, r)
	if got := w.Body.String(); got != "aud=mesh;scopes=mesh:home;sub=alice" {
		t.Fatalf("session identity = %q", got)
	}
}

func TestAnUnknownOrExpiredSessionIsRefused(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	past := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	os.WriteFile(authSsoSessionPath(), []byte("old|"+past+"|alice|*\n"), 0o600)
	for _, sid := range []string{"old", "nope"} {
		r := httptest.NewRequest("GET", "/x", nil)
		r.Header.Set("Accept", "application/json")
		r.AddCookie(&http.Cookie{Name: authSsoCookie, Value: sid})
		w := httptest.NewRecorder()
		withAuth(echo()).ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("session %q: status %d, want 401", sid, w.Code)
		}
	}
}

func TestASessionEndsWithTheIssuersSession(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	asked := 0
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.URL.Path == "/auth/session/k-live" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer issuer.Close()
	t.Setenv("RHO_AUTH_URL", issuer.URL)
	exp := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	os.WriteFile(authSsoSessionPath(), []byte("a|"+exp+"|ana|*|k-live\nb|"+exp+"|bob|*|k-ended\nc|"+exp+"|cy|*|\n"), 0o600)
	status := func(sid string) int {
		r := httptest.NewRequest("GET", "/x", nil)
		r.Header.Set("Accept", "application/json")
		r.AddCookie(&http.Cookie{Name: authSsoCookie, Value: sid})
		w := httptest.NewRecorder()
		withAuth(echo()).ServeHTTP(w, r)
		return w.Code
	}
	if got := status("a"); got != http.StatusOK {
		t.Errorf("a session whose issuer session stands: %d", got)
	}
	if got := status("b"); got != http.StatusUnauthorized {
		t.Errorf("a session whose issuer session ended: %d", got)
	}
	if got := status("c"); got != http.StatusOK {
		t.Errorf("a session opened before sessions were tied: %d", got)
	}
	before := asked
	status("a")
	if asked != before {
		t.Errorf("the issuer was asked again within the minute")
	}
	// An issuer that is down does not sign anyone out.
	issuer.Close()
	authSsoLiveMu.Lock()
	authSsoLiveSeen["k-live"] = authSsoLiveAnswer{live: true, at: time.Now().Add(-2 * time.Minute)}
	authSsoLiveMu.Unlock()
	if got := status("a"); got != http.StatusOK {
		t.Errorf("an issuer that is down signed a session out: %d", got)
	}
}

func TestReadOnlyScopesMayLookAndNotChange(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	exp := strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)
	os.WriteFile(authSsoSessionPath(), []byte("r|"+exp+"|vi|mesh:read,aquarium:read\nw|"+exp+"|op|mesh:greenhouse,mesh:read\n"), 0o600)
	status := func(method, sid string) int {
		req := httptest.NewRequest(method, "/x", nil)
		req.Header.Set("Accept", "application/json")
		req.AddCookie(&http.Cookie{Name: authSsoCookie, Value: sid})
		w := httptest.NewRecorder()
		withAuth(echo()).ServeHTTP(w, req)
		return w.Code
	}
	if got := status("GET", "r"); got != http.StatusOK {
		t.Errorf("a reader reading: %d", got)
	}
	if got := status("POST", "r"); got != http.StatusForbidden {
		t.Errorf("a reader changing something: %d", got)
	}
	if got := status("POST", "w"); got != http.StatusOK {
		t.Errorf("a writer changing something: %d", got)
	}
}

func es256(t *testing.T, key *ecdsa.PrivateKey, claims string) string {
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	sum := sha256.Sum256(der)
	head := base64.RawURLEncoding.EncodeToString([]byte(` + "`" + `{"alg":"ES256","typ":"JWT","kid":"` + "`" + ` + base64.RawURLEncoding.EncodeToString(sum[:12]) + ` + "`" + `"}` + "`" + `))
	body := base64.RawURLEncoding.EncodeToString([]byte(claims))
	digest := sha256.Sum256([]byte(head + "." + body))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return head + "." + body + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestARotatedKeyIsFetchedWhenATokenNamesIt(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	t.Setenv("RHO_AUTH_PUBLIC_KEY", filepath.Join(t.TempDir(), "absent.pem"))
	first, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	second, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	current := first
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		der, _ := x509.MarshalPKIXPublicKey(&current.PublicKey)
		w.Write(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	}))
	defer issuer.Close()
	t.Setenv("RHO_AUTH_URL", issuer.URL)
	authSsoKey, authSsoKeyTried, authSsoKeyReloaded = nil, time.Time{}, time.Time{}
	exp := strconv.FormatInt(time.Now().Add(time.Minute).Unix(), 10)
	claims := ` + "`" + `{"sub":"ana","iss":"keeper","aud":"mesh","scp":["mesh:*"],"exp":` + "`" + ` + exp + ` + "`" + `}` + "`" + `
	status := func(token string) int {
		r := httptest.NewRequest("GET", "/x", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		withAuth(echo()).ServeHTTP(w, r)
		return w.Code
	}
	if got := status(es256(t, first, claims)); got != http.StatusOK {
		t.Fatalf("a token of the first key: %d", got)
	}
	current = second
	if got := status(es256(t, second, claims)); got != http.StatusOK {
		t.Fatalf("a token of the rotated key was refused: %d", got)
	}
	if got := status(es256(t, first, claims)); got == http.StatusOK {
		t.Fatalf("a token of the retired key is still accepted")
	}
}

func TestStandaloneAnswersThisMachineOnly(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	t.Setenv("RHO_AUTH_STANDALONE", "1")
	request := func(remote, forwarded string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/x", nil)
		r.RemoteAddr = remote
		if forwarded != "" {
			r.Header.Set("X-Forwarded-For", forwarded)
		}
		w := httptest.NewRecorder()
		withAuth(echo()).ServeHTTP(w, r)
		return w
	}
	if w := request("127.0.0.1:5555", ""); w.Code != http.StatusOK || w.Body.String() != "aud=mesh;scopes=*;sub=local" {
		t.Errorf("this machine: %d %q", w.Code, w.Body.String())
	}
	if w := request("192.0.2.7:5555", ""); w.Code != http.StatusForbidden {
		t.Errorf("another machine: %d", w.Code)
	}
	if w := request("127.0.0.1:5555", "192.0.2.7"); w.Code != http.StatusForbidden {
		t.Errorf("a forwarded request: %d", w.Code)
	}
}

func TestTheIssuersKeyIsFetchedAndKept(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	t.Setenv("RHO_AUTH_PUBLIC_KEY", filepath.Join(t.TempDir(), "absent.pem"))
	private, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKIXPublicKey(&private.PublicKey)
	published := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/public-key" {
			http.NotFound(w, r)
			return
		}
		w.Write(published)
	}))
	t.Setenv("RHO_AUTH_URL", issuer.URL)
	authSsoKey, authSsoKeyTried = nil, time.Time{}
	key, err := authSsoLoadKey()
	if err != nil || key == nil || !key.Equal(&private.PublicKey) {
		t.Fatalf("the published key was not used: %v", err)
	}
	if kept, err := os.ReadFile(filepath.Join(authSsoStateDir(), "issuer.pem")); err != nil || string(kept) != string(published) {
		t.Fatalf("no copy of the key was kept: %v", err)
	}
	// An issuer that is down does not lock anyone out: the copy is used.
	issuer.Close()
	authSsoKey, authSsoKeyTried = nil, time.Time{}
	if key, err := authSsoLoadKey(); err != nil || !key.Equal(&private.PublicKey) {
		t.Fatalf("the kept key was not used while the issuer was down: %v", err)
	}
}

func TestLoginGoesToTheIssuer(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", t.TempDir())
	t.Setenv("RHO_AUTH_STANDALONE", "")
	t.Setenv("RHO_AUTH_URL", "https://keeper.example/")
	r := httptest.NewRequest("GET", "http://mesh.example/x", nil)
	w := httptest.NewRecorder()
	withAuth(echo()).ServeHTTP(w, r)
	if location := w.Header().Get("Location"); !strings.HasPrefix(location, "https://keeper.example/auth/login") {
		t.Errorf("login goes to %q", location)
	}
}

func TestEachServiceKeepsItsOwnSessionsAndCookie(t *testing.T) {
	t.Setenv("RHO_APP_WORKSPACE", "")
	t.Setenv("ACME_APP_WORKSPACE", "")
	t.Setenv("HOME", t.TempDir())
	if !strings.HasSuffix(filepath.Dir(authSsoSessionPath()), filepath.Join("auth-app", "mesh")) {
		t.Errorf("sessions are kept in %s, not this service's own directory", authSsoSessionPath())
	}
	if authSsoCookie != "rho_session_mesh" {
		t.Errorf("cookie = %q", authSsoCookie)
	}
}
`
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module authharness\n\ngo 1.22\n")
	write("auth.go", source)
	write("auth_test.go", harness)
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the generated middleware does not behave:\n%s", strings.TrimSpace(string(out)))
	}
}
