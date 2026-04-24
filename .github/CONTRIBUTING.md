# Contributing to Playground

Thank you for your interest in contributing to the Playground monorepo! This document outlines expectations for developers and how our CI pipeline works.

## Before You Push

**Always test locally before pushing:**

```bash
# Build all services
make build

# Run all tests
make test

# Run linters
make lint
```

Or test specific services:

```bash
make build-go && make test-go       # Go only
make build-java && make test-java   # Java only
make build-frontend && make test-frontend  # Frontend only
```

This catches issues early and prevents failed CI builds.

## CI Pipeline Overview

### Automatic Change Detection

Our CI uses Python-based change detection (`.github/scripts/ci_detect_changes.py`) to run selective tests:

- **On PR:** Tests only the services you modified
- **On push to main:** Same selective testing

**What triggers which tests:**

| Change Location | Triggers |
|-----------------|----------|
| `services/go/bff/` | Go build & tests |
| `services/go/backend/` | Go build & tests |
| `services/go/library/` | Go build & tests **for all Go services** (since all depend on shared library) |
| `services/java/keycloak-custom/` | Java tests for keycloak-custom |
| `services/java/[module]/` | Java tests for that module |
| `frontend/` | React build & tests |
| Root files (Makefile, .github/, etc.) | All services tested |

### New Branches

If you create a new branch (no comparison commit), CI compares against `origin/main`.

### Example: Modified `library` Package

**Your change:** Update `services/go/library/auth/token.go`

**CI runs:**
- ✅ Build and test `./services/go/library`
- ✅ Build and test `./services/go/bff` (depends on library)
- ✅ Build and test `./services/go/backend` (depends on library)
- ❌ Java tests skipped (no Java changes)
- ❌ React tests skipped (no React changes)

## Workflow Expectations

1. **Small, focused PRs** - One feature or fix per PR when possible
2. **Test locally first** - Use `make test` before pushing
3. **Update docs** - If your change affects architecture, update `docs/SERVICES.md` or add to `.github/CONTRIBUTING.md`
4. **Wait for CI** - All selective tests must pass before merge

## CI Pipeline Files

- **Detection script:** `.github/scripts/ci_detect_changes.py` — Identifies changed services
- **Workflow:** `.github/workflows/ci.yml` — Runs conditional jobs based on detection
- **Documentation:** `AGENTS.md` (CI section), `docs/SERVICES.md` (service catalog)

## Local Testing with CI Script

Test the change detection locally:

```bash
# Compare current branch against main
python3 .github/scripts/ci_detect_changes.py --base origin/main --current HEAD

# Output shows which services would be tested in CI
```

## Common Scenarios

### I modified only `frontend/`

```
$ make build-frontend && make test-frontend
```

CI will skip Go and Java tests.

### I updated the shared Go library

```
$ make build && make test
```

CI will test all three Go modules (library, bff, backend) and skip Java/React.

### I modified Java AND frontend

```
$ make build && make test
```

CI will test the changed Java module(s) and frontend; skip unchanged Go services.

## Troubleshooting

**Q: My local tests pass but CI fails**
- A: Ensure you're using the same Go, Java, and Node versions specified in `Makefile` and `.github/workflows/ci.yml`

**Q: Why are all tests running when I only changed `frontend/`?**
- A: Check that you didn't accidentally modify root-level files (Makefile, .github/, etc.). Those trigger all tests.

**Q: Can I skip CI?**
- A: No—all PRs and pushes to main run CI. This ensures code quality.

## Questions?

- See `docs/ARCHITECTURE.md` for system design
- See `docs/SERVICES.md` for service-specific details
- See `AGENTS.md` for monorepo conventions

Happy coding! 🚀
