#!/bin/bash

CUR_DIR="$(pwd)"
# cd "$CUR_DIR"/terraform/aws
# tflocal destroy -auto-approve && tflocal apply -auto-approve
cd "$CUR_DIR"/terraform/keycloak
tflocal apply -var-file="secrets.tfvars" -auto-approve
cd "$CUR_DIR"/terraform/nats
tflocal apply -auto-approve
cd "$CUR_DIR"
echo "✓ Terraform configurations applied successfully"
