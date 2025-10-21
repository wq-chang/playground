resource "keycloak_realm" "playground_realm" {
  realm                = "playground"
  registration_allowed = true
}

resource "keycloak_default_roles" "default_roles" {
  realm_id = keycloak_realm.playground_realm.id
  default_roles = [
    "account/manage-account",
    "account/view-profile",
    "uma_authorization",
    "offline_access",
  ]
}

resource "keycloak_realm_events" "realm_events" {
  realm_id = keycloak_realm.playground_realm.id

  events_enabled    = true
  events_expiration = 3600

  admin_events_enabled         = true
  admin_events_details_enabled = true

  # When omitted or left empty, keycloak will enable all event types
  enabled_event_types = [
    "REGISTER",
    "UPDATE_PROFILE",
    "UPDATE_EMAIL",
    "DELETE_ACCOUNT",
  ]

  events_listeners = [
    "jboss-logging", # keycloak enables the 'jboss-logging' event listener by default.
    "user-event-listener"
  ]
}
