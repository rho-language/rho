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
				Name:    "sensor-lib",
				Version: "1.4.2",
				GitURL:  "https://github.com/aurel/sensor-lib.git",
				Rev:     "1.4.2",
				Path:    "/deps/sensor-lib",
				Hash:    "sha256:abc123",
			},
			{
				Name:    "sensor-hal",
				Version: "0.7.1",
				GitURL:  "https://github.com/aurel/sensor-hal.git",
				Rev:     "0.7.1",
				Path:    "/deps/sensor-hal",
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
	if p.Name != "sensor-lib" {
		t.Fatalf("expected sensor-lib, got %s", p.Name)
	}
	if p.Version != "1.4.2" {
		t.Fatalf("expected version 1.4.2, got %s", p.Version)
	}
	if p.Hash != "sha256:abc123" {
		t.Fatalf("expected hash sha256:abc123, got %s", p.Hash)
	}
}
