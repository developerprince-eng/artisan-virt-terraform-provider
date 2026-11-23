data "artisanvirt_vms" "prod" {
  cluster_id = data.artisanvirt_cluster.prod.id
}
