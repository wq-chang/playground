#!/bin/bash

CUR_DIR="$(pwd)"
source "$CUR_DIR"/build.sh
# cd "$CUR_DIR"/terraform/aws
# tflocal destroy -auto-approve && tflocal apply -auto-approve
cd "$CUR_DIR"/terraform/keycloak
tflocal apply -var-file="secrets.tfvars" -auto-approve
cd "$CUR_DIR"/terraform/nats-js
tflocal apply -auto-approve
cd "$CUR_DIR"
echo "✓ Terraform configurations applied successfully"
