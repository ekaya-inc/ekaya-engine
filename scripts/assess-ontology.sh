#!/bin/bash
# Assess how well the final ontology enables SQL query generation.
#
# Goal: Rate how confidently an LLM could navigate the database and create
# correct SQL queries to answer user questions or generate insights.
#
# A score of 100 means:
#   - The ontology is complete (no unknowns)
#   - All relationships documented (or tables clearly standalone)
#   - No required questions pending
#
# Key factors that reduce the score:
#   - Pending required questions (gaps in understanding)
#   - Missing relationships (unclear table connections)
#   - Ambiguous entity descriptions
#   - Undocumented enumeration values
#
# Usage: ./scripts/assess-ontology.sh <project-id>
#
# Requires:
#   - ANTHROPIC_API_KEY environment variable
#   - PG* environment variables for database connection
#
# Output: JSON assessment with final score 0-100

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

if [ -z "$1" ]; then
    echo "Usage: $0 <project-id>" >&2
    echo "" >&2
    echo "Example: $0 f2324998-64c0-46e7-98d1-8a778be462f2" >&2
    exit 1
fi

if [ -z "$ANTHROPIC_API_KEY" ]; then
    echo "Error: ANTHROPIC_API_KEY environment variable required" >&2
    exit 1
fi

cd "$PROJECT_ROOT"
go run ./scripts/assess-ontology "$1"
