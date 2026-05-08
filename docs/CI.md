# CI Pipeline Architecture

This document describes the current **moon v2.2.3** based CI/CD flow for the Playground monorepo.

---

## Overview

The repository uses the moon project graph plus Git-based affected detection to limit install, format, lint, build, test, and Docker packaging work to projects touched by a change and their downstream dependents.

**Key components:**

- **Moon project graph** - defines explicit project IDs and cross-project `dependsOn` relationships
- **GitHub Actions CI workflow** (`.github/workflows/ci.yml`) - detects affected projects once, groups them by ecosystem, and runs per-language stages
- **GitHub Actions CD workflow** (`.github/workflows/cd.yml`) - identifies deployable affected projects after CI succeeds

---

## CI Workflow

### Location

`.github/workflows/ci.yml`

### Trigger Events

- Pull requests
- Pushes to `main`

### High-Level Flow

1. Compute the affected project list once in a dedicated detect job.
2. Expand that list with `--downstream deep` so project dependents keep the previous dependency propagation behavior.
3. Build explicit target lists from that affected project set for each language stage.
4. Run Docker packaging afterward with explicit `docker` / `docker-push` targets derived from the same affected project list.
5. Publish a workflow summary.

### Detect Job

The detect job computes affected projects with:

```bash
moon query projects --affected --downstream deep --base=<base-sha> --head=<head-sha>
```

The command output is converted to the stable project ID list:

- `frontend`
- `go-services`
- `go-library`
- `go-backend`
- `go-bff`
- `java-services`
- `keycloak-custom`
- `keycloak-user-event-listener`
- `keycloak-custom-image`

Then it emits:

- `has_go_projects`
- `has_java_projects`
- `has_web_projects`
- `affected_projects`

### Language Stage Jobs

Each language job runs only when its boolean is `true`. Inside the job, moon queries the affected projects again and turns only projects that actually own the requested task into explicit targets like `go-backend:build-go`.

#### Go

- `moon run go-services:install-go`
- query affected projects with `moon query projects --affected --downstream deep`
- run explicit `generate-go` targets for the affected Go projects that define that task
- `moon run go-services:format-go`
- fail if generation or formatting changed tracked Go files
- run explicit `lint-go` targets for affected Go projects
- run explicit `build-go` targets for affected Go projects
- run explicit `test-go` targets for affected Go projects

Go lint remains serial because `golangci-lint` has workspace locking issues when multiple Go lint processes run in parallel.

#### Java

- query affected projects with `moon query projects --affected --downstream deep`
- run explicit `install-java` targets for affected Java projects that define that task
- run explicit `format-java` targets for affected Java projects that define that task
- fail if formatting changed tracked Java files
- run explicit `lint-java` targets for affected Java projects
- run explicit `build-java` targets for affected Java projects
- run explicit `test-java` targets for affected Java projects

#### Web / Frontend

- query affected projects with `moon query projects --affected --downstream deep`
- run explicit `install-web` targets for affected frontend projects
- run explicit `format-web` targets for affected frontend projects
- fail if formatting changed tracked frontend files
- run explicit `lint-web` targets for affected frontend projects
- run explicit `build-web` targets for affected frontend projects
- run explicit `test-web` targets for affected frontend projects

### Docker Stage

After language verification completes, the Docker job runs:

The job queries affected projects with `moon query projects --affected --downstream deep`, filters to projects that define `docker`, and then runs those explicit targets.

On the main branch, push uses:

Push follows the same pattern, but targets `docker-push`.

### Cache Strategy

The migration intentionally avoids a paid remote cache. Moon still uses its local `.moon/cache` directory within each job, but the workflow currently does **not** persist `.moon/cache/hashes` or `.moon/cache/outputs` across runners.

This keeps the setup simple and deterministic while preserving the project graph and affected behavior first. Shared cache persistence can be added later if needed.

---

## Java / Keycloak Project Graph

The Java area is intentionally split between Maven inheritance and moon dependency orchestration.

### Maven Hierarchy

```text
services/java/pom.xml
└── services/java/keycloak-custom/pom.xml
    └── services/java/keycloak-custom/keycloak-user-event-listener/pom.xml
```

- `services/java/pom.xml` owns shared Java dependency and plugin versions.
- `services/java/keycloak-custom/pom.xml` owns Keycloak-family defaults and the Java 21 override.
- Each SPI owns its own module POM and artifact packaging.

### Moon Projects

```text
java-services
└── keycloak-custom
    └── keycloak-user-event-listener
        └── keycloak-custom-image
```

- `java-services` tracks the root Java parent POM.
- `keycloak-custom` tracks the Keycloak SPI parent POM.
- `keycloak-user-event-listener` owns the SPI build/test/lint work.
- `keycloak-custom-image` is the single Docker packaging project for the final Keycloak image.

### Why the Image Project Is Separate

The final Keycloak image is **not** another Maven parent/module. It is a downstream moon/Docker project that packages the SPI jars into one Keycloak container image. That separation keeps the Maven dependency tree clean while still letting moon propagate affected changes all the way to packaging.

---

## Affected Build Behavior

### Root Java Parent Change

If `services/java/pom.xml` changes:

- `java-services` becomes affected directly
- `keycloak-custom` becomes affected downstream
- `keycloak-user-event-listener` becomes affected downstream
- `keycloak-custom-image` becomes affected downstream

Result: the SPI modules are rebuilt and the final Keycloak image is packaged once.

### Keycloak Parent Change

If `services/java/keycloak-custom/pom.xml` changes:

- `keycloak-custom` becomes affected directly
- `keycloak-user-event-listener` becomes affected downstream
- `keycloak-custom-image` becomes affected downstream

Result: SPI work stays granular, but the final image is still built exactly once.

### Single SPI Change

If only `services/java/keycloak-custom/keycloak-user-event-listener/**` changes:

- only `keycloak-user-event-listener` is affected directly
- `keycloak-custom-image` becomes affected downstream

Result: only the changed SPI is built/tested/linted, and the image packaging step still runs once.

### Go Shared Library Change

If `services/go/library/**` changes:

- `go-library` becomes affected directly
- `go-backend` becomes affected downstream
- `go-bff` becomes affected downstream

Result: the shared library is verified once and both dependent Go applications are rebuilt and retested.

---

## CD Workflow

### Location

`.github/workflows/cd.yml`

### Purpose

The CD workflow runs after CI completes successfully on `main`. It computes the affected projects again and filters them to deployable projects.

Deployable project IDs are:

- `go-backend`
- `go-bff`
- `keycloak-custom-image`
- `frontend`

---

## Local Verification Commands

```bash
# Show all moon projects
moon query projects

# Show affected projects between two refs
moon query projects --affected --downstream deep --base=origin/main --head=HEAD

# Build/test/lint/format one SPI
moon run keycloak-user-event-listener:format
moon run keycloak-user-event-listener:build
moon run keycloak-user-event-listener:test
moon run keycloak-user-event-listener:lint

# Run language-specific tasks directly
moon run go-services:install-go
moon run go-backend:generate-go
moon run go-library:generate-go
moon run go-services:format-go
moon run frontend:install-web
moon run frontend:format-web

# Build the packaged Keycloak image
moon run keycloak-custom-image:docker
```

---

## Notes

- Generic `install` / `format` / `lint` / `build` / `test` tasks remain for local developer use, while CI calls the language-specific tasks (`*-go`, `*-java`, `*-web`) directly.
- Parent Java projects intentionally keep orchestration-style tasks where affected parent POM changes need formatting, linting, or dependency warmup at the parent level.
- Docker packaging stays on explicit application projects only: `frontend`, `go-backend`, `go-bff`, and `keycloak-custom-image`.

---

## Troubleshooting

### "Base ref does not exist"

**Cause:** GitHub's fetch depth is too shallow or the branch was newly created.

**Solution:** `ci.yml` uses `fetch-depth: 0`, and new branches fall back to `origin/main`.

### Shared cache is not being used between jobs

**Expected behavior.** The current migration intentionally avoids cross-runner cache persistence and any paid remote cache service.

---

## References

- **Workflow:** `.github/workflows/ci.yml`
- **Monorepo conventions:** `AGENTS.md`
