resource "keycloak_required_action" "required_update_password" {
  realm_id = keycloak_realm.playground_realm.id
  alias    = "UPDATE_PASSWORD"
  enabled  = true
  name     = "Update Password"
  config = {
    max_auth_age = "300"
  }
}

resource "keycloak_required_action" "required_configure_otp" {
  realm_id = keycloak_realm.playground_realm.id
  alias    = "CONFIGURE_TOTP"
  enabled  = true
  name     = "Configure OTP"
  config = {
    max_auth_age       = "300"
    add-recovery-codes = "false"
  }
}

resource "keycloak_required_action" "required_update_email" {
  realm_id = keycloak_realm.playground_realm.id
  alias    = "UPDATE_EMAIL"
  enabled  = true
  name     = "Update Email"
  config = {
    max_auth_age = "300"
    verifyEmail  = "false"
  }
}

resource "keycloak_required_action" "required_delete_account" {
  realm_id = keycloak_realm.playground_realm.id
  alias    = "delete_account"
  enabled  = true
  name     = "Delete Account"
}
