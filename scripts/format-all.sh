#!/bin/bash

echo "=== Formatting code across all services ==="
echo ""

# Format Go services
echo "Formatting Go services..."
cd services/go && gofmt -w . && golangci-lint run --fix && cd - >/dev/null
echo "✓ Go code formatted"
echo ""

# Format Java services
echo "Formatting Java services..."
cd services/java && mvn spotless:apply && cd - >/dev/null
echo "✓ Java code formatted"
echo ""

# Format Frontend
# TODO: Add Prettier or ESLint formatting for frontend code
# echo "Formatting frontend..."
# cd frontend && npm run format && cd - > /dev/null
# echo "✓ Frontend code formatted"
# echo ""

echo "=== Code formatting completed ==="
