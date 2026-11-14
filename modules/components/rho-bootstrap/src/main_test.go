package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"rho/src/schema"
)

func writeContract(t *testing.T, dir string) string {
	t.Helper()
	contract := schema.Provisioning{
		Device:    schema.ProvisioningDevice{ID: "garden-01", Kind: "node", Board: "esp32c3"},
		Placement: schema.ProvisioningPlacement{Project: "default", Area: 1, Node: 7},
		Secrets:   schema.ProvisioningSecrets{Store: filepath.Join(dir, "identity")},
		Permissions: schema.ProvisioningPermissions{
			Directory: "0700", Files: "0600",
		},
	}
	data, _ := json.Marshal(contract)
	path := filepath.Join(dir, "provisioning.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBootstrapIsIdempotentAndPrivate(t *testing.T) {
	dir := t.TempDir()
	contract := writeContract(t, dir)
	if err := bootstrap([]string{"-config", contract, "-offline"}); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	identityPath := filepath.Join(dir, "identity", "identity.json")
	first, err := readIdentity(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{
		filepath.Join(dir, "identity"):        0700,
		pathFromTest(t, dir, "identity.json"): 0600,
		pathFromTest(t, dir, "identity.key"):  0600,
		pathFromTest(t, dir, "identity.pub"):  0644,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Errorf("%s mode = %o, want %o", path, info.Mode().Perm(), mode)
		}
	}
	if err := bootstrap([]string{"-config", contract, "-offline"}); err != nil {
		t.Fatal(err)
	}
	second, err := readIdentity(identityPath)
	if err != nil {
		t.Fatal(err)
	}
	if first.InstanceID != second.InstanceID {
		t.Fatal("a second bootstrap regenerated the identity")
	}
}

func pathFromTest(t *testing.T, dir, name string) string {
	t.Helper()
	return filepath.Join(dir, "identity", name)
}

func TestBootstrapRefusesBadContract(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "provisioning.json")
	os.WriteFile(path, []byte(`{"device":{"id":"","kind":"node"}}`), 0644)
	if err := bootstrap([]string{"-config", path, "-offline"}); err == nil {
		t.Fatal("an invalid contract was accepted")
	}
}

func TestBootstrapEnrollsWithKeeper(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/enroll" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.FormValue("device_id") != "garden-01" || r.FormValue("token") != "once" {
			t.Errorf("unexpected form: %v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"credential":"issued-token"}`)
	}))
	defer server.Close()
	contract := schema.Provisioning{
		Device:    schema.ProvisioningDevice{ID: "garden-01", Kind: "node"},
		Placement: schema.ProvisioningPlacement{Project: "default", Area: 1, Node: 7},
		Keeper:    schema.ProvisioningKeeper{URL: server.URL},
		Secrets:   schema.ProvisioningSecrets{Store: filepath.Join(dir, "identity")},
	}
	data, _ := json.Marshal(contract)
	path := filepath.Join(dir, "provisioning.json")
	os.WriteFile(path, data, 0644)
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("once\n"), 0600)
	if err := bootstrap([]string{"-config", path, "-token-file", tokenFile}); err != nil {
		t.Fatalf("bootstrap enrollment failed: %v", err)
	}
	credential, err := os.ReadFile(filepath.Join(dir, "identity", "credential"))
	if err != nil || string(credential) != "issued-token\n" {
		t.Fatalf("credential = %q, %v", credential, err)
	}
}

func TestTokenOnTheCommandLineIsRefused(t *testing.T) {
	dir := t.TempDir()
	contract := writeContract(t, dir)
	err := bootstrap([]string{"-config", contract, "-token", "secret"})
	if err == nil || !strings.Contains(err.Error(), "-token-file") {
		t.Fatalf("a token on the command line was not refused: %v", err)
	}
}

func TestTokenIsReadFromAFileOrStandardInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	os.WriteFile(path, []byte("  abc123\nignored\n"), 0600)
	if value, err := readToken(path, nil); err != nil || value != "abc123" {
		t.Fatalf("file token = %q, %v", value, err)
	}
	if value, err := readToken("-", strings.NewReader("def456\n")); err != nil || value != "def456" {
		t.Fatalf("stdin token = %q, %v", value, err)
	}
	if _, err := readToken("-", strings.NewReader("\n")); err == nil {
		t.Fatal("an empty token was accepted")
	}
	if _, err := readToken("-", strings.NewReader("a|b\n")); err == nil {
		t.Fatal("a token line with a separator was accepted")
	}
}

func TestKeeperURLMustBeHTTPS(t *testing.T) {
	for raw, ok := range map[string]bool{
		"https://keeper.example.com":   true,
		"http://127.0.0.1:19090":       true,
		"http://localhost:19090":       true,
		"http://[::1]:19090":           true,
		"http://keeper.example.com":    false,
		"http://192.168.1.10:19090":    false,
		"ftp://keeper.example.com":     false,
		"keeper.example.com":           false,
		"http://127.0.0.1.example.com": false,
	} {
		err := checkKeeperURL(raw)
		if (err == nil) != ok {
			t.Errorf("checkKeeperURL(%q) = %v, want ok=%v", raw, err, ok)
		}
	}
}

func TestEnrollmentRefusesAPlainHTTPKeeper(t *testing.T) {
	dir := t.TempDir()
	contract := schema.Provisioning{
		Device:    schema.ProvisioningDevice{ID: "garden-01", Kind: "node"},
		Placement: schema.ProvisioningPlacement{Project: "default", Area: 1, Node: 7},
		Keeper:    schema.ProvisioningKeeper{URL: "http://keeper.example.com"},
		Secrets:   schema.ProvisioningSecrets{Store: filepath.Join(dir, "identity")},
	}
	data, _ := json.Marshal(contract)
	path := filepath.Join(dir, "provisioning.json")
	os.WriteFile(path, data, 0644)
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("once\n"), 0600)
	err := bootstrap([]string{"-config", path, "-token-file", tokenFile})
	if err == nil || !strings.Contains(err.Error(), "plain http") {
		t.Fatalf("a plain http keeper was accepted: %v", err)
	}
}

func TestTokenLinesAreAddedUnderTheLock(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "keeper", "enrollment.tokens")
	if err := token([]string{"-device", "garden-01", "-project", "default", "-area", "1", "-node", "7", "-file", file}); err != nil {
		t.Fatal(err)
	}
	if err := token([]string{"-device", "garden-01", "-area", "1", "-node", "7", "-reenroll", "-file", file}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("token file = %q", data)
	}
	fields := strings.Split(lines[0], "|")
	if len(fields) != 7 || fields[1] != "garden-01" || fields[2] != "default" || fields[3] != "1" ||
		fields[4] != "7" || fields[5] == "" || fields[5] == "0" || fields[6] != "" {
		t.Fatalf("first line = %q", lines[0])
	}
	if !strings.HasSuffix(lines[1], "|reenroll") {
		t.Fatalf("a re-enroll token is not marked: %q", lines[1])
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("token file mode = %o", info.Mode().Perm())
	}
	if _, err := os.Stat(file + ".lock"); !os.IsNotExist(err) {
		t.Fatal("the lock was left behind")
	}

	// Many writers at once lose no line.
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			if err := appendTokenLine(file, fmt.Sprintf("t%d|d|p|1|2|3|", i)); err != nil {
				t.Error(err)
			}
		}(i)
	}
	group.Wait()
	data, _ = os.ReadFile(file)
	if got := len(strings.Split(strings.TrimSpace(string(data)), "\n")); got != 18 {
		t.Fatalf("concurrent writers left %d lines, want 18", got)
	}

	// A writer waits while the keeper holds the lock.
	if err := os.Mkdir(file+".lock", 0700); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- appendTokenLine(file, "late|d|p|1|2|3|") }()
	select {
	case <-done:
		t.Fatal("a line was written while the lock was held")
	case <-time.After(300 * time.Millisecond):
	}
	os.Remove(file + ".lock")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(file)
	if !strings.Contains(string(data), "late|") {
		t.Fatal("the waiting line was lost")
	}
}

func TestTokenNeedsAnExpiryAndAPlacement(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tokens")
	if err := token([]string{"-device", "garden-01", "-ttl", "0s", "-file", file}); err == nil {
		t.Fatal("a token without expiry was made")
	}
	if err := token([]string{"-device", "garden-01", "-area", "x", "-file", file}); err == nil {
		t.Fatal("a token with a malformed area was made")
	}
	if err := token([]string{"-device", "../x", "-file", file}); err == nil {
		t.Fatal("a token for a malformed device was made")
	}
}

func TestBoardKeyIsFetchedAfterEnrollment(t *testing.T) {
	dir := t.TempDir()
	const uid = "0123456789abcdef"
	const key = "00112233445566778899aabbccddeeff"
	oldMesh := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/enroll":
			io.WriteString(w, `{"ok":true,"credential":"issued-credential"}`)
		case "/app/mesh/mesh/api/nodes/key":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Node garden-01:issued-credential" {
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, `{"ok":false,"error":"unknown node or credential"}`)
				return
			}
			if oldMesh {
				io.WriteString(w, `{"ok":true,"uid":"`+uid+`","key":"`+key+`"}`)
				return
			}
			io.WriteString(w, `{"ok":true,"uid":"`+uid+`","key":"`+key+`","epoch":3}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	contract := schema.Provisioning{
		Device:    schema.ProvisioningDevice{ID: "garden-01", Kind: "host"},
		Placement: schema.ProvisioningPlacement{Project: "default", Area: 1, Node: 7},
		Keeper:    schema.ProvisioningKeeper{URL: server.URL},
		Secrets:   schema.ProvisioningSecrets{Store: filepath.Join(dir, "identity")},
	}
	data, _ := json.Marshal(contract)
	path := filepath.Join(dir, "provisioning.json")
	os.WriteFile(path, data, 0644)
	tokenFile := filepath.Join(dir, "token")
	os.WriteFile(tokenFile, []byte("once\n"), 0600)
	if err := bootstrap([]string{"-config", path, "-token-file", tokenFile, "-board-key"}); err != nil {
		t.Fatalf("bootstrap with board key failed: %v", err)
	}
	keyPath := filepath.Join(dir, "identity", "board.key")
	stored, err := os.ReadFile(keyPath)
	if err != nil || string(stored) != "3:"+key+"\n" {
		t.Fatalf("board.key = %q, %v (want the epoch Mesh answered, then the key)", stored, err)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "identity", "board.env"))
	if !strings.Contains(string(env), "RH_SECURE_UID="+uid+"\n") ||
		!strings.Contains(string(env), "RH_SECURE_KEY_FILE="+keyPath+"\n") {
		t.Fatalf("board.env = %q", env)
	}
	for _, name := range []string{"board.key", "board.env"} {
		info, _ := os.Stat(filepath.Join(dir, "identity", name))
		if info.Mode().Perm() != 0600 {
			t.Errorf("%s mode = %o, want 600", name, info.Mode().Perm())
		}
	}
	// Fetching again replaces a key the node's keystore kept beside the file,
	// and an older Mesh, which names no epoch, has the key written alone.
	os.WriteFile(keyPath+".keystore", []byte("stale"), 0600)
	oldMesh = true
	if err := boardKey([]string{"-config", path}); err != nil {
		t.Fatalf("fetching the key again failed: %v", err)
	}
	if _, err := os.Stat(keyPath + ".keystore"); !os.IsNotExist(err) {
		t.Fatalf("the keystore beside the key was not removed: %v", err)
	}
	if stored, _ := os.ReadFile(keyPath); string(stored) != key+"\n" {
		t.Fatalf("board.key from an older Mesh = %q", stored)
	}
	oldMesh = false
	// A credential Mesh does not accept stores nothing.
	os.WriteFile(filepath.Join(dir, "identity", "credential"), []byte("wrong\n"), 0600)
	if err := boardKey([]string{"-config", path}); err == nil || !strings.Contains(err.Error(), "unknown node") {
		t.Fatalf("a refused credential was not reported: %v", err)
	}
}
