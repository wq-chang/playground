terraform {
  required_providers {
    jetstream = {
      source  = "nats-io/jetstream"
      version = "~> 0.2"
    }
  }
  required_version = "~> 1.13"
}

provider "jetstream" {
  servers = "http://localhost:4222"
}

resource "jetstream_kv_bucket" "session_store" {
  name = "session"
}
