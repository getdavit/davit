#!/usr/bin/env bash
set -euo pipefail

GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}"
OUT="dist/davit-${GOOS}-${GOARCH}"

mkdir -p dist

echo "Building davit ${VERSION} for ${GOOS}/${GOARCH}..."

docker run --rm \
  -v "$(cd "$(dirname "$0")/.." && pwd)":/app \
  -w /app \
  -e CGO_ENABLED=0 \
  -e GOOS="${GOOS}" \
  -e GOARCH="${GOARCH}" \
  golang:1.25-alpine \
  sh -c "apk add --no-cache git && go build -ldflags=\"-s -w -X github.com/getdavit/davit/internal/version.Version=${VERSION}\" -o ${OUT} ./cmd/davit"

echo "Built: ${OUT}"
