## v0.3.0 — Git Automation

This release adds automatic redeployment when watched Git repositories change. Two complementary modes: polling scheduler (check every N seconds) and webhook receiver (instant push notification). A background daemon — `davit daemon` — runs as a hardened systemd service and handles both modes on a single process.

### New commands

| Command | Description |
|---|---|
| `davit watch enable <app>` | Enable Git watching (polling or webhook) |
| `davit watch disable <app>` | Disable Git watching |
| `davit watch status [app]` | Show watch configuration and service status |
| `davit daemon` | Start the background watcher daemon |

### Polling mode

`davit watch enable myapp --poll-interval 30`

Polls the remote repository every 30 seconds. When a new commit hash is detected, the app is automatically redeployed.

### Webhook mode

`davit watch enable myapp --webhook`

Starts a webhook server on `127.0.0.1:2020` that receives push events from GitHub, GitLab, or any generic Git provider. HMAC-SHA256 signature validation is supported for providers that send `X-Hub-Signature-256`. The webhook URL and setup instructions are printed on enable.

### Security

- **HMAC validation** — optional webhook secret with constant-time comparison
- **Loopback-only** — webhook server binds to `127.0.0.1`, never exposed
- **Hardened service** — `davit-watcher.service` with NoNewPrivileges, ProtectSystem, ProtectHome, and restricted address families
- **Auto service lifecycle** — the daemon starts when the first app is watched, stops when the last is disabled

### Upgrading

No breaking changes from v0.2.0. After upgrading the binary, existing apps can be watched immediately:

```sh
davit watch enable myapp --poll-interval 60
```

For webhook mode, follow the printed setup instructions at your Git provider.