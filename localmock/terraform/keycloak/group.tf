data "keycloak_openid_client" "account" {
  realm_id  = keycloak_realm.playground_realm.id
  client_id = "account"
}
data "keycloak_role" "delete_account" {
  realm_id  = keycloak_realm.playground_realm.id
  client_id = data.keycloak_openid_client.account.id
  name      = "delete-account"
}

resource "keycloak_group" "default" {
  realm_id = keycloak_realm.playground_realm.id
  name     = "Default Group"
}

resource "keycloak_group_roles" "default" {
  realm_id = keycloak_realm.playground_realm.id
  group_id = keycloak_group.default.id
  role_ids = [
    data.keycloak_role.delete_account.id
  ]
}

resource "keycloak_default_groups" "default" {
  realm_id = keycloak_realm.playground_realm.id
  group_ids = [
    keycloak_group.default.id
  ]
}
