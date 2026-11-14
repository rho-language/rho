//
// @file provisioning.go
// @brief The local contract a node or host reads on first boot.
//
// @project Rho Programming Language
// @version 0.1
// @date 2026
//
// SPDX-License-Identifier: Apache-2.0 WITH LLVM-exception
//

package schema

import (
	"fmt"
	"strings"
)

// ProvisioningDevice identifies the thing being provisioned.
type ProvisioningDevice struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Board string `json:"board,omitempty"`
}

// ProvisioningPlacement is where the node sits in the project.
type ProvisioningPlacement struct {
	Project string `json:"project"`
	Area    uint32 `json:"area"`
	Node    uint32 `json:"node"`
}

// ProvisioningKeeper is the trust root and enrollment address.
type ProvisioningKeeper struct {
	URL   string `json:"url,omitempty"`
	Trust string `json:"trust,omitempty"`
}

// ProvisioningMesh is the gateway this instance talks to.
type ProvisioningMesh struct {
	Gateway string `json:"gateway,omitempty"`
}

// ProvisioningSecrets says what to generate and where it is kept.
type ProvisioningSecrets struct {
	Generate []string `json:"generate,omitempty"`
	Store    string   `json:"store,omitempty"`
}

// ProvisioningPermissions are the modes the bootstrap enforces.
type ProvisioningPermissions struct {
	Directory string `json:"directory,omitempty"`
	Files     string `json:"files,omitempty"`
}

// Provisioning is the bootstrap contract. It contains no long-lived secret:
// credentials are generated locally or issued by Keeper after enrollment.
type Provisioning struct {
	Device      ProvisioningDevice      `json:"device"`
	Placement   ProvisioningPlacement   `json:"placement"`
	Keeper      ProvisioningKeeper      `json:"keeper,omitempty"`
	Mesh        ProvisioningMesh        `json:"mesh,omitempty"`
	Secrets     ProvisioningSecrets     `json:"secrets,omitempty"`
	Permissions ProvisioningPermissions `json:"permissions,omitempty"`
}

// ValidateProvisioning refuses a contract that could claim another identity or
// put a credential somewhere unspecified.
func ValidateProvisioning(contract Provisioning) error {
	if strings.TrimSpace(contract.Device.ID) == "" {
		return fmt.Errorf("provisioning has no device id")
	}
	switch contract.Device.Kind {
	case "node", "host", "service":
	default:
		return fmt.Errorf("provisioning device %q has unknown kind %q", contract.Device.ID, contract.Device.Kind)
	}
	if strings.TrimSpace(contract.Placement.Project) == "" {
		return fmt.Errorf("provisioning has no project")
	}
	if contract.Device.Kind == "node" && contract.Placement.Node == 0 {
		return fmt.Errorf("node provisioning has no node id")
	}
	for _, mode := range []string{contract.Permissions.Directory, contract.Permissions.Files} {
		if mode == "" {
			continue
		}
		if mode != "0700" && mode != "0600" && mode != "0750" && mode != "0640" {
			return fmt.Errorf("provisioning asks for an unsafe permission %q", mode)
		}
	}
	return nil
}
