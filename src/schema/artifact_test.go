package schema

import "testing"

func validArtifact() ArtifactManifest {
	return ArtifactManifest{
		Name: "aquarium", Version: "0.4.0", Kind: "service",
		Target: "host-linux-amd64", Entrypoint: "aquarium",
		Service: "aquarium", Health: ArtifactHealth{URL: "/healthz", TimeoutSeconds: 20},
		Rollback: ArtifactRollback{Keep: 2, Automatic: true},
		SHA256:   "abc", Size: 12,
	}
}

func TestArtifactManifest(t *testing.T) {
	if err := ValidateArtifactManifest(validArtifact()); err != nil {
		t.Fatalf("valid artifact refused: %v", err)
	}
	manifest := validArtifact()
	manifest.Service = ""
	if err := ValidateArtifactManifest(manifest); err == nil {
		t.Fatal("service artifact without a service name was accepted")
	}
	manifest = validArtifact()
	manifest.Health.URL = "http://example/healthz"
	if err := ValidateArtifactManifest(manifest); err == nil {
		t.Fatal("remote health URL was accepted")
	}
	manifest = validArtifact()
	manifest.Kind = "container"
	if err := ValidateArtifactManifest(manifest); err == nil {
		t.Fatal("unknown kind was accepted")
	}
}
