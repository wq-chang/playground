#!/bin/bash
set -e # Exit immediately if a command fails

CUR_DIR="$(pwd)"

# Start Infrastructure
docker compose up -d

# Keycloak Terraform
echo "Applying Keycloak configurations..."
cd "$CUR_DIR/terraform/keycloak"
tflocal init -input=false
tflocal apply -var-file="secrets.tfvars" -auto-approve

# Kafka Terraform (Now that admin exists)
echo "Applying Kafka topics and ACLs..."
cd "$CUR_DIR/terraform/kafka"
tflocal init -input=false
tflocal apply -auto-approve

cd "$CUR_DIR"
echo "✓ Playground deployed successfully"
