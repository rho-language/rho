package deps

import (
	"os"
	"testing"
)

func TestLockfileSaveAndLoad(t *testing.T) {
	tmp := "test_rho.lock"

	lock := &Lockfile{
		Packages: []LockedPackage{
			{
				Name:    "robohub-common",
				Version: "1.4.2",
				GitURL:  "https://github.com/aurel/robohub-common.git",
				Rev:     "1.4.2",
				Path:    "/deps/robohub-common",
				Hash:    "sha256:abc123",
			},
			{
				Name:    "robohub-hal",
				Version: "0.7.1",
				GitURL:  "https://github.com/aurel/robohub-hal.git",
				Rev:     "0.7.1",
				Path:    "/deps/robohub-hal",
				Hash:    "sha256:def456",
			},
		},
	}

	if err := lock.Save(tmp); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	defer os.Remove(tmp)

	loaded, err := LoadLockfile(tmp)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if len(loaded.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(loaded.Packages))
	}

	p := loaded.Packages[0]
	if p.Name != "robohub-common" {
		t.Fatalf("expected robohub-common, got %s", p.Name)
	}
	if p.Version != "1.4.2" {
		t.Fatalf("expected version 1.4.2, got %s", p.Version)
	}
	if p.Hash != "sha256:abc123" {
		t.Fatalf("expected hash sha256:abc123, got %s", p.Hash)
	}
}
