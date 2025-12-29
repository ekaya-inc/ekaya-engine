#!/bin/bash
# test-model-outputs.sh - Test LLM response parsing across multiple models
#
# Usage: ./scripts/test-model-outputs.sh [options]
#
# Options:
#   -timeout duration  Timeout for each model call (default 120s)

set -e

cd "$(dirname "$0")/.."

echo "Building test-model-outputs..."
go build -o bin/test-model-outputs ./scripts/test-model-outputs/...

echo ""
./bin/test-model-outputs "$@"
