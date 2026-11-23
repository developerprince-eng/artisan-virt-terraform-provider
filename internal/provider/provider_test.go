package provider

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

var testProviders = map[string]func() (tfprotov6.ProviderServer, error){
	"artisanvirt": providerserver.NewProtocol6WithError(New("test")()),
}

// needTerraform skips the Terraform-driven tests when no terraform binary is
// available (they also honour TF_ACC_TERRAFORM_PATH).
func needTerraform(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform CLI not found in PATH")
	}
}

func providerBlock(f *fakeAPI) string {
	return fmt.Sprintf(`
provider "artisanvirt" {
  endpoint      = %q
  token         = %q
  poll_interval = 1
}
`, f.server.URL, fakeToken)
}

func TestProviderSchemaValid(t *testing.T) {
	// Instantiating the protocol server validates every schema.
	srv, err := testProviders["artisanvirt"]()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.GetProviderSchema(t.Context(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		t.Errorf("schema diagnostic: %s: %s", d.Summary, d.Detail)
	}
	if got := len(resp.ResourceSchemas); got != 10 {
		t.Errorf("expected 10 resources, got %d", got)
	}
	if got := len(resp.DataSourceSchemas); got != 8 {
		t.Errorf("expected 8 data sources, got %d", got)
	}
}

func TestEnrollmentTokenAndBackups(t *testing.T) {
	needTerraform(t)
	f := newFakeAPI(t)
	cfg := func(keep int) string {
		return providerBlock(f) + fmt.Sprintf(`
resource "artisanvirt_enrollment_token" "hv" {
  description = "rack 3"
  ttl_minutes = 60
}

resource "artisanvirt_backup_target" "nfs" {
  name       = "nas01"
  type       = "nfs"
  path       = "/mnt/backups"
  nfs_server = "10.0.0.5"
  nfs_export = "/exports/vm"
}

resource "artisanvirt_backup_policy" "nightly" {
  name      = "nightly"
  schedule  = "0 2 * * *"
  target_id = artisanvirt_backup_target.nfs.id
  keep_last = %d
  selector = {
    tags = { backup = "nightly" }
  }
}
`, keep)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: cfg(7),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("artisanvirt_enrollment_token.hv", tfjsonpath.New("value"), knownvalue.StringRegexp(regexp.MustCompile(`^avt_`))),
					statecheck.ExpectKnownValue("artisanvirt_backup_policy.nightly", tfjsonpath.New("enabled"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue("artisanvirt_backup_policy.nightly", tfjsonpath.New("keep_last"), knownvalue.Int64Exact(7)),
				},
			},
			{
				// keep_last updates in place via PUT.
				Config: cfg(14),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("artisanvirt_backup_policy.nightly", plancheck.ResourceActionUpdate),
				}},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("artisanvirt_backup_policy.nightly", tfjsonpath.New("keep_last"), knownvalue.Int64Exact(14)),
				},
			},
			{
				ResourceName:      "artisanvirt_backup_policy.nightly",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestVMLifecycle(t *testing.T) {
	needTerraform(t)
	f := newFakeAPI(t)
	cfg := func(power string, vcpus int) string {
		return providerBlock(f) + fmt.Sprintf(`
resource "artisanvirt_bridge" "lab" {
  node_id = "node-1"
  name    = "br-lab"
  mode    = "nat"
  ipv4    = "192.168.100.1/24"
}

resource "artisanvirt_vm" "web" {
  name        = "web01"
  vcpus       = %d
  memory_mib  = 2048
  power_state = %q

  disks = [
    { size_gib = 20 },
    { size_gib = 50 },
  ]
  nics = [
    { bridge = artisanvirt_bridge.lab.name },
  ]
  tags = { role = "web" }
}

resource "artisanvirt_vm_snapshot" "base" {
  vm_id = artisanvirt_vm.web.id
  name  = "base"
}

data "artisanvirt_vm" "web" {
  name       = artisanvirt_vm.web.name
  depends_on = [artisanvirt_vm_snapshot.base]
}
`, vcpus, power)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{
			{
				Config: cfg("running", 2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("artisanvirt_vm.web", tfjsonpath.New("state"), knownvalue.StringExact("running")),
					statecheck.ExpectKnownValue("artisanvirt_vm.web", tfjsonpath.New("node_id"), knownvalue.StringExact("node-1")),
					statecheck.ExpectKnownValue("artisanvirt_vm.web", tfjsonpath.New("disks").AtSliceIndex(1).AtMapKey("device"), knownvalue.StringExact("vdb")),
					statecheck.ExpectKnownValue("artisanvirt_vm.web", tfjsonpath.New("nics").AtSliceIndex(0).AtMapKey("model"), knownvalue.StringExact("virtio")),
					statecheck.ExpectKnownValue("data.artisanvirt_vm.web", tfjsonpath.New("snapshots"), knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("base")})),
				},
			},
			{
				// Power state changes in place.
				Config: cfg("stopped", 2),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("artisanvirt_vm.web", plancheck.ResourceActionUpdate),
				}},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("artisanvirt_vm.web", tfjsonpath.New("state"), knownvalue.StringExact("stopped")),
				},
			},
			{
				// Hardware changes replace the VM (and its snapshot).
				Config: cfg("stopped", 4),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("artisanvirt_vm.web", plancheck.ResourceActionDestroyBeforeCreate),
				}},
			},
			{
				ResourceName:            "artisanvirt_vm.web",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
	if len(f.vms) != 0 {
		t.Errorf("expected all VMs destroyed, %d left", len(f.vms))
	}
}

func TestProviderMissingConfig(t *testing.T) {
	needTerraform(t)
	t.Setenv("ARTISANVIRT_ENDPOINT", "")
	t.Setenv("ARTISANVIRT_TOKEN", "")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testProviders,
		Steps: []resource.TestStep{{
			Config:      `data "artisanvirt_clusters" "all" {}`,
			ExpectError: regexp.MustCompile(`Missing Artisan Virt endpoint`),
		}},
	})
}
