#!/bin/bash

echo "=== Running tests for all services ==="
echo ""

FAILED=0

# Test Go services
echo "Testing Go services..."
if cd services/go && go test ./...; then
	echo "✓ Go tests passed"
else
	echo "✗ Go tests failed"
	FAILED=$((FAILED + 1))
fi
cd - >/dev/null
echo ""

# Test Java services
echo "Testing Java services..."
if cd services/java && mvn test; then
	echo "✓ Java tests passed"
else
	echo "✗ Java tests failed"
	FAILED=$((FAILED + 1))
fi
cd - >/dev/null
echo ""

# Test Frontend
echo "Testing frontend..."
if cd frontend && npm test; then
	echo "✓ Frontend tests passed"
else
	echo "✗ Frontend tests failed"
	FAILED=$((FAILED + 1))
fi
cd - >/dev/null
echo ""

# Summary
echo "=== Test Summary ==="
if [ $FAILED -eq 0 ]; then
	echo "All tests passed!"
	exit 0
else
	echo "$FAILED test suite(s) failed"
	exit 1
fi
