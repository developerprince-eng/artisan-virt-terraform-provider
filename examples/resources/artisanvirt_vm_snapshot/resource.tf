resource "artisanvirt_vm_snapshot" "before_upgrade" {
  vm_id       = artisanvirt_vm.web.id
  name        = "before-upgrade"
  description = "taken before the OS upgrade"
}
