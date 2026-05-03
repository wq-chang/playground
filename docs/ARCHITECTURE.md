# Architecture

## Monorepo Overview

A polyglot monorepo combining Go backends, Java services, and a React frontend. Includes Keycloak for authentication and local development environment with Docker Compose.

## Directory Structure

```
services/
├── go/                          # Go services monorepo
│   ├── backend/                 # Core data layer & business logic
│   ├── bff/                     # API Gateway/Backend-for-Frontend
│   ├── library/                 # Shared Go utilities & generated code
│   ├── scripts/                 # Build and deployment automation
│   └── go.mod                   # Shared Go workspace
├── java/                        # Java services monorepo
│   ├── keycloak-custom/         # Keycloak SPI implementations
│   └── pom.xml                  # Parent POM for Maven
frontend/                        # React + TypeScript UI
├── package.json                 # NPM dependencies
├── src/                         # UI source code
└── vite.config.ts               # Vite bundler config
localmock/                       # Local development environment
├── docker-compose.yml           # Multi-service orchestration
└── scripts/                     # Startup & configuration
docs/                            # Architecture & operational docs
flake.nix                        # Nix development environment
```

## Services Architecture

### Go Services (`services/go/`)

**Backend** (`backend/`):

- Core business logic and data access layer
- Database models, migrations, and queries
- gRPC services for inter-service communication
- Run as: `./backend --config=...`

**BFF - Backend-for-Frontend** (`bff/`):

- RESTful API gateway for frontend clients
- Request validation, transformation, and authentication
- Routes requests to backend or external services

**Library** (`library/`):

- Shared protobuf definitions (gRPC contracts)
- Common utilities (logging, error handling, middleware)
- Generated code and type definitions

### Java Services (`services/java/`)

**Keycloak Custom SPIs**:

- Event listeners for authentication/authorization flows
- Custom realm event handlers
- User lifecycle hooks
- Built with Maven, deployed as JAR plugin to Keycloak

**Future Extensions**:

- Reporting service (scheduled batch processing)
- Theme service (dynamic UI themes)

### Frontend (`frontend/`)

- **Framework**: React 19 with TypeScript
- **Build Tool**: Vite (ESM, HMR in development)
- **Bundler**: Outputs optimized static assets
- **API Client**: Axios or fetch-based client pointing to BFF
- **State**: Zustand

## Data Flow

```
Frontend (React)
  ↓ (REST)
BFF (Go)
  ├→ gRPC → Backend (Go)
  └→ HTTP → Keycloak (Java)
     ↓
Backend (Go) → Database
Backend ↔ Event Bus (Kafka-ready) ↔ Event Handlers
```

**Communication Patterns**:

- **Frontend ↔ BFF**: RESTful HTTP with JSON
- **BFF ↔ Backend**: gRPC for performance & type safety
- **Services ↔ Keycloak**: HTTP or event-driven
- **Event Distribution**: Async messages (Kafka) for state changes

## Dependency Management

### Go Services

- **Workspace Model**: Single `services/go/go.mod` manages all Go modules
- **Modules**: `backend/`, `bff/`, and `library/` as local modules
- **Lock File**: `go.sum` ensures reproducible builds
- **Update**: `go get -u ./...` from `services/go/`

### Java Services

- **Parent POM**: `services/java/pom.xml` coordinates versions
- **Module Structure**: `pom.xml` in each service directory
- **Dependency Inheritance**: Child modules inherit parent configuration
- **Build**: `mvn clean install` from `services/java/`

### Frontend

- **Package Manager**: npm with `package-lock.json`
- **Dependency Scope**: Independent from backend services
- **Updates**: `npm update` in `frontend/`

## Local Development

The **`localmock/`** directory provides a complete local environment:

- **docker-compose.yml**: Orchestrates all services (backend, bff, Keycloak, database, optional Kafka)
- **Setup**: `docker compose up` creates isolated, reproducible environment

Start with: `cd localmock && docker compose up`

## Scaling Considerations

**Current**: Simple microservices deployed as containers

**Future Evolution**:

- **Service Mesh**: Istio or Linkerd for traffic management and observability
- **Code Generation**: OpenAPI specs and Protocol Buffers for stronger contracts
- **Observability**: Distributed tracing (OpenTelemetry), metrics (Prometheus), logs (ELK)
- **Testing**: Contract tests between services, end-to-end integration tests in localmock
