# Testing ci_detect_changes.py

This guide covers how to test the change detection script locally and verify its output is correct.

## Quick Start: Run These Tests

Run these commands to verify the script works correctly:

### Test 1: Help message
```bash
.github/scripts/ci_detect_changes.py --help
```
Expected: Argument help and usage information

### Test 2: Valid comparison (no changes expected)
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD
```
Expected output: All `has_*_changes` fields are `false`, all `*_modules` arrays are empty
```json
{
  "has_go_changes": false,
  "go_modules": [],
  "has_java_changes": false,
  "java_modules": [],
  "has_react_changes": false,
  "react_modules": []
}
```

### Test 3: Real commit comparison
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD
```
Expected output: JSON with detected changes (if files changed between HEAD~1 and HEAD)

### Test 4: New branch scenario
```bash
.github/scripts/ci_detect_changes.py --base 0000000000000000000000000000000000000000 --current HEAD
```
Expected: Compares HEAD against origin/main instead. Watch stderr for message: "ℹ️  New branch detected..."

### Test 5: Invalid ref (error handling)
```bash
.github/scripts/ci_detect_changes.py --base invalid-ref --current HEAD
```
Expected: Error message and exit code 1
```
❌ Error: Base ref 'invalid-ref' does not exist
```

### Test 6: Missing required argument
```bash
.github/scripts/ci_detect_changes.py --base HEAD
```
Expected: Argument parser error showing usage

## Testing Output Validation

### Verify JSON output is valid
Suppress stderr to see only JSON, then parse it:
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>/dev/null | python3 -m json.tool
```

This verifies:
- JSON is well-formed (parseable)
- All expected keys are present
- Values are correct types (booleans, arrays)

### Verify exit code
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD
echo "Exit code: $?"
```
Expected: `0` for success, `1` for errors

### Verify stderr output (summary)
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>&1 | head -20
```
This shows the human-readable summary printed to stderr.

### Verify stdout only (JSON)
```bash
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>/dev/null
```
This suppresses stderr, showing only the JSON output suitable for piping to `jq`.

## Testing Key Features

### ✅ Test: Library change detection
When `services/go/library/` changes, all Go modules (bff, backend) should be triggered.

**Setup**: Make a change to `services/go/library/some_file.go`:
```bash
touch services/go/library/testfile.go
git add services/go/library/testfile.go
```

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD 2>/dev/null | jq '.go_modules'
```

**Verify**: Output includes all three:
```json
[
  "./services/go/backend",
  "./services/go/bff",
  "./services/go/library"
]
```

### ✅ Test: New branch handling
When `base_ref` is all zeros, script should fall back to `origin/main`.

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base 0000000000000000000000000000000000000000 --current HEAD 2>&1 | grep "New branch"
```

**Verify**: Stderr contains: `ℹ️  New branch detected (base is all zeros). Using origin/main as base.`

### ✅ Test: Invalid ref error handling
Invalid refs should produce clear error messages and exit code 1.

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base nonexistent-ref-xyz --current HEAD; echo "Exit: $?"
```

**Verify**: 
- Stderr shows: `❌ Error: Base ref 'nonexistent-ref-xyz' does not exist`
- Exit code is `1`

### ✅ Test: Empty diff handling
When base and current are the same, should return no changes.

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD 2>/dev/null | jq 'to_entries | map(.value == false or .value == []) | all'
```

**Verify**: Output is `true` (all fields are `false` or empty arrays)

### ✅ Test: Java module detection
Changes to nested Java modules should detect the deepest (most specific) module.

**Setup**: Make a change to `services/java/keycloak-custom/keycloak-user-event-listener/src/SomeClass.java`:
```bash
touch services/java/keycloak-custom/keycloak-user-event-listener/src/TestClass.java
git add services/java/keycloak-custom/keycloak-user-event-listener/src/TestClass.java
```

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD 2>/dev/null | jq '.java_modules'
```

**Verify**: Output is only the leaf module, not parent:
```json
[
  "./services/java/keycloak-custom/keycloak-user-event-listener"
]
```

### ✅ Test: React detection
Any change under `frontend/` should trigger React.

**Setup**: Make a change to `frontend/src/App.tsx`:
```bash
touch frontend/src/TestComponent.tsx
git add frontend/src/TestComponent.tsx
```

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD 2>/dev/null | jq '.react_modules'
```

**Verify**:
```json
[
  "./frontend"
]
```

### ✅ Test: JSON output format
Verify all expected keys are present and types are correct.

**Run**:
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD 2>/dev/null | jq 'keys'
```

**Verify**: Output contains all six keys:
```json
[
  "go_modules",
  "has_go_changes",
  "has_java_changes",
  "has_react_changes",
  "java_modules",
  "react_modules"
]
```

**Run** (verify types):
```bash
.github/scripts/ci_detect_changes.py --base HEAD --current HEAD 2>/dev/null | jq 'to_entries | map({key: .key, type: (.value | type)}) | from_entries'
```

**Verify**: 
- `has_*` keys are `"boolean"`
- `*_modules` keys are `"array"`

## Batch Testing Script

For automated testing, save this as `test_script.sh`:

```bash
#!/bin/bash
set -e

SCRIPT=".github/scripts/ci_detect_changes.py"
TESTS_PASSED=0
TESTS_FAILED=0

# Test helper
run_test() {
    local name="$1"
    local command="$2"
    local expect_exit="$3"
    
    echo -n "Testing: $name ... "
    
    if bash -c "$command" > /tmp/test_output.txt 2>&1; then
        actual_exit=0
    else
        actual_exit=$?
    fi
    
    if [ "$actual_exit" -eq "$expect_exit" ]; then
        echo "✅ PASS"
        ((TESTS_PASSED++))
    else
        echo "❌ FAIL (expected exit $expect_exit, got $actual_exit)"
        cat /tmp/test_output.txt
        ((TESTS_FAILED++))
    fi
}

# Run tests
run_test "Help message" "$SCRIPT --help" 0
run_test "No changes (HEAD vs HEAD)" "$SCRIPT --base HEAD --current HEAD" 0
run_test "Recent history" "$SCRIPT --base HEAD~1 --current HEAD" 0
run_test "Invalid ref" "$SCRIPT --base invalid-ref --current HEAD" 1
run_test "Missing argument" "$SCRIPT --base HEAD" 2

# Summary
echo ""
echo "Results: $TESTS_PASSED passed, $TESTS_FAILED failed"
[ "$TESTS_FAILED" -eq 0 ] && echo "✅ All tests passed!" || echo "❌ Some tests failed"
exit $TESTS_FAILED
```

Run with:
```bash
chmod +x test_script.sh
./test_script.sh
```

## Integration with CI

The script is tested by the actual CI pipeline when:
1. A PR is opened (runs on head commit vs base branch)
2. A push to main (runs on new commit vs old commit)

**Key CI workflow location**: `.github/workflows/ci.yml`

Monitor CI output at: GitHub Actions → Workflows → CI → detect-changes job

## Testing Locally Before Committing

**Complete pre-commit checklist**:

```bash
# 1. Verify script syntax
python3 -m py_compile .github/scripts/ci_detect_changes.py

# 2. Run basic tests
.github/scripts/ci_detect_changes.py --base HEAD~1 --current HEAD 2>/dev/null | python3 -m json.tool

# 3. Verify exit code on error
.github/scripts/ci_detect_changes.py --base invalid-ref --current HEAD > /dev/null 2>&1 || echo "Error exit code: $?"

# 4. Test new branch scenario
.github/scripts/ci_detect_changes.py --base 0000000000000000000000000000000000000000 --current HEAD 2>&1 | head

# 5. If you modified any detection logic, create test commits:
#    - Add file to services/go/ and verify it's detected
#    - Add file to services/java/ and verify correct module is detected
#    - Add file to frontend/ and verify React is detected
```

## Troubleshooting Failed Tests

| Issue | Solution |
|-------|----------|
| `git command failed: fatal: bad revision 'HEAD~1'` | Repository has fewer than 2 commits. Use `--base HEAD --current HEAD` instead. |
| `Cannot resolve base ref: origin/main does not exist` | Repository isn't configured with remote. Run `git remote add origin <url>` or use `--base HEAD~1`. |
| JSON output contains error text mixed in | Ensure you're redirecting stderr separately: `2>/dev/null` for JSON-only. |
| Script returns exit code 0 but should be 1 | Check stderr to see actual error (it's printed but exit code might differ). Add explicit error checking. |
| Test files left behind after testing | Clean up with: `git clean -fd` (remove untracked files) or manually delete test files. |


