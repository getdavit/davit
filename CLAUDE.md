# Davit — Claude Code Instructions

## Development Environment

**Do not modify the local system.** This rule is absolute:

- Do not install any tools, packages, runtimes, or compilers on the local machine.
- Do not run `brew`, `apt`, `go install`, `npm install -g`, or any other system-level package manager commands.
- Do not create or modify system configuration files outside this project directory.

**Use Docker for everything that is not already available locally.** If a task requires a tool (Go compiler, linter, test runner, database migration tool, etc.) that is not confirmed to be installed on the local machine, run it inside a Docker container. Use `docker run --rm` with a volume mount to the project directory. Example pattern:

```sh
docker run --rm -v "$(pwd)":/app -w /app golang:1.24 go build ./...
```

## Project Structure

Keep the project directory clean and well-structured. Separate concerns strictly:

```
davit/
├── CLAUDE.md             # this file
├── specifications.md     # product specification
├── cmd/                  # main entry points
├── internal/             # private application packages
├── pkg/                  # exported packages (if any)
├── docker/               # Dockerfiles and compose files for dev/test
├── scripts/              # helper shell scripts (non-application)
└── testdata/             # test fixtures
```

- **Do not** place build artefacts, binaries, or generated files in the project root.
- **Do not** scatter dev tooling config files in the root unless the tool requires it (e.g. `go.mod` is fine; ad-hoc scratch files are not).
- **Do not** commit editor configs, OS files (`.DS_Store`), or personal tooling configs.
- Development and test infrastructure (Dockerfiles, compose files, scripts) lives under `docker/` or `scripts/` — never mixed into `cmd/` or `internal/`.

## Building & Testing

All build and test commands must be runnable without any local Go installation:

```sh
# Build
docker run --rm -v "$(pwd)":/app -w /app golang:1.24 go build ./...

# Test
docker run --rm -v "$(pwd)":/app -w /app golang:1.24 go test ./...

# Lint
docker run --rm -v "$(pwd)":/app -w /app golangci/golangci-lint:latest golangci-lint run
```

If a `Makefile` or `scripts/` helper exists, it must wrap Docker invocations — never bare local tool calls.
