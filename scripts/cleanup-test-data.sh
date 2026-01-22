#!/bin/bash
# Clean up test-like glossary terms from the database
# Usage: ./scripts/cleanup-test-data.sh <project-id> [-dry-run=false]
#
# This tool removes glossary terms matching test patterns (case-insensitive):
# - ^test (starts with "test")
# - test$ (ends with "test")
# - ^uitest (UI test prefix)
# - ^debug (debug prefix)
# - ^todo (todo prefix)
# - ^fixme (fixme prefix)
# - ^dummy (dummy prefix)
# - ^sample (sample prefix)
# - ^example (example prefix)
# - \d{4}$ (ends with 4 digits, e.g., "Term2026")
#
# IMPORTANT: Keep in sync with testTermPatterns in scripts/cleanup-test-data/main.go
#
# By default runs in dry-run mode (shows what would be deleted).
# Use -dry-run=false to actually delete.
#
# Requires:
#   - PG* environment variables for database connection
#
# Output: List of deleted terms with counts

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if [ -z "$1" ]; then
    echo "Usage: $0 <project-id> [-dry-run=false]" >&2
    echo "" >&2
    echo "Example (dry run): $0 f2324998-64c0-46e7-98d1-8a778be462f2" >&2
    echo "Example (delete):  $0 f2324998-64c0-46e7-98d1-8a778be462f2 -dry-run=false" >&2
    exit 1
fi

cd "$PROJECT_ROOT"
go run ./scripts/cleanup-test-data "$@"
