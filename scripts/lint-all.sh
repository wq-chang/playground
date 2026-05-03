#!/bin/bash

set -e

echo "=== Linting all services ==="
echo ""

# Lint Go services
echo "Linting Go services..."
cd services/go && golangci-lint run ./... && cd - >/dev/null
echo "✓ Go linting passed"
echo ""

# Lint Java services
echo "Linting Java services..."
cd services/java && mvn spotless:check && cd - >/dev/null
echo "✓ Java linting passed"
echo ""

# Lint Frontend
echo "Linting frontend..."
cd frontend && npm run lint && cd - >/dev/null
echo "✓ Frontend linting passed"
echo ""

echo "=== All services passed linting ==="
