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
