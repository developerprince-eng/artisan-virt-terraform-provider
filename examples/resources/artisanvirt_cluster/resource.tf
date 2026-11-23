resource "artisanvirt_cluster" "fsn1" {
  name             = "fsn1-prod"
  location         = "fsn1"
  initial_node_id  = artisanvirt_node.hv01.id
  ring_subnet      = "10.10.0.0/24"
  storage_strategy = "replicated"

  fencing_enabled = true
  fencing_agent   = "fence_ipmilan"

  timeouts {
    create = "90m"
  }
}
