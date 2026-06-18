# Security

> Security posture and known tradeoffs for Davit v0.3.

## Design principles

Davit is designed for single-server deployments where the operator has root
access. The threat model assumes:

- **Adversary:** External attacker with network access to ports 22, 80, 443,
  or the webhook endpoint.
- **Trust boundary:** The server root is trusted. Davit's state database,
  configuration, and encryption keys are on-disk and accessible to root.
- **Agent access:** AI agents connect via SSH with a restricted Ed25519 key
  (`forced-command` in `authorized_keys`). The agent can only invoke the
  `davit` binary — never a shell.

## Security features

| Feature | Status |
|---|---|
| SSH key with forced-command restriction | ✅ Implemented |
| AES-256-GCM env var encryption | ✅ Implemented |
| SSH hardening (config block, password auth disabled) | ✅ Implemented |
| Firewall (default deny inbound) | ✅ Implemented |
| fail2ban with SSH jail | ✅ Implemented |
| Webhook HMAC-SHA256 signature validation | ✅ Implemented |
| Daemon service systemd hardening | ✅ Implemented |
| Webhook server loopback-only binding | ✅ Implemented |

## Known tradeoffs

These are documented design decisions, not unaddressed vulnerabilities.
They are accepted tradeoffs for a single-user CLI tool.

### Webhook token generation

**Finding:** Webhook tokens are generated using `time.Now().UnixNano()`
formatted as hex, not `crypto/rand`.

**Impact:** Low. An attacker who can predict nanoseconds-since-epoch at
token-creation time could guess the token. In practice, the token is created
once per app (when `watch enable --webhook` runs), and guessing the nanosecond
timestamp is infeasible without access to the server's clock state at that
exact moment.

**Recommendation for v0.4+:** Replace with `crypto/rand` for a stronger
guarantee. Tracked in `internal/state/db.go` (`UpdateAppWatch`).

### Config file permissions

**Finding:** `config.Write()` creates the TOML config file with mode `0644`
(world-readable).

**Impact:** Low — the config file contains no secrets. It stores server
hostname, timezone, admin email, daemon listen address, and Caddy admin API
address. No API keys, tokens, or passwords are written here. Environment
variables are stored encrypted in the SQLite database, not in the config file.

**Recommendation for v0.4+:** Restrict to `0640` as a defence-in-depth
measure. Tracked in `internal/config/config.go` (`Write`).

### Encryption key storage

**Finding:** The AES-256-GCM encryption key for environment variables is
stored in the SQLite `system_info` table alongside the ciphertext.

**Impact:** Expected — this is a single-user tool running on a single server.
The encryption protects against offline theft of the SQLite database file
(e.g. a backup leaking). On the live server, root access implies access to
both the database and the key, which is the intended design. A Hardware
Security Module or key management service is out of scope for v0.3.

**Mitigation:** The key is generated once during `davit server init` and never
written to disk outside the SQLite database. No key file exists on the
filesystem.

### Strict host key checking

**Finding:** Git operations use `StrictHostKeyChecking=no`.

**Impact:** Accepted — Davit manages Git repositories it controls. The setting
is applied per-command via GIT_SSH_COMMAND, not globally. In a future release,
a config option (`git.strict_host_key`) will allow operators to pin known host
keys for their deployment repositories.

### RBAC / multi-tenancy

**Finding:** No role-based access control or user separation.

**Impact:** Accepted — Davit is a single-user CLI tool for a single server.
All operations run as root (via the forced-command SSH key or local shell).
Adding RBAC would add complexity without benefit for the intended use case.

## Reporting a vulnerability

This project is maintained by the community. If you find a security issue,
open a GitHub issue with the "security" label at:
https://github.com/getdavit/davit/issues