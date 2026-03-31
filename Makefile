.PHONY: build build-arm64 build-all test lint clean release

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)

build:
	bash scripts/build.sh

build-arm64:
	GOARCH=arm64 bash scripts/build.sh

build-all: build build-arm64

test:
	bash scripts/test.sh

lint:
	docker run --rm \
		-v "$(PWD)":/app \
		-w /app \
		golangci/golangci-lint:latest \
		golangci-lint run

clean:
	rm -rf dist/

# Creates a GitHub release and uploads both binaries.
# Requires: gh CLI authenticated (brew install gh && gh auth login)
# Usage: make release VERSION=v0.1.0
release: build-all
	@test -n "$(VERSION)" || (echo "Usage: make release VERSION=v0.1.0" && exit 1)
	@echo "Creating release $(VERSION)..."
	gh release create $(VERSION) \
		dist/davit-linux-amd64 \
		dist/davit-linux-arm64 \
		--title "davit $(VERSION)" \
		--notes-file scripts/release-notes.md
