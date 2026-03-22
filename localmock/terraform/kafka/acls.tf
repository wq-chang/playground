# Allow the 'keycloak' user to WRITE to the user events topic
resource "kafka_acl" "keycloak_writer" {
  resource_name       = kafka_topic.iam_user_events.name
  resource_type       = "Topic"
  acl_principal       = "User:keycloak"
  acl_host            = "*" # In prod, restrict this to the Keycloak IP/Subnet
  acl_operation       = "Write"
  acl_permission_type = "Allow"
}

# Required: Allow Keycloak to DESCRIBE the topic to get metadata
resource "kafka_acl" "keycloak_describe" {
  resource_name       = kafka_topic.iam_user_events.name
  resource_type       = "Topic"
  acl_principal       = "User:keycloak"
  acl_host            = "*"
  acl_operation       = "Describe"
  acl_permission_type = "Allow"
}
