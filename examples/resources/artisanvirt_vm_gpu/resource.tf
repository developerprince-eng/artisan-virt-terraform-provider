resource "artisanvirt_vm" "ml" {
  name        = "ml01"
  node_id     = artisanvirt_node.gpu01.id
  vcpus       = 16
  memory_mib  = 65536
  firmware    = "uefi"
  disks       = [{ size_gib = 200 }]
  power_state = "stopped" # GPUs attach only to stopped VMs
}

resource "artisanvirt_vm_gpu" "a100" {
  vm_id   = artisanvirt_vm.ml.id
  address = "0000:41:00.0"
}
