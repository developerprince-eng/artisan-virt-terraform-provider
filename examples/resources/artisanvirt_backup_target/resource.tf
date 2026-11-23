resource "artisanvirt_backup_target" "nas" {
  name        = "nas01"
  type        = "nfs"
  path        = "/mnt/artisan-backups"
  nfs_server  = "10.0.0.5"
  nfs_export  = "/exports/vm-backups"
  cluster_ids = [artisanvirt_cluster.fsn1.id]
}
