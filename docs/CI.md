# CI Pipeline Architecture

This document describes the current Nx-based CI/CD flow for the Playground monorepo.

---

## Overview

The repository uses **Nx affected project detection** to limit lint, build, test, and Docker packaging work to the projects touched by a change. The current pipeline does not rely on the older Python change-detection script for CI execution.

**Key components:**

- **Nx project graph** - determines which projects are affected by a change
- **GitHub Actions CI workflow** (`.github/workflows/ci.yml`) - runs lint/build/test on affected projects
- **GitHub Actions CD workflow** (`.github/workflows/cd.yml`) - identifies deployable affected projects after CI succeeds

---

## CI Workflow

### Location

`.github/workflows/ci.yml`

### Trigger Events

- Pull requests
- Pushes to `main`

### High-Level Flow

1. Install toolchains (Node, Go, Java) and Nx.
2. Compute the affected project list with:

   ```bash
   npx nx show projects --affected --base=<base-sha> --head=<head-sha> --json
   ```

3. Run `lint`, `build`, and `test` only for affected projects.
4. Filter the affected list down to Docker-enabled projects.
5. Build Docker images once per affected deployable project.
6. Publish a workflow summary.

### Current Docker-Enabled Projects

- `go-backend`
- `go-bff`
- `frontend`
- `keycloak-custom-image`

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

# Build/test/lint one SPI
npx nx run keycloak-user-event-listener:build
npx nx run keycloak-user-event-listener:test
npx nx run keycloak-user-event-listener:lint

# Build the packaged Keycloak image
npx nx run keycloak-custom-image:docker
```

---

## Notes

- The legacy `.github/scripts/ci_detect_changes.py` script remains in the repository as a standalone utility, but the primary CI workflow uses Nx affected projects.
- Parent Java Nx projects intentionally use no-op `build`, `test`, and `lint` targets. Their job is to propagate change impact, not to duplicate the heavy Maven work already performed by child SPI projects.

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
