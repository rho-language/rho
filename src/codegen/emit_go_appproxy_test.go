package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The proxy is generated into a real program, put in front of an application
// that writes root-relative links and cookies, and asked for its pages.
func TestAppProxyServesAnApplicationUnderItsPrefix(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	apps := filepath.Join(dir, "proxy.txt")
	source := emitAppProxyMiddleware("withAppProxy", MiddlewareSpec{Options: map[string]string{
		"apps_file": apps, "strip_cookie": "keeper_session"}})
	harness := `package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var _ = filepath.Join

` + source + `

func main() {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/go":
			http.Redirect(w, r, "/water", http.StatusFound)
		case "/back":
			http.Redirect(w, r, "/app/aquarium/water", http.StatusFound)
		default:
			http.SetCookie(w, &http.Cookie{Name: "rho_session_aquarium", Value: "s", Path: "/"})
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, "<a href=\"/water\">w</a><form action='/rules'></form><button hx-post=\"/x\"></button><a href=\"//cdn.example/x\">c</a>|cookies=%s|prefix=%s|host=%s",
				r.Header.Get("Cookie"), r.Header.Get("X-Forwarded-Prefix"), r.Host)
		}
	}))
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(app.URL, "http://"))
	os.WriteFile(os.Args[1], []byte("aquarium "+port+"\n"), 0o600)
	front := httptest.NewServer(withAppProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "keeper")
	})))
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", front.URL+path, nil)
		req.Header.Set("Cookie", "keeper_session=admin; other=1")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("ERR", err)
			os.Exit(1)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, string(body)
	}
	resp, body := get("/app/aquarium/page")
	fmt.Println("BODY", body)
	fmt.Println("COOKIE", resp.Header.Get("Set-Cookie"))
	resp, _ = get("/app/aquarium/go")
	fmt.Println("LOCATION", resp.Header.Get("Location"))
	resp, _ = get("/app/aquarium/back")
	fmt.Println("BACK", resp.Header.Get("Location"))
	resp, _ = get("/app/aquarium")
	fmt.Println("BARE", resp.StatusCode, resp.Header.Get("Location"))
	resp, _ = get("/app/nobody/x")
	fmt.Println("UNKNOWN", resp.StatusCode)
	_, body = get("/elsewhere")
	fmt.Println("KEEPER", body)
	_ = bytes.NewReader
	_ = httputil.NewSingleHostReverseProxy
	_ = url.Parse
	_ = regexp.MustCompile
	_ = strconv.Itoa
	_ = sync.Mutex{}
	_ = time.Now
}
`
	main := filepath.Join(dir, "main.go")
	if err := os.WriteFile(main, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "run", main, apps).CombinedOutput()
	if err != nil {
		t.Fatalf("go run: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{
		`href="/app/aquarium/water"`,
		`action='/app/aquarium/rules'`,
		`hx-post="/app/aquarium/x"`,
		`href="//cdn.example/x"`,
		"|cookies=other=1|",
		"|prefix=/app/aquarium|",
		"Path=/app/aquarium/",
		"LOCATION /app/aquarium/water",
		"BACK /app/aquarium/water",
		"BARE 302 /app/aquarium/",
		"UNKNOWN 404",
		"KEEPER keeper",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "keeper_session") {
		t.Errorf("the supervisor's cookie reached the application:\n%s", got)
	}
}
