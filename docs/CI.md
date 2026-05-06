# CI Pipeline Architecture

This document describes the current Nx-based CI/CD flow for the Playground monorepo.

---

## Overview

The repository uses **Nx affected project detection** to limit install, format, lint, build, test, and Docker packaging work to the languages and projects touched by a change. The current pipeline does not rely on the older Python change-detection script for CI execution.

**Key components:**

- **Nx project graph** - determines which projects are affected by a change
- **GitHub Actions CI workflow** (`.github/workflows/ci.yml`) - detects affected projects once, groups them by language, and runs per-language stages
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
2. Group affected projects by language tag:

   - `language:go`
   - `language:java`
   - `language:ts`

3. Run language-specific CI jobs only when that language has affected projects.
4. Run Docker packaging afterward with the shared `docker` / `docker-push` Nx targets over the affected project list.
5. Publish a workflow summary.

### Detect Job

The detect job computes affected projects with:

   ```bash
   npx nx show projects --affected --base=<base-sha> --head=<head-sha> --json
   ```

The detect job fails if Nx cannot resolve the affected list or if the output is not valid JSON. CI does not silently convert detection errors into an empty project list.

Then it inspects project tags and emits:

- `has_go_projects`
- `has_java_projects`
- `has_web_projects`
- `affected_projects`

The detect step does **not** try to pre-filter Docker projects.

### Language Stage Jobs

Each language job runs only when its boolean is `true`.
When it runs, it uses `nx affected -t <language-target>` with the resolved base/head refs instead of carrying a precomputed project list across jobs.

#### Go

- `go-services:install-go`
- `go-services:format-go`
- fail if formatting changed tracked Go files
- `npx nx affected -t lint-go`
- `npx nx affected -t build-go`
- `npx nx affected -t test-go`

Go lint remains serial because `golangci-lint` has workspace locking issues when multiple Go lint processes run in parallel.

#### Java

- `npx nx affected -t install-java`
- `npx nx affected -t format-java`
- fail if formatting changed tracked Java files
- `npx nx affected -t lint-java`
- `npx nx affected -t build-java`
- `npx nx affected -t test-java`

#### Web / Frontend

- `npx nx affected -t install-web`
- `npx nx affected -t format-web`
- fail if formatting changed tracked frontend files
- `npx nx affected -t lint-web`
- `npx nx affected -t build-web`
- `npx nx affected -t test-web`

### Docker Stage

After language verification completes, the Docker job runs:

```bash
npx nx affected -t docker --base=<base-sha> --head=<head-sha>
```

On the main branch, push uses:

```bash
npx nx affected -t docker-push --base=<base-sha> --head=<head-sha>
```

Metadata-only projects keep no-op `docker` / `docker-push` targets where needed so the full affected project set can be passed to the Docker stage without a separate filtering phase.

### Nx Cache

Because the workflow is split into multiple jobs and each job runs on a different runner, `.nx/cache` is restored inside each language job with a per-job cache key.

Recommended key shape:

```text
nx-cache-${runner.os}-${github.ref_name}-${github.job}-${github.sha}
```

This avoids collisions between parallel language jobs while still letting each job reuse prior Nx results from the same branch.

---

## Java / Keycloak Project Graph

The Java area is intentionally split between Maven inheritance and Nx dependency orchestration.

### Maven Hierarchy

```text
services/java/pom.xml
└── services/java/keycloak-custom/pom.xml
    └── services/java/keycloak-custom/keycloak-user-event-listener/pom.xml
```

- `services/java/pom.xml` owns shared Java dependency and plugin versions.
- `services/java/keycloak-custom/pom.xml` owns Keycloak-family defaults and the Java 21 override.
- Each SPI owns its own module POM and artifact packaging.

### Nx Projects

```text
java-services
└── keycloak-custom
    └── keycloak-user-event-listener
        └── keycloak-custom-image
```

- `java-services` tracks the root Java parent POM.
- `keycloak-custom` tracks the Keycloak SPI parent POM.
- Each SPI is its own Nx project with Maven-scoped build/test/lint targets.
- `keycloak-custom-image` is the single Docker packaging project for the final Keycloak image.

### Why the Image Project Is Separate

The final Keycloak image is **not** another Maven parent/module. It is a downstream Nx/Docker project that packages the SPI jars into one Keycloak container image. That separation keeps the Maven dependency tree clean while still letting Nx propagate affected changes all the way to packaging.

---

## Affected Build Behavior

### Root Java Parent Change

If `services/java/pom.xml` changes:

- `java-services` becomes affected directly
- `keycloak-custom` becomes affected through Nx dependencies
- all SPI projects become affected
- `keycloak-custom-image` becomes affected

Result: the SPI modules are rebuilt and the final Keycloak image is packaged once.

### Keycloak Parent Change

If `services/java/keycloak-custom/pom.xml` changes:

- `keycloak-custom` becomes affected directly
- SPI projects become affected
- `keycloak-custom-image` becomes affected

Result: SPI work stays granular, but the final image is still built exactly once.

### Single SPI Change

If only `services/java/keycloak-custom/keycloak-user-event-listener/**` changes:

- only `keycloak-user-event-listener` is affected directly
- `keycloak-custom-image` becomes affected downstream

Result: only the changed SPI is built/tested/linted, and the image packaging step still runs once.

### Multiple SPI Changes in One PR

If several SPI modules change in the same PR:

- each changed SPI is included in the affected set
- `keycloak-custom-image` appears once in the affected project list

Result: Docker packaging happens once per CI run, not once per SPI.

---

## CD Workflow

### Location

`.github/workflows/cd.yml`

### Purpose

The CD workflow runs after CI completes successfully on `main`. It computes the affected projects again and filters them to deployable projects.

For Java / Keycloak work, the deployable project is:

- `keycloak-custom-image`

This preserves SPI-level change detection while keeping deployment centered on the single final Keycloak image.

---

## Local Verification Commands

```bash
# Show all Nx projects
npx nx show projects

# Show affected projects between two refs
npx nx show projects --affected --base=origin/main --head=HEAD --json

# Build/test/lint/format one SPI
npx nx run keycloak-user-event-listener:format
npx nx run keycloak-user-event-listener:build
npx nx run keycloak-user-event-listener:test
npx nx run keycloak-user-event-listener:lint

# Run language-specific targets directly
npx nx run go-services:install-go
npx nx run go-services:format-go
npx nx run frontend:install-web
npx nx run frontend:format-web

# Build the packaged Keycloak image
npx nx run keycloak-custom-image:docker
```

---

## Notes

- The legacy `.github/scripts/ci_detect_changes.py` script remains in the repository as a standalone utility, but the primary CI workflow uses Nx affected projects.
- Generic `install` / `format` / `lint` / `build` / `test` targets remain for local developer use, but CI calls the language-specific targets (`*-go`, `*-java`, `*-web`) directly.
- Parent Java Nx projects intentionally keep orchestration/no-op targets where mixed affected batches need them.
- Metadata-only projects may also keep no-op `docker` / `docker-push` targets so the Docker job can run against the full affected project set.

### Step 3: Summary

```
### Go Modules Tested
- ./services/go/backend
- ./services/go/bff
- ./services/go/library
✅ All Go lint, build, and tests passed

### Java Modules
- No Java modules changed

### React Modules
- No React modules changed
```

---

## Running Locally

### Test Change Detection

```bash
# See what would be tested for your current branch
python3 .github/scripts/ci_detect_changes.py \
  --base origin/main \
  --current HEAD
```

**Example output:**

```
======================================================================
📊 Change Detection Summary
======================================================================

📝 Total changed files: 3
✅ Go services changed: ./services/go/library, ./services/go/bff, ./services/go/backend
⏭️  No Java services changed
⏭️  No React services changed
======================================================================
```

### Replicate CI Locally

For a go library change:

```bash
# Do what CI does for Go modules
cd services/go/library && gotestsum ./... && cd -
cd services/go/bff && gotestsum ./... && cd -
cd services/go/backend && gotestsum ./... && cd -
```

Or use Makefile:

```bash
make build && make test
```

---

## Performance Implications

### Without Change Detection (Old Approach)

- All tests run on every PR/push
- Average CI time: ~5-10 minutes
- Wastes CI minutes on irrelevant tests

### With Change Detection (New Approach)

- Only modified services tested
- Frontend-only change: ~2 minutes (skip Go, Java)
- Go library change: ~8 minutes (test all Go modules)
- Average: 30-50% faster

---

## Troubleshooting

### "Base ref does not exist"

**Cause:** GitHub's fetch-depth is too shallow and missing base commit.

**Solution:** `ci.yml` uses `fetch-depth: 10` to ensure base ref exists.

### All tests running when I only changed frontend

**Cause:** You modified a root-level file (Makefile, .github/workflows/ci.yml, etc.).

**Solution:** Root changes trigger all tests to ensure consistency. Keep those files stable.

### New branch shows "Base is all zeros"

**Expected behavior.** Script automatically uses `origin/main` as base for new branches.

### "git find didn't work, using fallback poms"

**Info message.** Script falls back to hardcoded pom list if git find fails. Not a problem.

---

## Future Improvements

- **Build cache:** Use GitHub Actions cache for Go modules, Maven, and npm
- **Parallel test execution:** Run test-go, test-java, test-react in parallel (they're independent)
- **Artifact uploads:** Store build artifacts for verification
- **Release automation:** Detect version bumps and auto-tag releases

---

## References

- **Change detection script:** `.github/scripts/ci_detect_changes.py`
- **Workflow:** `.github/workflows/ci.yml`
- **Monorepo conventions:** `AGENTS.md`
