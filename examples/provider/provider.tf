terraform {
  required_providers {
    artisanvirt = {
      source  = "developerprince/artisanvirt"
      version = "~> 0.1"
    }
  }
}

# The endpoint and token can also come from ARTISANVIRT_ENDPOINT and
# ARTISANVIRT_TOKEN, which keeps the admin token out of configuration files.
provider "artisanvirt" {
  endpoint = "https://artisan.example.com"
}
