# CI Pipeline Architecture

This document describes the current **moon v2.2.3** based CI/CD flow for the Playground monorepo.

---

## Overview

The repository uses the moon project graph plus Git-based affected detection to limit install, format, lint, build, test, and Docker packaging work to projects touched by a change and their downstream dependents.

**Key components:**

- **Moon project graph** - defines explicit project IDs and cross-project `dependsOn` relationships
- **Inherited Moon tasks** - `.moon/tasks/*.yml` defines shared tasks by language and by application Docker support
- **GitHub Actions CI workflow** (`.github/workflows/ci.yml`) - detects affected projects once, groups them with Moon's native language filters, and runs per-language stages
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
3. Derive `has_go_projects`, `has_java_projects`, `has_web_projects`, and `has_docker_projects` with `moon query projects --language ... --tasks ...`.
4. Run most task families directly with `moon run :<task> --affected --summary minimal` instead of building explicit target lists inside each job.
5. Restore `.moon/cache/hashes` and `.moon/cache/outputs` with `actions/cache@v5` in each task-running job.
6. Run Docker packaging with a two-layer strategy: Moon affected execution on PRs, and Buildx layer caching underneath the Docker build process.
7. Publish a workflow summary.

### Detect Job

The detect job computes affected projects with:

```bash
MOON_BASE=<base-sha> MOON_HEAD=<head-sha> moon query projects --affected --downstream deep
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

It also uses Moon's native `--language` and `--tasks` filters to emit:

- `has_go_projects`
- `has_java_projects`
- `has_web_projects`
- `has_docker_projects`
- `affected_projects`

### Language Stage Jobs

Each language job runs only when its boolean is `true`. Inside the job, Moon reads `MOON_BASE` / `MOON_HEAD` from the job environment and runs the relevant task family directly against affected projects with `moon run :<task> --affected --summary minimal`.

#### Go

- `moon run go-services:install-go`
- `moon run :generate-go --affected`
- `moon run go-services:format-go`
- fail if generation or formatting changed tracked Go files
- `moon run :lint-go --affected`
- `moon run :build-go --affected`
- `moon run :test-go --affected`

Go lint remains serial because `golangci-lint` has workspace locking issues when multiple Go lint processes run in parallel. Go formatting remains rooted at `go-services:format-go` so formatting still runs once from the shared Go module root instead of fan-out execution across overlapping Go subprojects.

#### Java

- `moon run :install-java --affected`
- `moon run :format-java --affected`
- fail if formatting changed tracked Java files
- `moon run :lint-java --affected`
- `moon run :build-java --affected`
- `moon run :test-java --affected`

#### Web / Frontend

- `moon run :install-web --affected`
- `moon run :format-web --affected`
- fail if formatting changed tracked frontend files
- `moon run :lint-web --affected`
- `moon run :build-web --affected`
- `moon run :test-web --affected`

### Docker Stage

After language verification completes, the Docker stage behaves differently by event:

- images are tagged as `ghcr.io/${{ github.repository_owner }}/<project-id>:<tag>`
- pull requests use `moon run :docker --affected`, so unaffected image projects are skipped by Moon's affected/project-graph logic
- pushes to `main` always run `moon run :docker-push`, so all deployable images are rebuilt and pushed
- `docker/setup-buildx-action` enables Buildx for cached CI builds underneath the Moon task execution
- pull requests use Buildx layer cache with `type=gha,scope=<project>`
- `main` uses both `type=gha` and GHCR registry cache (`:buildcache`) per image while rebuilding and pushing all deployable images
- `docker/login-action` authenticates to GHCR before main-branch pushes
- final image push happens through the Moon `docker-push` task on `main`

### Shared Task Layout

The repository now uses Moon task inheritance to avoid repeating the same commands in every project file:

- `.moon/tasks/go-common.yml` - shared Go formatting
- `.moon/tasks/go-root.yml` - Go workspace-root install task
- `.moon/tasks/go-build.yml` - Go build/test/lint/generate tasks for non-aggregator Go projects
- `.moon/tasks/java-common.yml` - shared Java install/format/lint tasks for Maven projects
- `.moon/tasks/java-build.yml` - Java build/test tasks for non-aggregator Maven projects
- `.moon/tasks/typescript.yml` - frontend web install/format/build/test/lint tasks
- `.moon/tasks/docker.yml` - shared Docker build/push tasks for application projects with a `Dockerfile`; CI enables Buildx cache through environment variables, while local Moon usage still defaults to plain Docker with bare project image names

Aggregator projects are tagged with `aggregator` so they only inherit the subset of tasks they should own, while the shared language task files now match projects by their `language` field instead of custom `language-*` tags.

### Cache Strategy

The workflow intentionally avoids a paid remote cache, but it **does** persist the safe, portable portions of Moon's local cache with GitHub Actions cache v5:

- `.moon/cache/hashes`
- `.moon/cache/outputs`

This cache is restored in each Moon task-running CI job with `actions/cache@v5`.

`moonrepo/setup-toolchain` is still used, but only for installing and caching the Moon CLI/toolchain setup. It does **not** automatically persist `.moon/cache` task outputs across jobs, which is why the explicit GitHub cache step is required.

The Docker stage uses two layers of caching/avoidance:

1. **Moon affected execution on PRs** — `moon run :docker --affected` skips unrelated image projects entirely.
2. **Docker layer cache when builds do run** — Buildx uses:
   - **PR builds:** `cache-from/to=type=gha`
   - **main builds:** `cache-from/to=type=gha` plus `cache-from/to=type=registry` in GHCR

This means PRs avoid unnecessary Docker work at the task level, while main always rebuilds and pushes deployable images with warm layer cache.

### Download Hardening

The Go job downloads release binaries for `golangci-lint`, `gotestsum`, and `sqlc`. Each artifact is downloaded directly from its GitHub release URL and verified with SHA-256 before extraction or installation.

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
MOON_BASE=origin/main MOON_HEAD=HEAD moon query projects --affected --downstream deep

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
- Shared language tasks are inherited from `.moon/tasks/*.yml`, while project-local `moon.yml` files mainly keep metadata, `dependsOn`, and true per-project overrides like `frontend:dev`.
- Parent Java projects intentionally keep orchestration-style tasks where affected parent POM changes need formatting, linting, or dependency warmup at the parent level.
- Docker packaging stays on explicit application projects only: `frontend`, `go-backend`, `go-bff`, and `keycloak-custom-image`.

---

## Troubleshooting

### "Base ref does not exist"

**Cause:** GitHub's fetch depth is too shallow or the branch was newly created.

**Solution:** `ci.yml` uses `fetch-depth: 0`, and new branches fall back to `origin/main`.

### Shared cache is not being used between jobs

**Cause:** GitHub Actions cache did not restore a matching `.moon/cache/hashes` / `.moon/cache/outputs` entry for the current job.

**Solution:** Check the `Restore Moon cache` step in the relevant job. Also remember that `moonrepo/setup-toolchain` does not replace the explicit Moon task cache step.

---

## References

- **Workflow:** `.github/workflows/ci.yml`
- **Monorepo conventions:** `AGENTS.md`
