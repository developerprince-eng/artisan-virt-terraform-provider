resource "artisanvirt_enrollment_token" "rack3" {
  description = "hypervisors in rack 3"
  ttl_minutes = 120
}

# Hand the secret to your host bootstrap (cloud-init, Ansible, ...).
output "enrollment_token" {
  value     = artisanvirt_enrollment_token.rack3.value
  sensitive = true
}
