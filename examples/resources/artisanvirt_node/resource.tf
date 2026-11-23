# Approve every host that enrolled and is waiting for approval.
data "artisanvirt_nodes" "discovered" {
  state = "discovered"
}

resource "artisanvirt_node" "hv" {
  for_each = { for n in data.artisanvirt_nodes.discovered.nodes : n.name => n.id }

  node_id     = each.value
  maintenance = false

  # Remove the agent and drop the node from inventory on destroy.
  decommission_on_destroy = true
}
