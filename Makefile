.PHONY: help build build-go build-java build-frontend test test-go test-java test-frontend lint format clean local-up local-down dev

# Variables
SCRIPTS_DIR := scripts

# Default target - show help
help:
	@echo "Makefile targets for playground project"
	@echo ""
	@echo "Build Targets:"
	@echo "  make build              - Build all services (Go, Java, frontend)"
	@echo "  make build-go           - Build Go services"
	@echo "  make build-java         - Build Java services"
	@echo "  make build-frontend     - Build frontend"
	@echo ""
	@echo "Test Targets:"
	@echo "  make test               - Run tests for all services"
	@echo "  make test-go            - Run Go service tests"
	@echo "  make test-java          - Run Java service tests"
	@echo "  make test-frontend      - Run frontend tests"
	@echo ""
	@echo "Lint & Format Targets:"
	@echo "  make lint               - Lint all services"
	@echo "  make format             - Format all code"
	@echo ""
	@echo "Development Targets:"
	@echo "  make dev                - Build all and start local dev environment"
	@echo "  make local-up           - Start local dev environment (docker compose)"
	@echo "  make local-down         - Stop local dev environment"
	@echo ""
	@echo "Utility Targets:"
	@echo "  make clean              - Clean all build artifacts"
	@echo "  make help               - Show this help message"

# ============================================================================
# BUILD TARGETS
# ============================================================================

# Build all services
build: build-go build-java build-frontend
	@echo "✓ All services built successfully"

# Build Go services
build-go:
	@echo "Building Go services..."
	@cd services/go && go build ./... && cd - > /dev/null
	@echo "✓ Go services built successfully"

# Build Java services
build-java:
	@echo "Building Java services..."
	@cd services/java && mvn clean package -DskipTests && cd - > /dev/null
	@echo "✓ Java services built successfully"

# Build frontend
build-frontend:
	@echo "Building frontend..."
	@cd frontend && npm install && npm run build && cd - > /dev/null
	@echo "✓ Frontend built successfully"

# ============================================================================
# TEST TARGETS
# ============================================================================

# Run all tests
test: test-go test-java test-frontend
	@echo "✓ All tests passed"

# Test Go services
test-go:
	@echo "Testing Go services..."
	@cd services/go && go test ./... && cd - > /dev/null
	@echo "✓ Go tests passed"

# Test Java services
test-java:
	@echo "Testing Java services..."
	@cd services/java && mvn test && cd - > /dev/null
	@echo "✓ Java tests passed"

# Test frontend
test-frontend:
	@echo "Testing frontend..."
	@cd frontend && npm test && cd - > /dev/null
	@echo "✓ Frontend tests passed"

# ============================================================================
# LINT & FORMAT TARGETS
# ============================================================================

# Lint all services
lint:
	@bash $(SCRIPTS_DIR)/lint-all.sh

# Format all code
format:
	@bash $(SCRIPTS_DIR)/format-all.sh

# ============================================================================
# DEVELOPMENT TARGETS
# ============================================================================

# Start local dev environment (docker compose)
local-up:
	@echo "Starting local dev environment..."
	@cd localmock && docker compose up -d && cd - > /dev/null
	@echo "✓ Local dev environment started"

# Stop local dev environment
local-down:
	@echo "Stopping local dev environment..."
	@cd localmock && docker compose down && cd - > /dev/null
	@echo "✓ Local dev environment stopped"

# Build all services and start local environment
dev: build local-up
	@echo "✓ Dev environment ready"

# ============================================================================
# UTILITY TARGETS
# ============================================================================

# Clean all build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@cd services/go && go clean ./... && cd - > /dev/null
	@echo "✓ Go artifacts cleaned"
	@cd services/java && mvn clean && cd - > /dev/null
	@echo "✓ Java artifacts cleaned"
	@cd frontend && rm -rf dist build && cd - > /dev/null
	@echo "✓ Frontend artifacts cleaned"
	@rm -rf target
	@echo "✓ All build artifacts cleaned"
