#!/usr/bin/env python3
"""
Convert ci_detect_changes.py JSON output to GitHub Actions outputs.

Reads JSON from stdin or file, converts lists to space-separated strings,
and writes to GITHUB_OUTPUT environment variable.

Usage:
    python3 convert_detect_output.py [input_file]
    
Arguments:
    input_file: Path to JSON file (default: stdin)
"""

import json
import os
import sys


def format_modules(modules_list):
    """Convert list of module paths to space-separated string."""
    return ' '.join(modules_list) if modules_list else ''


def main():
    # Read JSON from file or stdin
    if len(sys.argv) > 1:
        input_file = sys.argv[1]
        with open(input_file) as f:
            data = json.load(f)
    else:
        data = json.load(sys.stdin)
    
    # Convert outputs to GitHub Actions format
    outputs = {
        'has_go_changes': 'true' if data['has_go_changes'] else 'false',
        'go_modules': format_modules(data['go_modules']),
        'has_java_changes': 'true' if data['has_java_changes'] else 'false',
        'java_modules': format_modules(data['java_modules']),
        'has_react_changes': 'true' if data['has_react_changes'] else 'false',
        'react_modules': format_modules(data['react_modules']),
    }
    
    # Write to GITHUB_OUTPUT
    if 'GITHUB_OUTPUT' in os.environ:
        with open(os.environ['GITHUB_OUTPUT'], 'a') as f:
            for key, value in outputs.items():
                f.write(f"{key}={value}\n")
    
    # Print to stdout for debugging
    print(json.dumps(outputs, indent=2))


if __name__ == '__main__':
    main()
