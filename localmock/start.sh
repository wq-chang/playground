#!/bin/bash
set -e # Exit immediately if a command fails

CUR_DIR="$(pwd)"

# Start kafka first
docker compose up kafka -d

# Kafka Terraform
echo "Applying Kafka topics and ACLs..."
cd "$CUR_DIR/terraform/kafka"
tflocal init -input=false
tflocal apply -auto-approve

# Start Infrastructure
docker compose up -d

# Wait for Keycloak to be ready
echo "Waiting for Keycloak to be fully operational..."
until curl -s --head -f http://localhost:9000/health/ready >/dev/null; do
	echo "Keycloak is still starting... (retrying in 5s)"
	sleep 5
done

echo "✓ Keycloak is ready!"

# Keycloak Terraform
echo "Applying Keycloak configurations..."
cd "$CUR_DIR/terraform/keycloak"
tflocal init -input=false
tflocal apply -var-file="secrets.tfvars" -auto-approve

cd "$CUR_DIR"
echo "✓ Playground deployed successfully"
