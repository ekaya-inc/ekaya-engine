#!/bin/bash
# Assess LLM response quality for ontology extraction
# Usage: ./scripts/assess-llm-responses.sh <project-id>
#
# This tool evaluates the LLM RESPONSE quality during ontology extraction:
# - Structural validity: Is JSON parseable and well-formed?
# - Schema compliance: Does response match expected structure for prompt type?
# - Hallucination detection: Do referenced entities exist in actual schema?
# - Completeness: Are all required fields present?
# - Value validation: Are enum values valid? Priority 1-5? Domains non-empty?
#
# This tool does NOT use an LLM for assessment - all checks are deterministic.
# It assesses how well the LLM performed, not the code quality.
#
# Requires:
#   - PG* environment variables for database connection
#
# Output: JSON assessment with detailed scoring

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if [ -z "$1" ]; then
    echo "Usage: $0 <project-id>" >&2
    echo "" >&2
    echo "Example: $0 f2324998-64c0-46e7-98d1-8a778be462f2" >&2
    exit 1
fi

cd "$PROJECT_ROOT"
go run ./scripts/assess-llm-responses "$1"
