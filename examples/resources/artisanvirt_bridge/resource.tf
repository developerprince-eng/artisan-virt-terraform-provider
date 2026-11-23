resource "artisanvirt_bridge" "lab" {
  node_id    = artisanvirt_node.hv01.id
  name       = "br-lab"
  mode       = "nat"
  ipv4       = "192.168.100.1/24"
  dhcp_start = "192.168.100.10"
  dhcp_end   = "192.168.100.250"
}
