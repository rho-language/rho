package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The same-origin wrapper is run against real requests: a write from another
// port of the same host is the case it exists for, and that case is decided by
// headers a reading of the source cannot exercise.
func TestGeneratedSameOriginMiddlewareBehaves(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go toolchain is not on the path")
	}
	dir := t.TempDir()
	body := emitSameOriginMiddleware("withSameOrigin", MiddlewareSpec{Options: map[string]string{
		"skip_paths": "/api/enroll, /hooks/",
	}})
	source := "package main\n\nimport (\n\t\"net/http\"\n\t\"strings\"\n)\n\n" + body
	harness := `package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func status(method, path string, headers map[string]string) int {
	h := withSameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	r := httptest.NewRequest(method, "http://127.0.0.1:19090"+path, nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestWrites(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		path    string
		headers map[string]string
		want    int
	}{
		{"a read from anywhere", "GET", "/", map[string]string{"Sec-Fetch-Site": "cross-site"}, 200},
		{"a write from this page", "POST", "/x", map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"a write from another port", "POST", "/x", map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"a write from another site", "POST", "/x", map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"a write typed into the bar", "POST", "/x", map[string]string{"Sec-Fetch-Site": "none"}, 200},
		{"an older browser, this origin", "POST", "/x", map[string]string{"Origin": "http://127.0.0.1:19090"}, 200},
		{"an older browser, another port", "POST", "/x", map[string]string{"Origin": "http://127.0.0.1:8478"}, 403},
		{"an opaque origin", "POST", "/x", map[string]string{"Origin": "null"}, 403},
		{"a program, no browser headers", "POST", "/x", nil, 200},
		{"a path that takes it on purpose", "POST", "/api/enroll", map[string]string{"Sec-Fetch-Site": "cross-site"}, 200},
		{"under a skipped prefix", "DELETE", "/hooks/a", map[string]string{"Sec-Fetch-Site": "cross-site"}, 200},
		{"a delete from another port", "DELETE", "/x", map[string]string{"Origin": "http://127.0.0.1:8478"}, 403},
	}
	for _, c := range cases {
		if got := status(c.method, c.path, c.headers); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
`
	for name, text := range map[string]string{"go.mod": "module soharness\n\ngo 1.22\n", "mw.go": source, "mw_test.go": harness} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the generated wrapper does not behave:\n%s", out)
	}
}
