package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientRegistryStoresAHashAndAuthenticates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth", "clients")
	secret, err := AddClient(path, "aquarium", []string{"mesh"}, []string{"mesh:home", "mesh:home:actuate"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), secret) {
		t.Fatal("the registry holds the secret itself")
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("registry mode %o, want 600", info.Mode().Perm())
	}
	if _, err := Authenticate(path, "aquarium", secret); err != nil {
		t.Fatalf("the right secret was refused: %v", err)
	}
	for _, bad := range [][2]string{{"aquarium", secret + "x"}, {"aquarium", ""}, {"energy", secret}} {
		if _, err := Authenticate(path, bad[0], bad[1]); err != ErrClientDenied {
			t.Errorf("%v: err = %v, want ErrClientDenied", bad, err)
		}
	}
}

func TestGrantNeverWidens(t *testing.T) {
	c := Client{ID: "mesh", Audiences: []string{"gateway:*"}, Scopes: []string{"gateway:*:read", "outbox:read"}}
	if _, err := c.Grant("mesh", nil); err == nil {
		t.Error("an audience the client is not registered for was granted")
	}
	got, err := c.Grant("gateway:edge-1", []string{"outbox:read"})
	if err != nil || len(got) != 1 || got[0] != "outbox:read" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := c.Grant("gateway:edge-1", []string{"gateway:edge-1:command"}); err == nil {
		t.Error("a scope outside the client's registration was granted")
	}
	if got, _ := c.Grant("gateway:edge-1", []string{"gateway:edge-1:read"}); len(got) != 1 {
		t.Error("a scope matching a registered prefix was refused")
	}
	if got, _ := c.Grant("gateway:edge-1", nil); len(got) != 1 || got[0] != "outbox:read" {
		t.Errorf("asking for nothing should grant the concrete scopes only, got %v", got)
	}
}

func TestRotatingAndRemovingAClient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clients")
	first, _ := AddClient(path, "energy", []string{"mesh"}, nil)
	second, _ := AddClient(path, "energy", []string{"mesh"}, nil)
	if _, err := Authenticate(path, "energy", first); err == nil {
		t.Error("the rotated-out secret still authenticates")
	}
	if _, err := Authenticate(path, "energy", second); err != nil {
		t.Error("the new secret does not authenticate")
	}
	if err := RemoveClient(path, "energy"); err != nil {
		t.Fatal(err)
	}
	if _, err := Authenticate(path, "energy", second); err == nil {
		t.Error("a removed client authenticates")
	}
	if _, err := AddClient(path, "bad|id", []string{"mesh"}, nil); err == nil {
		t.Error("an id that breaks the line format was accepted")
	}
}

func TestMatchPattern(t *testing.T) {
	for _, c := range []struct {
		pattern, value string
		want           bool
	}{
		{"mesh", "mesh", true},
		{"gateway:*", "gateway:edge-1", true},
		{"gateway:*", "gateway", false},
		{"gateway:*:read", "gateway:edge-1:read", true},
		{"gateway:*:read", "gateway:edge-1:command", false},
		{"mesh:*", "mesh:home:actuate", true},
		{"mesh:home", "mesh:home:actuate", false},
		{"mesh:home:actuate", "mesh:home", false},
	} {
		if got := matchPattern(c.pattern, c.value); got != c.want {
			t.Errorf("matchPattern(%q, %q) = %v", c.pattern, c.value, got)
		}
	}
}
