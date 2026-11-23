data "artisanvirt_nodes" "ready" {
  state = "ready"
}

output "ready_nodes" {
  value = data.artisanvirt_nodes.ready.nodes[*].name
}
