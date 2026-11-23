data "artisanvirt_clusters" "all" {}

output "clusters_without_quorum" {
  value = [for c in data.artisanvirt_clusters.all.clusters : c.name if !c.has_quorum]
}
