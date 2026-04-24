# CI Pipeline Architecture

This document describes the Playground CI/CD system, including change detection, selective testing, and workflow execution.

---

## Overview

The CI pipeline automatically detects which services changed and runs selective tests to speed up feedback. This is critical for a polyglot monorepo where not all changes require full test suite execution.

**Key components:**

- **Change Detection Script** (`.github/scripts/ci_detect_changes.py`) - Identifies which services changed
- **GitHub Actions Workflow** (`.github/workflows/ci.yml`) - Executes conditional jobs based on detection results
- **Local Testing** - Developers can test detection locally before pushing

---

## Change Detection Script

### Location

`.github/scripts/ci_detect_changes.py`

### Purpose

Compares two git refs and determines which services (Go, Java, React) have changes. Outputs JSON for GitHub Actions workflow consumption.

### Usage

```bash
# Compare current branch against origin/main
python3 .github/scripts/ci_detect_changes.py --base origin/main --current HEAD

# Compare specific commits
python3 .github/scripts/ci_detect_changes.py --base abc1234 --current def5678

# For new branches (no base commit)
python3 .github/scripts/ci_detect_changes.py --base 0 --current HEAD
```

### Detection Logic

#### Go Services

**Directory structure:** `services/go/{bff,backend,library}`

**Detection:**
- Files matching `services/go/bff/*` → trigger BFF module tests
- Files matching `services/go/backend/*` → trigger Backend module tests
- Files matching `services/go/library/*` → trigger **all Go modules** (BFF + Backend depend on shared library)

**Output:**

```json
{
  "has_go_changes": true,
  "go_modules": ["./services/go/library", "./services/go/bff", "./services/go/backend"]
}
```

#### Java Services

**Directory structure:** Maven multi-module at `services/java/`

**Detection:**
- Finds all `pom.xml` files in `services/java/`
- For each changed file, locates closest ancestor `pom.xml`
- Determines which Maven module owns the file

**Example:**
- Change in `services/java/keycloak-custom/keycloak-user-event-listener/src/Main.java`
- Script finds `services/java/keycloak-custom/keycloak-user-event-listener/pom.xml` as closest ancestor
- Outputs: `./services/java/keycloak-custom/keycloak-user-event-listener`

**Output:**

```json
{
  "has_java_changes": true,
  "java_modules": ["./services/java/keycloak-custom"]
}
```

#### React Services

**Directory structure:** Single frontend at `frontend/`

**Detection:**
- Any file matching `frontend/*` → trigger React tests

**Output:**

```json
{
  "has_react_changes": true,
  "react_modules": ["./frontend"]
}
```

### Branch Handling

**New branch detection:**
- If base SHA is all zeros (`0000000000000000000000000000000000000000`), script treats it as a new branch
- Automatically uses `origin/main` as comparison base
- Falls back to `origin/master` if `origin/main` doesn't exist

**Example in CI:**

```yaml
if [ "${{ github.event_name }}" == "pull_request" ]; then
  BASE_SHA="${{ github.event.pull_request.base.sha }}"
  CURRENT_SHA="${{ github.event.pull_request.head.sha }}"
fi

python3 .github/scripts/ci_detect_changes.py \
  --base "$BASE_SHA" \
  --current "$CURRENT_SHA"
```

### Error Handling

- If git command fails: Prints error to stderr, exits with code 1
- If ref doesn't exist: Raises `RuntimeError` with helpful message
- Script outputs both JSON (stdout) and human-readable summary (stderr)

**Example error output:**

```
❌ Error: Base ref 'origin/main' does not exist
```

---

## GitHub Actions Workflow

### Location

`.github/workflows/ci.yml`

### Trigger Events

- **Pull requests:** All PRs trigger change detection + selective tests
- **Push to main:** Pushes to main branch trigger change detection + selective tests
- **Pull request contexts:** Check out `github.event.pull_request.base.sha` (base branch) and `github.event.pull_request.head.sha` (PR branch)
- **Push contexts:** Check out `github.event.before` (previous state) and `github.sha` (current commit)

### Job Structure

```
detect-changes (always runs)
├─ detect-go-changes: true/false
├─ go_modules: ["./services/go/..."]
│
test-go-modules (if go_changes)
├─ Runs golangci-lint
├─ Runs go build
├─ Runs go test
│
test-java-modules (if java_changes)
├─ Runs mvn test
│
test-react-modules (if react_changes)
├─ Runs npm test
│
post (always runs, after all tests)
└─ Generates GitHub Actions summary
```

### Conditional Job Execution

Jobs use GitHub Actions `if` conditions:

```yaml
test-go-modules:
  if: needs.detect-changes.outputs.has_go_changes == 'true'
```

This means:
- If no Go files changed: `test-go-modules` skipped (saves CI minutes)
- If Go files changed: `test-go-modules` runs

### Summary Generation

The `post` job generates a markdown summary for GitHub:

```
### Go Modules Tested
- ./services/go/library
- ./services/go/bff
- ./services/go/backend
✅ All Go lint, build, and tests passed

### Java Modules Tested
- No Java modules changed

### React Modules Tested
- ./frontend
✅ All React tests passed
```

---

## Complete Example: Library Change

### Scenario

Developer modifies `services/go/library/auth/token.go` and pushes PR.

### Step 1: Change Detection

```bash
python3 .github/scripts/ci_detect_changes.py \
  --base main \
  --current pr-branch
```

**Files changed:** `services/go/library/auth/token.go`

**Detection output:**

```json
{
  "has_go_changes": true,
  "go_modules": [
    "./services/go/library",
    "./services/go/bff",
    "./services/go/backend"
  ],
  "has_java_changes": false,
  "java_modules": [],
  "has_react_changes": false,
  "react_modules": []
}
```

**Why three modules?** Because BFF and Backend depend on the shared library, they must be retested.

### Step 2: Workflow Execution

**Jobs executed:**

| Job | Condition | Action |
|-----|-----------|--------|
| detect-changes | Always | ✅ Ran |
| test-go-modules | has_go_changes = true | ✅ Ran for all three modules |
| test-java-modules | has_java_changes = true | ❌ Skipped |
| test-react-modules | has_react_changes = true | ❌ Skipped |
| post | Always | ✅ Ran, generated summary |

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
cd services/go/library && go test ./... && cd -
cd services/go/bff && go test ./... && cd -
cd services/go/backend && go test ./... && cd -
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
- **Contributing guide:** `.github/CONTRIBUTING.md`
- **Monorepo conventions:** `AGENTS.md`
