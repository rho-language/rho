//
// @file artifact.go
// @brief The manifest a deployable or flashable artifact carries.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0
//

package schema

import (
	"fmt"
	"strings"
)

// ArtifactHealth is the check a host makes after starting a new service.
type ArtifactHealth struct {
	URL            string `json:"url,omitempty"`
	TimeoutSeconds int    `json:"timeout_s,omitempty"`
}

// ArtifactRollback says how many previous versions to keep and whether a failed
// health check restores one automatically.
type ArtifactRollback struct {
	Keep      int  `json:"keep,omitempty"`
	Automatic bool `json:"automatic,omitempty"`
}

// ArtifactManifest describes one artifact. The signature covers the manifest
// with Signature empty; the checksum covers the payload, not the manifest.
type ArtifactManifest struct {
	Name       string           `json:"name"`
	Version    string           `json:"version"`
	Kind       string           `json:"kind"`
	Target     string           `json:"target"`
	Entrypoint string           `json:"entrypoint,omitempty"`
	Service    string           `json:"service,omitempty"`
	Health     ArtifactHealth   `json:"health,omitempty"`
	Rollback   ArtifactRollback `json:"rollback,omitempty"`
	SHA256     string           `json:"sha256"`
	Size       int64            `json:"size"`
	Signature  string           `json:"signature,omitempty"`
}

// ValidateArtifactManifest is the contract every artifact has to satisfy before
// a deployer stages it.
func ValidateArtifactManifest(manifest ArtifactManifest) error {
	if strings.TrimSpace(manifest.Name) == "" {
		return fmt.Errorf("artifact has no name")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return fmt.Errorf("artifact %q has no version", manifest.Name)
	}
	switch manifest.Kind {
	case "service":
		if manifest.Service == "" {
			return fmt.Errorf("artifact %q is a service and has no service name", manifest.Name)
		}
		if manifest.Entrypoint == "" {
			return fmt.Errorf("artifact %q has no entrypoint", manifest.Name)
		}
		if manifest.Health.URL != "" && !strings.HasPrefix(manifest.Health.URL, "/") {
			return fmt.Errorf("artifact %q health URL must be a local path", manifest.Name)
		}
	case "firmware":
		if manifest.Entrypoint == "" {
			return fmt.Errorf("artifact %q has no entrypoint", manifest.Name)
		}
	default:
		return fmt.Errorf("artifact %q has unknown kind %q", manifest.Name, manifest.Kind)
	}
	if strings.TrimSpace(manifest.Target) == "" {
		return fmt.Errorf("artifact %q has no target", manifest.Name)
	}
	if manifest.SHA256 == "" {
		return fmt.Errorf("artifact %q has no checksum", manifest.Name)
	}
	if manifest.Size <= 0 {
		return fmt.Errorf("artifact %q has no payload size", manifest.Name)
	}
	if manifest.Health.TimeoutSeconds < 0 {
		return fmt.Errorf("artifact %q has a negative health timeout", manifest.Name)
	}
	if manifest.Rollback.Keep < 0 {
		return fmt.Errorf("artifact %q has a negative rollback depth", manifest.Name)
	}
	return nil
}
