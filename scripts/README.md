# Monorepo Automation Scripts

This directory contains scripts to automate common tasks across the monorepo services.

## Scripts

### build-all.sh

Builds all services in the monorepo (Go, Java, and Frontend).

**Usage:**

```bash
./build-all.sh
```

**What it does:**

- Builds Go services with `go build`
- Builds Java services with Maven clean package
- Installs frontend dependencies and builds the frontend

**When to use:** After pulling new changes or before deployment to ensure all services compile successfully.

---

### test-all.sh

Runs test suites for all services and provides a summary.

**Usage:**

```bash
./test-all.sh
```

**What it does:**

- Runs Go tests with `go test`
- Runs Java tests with Maven test
- Runs frontend tests with npm test
- Displays a summary of all test results

**When to use:** Before committing code or as part of CI/CD pipeline to verify all services pass their tests.

---

### lint-all.sh

Lints code across all services to check for style and quality issues.

**Usage:**

```bash
./lint-all.sh
```

**What it does:**

- Lints Go services with golangci-lint
- Lints Java services with Maven spotless:check
- Lints frontend code with npm run lint

**When to use:** To verify code quality and style compliance before pushing changes. Fails if any linter finds issues.

---

### format-all.sh

Automatically formats code across all services according to language standards.

**Usage:**

```bash
./format-all.sh
```

**What it does:**

- Formats Go code with gofmt and golangci-lint --fix
- Formats Java code with Maven spotless:apply
- Formats frontend code with npm run format

**When to use:** To automatically fix formatting issues across the monorepo. Useful before committing or as a development workflow step.

---

## Quick Start

Make scripts executable:

```bash
chmod +x build-all.sh test-all.sh lint-all.sh format-all.sh
```

Run a typical development workflow:

```bash
./format-all.sh  # Format code
./lint-all.sh    # Check quality
./build-all.sh   # Build everything
./test-all.sh    # Run all tests
```

## Notes

- All scripts exit with error status if any step fails (except test-all.sh which reports but doesn't exit on failure)
- Scripts assume they are run from the monorepo root directory
- Each script changes to the appropriate service directory and returns to the previous directory
