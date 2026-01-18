#!/usr/bin/env bash
set -e

ENVIRONMENT="$1"
shift # remove env argument
GOOSE_ARGS="$@"

if [[ -z "$ENVIRONMENT" ]]; then
	echo "Usage: $0 <environment> <goose commands>"
	exit 1
fi

if [[ -z "$GOOSE_ARGS" ]]; then
	echo "No goose command specified."
	echo "Example: $0 dev up"
	echo "Or:      $0 dev -v up"
	exit 1
fi

if [[ "$ENVIRONMENT" == "dev" ]]; then
	echo "Using local dev DB config"
	DBSTRING="postgres://backend:password@localhost:5432/backend?sslmode=disable"
else
	echo "Environment '$ENVIRONMENT' not implemented yet"
	exit 1

	# echo "Fetching DB credentials from AWS Secrets Manager..."
	#
	# SECRET_NAME="backend/${ENVIRONMENT}/db"
	#
	# SECRET_JSON=$(aws secretsmanager get-secret-value \
	# 	--secret-id "$SECRET_NAME" \
	# 	--query SecretString \
	# 	--output text)
	#
	# DB_USER=$(echo "$SECRET_JSON" | jq -r '.username')
	# DB_PASS=$(echo "$SECRET_JSON" | jq -r '.password')
	# DB_HOST=$(echo "$SECRET_JSON" | jq -r '.host')
	# DB_PORT=$(echo "$SECRET_JSON" | jq -r '.port')
	# DB_NAME=$(echo "$SECRET_JSON" | jq -r '.dbname')
	#
	# DBSTRING="postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=require"
fi

echo "Running goose migration..."

env \
	GOOSE_DRIVER=postgres \
	GOOSE_DBSTRING="$DBSTRING" \
	GOOSE_MIGRATION_DIR=./migrations \
	GOOSE_TABLE=goose \
	goose $GOOSE_ARGS
