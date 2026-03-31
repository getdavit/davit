#!/usr/bin/env bash
set -euo pipefail

echo "Running tests..."

docker run --rm \
  -v "$(cd "$(dirname "$0")/.." && pwd)":/app \
  -w /app \
  -e CGO_ENABLED=0 \
  golang:1.22-alpine \
  sh -c "apk add --no-cache git && go test ./..."
