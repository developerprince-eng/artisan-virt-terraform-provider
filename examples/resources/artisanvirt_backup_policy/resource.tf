resource "artisanvirt_backup_policy" "nightly" {
  name      = "nightly"
  schedule  = "0 2 * * *"
  target_id = artisanvirt_backup_target.nas.id
  keep_last = 7

  selector = {
    tags = { backup = "nightly" }
  }
}
