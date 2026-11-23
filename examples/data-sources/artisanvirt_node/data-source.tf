data "artisanvirt_node" "hv01" {
  name = "hv01"
}

output "hv01_free_pools" {
  value = { for p in data.artisanvirt_node.hv01.storage_pools : p.name => p.available_bytes }
}
