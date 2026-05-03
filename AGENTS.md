# AI Agent Guide for Playground Monorepo

This document helps AI assistants understand the repository structure, conventions, and patterns. For language-specific guidance, see `services/<language>/AGENTS.md`.

**Last verified:** 2026-04-23  
**Repository type:** Polyglot monorepo for tech exploration (learning, not production)  
**Primary tech stack:** Go, Java, TypeScript/React, Kafka, PostgreSQL, Keycloak

---

## Quick Start

Key constraint: **This is a tech exploration project**, not production. The goal is learning new technologies and patterns.

Tech stack by service:

- **Go services** (`services/go/`) - net/http, Kafka consumer, PostgreSQL, manual queries
- **Java services** (`services/java/`) - Keycloak SPI (Java 21) + future reporting service (Java 25)
- **React frontend** (`frontend/`) - React 19, TypeScript, TanStack Router, Keycloak auth
- **Event stream** - Kafka for Keycloak user events → Go backend consumer
- **Local dev** (`localmock/`) - Docker Compose for all services

---

## Repository Layout

```
playground/
├── services/
│   ├── go/                    # Go services (shared go.mod)
│   │   ├── go.mod           # Single module for all services
│   │   └── */                # Individual services with main.go
│   └── java/                 # Java Maven multi-module
│       ├── pom.xml          # Parent (Java 25 default)
│       ├── keycloak-custom/  # Keycloak SPI parent (Java 21 override)
│       │   └── keycloak-user-event-listener/  # Custom listener
│       └── [future] reporting-service/  # Sibling module (Java 25)
├── frontend/                  # React 19 + TypeScript + Vite
│   ├── src/
│   │   ├── routes/          # TanStack Router definitions
│   │   ├── pages/           # Page components
│   │   ├── layouts/         # Layout components
│   │   └── services/        # Client-side utilities
│   └── dist/                # Built output
├── docs/                      # Central documentation (single source of truth)
│   ├── ARCHITECTURE.md       # System design, tech choices, exploration goals
│   ├── SERVICES.md          # Service catalog, ports, build commands, integration
│   └── DEVELOPMENT.md       # Local setup, troubleshooting, testing
├── localmock/                 # Docker Compose environment + secrets
├── scripts/                   # Automation scripts
├── Makefile                   # Primary task runner
├── README.md                  # Project overview
└── AGENTS.md                  # This file
```

---

## Key Conventions

### **Documentation Authority**

Detailed information lives in `docs/` (single source of truth). `AGENTS.md` guides AI decisions; `docs/*` contains implementation details. Update docs in the same commit as code changes.

### **Java: Multi-Module Maven**

- Parent POM at `services/java/pom.xml` defaults to Java 25
- `keycloak-custom/pom.xml` overrides to Java 21 for Keycloak compatibility
- Children inherit via parent reference; override only what's needed

### **Docker Build Context**

Maven builds require context at parent directory: `context: ../services/java/` (not at child module level). This allows the Dockerfile to access the full POM hierarchy.

### **Environment Variables**

Use `direnv exec <dir>` in Makefile to load `.envrc` before running commands. Make runs subshells that don't automatically load environment files.

Example:

```makefile
local-up:
	direnv exec localmock docker compose up -d
```

### **React Component Organization**

Route files export only the Route definition; components live in separate files. This satisfies ESLint fast-refresh rules.

✅ **Correct:**

```typescript
// routes/about.tsx - only route definition
export const Route = {
  path: '/about',
  component: About
};

// pages/About.tsx - component lives here
export function About() {
  return <h1>About</h1>;
}
```

---

## Useful Commands

See `make help` for complete list. Common commands:

```bash
# Build (all services or individual)
make build              # Build Go, Java, Frontend
make build-go           # Go only
make build-java         # Java/Maven only
make build-frontend     # Frontend/Vite only
make build-docker       # Build docker images

# Test
make test               # Run all tests (passWithNoTests flag for empty suites)
make test-go            # Go only
make test-java          # Java only
make test-frontend      # Frontend only

# Lint
make lint               # All services

# Local development
make local-up           # Start all services (uses direnv)
make local-down         # Stop all services
```

**Docker commands** (from `localmock/`):

```bash
docker compose build    # Build all images
docker compose up -d    # Start services
docker compose logs -f  # Follow logs
```

---

## Integration Points

See `docs/ARCHITECTURE.md` for system design and `docs/SERVICES.md` for ports/services.

Quick reference:

- **Keycloak → Kafka → Go Backend:** User events flow from Keycloak (published via custom SPI listener) → Kafka topic → Go service consumes and updates PostgreSQL
- **Frontend Auth:** Keycloak integration via `@keycloak/keycloak-js`; token refresh in `frontend/src/services/authService.ts`
- **Databases:** Keycloak has own PostgreSQL; backend has separate PostgreSQL (both in `docker compose`)
- **Common ports:** Frontend 5173, Keycloak 8080, Kafka 9092, PostgreSQL 5432/5433

---

## Common Patterns and Decision Trees

**When to add a Go service:** Lightweight microservice for event processing, API endpoint with minimal dependencies. Use `net/http` (no framework); add to `services/go/` with own `main.go`.

**When to add a Java service:** Keycloak SPI extension → add to `keycloak-custom/` parent with Java 21. New reporting/analytics service → add sibling module at `services/java/` with Java 25. Use Maven; inherit from parent POM.

---

## Known Limitations and Workarounds

| Issue                         | Cause                                                                 | Solution                                                                                                  |
| ----------------------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **Java version conflict**     | Keycloak requires Java 21; future services may need Java 25+          | Maven multi-module allows per-module overrides; `keycloak-custom/pom.xml` overrides `<source>21</source>` |
| **direnv not loaded**         | `make` runs subshells that don't auto-load `.envrc`                   | Use `direnv exec localmock` in Makefile; Makefile already does this                                       |
| **ESLint fast-refresh fails** | Route files exporting both Route config and components                | Separate components to `frontend/src/pages/`; route files only export Route                               |
| **Docker build fails**        | Maven can't find parent POM if build context is at child module level | Build context must be at parent directory (`../services/java/`)                                           |

**Troubleshooting common issues:**

- Docker: "Could not find parent pom.xml" → check `docker-compose.yml` build context is `../services/java/`, not nested
- Make secrets missing → Use `direnv exec localmock make local-up` or rely on Makefile (which wraps commands with `direnv exec`)
- Frontend tests timeout → Current test count is 0; `npm test` uses `--passWithNoTests` flag to complete

---

## CI & Automation

### Python-Based Change Detection

The CI pipeline uses a Python script (`.github/scripts/ci_detect_changes.py`) to detect which services changed and selectively run tests:

- **Detection logic:** Script compares git refs and identifies changed files in `services/go/`, `services/java/`, and `frontend/`
- **Service detection:** Go services at `services/go/{bff,backend,library}`; Java modules via closest ancestor `pom.xml`; React at `frontend/`
- **Library changes:** If `services/go/library` changes, all Go services (BFF, Backend) are rebuilt to use updated shared code
- **Branch handling:** New branches (base SHA = all zeros) use `origin/main` as comparison base
- **Output:** JSON format with `has_*_changes` booleans and `*_modules` lists for each language

See `docs/CI.md` for detailed architecture.

### GitHub Actions Workflow

The `.github/workflows/ci.yml` workflow:

1. Detects changes on every PR and push to main
2. Runs selective tests based on detection results
3. Conditional job execution: `if: needs.detect-changes.outputs.has_*_changes == 'true'`
4. Posts summary of tested modules to PR/commit

No hardcoded environment variable references for service paths—script uses fixed directory structure.

---

## Maintenance

- **Last verified:** 2026-04-23
- **Version compatibility:** Go 1.26, Java 21/25, Node v24, PostgreSQL, Kafka, Python 3.12
- **Discipline:** Code changes must update docs in same commit to prevent drift

**Verify against codebase:** Run `make build && make test && make lint` to catch issues; check `docs/SERVICES.md` ports against `docker-compose.yml`; verify Java versions in POMs match constraints; test CI script locally with `.github/scripts/ci_detect_changes.py --base main --current HEAD`.
