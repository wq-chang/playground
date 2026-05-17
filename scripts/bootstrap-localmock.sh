#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"
LOCALMOCK_DIR="$REPO_ROOT/localmock"

cd "$LOCALMOCK_DIR"

docker compose up kafka -d

echo "Applying Kafka topics and ACLs..."
cd "$LOCALMOCK_DIR/terraform/kafka"
tflocal init -input=false
tflocal apply -auto-approve

cd "$LOCALMOCK_DIR"
docker compose up -d

echo "Waiting for Keycloak to be fully operational..."
until curl -s --head -f http://localhost:9000/health/ready >/dev/null; do
  echo "Keycloak is still starting... (retrying in 5s)"
  sleep 5
done

echo "Keycloak is ready."

echo "Applying Keycloak configurations..."
cd "$LOCALMOCK_DIR/terraform/keycloak"
tflocal init -input=false
tflocal apply -auto-approve

echo "Playground localmock bootstrap completed."
