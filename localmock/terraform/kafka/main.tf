terraform {
  required_providers {
    kafka = {
      source  = "Mongey/kafka"
      version = "~> 0.13"
    }
  }
  required_version = "~> 1.14"
}

variable "kafka_admin_password" {
  description = "Kafka admin secret"
  type        = string
  sensitive   = true
}

variable "kafka_keycloak_password" {
  description = "Kafka admin secret"
  type        = string
  sensitive   = true
}

resource "kafka_user_scram_credential" "admin" {
  username        = "admin"
  scram_mechanism = "SCRAM-SHA-512"
  password        = var.kafka_admin_password
}

resource "kafka_user_scram_credential" "keycloak" {
  username        = "keycloak"
  scram_mechanism = "SCRAM-SHA-512"
  password        = var.kafka_keycloak_password
}

provider "kafka" {
  bootstrap_servers = ["localhost:9092"]
  sasl_username     = "admin"
  sasl_password     = var.kafka_admin_password
  sasl_mechanism    = "scram-sha512"
  tls_enabled       = false
}

