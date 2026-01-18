
variable "keycloak_bff_client_secret" {
  description = "Bff client secret"
  type        = string
  sensitive   = true
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

resource "keycloak_openid_client" "bff_client" {
  realm_id                 = keycloak_realm.playground_realm.id
  client_id                = "bff"
  name                     = "Backend for frontend"
  access_type              = "CONFIDENTIAL"
  client_secret_wo         = var.keycloak_bff_client_secret
  client_secret_wo_version = "1"
  standard_flow_enabled    = true
  valid_redirect_uris      = ["http://localhost:7777/callback"]
}
