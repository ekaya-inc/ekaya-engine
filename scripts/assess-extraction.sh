#!/bin/bash
# Assess ontology extraction quality for a given project
# Usage: ./scripts/assess-extraction.sh <project-id>
#
# This tool rates how well the ontology extraction system worked with the
# data it was given. A score of 100 means perfect extraction - the system
# did everything possible with the available inputs.
#
# Unlike assess-ontology (which evaluates overall ontology quality including
# knowledge gaps), this tool focuses on extraction accuracy:
# - Did we extract the correct information to give to the LLM?
# - Did the LLM generate the ontology correctly from that input?
# - Are questions reasonable given what is known?
# - Is required vs optional classification appropriate?
#
# Knowledge gaps do NOT affect the score - those are input data problems,
# not extraction problems.
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
go run ./scripts/assess-extraction "$1"
