# Playground

A personal tech exploration playground — a polyglot monorepo for experimenting modern tech stack.

---

## Quick Start

### Prerequisites

Before you begin, ensure you have the following installed:

- **Go** (1.26+) - For backend services
- **Java** (21+) - For Java services
- **Node.js** (v24+) - For BFF and frontend
- **Docker** & **Docker Compose** - For services (Postgres, Keycloak, etc.)

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/wq-chang/playground.git
cd playground

# Build all services
moon run :build
```

### Running Locally

```bash
# Start local dependencies
moon run :local-up

# Run the frontend
cd frontend
npm install
npm run dev

# Or build and test all services
moon run :build
moon run :test
```

---

## Project Structure

This monorepo contains multiple services and applications organized as follows:

- `services/`: Backend logic split by language.
- `frontend/`: React/TypeScript web application.
- `localmock/`: Dockerized environment for local development.

For a detailed explanation of the architecture and system design, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## Services Overview

This project implements a microservices architecture with the following components:

- **Backend (Go/Java)**: Data services handling business logic and persistence
- **BFF (Go)**: Backend-for-Frontend API layer providing REST endpoints
- **Frontend (React)**: Modern UI consuming BFF APIs
- **Keycloak**: Identity provider for authentication and authorization
- **Data Layer**: PostgreSQL + Kafka for event-driven communication

For a comprehensive services catalog, API documentation, and deployment info, see [docs/SERVICES.md](docs/SERVICES.md).

---

## Development

### Local Development Environment

For detailed setup instructions, running individual services, debugging, and troubleshooting, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

### Common Commands

Use Moon for the common repository workflows:

```bash
moon run :build              # Build all services
moon run :test               # Run all tests
moon run :lint               # Lint all code
moon run :format             # Format all code
moon run :dev                # Build and start local environment
moon run :clean              # Clean build artifacts
moon run :help               # Show repository task shortcuts
moon run :local-bootstrap    # Start localmock and apply Terraform bootstrap
```

---

## Building & Testing

Use the repository Moon tasks to build and test:

```bash
moon run :build
moon run :test
```

See [scripts/README.md](scripts/README.md) for the repo/localmock task map and the localmock bootstrap helper.

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
