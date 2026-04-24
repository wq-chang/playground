# CI Detect Changes Script

Python script to detect which services have changed in the monorepo and output JSON compatible with GitHub Actions workflows.

## Overview

After a massive file structure reorganization of the playground monorepo, selective CI testing was needed. This script enables the CI/CD pipeline to:
- Detect which services (Go, Java, React) were actually changed
- Build and test only affected modules (reducing CI time and resource usage)
- Handle edge cases like new branches, merge commits, and repository reorganizations

**The script is essential for efficient CI in polyglot monorepos.** Without it, every commit would trigger a full rebuild of all services, even when only documentation changed.

## Architecture

```
.github/scripts/ci_detect_changes.py
        ↓
    [Parse refs: --base and --current]
        ↓
    [Resolve base ref (handle new branches → origin/main)]
        ↓
    [Get changed files: git diff base...current]
        ↓
    ┌───────────┬──────────────┬──────────────┐
    ↓           ↓              ↓              ↓
 detect_go   detect_java   detect_react   [Combine results]
    ↓           ↓              ↓              ↓
 services/go  services/java  frontend/     [Print to stderr: summary]
    ↓                                       [Print to stdout: JSON]
    └───────────┬──────────────┬──────────────┘
                ↓
         GitHub Actions outputs
                ↓
    [Conditional jobs: build/test only changed services]
```

## Usage

```bash
.github/scripts/ci_detect_changes.py --base <ref> --current <ref>
```

### Arguments

- `--base <ref>`: Base git ref (SHA, branch name, or tag). Use `0` or `0000000000000000000000000000000000000000` for new branches.
- `--current <ref>`: Current git ref (SHA, branch name, or tag)

### Output

**stdout:** JSON with change detection results
```json
{
  "has_go_changes": true/false,
  "go_modules": ["./services/go/bff", "./services/go/backend"],
  "has_java_changes": true/false,
  "java_modules": ["./services/java/keycloak-custom/keycloak-user-event-listener"],
  "has_react_changes": true/false,
  "react_modules": ["./frontend"]
}
```

**stderr:** Human-readable summary for CI logs
```
======================================================================
📊 Change Detection Summary
======================================================================

📝 Total changed files: 5
✅ Go services changed: ./services/go/bff, ./services/go/library
✅ Java services changed: ./services/java/keycloak-custom/keycloak-user-event-listener
⏭️  No React services changed
======================================================================
```

**Exit code:**
- `0`: Success (even if no changes detected)
- `1`: Error (invalid ref, git command failed, etc.)

## Output Format Explained

### `has_go_changes` and `go_modules`
- `true` if any files under `services/go/bff/`, `services/go/backend/`, or `services/go/library/` changed
- `go_modules` lists affected directories (e.g., `["./services/go/bff"]`)
- **Special behavior**: If `services/go/library/` changes, all Go modules (bff + backend) are included to ensure they rebuild with the updated library

### `has_java_changes` and `java_modules`
- `true` if any files under `services/java/` changed
- `java_modules` lists the most specific Maven modules affected
  - Example: If `services/java/keycloak-custom/keycloak-user-event-listener/src/MyClass.java` changed, only that leaf module is included, not the parent modules
  - Determined by finding the deepest `pom.xml` ancestor of each changed file

### `has_react_changes` and `react_modules`
- `true` if any files under `frontend/` changed
- `react_modules` is `["./frontend"]` if changed, empty otherwise
- Currently treats frontend as a single module; can be refined if frontend grows to multiple workspaces

## Examples

### Compare between two commits
```bash
.github/scripts/ci_detect_changes.py --base abc1234 --current def5678
```

### New branch (compare against main)
```bash
.github/scripts/ci_detect_changes.py --base 0000000000000000000000000000000000000000 --current HEAD
```

### Pull request (compare fork against main)
```bash
.github/scripts/ci_detect_changes.py --base origin/main --current HEAD
```

### Local testing: Only frontend changed
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD
```
Output (if only `frontend/src/App.tsx` changed):
```json
{
  "has_go_changes": false,
  "go_modules": [],
  "has_java_changes": false,
  "java_modules": [],
  "has_react_changes": true,
  "react_modules": ["./frontend"]
}
```

### Local testing: Library shared by all Go modules changed
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD
```
Output (if only `services/go/library/utils/helper.go` changed):
```json
{
  "has_go_changes": true,
  "go_modules": [
    "./services/go/backend",
    "./services/go/bff",
    "./services/go/library"
  ],
  "has_java_changes": false,
  "java_modules": [],
  "has_react_changes": false,
  "react_modules": []
}
```
Notice how all three Go modules are included, even though the change was only to the library. This is intentional—it ensures bff and backend are rebuilt with the updated library code.

## Integration with GitHub Actions

### Simple Example: Conditional Job Execution

```yaml
jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      has_go_changes: ${{ steps.detect.outputs.has_go_changes }}
      go_modules: ${{ steps.detect.outputs.go_modules }}
    steps:
      - uses: actions/checkout@v6
        with:
          fetch-depth: 10

      - name: Set up Python 3
        uses: actions/setup-python@v5
        with:
          python-version: "3.12"

      - name: Detect changed modules
        id: detect
        run: |
          RESULT=$(python3 .github/scripts/ci_detect_changes.py \
            --base "${{ github.event.pull_request.base.sha }}" \
            --current "${{ github.sha }}")
          
          echo "has_go_changes=$(echo "$RESULT" | python3 -c 'import json, sys; print(json.load(sys.stdin)["has_go_changes"])')" >> $GITHUB_OUTPUT
          echo "go_modules=$(echo "$RESULT" | python3 -c 'import json, sys; print(" ".join(json.load(sys.stdin)["go_modules"]))')" >> $GITHUB_OUTPUT

  test-go:
    needs: detect-changes
    if: needs.detect-changes.outputs.has_go_changes == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - run: |
          for module in ${{ needs.detect-changes.outputs.go_modules }}; do
            make test-go MODULE=$module
          done
```

See `.github/workflows/ci.yml` for the complete implementation.

## Edge Cases Handled

| Case | Behavior |
|------|----------|
| **New branch (base = all zeros)** | Automatically uses `origin/main` (or `origin/master` as fallback) |
| **Library changes in Go** | Triggers rebuild of all Go modules to ensure they use updated library |
| **Shared library not detected** | Script will error with clear message; won't silently skip modules |
| **Same base and current (no changes)** | Returns all `has_*_changes: false` and empty module lists |
| **Invalid/non-existent ref** | Returns helpful error message and exit code 1 |
| **Git command failures** | Returns descriptive error output (stderr shows the actual git error) |
| **Merge commits** | Uses three-dot diff syntax (`base...current`) to correctly show what changed relative to merge-base |
| **Empty diff** | Returns empty file list (not an error); all `has_*_changes` are false |
| **Parent pom.xml changes** | Only parent module returned (child modules inherit through Maven); doesn't trigger unnecessary child rebuilds |

## Maintenance: Updating for Directory Structure Changes

If the monorepo's directory structure changes in the future (e.g., new services, renamed directories), follow these steps:

### 1. Update Go Module Detection
Edit `detect_go_changes()` function in the script:
```python
def detect_go_changes(files: List[str]) -> Dict[str, Any]:
    go_prefixes = [
        "services/go/bff/",
        "services/go/backend/",
        "services/go/library/",
        # ADD HERE: "services/go/newservice/"
    ]
    # ... rest of function
    
    # If library changes, trigger all services:
    if "./services/go/library" in modules:
        modules.add("./services/go/bff")
        modules.add("./services/go/backend")
        # ADD HERE: modules.add("./services/go/newservice")
```

### 2. Update Java Module Detection
If adding new Maven modules, the script auto-discovers them by finding all `pom.xml` files. Update the fallback list if `git find` fails:
```python
def detect_java_changes(files: List[str]) -> Dict[str, Any]:
    # ... 
    except RuntimeError:
        # Fallback if git find doesn't work
        available_poms = [
            "services/java/pom.xml",
            "services/java/keycloak-custom/pom.xml",
            "services/java/keycloak-custom/keycloak-user-event-listener/pom.xml",
            # ADD HERE: "services/java/reporting-service/pom.xml",
        ]
```

### 3. Update React Detection
If frontend splits into multiple workspaces:
```python
def detect_react_changes(files: List[str]) -> Dict[str, Any]:
    # Current: treats all frontend as one module
    # Future: detect multiple packages like:
    react_prefixes = [
        "frontend/",
        # ADD HERE if needed
    ]
```

### 4. Test Changes
After updating the detection functions:
```bash
# Test with local changes
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD

# Verify JSON is valid
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>/dev/null | python3 -m json.tool
```

### 5. Verify in CI
- Create a test PR that touches the new service/module
- Monitor `.github/workflows/ci.yml` logs to confirm the detection is correct
- Check that the right jobs run

## Implementation Details

- **Language**: Python 3 (no external dependencies beyond stdlib)
- **Git operations**: 
  - `git rev-parse --verify <ref>` — validate refs exist
  - `git diff --name-only base...current` — get changed files
  - `git find ...` — discover all pom.xml files
- **Key design decision**: Uses three-dot diff syntax (`base...current`) instead of two-dot (`base..current`) to handle merge commits and force pushes correctly
- **Output streams**: JSON to stdout (for programmatic parsing), human summary to stderr (visible in CI logs), separate streams allow clean piping

## Troubleshooting

| Issue | Solution |
|-------|----------|
| `Error: Base ref 'origin/main' does not exist` | Repository doesn't have upstream configured. Add `-c branch.autosetuprebase=always` to git config or check remote URL. |
| Script returns empty modules but files definitely changed | Verify file paths start with correct prefix (e.g., `services/go/`, not `services-go/` or similar). Check `.gitignore` isn't excluding the files. |
| JSON output on stdout is mangled with error text | Redirect stderr separately: `.../ci_detect_changes.py ... 2>/dev/null \| python3 -m json.tool` |
| All modules triggered even though only docs changed | Unlikely—script only detects changes under `services/go/`, `services/java/`, `frontend/`. Docs changes should not trigger anything. |


