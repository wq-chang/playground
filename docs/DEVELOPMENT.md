# Local Development Setup Guide

## Prerequisites

Ensure you have the following installed before starting:

- **Go** 1.26 or higher
- **Java** 21 or higher (for services in `services/java`)
- **Docker** and **Docker Compose**
- **Node.js** (v24+) and npm
- **Git**

## Quick Start

1. **Clone the repository** (if not already done):

   ```bash
   git clone <repository-url>
   cd playground
   ```

2. **Review localmock setup** - Check `localmock/` for local mock service configuration

3. **Start Docker services**:

   ```bash
   docker compose up -d
   ```

4. **Run individual services** - Follow the steps in the next section

## Running Services Locally

### Go Services

```bash
cd services/go
go run ./backend/cmd/backend
```

### Java Services

```bash
cd services/java
mvn compile exec:java
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

The frontend will typically run on `http://localhost:5173` by default (Vite development server).

## Common Development Tasks

### Database Migrations

Check the service-specific `migrations/` directories. Run migrations using your service's typical approach (Go migrations via `migrate`, Java via Flyway, etc.).

### Code Generation

- **sqlc**: Generate type-safe database queries in Go services
- **Protocol Buffers**: Generate gRPC and data serialization code
- **OpenAPI**: Generate API documentation and client stubs

Run code generation tools as documented in individual service READMEs.

### Running Tests

```bash
# Go tests
cd services/go && go test ./...

# Java tests
cd services/java && mvn test

# Frontend tests (with passWithNoTests since we're still exploring)
cd frontend && npm test -- run --passWithNoTests
```

### Debugging

- **Go**: Use `dlv` debugger or IDE breakpoints (VSCode, GoLand)
- **Java**: Use IDE debugger or `mvn jdwp:execute`
- **Frontend**: Use browser DevTools or IDE debugger

## Environment Setup

- Review `.envrc` files in the root and service directories for development environment variables
- Check `localmock/` configuration for running local mocks of external dependencies
- Services typically use environment-based configuration for database connections, API endpoints, and logging levels

## Troubleshooting

**Port conflicts**: Ensure ports (5173, 2080, 7777, 5432, 9092) are not in use:

```bash
lsof -i :<port>  # Check if port is in use
```

**Database connection errors**: Verify Docker services are running:

```bash
docker compose ps  # List running containers
```

**Go module issues**: Clear module cache if experiencing stale dependencies:

```bash
go clean -modcache
```

**Java compilation errors**: Ensure Java 21+ is in use:

```bash
java -version
```

## Additional Resources

- [`SERVICES.md`](./SERVICES.md) - Complete service catalog, tech stacks, dependencies, and APIs
- [`ARCHITECTURE.md`](./ARCHITECTURE.md) - System design and component interactions
- [`services/README.md`](../services/README.md) - Backend services navigation
- [`localmock/README.md`](../localmock/README.md) - Local mock setup and configuration
- Individual service READMEs in `services/go`, `services/java`, and `frontend/`
