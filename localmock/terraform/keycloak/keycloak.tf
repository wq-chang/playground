resource "keycloak_realm" "playground_realm" {
  realm                = "playground"
  registration_allowed = true
}

resource "keycloak_openid_client" "fe_client" {
  realm_id                        = keycloak_realm.playground_realm.id
  client_id                       = "fe"
  name                            = "Frontend"
  access_type                     = "PUBLIC"
  standard_flow_enabled           = true
  pkce_code_challenge_method      = "S256"
  valid_redirect_uris             = ["http://localhost:5173/", "http://localhost:5173/silent-check-sso.html"]
  web_origins                     = ["http://localhost:5173"]
  valid_post_logout_redirect_uris = ["http://localhost:5173/"]
}
