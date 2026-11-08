package schema

import "testing"

func validProvisioning() Provisioning {
	return Provisioning{
		Device:    ProvisioningDevice{ID: "garden-01", Kind: "node", Board: "esp32c3"},
		Placement: ProvisioningPlacement{Project: "default", Area: 1, Node: 7},
		Keeper:    ProvisioningKeeper{URL: "https://keeper.local:19090"},
		Secrets:   ProvisioningSecrets{Generate: []string{"instance_id", "identity_key"}},
		Permissions: ProvisioningPermissions{
			Directory: "0700", Files: "0600",
		},
	}
}

func TestProvisioning(t *testing.T) {
	if err := ValidateProvisioning(validProvisioning()); err != nil {
		t.Fatalf("valid contract refused: %v", err)
	}
	contract := validProvisioning()
	contract.Device.ID = ""
	if err := ValidateProvisioning(contract); err == nil {
		t.Fatal("missing device id was accepted")
	}
	contract = validProvisioning()
	contract.Device.Kind = "container"
	if err := ValidateProvisioning(contract); err == nil {
		t.Fatal("unknown kind was accepted")
	}
	contract = validProvisioning()
	contract.Permissions.Files = "0777"
	if err := ValidateProvisioning(contract); err == nil {
		t.Fatal("world-writable secret mode was accepted")
	}
}
