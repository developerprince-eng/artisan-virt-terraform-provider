# hv01 bootstraps the cluster; hv02 and hv03 join it one after another
# (the control plane allows one membership change at a time).
resource "artisanvirt_cluster_member" "hv02" {
  cluster_id = artisanvirt_cluster.fsn1.id
  node_id    = artisanvirt_node.hv02.id
}

resource "artisanvirt_cluster_member" "hv03" {
  cluster_id = artisanvirt_cluster.fsn1.id
  node_id    = artisanvirt_node.hv03.id

  depends_on = [artisanvirt_cluster_member.hv02]
}
