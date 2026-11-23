resource "artisanvirt_vm" "web" {
  name       = "web01"
  cluster_id = artisanvirt_cluster.fsn1.id
  vcpus      = 4
  memory_mib = 8192
  firmware   = "uefi"

  disks = [
    { size_gib = 40, source_image = "/var/lib/libvirt/images/templates/rocky-10.qcow2" },
    { size_gib = 200 },
  ]

  nics = [
    { bridge = "br-lab" },
  ]

  tags = {
    role   = "web"
    backup = "nightly"
  }

  power_state = "running"
}
