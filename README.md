# Playground

A modern polyglot monorepo showcasing microservices architecture with Go backend services, Java backends, Node.js BFF (Backend-for-Frontend), React frontend, and integrated authentication via Keycloak.

---

## Quick Start

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go** (1.26+) - For backend services
- **Java** (21+) - For Java services
- **Node.js** (v24+) - For BFF and frontend
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

### Local Development Environment

For detailed setup instructions, running individual services, debugging, and troubleshooting, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

### Common Commands

Use the `Makefile` or scripts for common tasks:

```bash
make build         # Build all services
make test          # Run all tests
make lint          # Lint all code
make dev           # Build and start local environment
make clean         # Clean build artifacts
make help          # Show all available targets
```

Or use the shell scripts in `scripts/`:
- `./scripts/build-all.sh` - Build all services
- `./scripts/test-all.sh` - Run all tests
- `./scripts/lint-all.sh` - Lint all code
- `./scripts/format-all.sh` - Format all code

---

## Building & Testing

Use the provided scripts or Makefile to build and test:

```bash
make build test    # Build and test everything
```

Or see [scripts/README.md](scripts/README.md) for individual build and test commands.

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
