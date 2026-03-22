resource "kafka_topic" "iam_user_events" {
  name               = "iam.user.event.v1"
  replication_factor = 1
  partitions         = 3

  config = {
    "cleanup.policy" = "delete"
    "retention.ms"   = "604800000" # 7 days
    "segment.ms"     = "86400000"  # 1 day
  }
}
