# Changelog

All notable changes to Davit are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.3.0] — 2025-06-18

### Added
- **Git automation** — automatic redeployment when watched repositories change,
  with two modes: polling and webhook.
- `davit watch enable <app>` — enable Git watching for an application.
  Supports `--poll-interval` (polling every N seconds, default 30) and
  `--webhook` (HTTP push notification mode).
- `davit watch disable <app>` — disable Git watching for an application.
- `davit watch status [app]` — show watch configuration and service status
  for one or all applications.
- `davit daemon` — run the background Git watcher and webhook receiver
  daemon (managed as a systemd service).
- **Webhook receiver** — listens on `127.0.0.1:2020` and handles push events
  from GitHub, GitLab, and generic Git providers. HMAC-SHA256 signature
  validation via `X-Hub-Signature-256`.
- **Polling scheduler** — periodically checks watched repositories for new
  commits and triggers redeployment when changes are detected.
- **Daemon systemd service** — `davit-watcher.service` with security
  hardening (NoNewPrivileges, ProtectSystem, ProtectHome, etc.).
  Service auto-starts when the first app is enabled and auto-stops when
  the last app is disabled.
- **Webhook setup instructions** — `davit watch enable --webhook` prints
  provider-specific setup steps (GitHub, GitLab, generic).

### Security
- Webhook tokens are validated server-side before any deploy is triggered.
- HMAC-SHA256 signature verification when a webhook secret is configured.
- Daemon service runs with `Type=notify` and restricted systemd hardening.
- Webhook server binds to loopback address only.

### Notes
- Webhook token generation uses `time.Now().UnixNano()` — see
  [SECURITY.md](./SECURITY.md#webhook-token-generation) for the tradeoff.
- Config file permissions default to 0644 — see
  [SECURITY.md](./SECURITY.md#config-file-permissions) for the tradeoff.
- Encryption key is co-located with ciphertext in SQLite —
  see [SECURITY.md](./SECURITY.md#encryption-key-storage) for context.

## [v0.2.0] — 2025-05-15

### Added
- `davit app stop <name>` — stop containers and remove Caddy route.
- `davit app start <name>` — start containers and re-register Caddy route.
- `davit app restart <name>` — in-place container restart with health check.
- `davit app remove <name>` — tear down and soft-delete
  (add `--purge-data` to wipe disk and Docker volumes).
- `davit app env set <app> <KEY> <VALUE>` — store an encrypted environment
  variable (add `--redeploy` to redeploy immediately).
- `davit app env get <app> <KEY>` — retrieve a decrypted env var.
- `davit app env list <app>` — list all env var keys for an app.
- `davit app env unset <app> <KEY>` — remove an env var.
- `davit logs <name>` — tail or stream Docker Compose logs
  (`--tail`, `--follow`).
- `davit diagnose <name>` — JSON snapshot of app health, container state,
  Caddy route, and recent deployments.

### Changed
- **Encrypted env vars** — AES-256-GCM, per-installation key auto-generated
  on first use and stored in the SQLite `system_info` table.
- **Deploy now writes `.env`** — decrypts all stored env vars and writes
  them to `<appDir>/.env` before running `docker compose up`.
- **Caddy `RouteExists`** — new helper used by `diagnose` to report route
  status.

### Error codes added
- `DOCKER_STOP_FAILED` (exit 35) — `docker compose down` returned non-zero.
- `ENV_KEY_NOT_FOUND` (exit 26) — requested env key does not exist.

## [v0.1.0] — 2025-04-01

### Added
- **Server provisioning** — 11 idempotent hardening steps: OS update,
  core utilities, timezone, SSH hardening, firewall, fail2ban, Docker,
  Caddy, directory structure, state DB init, daemon unit.
- **Agent SSH keys** — Ed25519 keypair generation with forced-command
  restriction (`davit agent key create`).
- **Application deployment** — `davit app create`, `davit app deploy`,
  `davit app list` with full Git sync, Docker Compose lifecycle, Caddy
  route management.
- **Multi-distro support** — Ubuntu, Debian, Fedora, RHEL/Rocky/AlmaLinux,
  openSUSE, Arch Linux, Alpine Linux.
- **Multi-arch builds** — `linux/amd64` and `linux/arm64` via Docker.
- **Structured error codes** — machine-readable error envelope with
  stable codes, documented exit conventions.

[v0.3.0]: https://github.com/getdavit/davit/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/getdavit/davit/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/getdavit/davit/releases/tag/v0.1.0
