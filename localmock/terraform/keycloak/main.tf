terraform {
  required_providers {
    keycloak = {
      source  = "keycloak/keycloak"
      version = "~> 5.0"
    }
  }

  required_version = "~> 1.12"
}

variable "keycloak_client_secret" {
  description = "Keycloak client secret"
  type        = string
  sensitive   = true
}

provider "keycloak" {
  client_id     = "terraform"
  client_secret = var.keycloak_client_secret
  url           = "http://localhost:7777"
}
