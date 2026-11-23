# Changelog

All notable changes to this provider are documented here. Versions follow
[Semantic Versioning](https://semver.org/); release tags are `vX.Y.Z`.

## Unreleased

### Added

- Provider configuration: `endpoint`, `token`, `insecure`, `ca_cert`, `request_timeout`, `poll_interval`
  (with `ARTISANVIRT_*` environment variables).
- Resources: `artisanvirt_enrollment_token`, `artisanvirt_node`, `artisanvirt_bridge`, `artisanvirt_cluster`,
  `artisanvirt_cluster_member`, `artisanvirt_vm`, `artisanvirt_vm_snapshot`, `artisanvirt_vm_gpu`,
  `artisanvirt_backup_target`, `artisanvirt_backup_policy`.
- Data sources: `artisanvirt_node`, `artisanvirt_nodes`, `artisanvirt_cluster`, `artisanvirt_clusters`,
  `artisanvirt_vm`, `artisanvirt_vms`, `artisanvirt_backup_targets`, `artisanvirt_backups`.
