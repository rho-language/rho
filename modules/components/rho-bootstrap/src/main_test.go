package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	if err := bootstrap([]string{"-config", path, "-token", "once"}); err != nil {
		t.Fatalf("bootstrap enrollment failed: %v", err)
	}
	credential, err := os.ReadFile(filepath.Join(dir, "identity", "credential"))
	if err != nil || string(credential) != "issued-token\n" {
		t.Fatalf("credential = %q, %v", credential, err)
	}
}
