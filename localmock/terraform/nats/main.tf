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

resource "jetstream_stream" "user_event" {
  name      = "USER_EVENT"
  subjects  = ["USER_EVENT.*"]
  storage   = "file"
  max_age   = 60 * 60 * 24 * 365
  retention = "workqueue"
}

resource "jetstream_consumer" "user_event_consumer" {
  stream_id    = jetstream_stream.user_event.id
  durable_name = "ALL"
  deliver_all  = true
  sample_freq  = 100
}
