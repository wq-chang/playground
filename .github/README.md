# .github/ - CI/CD and Automation

This directory contains GitHub Actions workflows, scripts, and configuration for the playground monorepo's continuous integration and automation.

## Directory Structure

```
.github/
├── README.md                          # This file
├── workflows/
│   └── ci.yml                         # Main CI workflow
├── scripts/
│   ├── README.md                      # Detailed script documentation
│   ├── TESTING.md                     # How to test the scripts
│   └── ci_detect_changes.py           # Change detection for selective CI
└── dependabot.yml                     # Dependency update automation
```

## Quick Links

- **For script documentation**: See [`scripts/README.md`](scripts/README.md)
- **For script testing**: See [`scripts/TESTING.md`](scripts/TESTING.md)
- **For CI workflow details**: See [`workflows/ci.yml`](workflows/ci.yml)

## What Happens in CI

The CI pipeline runs automatically on:
- **Pull requests**: Detect changes, build and test only affected services
- **Pushes to main**: Same as PR, but comparing against previous commit

### Workflow: `ci.yml`

**Jobs in order**:

1. **`detect-changes`** (always runs)
   - Calls `scripts/ci_detect_changes.py` to detect which services changed
   - Outputs: `has_go_changes`, `has_java_changes`, `has_react_changes`, plus module lists
   - Used by downstream jobs to conditionally run builds/tests

2. **`test-go-modules`** (runs if `has_go_changes == true`)
   - Downloads dependencies with `go mod download`
   - Builds changed Go modules
   - Lints with `golangci-lint`
   - Runs tests with `gotestsum`

3. **`test-java-modules`** (runs if `has_java_changes == true`)
   - Runs `mvn test` on changed modules

4. **`test-react-modules`** (runs if `has_react_changes == true`)
   - Runs `npm ci` and `npm test` on changed modules

5. **`post`** (always runs last)
   - Generates a summary of which services were tested and their results
   - Visible in GitHub Actions job summary and PR checks

### Why Selective Testing?

The monorepo has three completely separate tech stacks (Go, Java, React) with different dependencies, build systems, and test frameworks. Running all tests on every commit would be slow and wasteful. Selective testing ensures:
- **Faster feedback**: PRs get results in minutes, not hours
- **Lower resource usage**: Only necessary CI containers spin up
- **Clearer results**: Output isn't cluttered with unrelated service logs

## Scripts

### `ci_detect_changes.py`

Detects which services (Go, Java, React) have changed between two git refs and outputs JSON for GitHub Actions to use.

**Key features**:
- Handles new branches (compares against origin/main)
- Detects shared library changes in Go (triggers all consumers)
- Identifies specific Maven modules by pom.xml location
- Outputs JSON to stdout (for automation) and human summary to stderr (for logs)

**Usage**:
```bash
.github/scripts/ci_detect_changes.py --base <ref> --current <ref>
```

**Examples**:
```bash
# Compare two commits
.github/scripts/ci_detect_changes.py --base abc1234 --current def5678

# New branch (compare against main)
.github/scripts/ci_detect_changes.py --base 0 --current HEAD

# Pull request
.github/scripts/ci_detect_changes.py --base origin/main --current HEAD
```

For full documentation, see [`scripts/README.md`](scripts/README.md).  
For testing guide, see [`scripts/TESTING.md`](scripts/TESTING.md).

## Maintenance

### Adding a New Service

If you add a new Go, Java, or React service:

1. **Go**: Add the service directory to `detect_go_changes()` in `scripts/ci_detect_changes.py`
   - Update the `go_prefixes` list
   - Update the shared library trigger (if applicable)

2. **Java**: No changes needed—the script auto-discovers Maven modules by finding `pom.xml` files. Just add a new module directory under `services/java/` with a `pom.xml`.

3. **React**: If adding multiple React apps instead of one, update `detect_react_changes()` to detect each app separately. Currently treats all frontend as one module.

4. **Test locally** before committing:
   ```bash
   python3 -m py_compile .github/scripts/ci_detect_changes.py
   .github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>/dev/null | python3 -m json.tool
   ```

### Changing Directory Structure

If you reorganize the monorepo (e.g., rename `services/go/bff/` to `services/go/api-server/`):

1. Update `scripts/ci_detect_changes.py` with the new paths
2. See "Maintenance: Updating for Directory Structure Changes" in [`scripts/README.md`](scripts/README.md) for detailed steps
3. Create a test commit that touches the new path and verify detection works
4. Update any documentation that references the old paths

### Updating Workflows

Edit `workflows/ci.yml` to:
- Add new jobs (e.g., for a new service type)
- Change triggers (e.g., run on different branches)
- Adjust environment variables (Go version, Node version, etc.)
- Modify build/test commands

See the [GitHub Actions documentation](https://docs.github.com/en/actions) for workflow syntax.

## Common Issues

| Issue | Solution |
|-------|----------|
| CI job doesn't run even though I changed a service | Check `scripts/ci_detect_changes.py` detects the service. Run locally: `.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>/dev/null \| jq` |
| Script errors with `fatal: bad revision 'origin/main'` | Repository isn't configured with upstream remote. Either configure git remote or use a commit SHA instead of branch name. |
| CI job runs but service test fails | Check the job logs in GitHub Actions. Look for the service-specific error (e.g., Go build error, Java test failure). |
| Want to run tests locally without pushing | Use the script directly: `.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD` then run `make build-*` manually. |
| Workflow YAML syntax error | Check `.github/workflows/ci.yml` syntax with: `python3 -m yaml -c <file>` or use [GitHub's workflow validator](https://docs.github.com/en/actions/learn-github-actions/workflow-syntax-for-github-actions). |

## Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Workflow Syntax Reference](https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions)
- [Available Runners](https://docs.github.com/en/actions/using-github-hosted-runners/about-github-hosted-runners)
- [Script Development Tips](scripts/README.md)
