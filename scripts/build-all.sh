#!/bin/bash

set -e

echo "=== Building all services ==="

# Build Go services
echo "Building Go services..."
cd services/go && go build ./... && cd - >/dev/null
echo "✓ Go services built successfully"

# Build Java services
echo "Building Java services..."
cd services/java && mvn clean package -DskipTests && cd - >/dev/null
echo "✓ Java services built successfully"

# Build frontend
echo "Building frontend..."
cd frontend && npm install && npm run build && cd - >/dev/null
echo "✓ Frontend built successfully"

echo ""
echo "=== All services built successfully ==="
