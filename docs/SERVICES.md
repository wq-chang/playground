# Services Catalog

This document describes all services in the monorepo, their technologies, responsibilities, and integration points.

---

## Frontend

**Technology**: React 19 + TypeScript + Vite

**Description**:
The user-facing single-page application built with React and TypeScript. Provides the UI for the entire system.

**Location**: `/frontend`

**Port**: 5173 (development)

**Key Features**:

- Modern SPA architecture with React Router (TanStack Router)
- TypeScript for type safety
- Tailwind CSS for styling
- Vite for fast development and builds
- ESLint + Prettier for code quality

**How to Run**:

```bash
cd frontend
npm install
npm run dev    # Development server at localhost:5173
npm run build  # Production build
npm run test   # Run tests with vitest
```

**Dependencies**:

- React 19
- TanStack Router
- Vite
- TypeScript
- Tailwind CSS

**Integration**:

- Makes HTTP/REST calls to BFF at `localhost:2080`
- Sends authentication requests to BFF (which validates with Keycloak)

---

## BFF (Backend for Frontend)

**Technology**: Go + net/http

**Description**:
The API gateway that sits between the frontend and backend services. Handles CORS, authentication middleware, and proxies requests to the backend.

**Location**: `/services/go/bff`

**Port**: 2080

**Key Features**:

- Go's standard `net/http` library for HTTP routing
- CORS middleware for frontend requests
- JWT token validation with Keycloak
- Request/response logging
- Error handling and transformation

**How to Run**:

```bash
cd services/go
go run ./bff/cmd  # Run BFF server
```

**Dependencies**:

- Keycloak Go library (from library package)
- Kafka client (franz-go)

**Environment Variables**:

- `KEYCLOAK_URL`: Keycloak server URL
- `KEYCLOAK_CLIENT_ID`: OAuth client ID
- `KEYCLOAK_CLIENT_SECRET`: OAuth client secret
- `KAFKA_BROKERS`: Comma-separated Kafka broker addresses
- `LOG_LEVEL`: Logging level (debug, info, warn, error)

**Integration**:

- Receives requests from Frontend (localhost:5173)
- Validates tokens with Keycloak (localhost:7777)
- Forwards requests to Backend service
- Publishes events to Kafka for event-driven features

---

## Backend Service

**Technology**: Go + SQLC + PostgreSQL

**Description**:
The core business logic service handling domain operations, data persistence, and Kafka event consumption.

**Location**: `/services/go/backend`

**Port**: 8080 (internal only)

**Key Features**:

- SQL-first approach with SQLC for type-safe queries
- PostgreSQL as primary data store
- Kafka consumer for event-driven workflows
- Database migrations managed with goose
- Comprehensive error handling
- Structured logging

**How to Run**:

```bash
cd services/go
go run ./backend/cmd  # Run Backend server
```

**Environment Variables**:

- `DATABASE_URL`: PostgreSQL connection string (postgres://user:password@localhost:5432/dbname)
- `KAFKA_BROKERS`: Comma-separated Kafka broker addresses
- `LOG_LEVEL`: Logging level (debug, info, warn, error)

**Database**:

- Name: `playground`
- Host: `localhost`
- Port: `5432`
- All migrations in `/services/go/backend/migrations`

**Integration**:

- Receives requests via BFF
- Consumes user events from Kafka (published by Keycloak Custom SPI)
- Performs business logic and data persistence
- Writes to PostgreSQL

---

## Keycloak (Authentication Provider)

**Technology**: Keycloak 26.6.0 (Java 21)

**Description**:
The centralized authentication and authorization service using the OpenID Connect/OAuth2 protocols. Provides user management, single sign-on, and custom event listeners via SPIs.

**Location**: `/services/java/keycloak-custom` (custom SPIs)

**Port**: 7777 (development)

**Key Components**:

- User management and realm configuration
- OAuth2 / OpenID Connect provider
- Custom Event Listener SPI for publishing user events
- Session management
- Role and permission management

**Custom SPIs**:
The Keycloak Custom module provides pluggable extensions to Keycloak:

### Keycloak User Event Listener SPI

**Technology**: Java 21 + Maven

**Purpose**: Listens to user-related events in Keycloak (create, update, delete) and publishes them to Kafka for downstream consumption.

**Location**: `/services/java/keycloak-custom/keycloak-user-event-listener`

**How It Works**:

1. User/admin updates a user in Keycloak UI or via API
2. Keycloak triggers EventListener SPI
3. Custom SPI implementation captures the event
4. Event is serialized and published to Kafka topic: `user-events`
5. Backend service consumes the event for processing

**Configuration**:

- Kafka brokers configured via environment variables in Keycloak deployment
- Topic name: `user-events`
- Event types: USER_CREATE, USER_UPDATE, USER_DELETE

**Dependencies**:

- Keycloak Server libraries (Java 21)
- Kafka client (franz-go via Gradle)
- SLF4J for logging

**How to Build**:

```bash
cd services/java/keycloak-custom
mvn clean package
```

---

## Shared Go Library

**Technology**: Go Modules

**Description**:
Shared packages used across all Go services (BFF, Backend). Provides utilities for authentication, Kafka operations, Keycloak integration, and testing.

**Location**: `/services/go/library`

**Packages**:

| Package       | Purpose                                                    |
| ------------- | ---------------------------------------------------------- |
| `auth/`       | Token validation and JWT handling                          |
| `kafka/`      | Kafka client utilities using franz-go                      |
| `keycloak/`   | Keycloak integration helpers                               |
| `transactor/` | Database transaction management with PostgreSQL support    |
| `testenv/`    | Test environment setup (Kafka, PostgreSQL, Testcontainers) |
| `gsync/`      | Goroutine synchronization utilities                        |
| `cmd/`        | CLI utilities                                              |
| `apperror/`   | Application error handling                                 |
| `assert/`     | Testing assertions                                         |
| `internal/`   | Internal utilities                                         |
| `pretty/`     | Pretty printing utilities                                  |
| `redact/`     | Data redaction for logs                                    |
| `require/`    | Requirement checks                                         |
| `testlogger/` | Structured logging for tests                               |

**How to Use**:

- Imported as `go-services/library/packagename` in other services
- Used for testing with testenv (PostgreSQL containers, Kafka setup)

**Dependencies**:

- PostgreSQL drivers (jackc/pgx)
- Keycloak client libraries
- JWT libraries (golang-jwt)
- Kafka clients (franz-go)
- Test containers (testcontainers-go)

---

## Java Services Parent POM

**Technology**: Maven 3.9+, Java 25 (default)

**Location**: `/services/java/pom.xml`

**Purpose**:
Central Maven parent POM for all Java services. Manages dependency versions, plugin configurations, and provides consistent build settings across Java modules.

**Key Features**:

- Java 25 as default version (can be overridden per module)
- Keycloak 26.6.0 parent imported for dependency management
- JUnit 5 (Jupiter) for testing
- Common plugin versions (compiler, surefire, etc.)
- Consistent build directory structure

**Dependency Management**:

- junit-bom: Unified JUnit 5 versions
- keycloak-parent: Keycloak server libraries
- Other common dependencies defined as needed

**Java Version Override Pattern**:
Child modules can override the default Java version:

```xml
<properties>
    <java.version>21</java.version>
</properties>
```

Example: keycloak-custom/pom.xml overrides to Java 21 because Keycloak 26.6.0 requires Java 21.

**How to Use**:

```bash
# Build all Java modules
cd services/java
mvn clean package

# Build specific module
mvn -pl keycloak-custom clean package

# Skip tests
mvn -DskipTests clean package
```

---

## Architecture Overview

```
┌──────────────────────────────────────────────┐
│         Frontend (React/Vite)                │
│         (localhost:5173)                     │
└──────────────────┬─────────────────────────┘
                   │
                   │ HTTP/REST (Auth)
                   ▼
        ┌──────────────────────────────┐
        │  BFF (Go)                    │
        │  (localhost:2080)            │
        │  - CORS middleware           │
        │  - Auth middleware           │
        └──────────────┬───────────────┘
                       │
                       ▼
        ┌──────────────────────────────┐
        │  Keycloak (Auth Provider)    │
        │  (localhost:7777)            │
        │  - User Management           │
        │  - Event Listener SPI        │
        └────────────┬─────────────────┘
                     │
                     │ User Update Event
                     ▼
        ┌──────────────────────────────┐
        │  Keycloak Custom SPI         │
        │  - User Event Listener       │
        │  - Publishes to Kafka        │
        └────────────┬─────────────────┘
                     │
                     │ User Events
                     ▼
                Kafka Broker
                (localhost:9092)
                     │
                     │ Consumed by Backend
                     ▼
        ┌──────────────────────────────┐
        │  Backend (Go)                │
        │  - Kafka Consumer            │
        │  - Business Logic            │
        │  - PostgreSQL (5432)         │
        └──────────────────────────────┘
```

---

## Service Dependencies Matrix

| Service      | Depends On                    | Depended By                        |
| ------------ | ----------------------------- | ---------------------------------- |
| Frontend     | BFF, Keycloak                 | (Browser)                          |
| BFF          | Backend, Keycloak, Shared Lib | Frontend                           |
| Backend      | PostgreSQL, Kafka, Shared Lib | BFF                                |
| Keycloak     | PostgreSQL                    | Frontend, BFF, Keycloak Custom SPI |
| Keycloak SPI | Keycloak, Kafka               | (Plugin to Keycloak)               |
| PostgreSQL   | (Database)                    | Keycloak, Backend                  |
| Kafka        | (Message Broker)              | Keycloak SPI, Backend              |

---

## Port Reference

| Service    | Port | Environment |
| ---------- | ---- | ----------- |
| Frontend   | 5173 | Development |
| BFF        | 2080 | All         |
| Backend    | 8080 | Internal    |
| Keycloak   | 7777 | Development |
| Kafka      | 9092 | All         |
| PostgreSQL | 5432 | All         |

---

## Environment Setup

All services can be run locally using `docker compose` (see `localmock/docker-compose.yml` for configuration).

For local development:

1. Start services: `docker compose -f localmock/docker-compose.yml up`
2. Services become available at their respective ports
3. See `docs/DEVELOPMENT.md` for detailed setup instructions

---

## CI/CD Integration

The monorepo uses **automatic change detection** for selective testing on every PR and push to main.

### Service Detection

CI identifies service changes based on directory structure:

- **Go services:** `services/go/{bff,backend,library}`
- **Java services:** `services/java/` (Maven modules identified by closest ancestor `pom.xml`)
- **React services:** `frontend/`

### Change-Triggered Testing

| Change | Triggers |
|--------|----------|
| `services/go/bff/*` | Test BFF module |
| `services/go/backend/*` | Test Backend module |
| `services/go/library/*` | Test **all Go modules** (BFF, Backend depend on library) |
| `services/java/keycloak-custom/*` | Test keycloak-custom Maven module |
| `services/java/[other-module]/*` | Test that Maven module |
| `frontend/*` | Test React application |

### Before Pushing

Always test locally:

```bash
make test          # Run all tests for changed services
make build-go      # Build Go only
make test-frontend # Test React only
```

Or validate what CI will test:

```bash
python3 .github/scripts/ci_detect_changes.py --base origin/main --current HEAD
```

### Library Changes

When `services/go/library/` changes, **all Go services are rebuilt and tested** because they depend on the shared library. This prevents integration issues from library updates.

**Example:** Updating `services/go/library/auth/token.go` triggers:
- ✅ Test `./services/go/library`
- ✅ Test `./services/go/bff` (depends on library)
- ✅ Test `./services/go/backend` (depends on library)
- ❌ Skip Java and React (no changes)

### CI Pipeline

See `docs/CI.md` for detailed architecture, change detection logic, and troubleshooting.
