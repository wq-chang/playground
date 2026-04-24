#!/usr/bin/env python3
"""
CI Change Detection Script for Monorepo

Detects which services have changed between two git refs and outputs JSON
compatible with GitHub Actions workflow.

Usage:
    ci_detect_changes.py --base <ref> --current <ref>

Output:
    JSON to stdout (for programmatic parsing)
    Human-readable summary to stderr (for CI logs)
"""

import json
import subprocess
import sys
from pathlib import Path


def run_git_command(args: list[str]) -> str:
    """
    Execute a git command and return stdout.

    This is the central point for all git operations. It captures both stdout and
    stderr to provide descriptive error messages to callers.

    Args:
        args: List of git command arguments (without 'git' prefix).
              Example: ["diff", "--name-only", "main...HEAD"]

    Returns:
        stdout as string (stripped of leading/trailing whitespace).

    Raises:
        RuntimeError: If git command returns non-zero exit code, with descriptive error.
    """
    try:
        result = subprocess.run(
            ["git"] + args,
            cwd=Path.cwd(),
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout.strip()
    except subprocess.CalledProcessError as e:
        error_msg = (e.stderr or str(e)).strip()
        raise RuntimeError(f"git command failed: {error_msg}") from e


def is_new_branch(ref: str) -> bool:
    """
    Check if ref is all zeros (new branch marker in CI).

    GitHub Actions sets github.event.before to all zeros when pushing to a new branch.
    This function recognizes all common representations:
    - "0000000000000000000000000000000000000000" (full null SHA)
    - "0" * 40 (40 zeros)
    - "0" (single zero, though less common)

    Args:
        ref: Git reference string to check.

    Returns:
        True if ref is a null marker (new branch), False otherwise.
    """
    return (
        ref == "0000000000000000000000000000000000000000"
        or ref == "0" * 40
        or ref == "0"
    )


def resolve_base_ref(base_ref: str) -> str:
    """
    Resolve base ref, handling new branch case.

    When a new branch is pushed for the first time, GitHub Actions sets github.event.before
    to all zeros. This function detects that case and falls back to origin/main (or
    origin/master as a backup). For all other refs, it verifies they exist and returns them.

    EDGE CASES:
    - New branch (base_ref = all zeros) → Uses origin/main or origin/master
    - Non-existent ref → Raises RuntimeError with clear message
    - Detached HEAD or other valid refs → Returns as-is if they exist

    Args:
        base_ref: The base git ref (SHA, branch name, tag, or null marker)

    Returns:
        Resolved git ref that can be used for git diff operations.
        For new branches, returns "origin/main" or "origin/master".

    Raises:
        RuntimeError: If ref cannot be resolved (not a new branch and doesn't exist,
                     or is a new branch but neither origin/main nor origin/master exist).
    """
    # Check if this is a new branch (all zeros)
    if is_new_branch(base_ref):
        msg = "ℹ️  New branch detected (base is all zeros). Using origin/main as base."
        print(msg, file=sys.stderr)
        # Verify origin/main exists
        try:
            _ = run_git_command(["rev-parse", "--verify", "origin/main"])
            return "origin/main"
        except RuntimeError:
            try:
                # Fallback to origin/master if origin/main doesn't exist
                _ = run_git_command(["rev-parse", "--verify", "origin/master"])
                return "origin/master"
            except RuntimeError as e:
                msg = (
                    "Cannot resolve base ref: new branch but origin/main and "
                    "origin/master do not exist"
                )
                raise RuntimeError(msg) from e

    # Verify ref exists
    try:
        _ = run_git_command(["rev-parse", "--verify", base_ref])
        return base_ref
    except RuntimeError as e:
        raise RuntimeError(f"Base ref '{base_ref}' does not exist") from e


def get_changed_files(base_ref: str, current_ref: str) -> list[str]:
    """
    Get list of changed files between two refs.

    Uses three-dot diff syntax (base...current) to properly handle merge commits and
    ensure we get the actual differences between the two branches. This is important
    when dealing with pull requests where the base branch may have advanced.

    EDGE CASES:
    - base_ref and current_ref are the same → Returns empty list
    - Merge commits → Three-dot syntax correctly shows what changed
    - Empty diff → Returns empty list (no RuntimeError)

    Args:
        base_ref: Base git ref (SHA, branch name, or tag)
        current_ref: Current git ref (SHA, branch name, or tag)

    Returns:
        List of changed file paths. Returns empty list if no changes.
        Files are stripped of whitespace.

    Raises:
        RuntimeError: If current_ref doesn't exist or diff command fails.
    """
    # Verify current ref exists
    try:
        _ = run_git_command(["rev-parse", "--verify", current_ref])
    except RuntimeError as e:
        raise RuntimeError(f"Current ref '{current_ref}' does not exist") from e

    # Get diff using three-dot syntax (shows changes from base to current, relative to merge-base)
    try:
        output = run_git_command(["diff", "--name-only", f"{base_ref}...{current_ref}"])
        return [f.strip() for f in output.split("\n") if f.strip()]
    except RuntimeError as e:
        raise RuntimeError(
            f"Failed to get diff between {base_ref} and {current_ref}"
        ) from e


def detect_go_changes(files: list[str]) -> dict[str, bool | list[str]]:
    """
    Detect Go service changes.

    Monorepo structure uses a single go.mod at services/go/ with multiple service
    binaries (bff, backend) and a shared library. This function identifies which
    top-level services are affected by the changes.

    IMPORTANT: If the shared library (services/go/library/) changes, all consumers
    are marked for rebuild to ensure they use the updated library. This prevents
    subtle bugs from stale dependencies.

    Structure:
        services/go/
        ├── go.mod (single module for entire Go monorepo)
        ├── bff/main.go
        ├── backend/main.go
        └── library/ (shared packages)

    Args:
        files: List of changed file paths

    Returns:
        Dict with:
        - has_go_changes (bool): True if any Go files changed
        - go_modules (list): Paths to affected modules (e.g., ["./services/go/bff"])
                             If library changes, includes all consumers.
    """
    go_prefixes = ["services/go/bff/", "services/go/backend/", "services/go/library/"]
    modules: set[str] = set()

    for file_path in files:
        for prefix in go_prefixes:
            if file_path.startswith(prefix):
                # Extract module name (bff, backend, or library)
                module = prefix.rstrip("/")
                modules.add(f"./{module}")
                break

    # CRITICAL: If library changes, trigger all Go modules to rebuild with updated library.
    # This ensures all services get the latest code from the shared library.
    if "./services/go/library" in modules:
        modules.add("./services/go/bff")
        modules.add("./services/go/backend")

    return {
        "has_go_changes": len(modules) > 0,
        "go_modules": sorted(list(modules)),
    }


def detect_java_changes(files: list[str]) -> dict[str, bool | list[str]]:
    """
    Detect Java service changes.

    Maven multi-module project with a hierarchical structure. This function identifies
    the most specific module (deepest pom.xml ancestor) for each changed file, allowing
    precise CI targeting of only affected modules.

    Structure:
        services/java/
        ├── pom.xml (parent, Java 25 by default)
        ├── keycloak-custom/ (Java 21 override for Keycloak compatibility)
        │   ├── pom.xml
        │   └── keycloak-user-event-listener/
        │       └── pom.xml
        └── [future] reporting-service/ (Java 25)

    ALGORITHM:
    1. Find all pom.xml files in services/java/ (git find)
    2. For each changed file, find matching poms (where file is under pom parent dir)
    3. Choose the DEEPEST matching pom (most specific module)
    4. Add that module's directory to the result

    EDGE CASES:
    - Parent pom.xml changes (e.g., dependency update) → Only parent module triggered
      (other modules inherit automatically)
    - Keycloak SPI changes → Only keycloak-user-event-listener triggered
    - Files outside services/java/ → Ignored (no Java changes)

    Args:
        files: List of changed file paths

    Returns:
        Dict with:
        - has_java_changes (bool): True if any Java files changed
        - java_modules (list): Paths to affected Maven modules (e.g.,
                               ["./services/java/keycloak-custom/keycloak-user-event-listener"])
    """
    java_prefix = "services/java/"
    modules: set[str] = set()

    # Find all pom.xml files in services/java to identify available modules
    try:
        pom_files = run_git_command(
            ["find", "services/java", "-name", "pom.xml", "-type", "f"]
        )
        available_poms = [p.strip() for p in pom_files.split("\n") if p.strip()]
    except RuntimeError:
        # Fallback if git find doesn't work (should rarely happen, but defensive coding)
        available_poms = [
            "services/java/pom.xml",
            "services/java/keycloak-custom/pom.xml",
            "services/java/keycloak-custom/keycloak-user-event-listener/pom.xml",
        ]

    for file_path in files:
        if not file_path.startswith(java_prefix):
            continue

        # Find the closest pom.xml ancestor for this file.
        # We match by checking if the file is under the pom's parent directory.
        matching_poms = [
            pom
            for pom in available_poms
            if file_path.startswith(str(Path(pom).parent) + "/")
        ]

        if matching_poms:
            # Get the deepest matching pom (most specific module).
            # Deepest = highest number of "/" separators in the path
            closest_pom = max(matching_poms, key=lambda p: len(p.split("/")))
            # Add the module directory (parent of pom.xml)
            module_dir = str(Path(closest_pom).parent)
            modules.add(f"./{module_dir}")

    return {
        "has_java_changes": len(modules) > 0,
        "java_modules": sorted(list(modules)),
    }


def detect_react_changes(files: list[str]) -> dict[str, bool | list[str]]:
    """
    Detect React service changes.

    Currently, all frontend is in a single React application. Any change in the
    frontend/ directory triggers a build and test of the entire frontend. If the
    frontend grows to multiple packages/workspaces, this logic can be refined to
    detect more specific changes.

    Args:
        files: List of changed file paths

    Returns:
        Dict with:
        - has_react_changes (bool): True if any files under frontend/ changed
        - react_modules (list): ["./frontend"] if changes, empty list otherwise
    """
    react_prefix = "frontend/"
    has_changes = any(f.startswith(react_prefix) for f in files)

    return {
        "has_react_changes": has_changes,
        "react_modules": ["./frontend"] if has_changes else [],
    }


def print_summary(output: dict[str, bool | list[str]], files: list[str]) -> None:
    """
    Print human-readable summary to stderr for CI logs.

    This is intended for human consumption in CI logs, while JSON is written to
    stdout for programmatic use. The summary is printed to stderr to avoid mixing
    with JSON output.

    Args:
        output: Detection results dict (has_*_changes, *_modules)
        files: List of changed files (used only for counting)
    """
    print("", file=sys.stderr)
    print("=" * 70, file=sys.stderr)
    print("📊 Change Detection Summary", file=sys.stderr)
    print("=" * 70, file=sys.stderr)

    print(f"\n📝 Total changed files: {len(files)}", file=sys.stderr)

    if output["has_go_changes"]:
        print(
            f"✅ Go services changed: {', '.join(output['go_modules'])}",
            file=sys.stderr,
        )
    else:
        print("⏭️  No Go services changed", file=sys.stderr)

    if output["has_java_changes"]:
        print(
            f"✅ Java services changed: {', '.join(output['java_modules'])}",
            file=sys.stderr,
        )
    else:
        print("⏭️  No Java services changed", file=sys.stderr)

    if output["has_react_changes"]:
        print(
            f"✅ React services changed: {', '.join(output['react_modules'])}",
            file=sys.stderr,
        )
    else:
        print("⏭️  No React services changed", file=sys.stderr)

    print("=" * 70, file=sys.stderr)
    print("", file=sys.stderr)


def main() -> int:
    """
    Main entry point.

    WORKFLOW:
    1. Parse command-line arguments (--base and --current refs)
    2. Resolve base ref (handle special case of new branches)
    3. Get list of changed files between base and current
    4. Detect changes for each service type (Go, Java, React)
    5. Print human-readable summary to stderr (for CI logs)
    6. Print JSON to stdout (for programmatic use)

    OUTPUT:
    - Exit code 0: Success (even if no changes detected)
    - Exit code 1: Error (ref doesn't exist, git command failed, etc.)

    STDERR: Human-readable summary with emoji indicators
    STDOUT: JSON output (must be valid for downstream processing)
    """
    # Parse command-line arguments
    if len(sys.argv) < 5 or sys.argv[1] != "--base" or sys.argv[3] != "--current":
        msg = (
            "Usage: ci_detect_changes.py --base <ref> --current <ref>\n"
            "  ref: Git SHA, branch name, or tag"
        )
        print(msg, file=sys.stderr)
        return 1

    base_ref = sys.argv[2]
    current_ref = sys.argv[4]

    try:
        # Resolve base ref (handle new branch case where base is all zeros)
        resolved_base = resolve_base_ref(base_ref)

        # Get changed files between base and current
        changed_files = get_changed_files(resolved_base, current_ref)

        # Detect changes by service type
        go_result = detect_go_changes(changed_files)
        java_result = detect_java_changes(changed_files)
        react_result = detect_react_changes(changed_files)

        # Combine results into single output dict
        output = {
            **go_result,
            **java_result,
            **react_result,
        }

        # Print human-readable summary to stderr (visible in CI logs)
        print_summary(output, changed_files)

        # Print JSON to stdout (for GitHub Actions to parse and set outputs)
        print(json.dumps(output, indent=2))

        return 0

    except RuntimeError as e:
        print(f"❌ Error: {e}", file=sys.stderr)
        return 1
    except Exception as e:
        print(f"❌ Unexpected error: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
