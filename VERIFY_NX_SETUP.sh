#!/bin/bash
echo "🔍 Verifying Nx CI/CD Setup..."
echo ""

cd "$(git rev-parse --show-toplevel)" || exit 1

# Check Dockerfiles
echo "📦 Dockerfiles:"
[ -f .docker/Dockerfile.go ] && echo "  ✓ Dockerfile.go" || echo "  ✗ Dockerfile.go MISSING"
[ -f .docker/Dockerfile.java ] && echo "  ✓ Dockerfile.java" || echo "  ✗ Dockerfile.java MISSING"
[ -f .docker/Dockerfile.frontend ] && echo "  ✓ Dockerfile.frontend" || echo "  ✗ Dockerfile.frontend MISSING"
[ -f .docker/nginx.conf ] && echo "  ✓ nginx.conf" || echo "  ✗ nginx.conf MISSING"
echo ""

# Check project.json files
echo "🔧 Project Configurations:"
for proj in frontend services/go/backend services/go/bff services/java services/go/library; do
  if [ -f "$proj/project.json" ]; then
    if grep -q '"docker"' "$proj/project.json"; then
      echo "  ✓ $proj/project.json (has docker targets)"
    else
      echo "  ✓ $proj/project.json (no docker targets)"
    fi
  else
    echo "  ✗ $proj/project.json MISSING"
  fi
done
echo ""

# Check CI workflow
echo "📋 CI Workflow:"
if [ -f .github/workflows/ci.yml ]; then
  lines=$(wc -l < .github/workflows/ci.yml)
  echo "  ✓ .github/workflows/ci.yml ($lines lines)"
  if grep -q "detect-and-test" .github/workflows/ci.yml; then
    echo "    ✓ Unified job found"
  fi
  if grep -q "nx-cache" .github/workflows/ci.yml; then
    echo "    ✓ Nx cache enabled"
  fi
else
  echo "  ✗ .github/workflows/ci.yml MISSING"
fi
echo ""

# Check CD workflow
echo "🚀 CD Workflow:"
if [ -f .github/workflows/cd.yml ]; then
  echo "  ✓ .github/workflows/cd.yml (template ready)"
else
  echo "  ✗ .github/workflows/cd.yml MISSING"
fi
echo ""

# Test Nx commands
echo "⚙️  Nx Commands:"
if npx nx show projects > /dev/null 2>&1; then
  count=$(npx nx show projects 2>/dev/null | wc -l)
  echo "  ✓ Nx projects recognized ($count projects)"
else
  echo "  ✗ Nx not working"
fi
echo ""

echo "✅ Verification complete!"
