# Playground

A modern polyglot monorepo showcasing microservices architecture with Go backend services, Java backends, Node.js BFF (Backend-for-Frontend), React frontend, and integrated authentication via Keycloak.

---

## Quick Start

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go** (1.21+) - For backend services
- **Java** (17+) - For Java services
- **Node.js** (18+) - For BFF and frontend
- **Docker** & **Docker Compose** - For services (Postgres, Keycloak, NATS, etc.)

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/wq-chang/playground.git
cd playground

# Build all services
./scripts/build-all.sh
```

### Running Locally

```bash
# Using localmock for local development
cd frontend
npm install
npm run dev

# Or build and test all services
./scripts/build-all.sh
./scripts/test-all.sh
```

---

## Project Structure

This monorepo contains multiple services and applications organized as follows:

```
playground/
├── services/              # Backend services (Go & Java)
│   ├── go/               # Go services
│   └── java/             # Java services
├── frontend/              # React frontend application
├── localmock/            # Local development mocks & fixtures
├── scripts/              # Build, test, lint, and format scripts
├── docs/                 # Project documentation
│   ├── ARCHITECTURE.md   # System architecture overview
│   ├── SERVICES.md       # Services catalog & API specs
│   └── DEVELOPMENT.md    # Development guide
├── .github/workflows/    # CI/CD pipelines
└── flake.nix            # Nix development environment
```

For a detailed explanation of the architecture and system design, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## Services Overview

This project implements a microservices architecture with the following components:

- **Backend (Go/Java)**: Data services handling business logic and persistence
- **BFF (Node.js/Express)**: Backend-for-Frontend API layer providing REST endpoints
- **Frontend (React)**: Modern UI consuming BFF APIs
- **Keycloak**: Identity provider for authentication and authorization
- **Data Layer**: PostgreSQL + NATS for event-driven communication

For a comprehensive services catalog, API documentation, and deployment info, see [docs/SERVICES.md](docs/SERVICES.md).

---

## Development

### Local Development with localmock/

The `localmock/` directory provides mock implementations and test fixtures for local development without running the full infrastructure:

```bash
# Start development environment with mocks
npm run dev
```

### Common Commands

Build, test, lint, and format scripts are located in `scripts/`:

```bash
./scripts/build-all.sh      # Build all services
./scripts/test-all.sh       # Run tests for all services
./scripts/lint-all.sh       # Lint all code
./scripts/format-all.sh     # Format code
```

For more development information, build scripts details, and troubleshooting, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

---

## Building & Testing

### Build All Services

```bash
./scripts/build-all.sh
```

This command compiles:
- Go services in `services/go/`
- Java services in `services/java/`
- Frontend application in `frontend/`
- BFF service in Node.js

### Run Tests

```bash
./scripts/test-all.sh
```

Runs unit and integration tests across all services.

For additional build configurations and troubleshooting, refer to [scripts/README.md](scripts/README.md).

---

## Documentation

- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** - System architecture, component interactions, and design decisions
- **[SERVICES.md](docs/SERVICES.md)** - Services catalog, API endpoints, and deployment guide
- **[DEVELOPMENT.md](docs/DEVELOPMENT.md)** - Development setup, common workflows, and debugging
- **[services/README.md](services/README.md)** - Backend services documentation

---

## Future Plans

The following features and improvements are planned:

- 📊 **Reporting Service** - Analytics and reporting capabilities
- 🎨 **Keycloak Theme** - Custom theme for authentication UI
- 📈 **Observability Stack** - Distributed tracing, metrics, and logging (OTEL)
- 🚀 **Deployment** - Kubernetes manifests and cloud deployment options
- ⚙️ **Format Checking** - Automated code formatting in CI pipeline
- 📦 **Command Runner** - Unified task automation

---

## License

This project is maintained by the Playground team. See individual service READMEs for license information.
