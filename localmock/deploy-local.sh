#!/bin/bash

CUR_DIR="$(pwd)"
# cd "$CUR_DIR"/terraform/aws
# tflocal destroy -auto-approve && tflocal apply -auto-approve

# Keycloak
cd "$CUR_DIR/terraform/keycloak"
if ! tflocal providers >/dev/null 2>&1; then
	tflocal init
fi
tflocal apply -var-file="secrets.tfvars" -auto-approve

# NATS
cd "$CUR_DIR/terraform/nats"
if ! tflocal providers >/dev/null 2>&1; then
	tflocal init
fi
tflocal apply -auto-approve

cd "$CUR_DIR"
echo "✓ Terraform configurations applied successfully"
