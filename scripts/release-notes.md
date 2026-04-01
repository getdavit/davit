## v0.2.0 — App Lifecycle

This release completes the application lifecycle surface: every app can now be stopped, started, restarted, or removed via a single idempotent JSON command. Environment variables are stored AES-256-GCM encrypted in the SQLite state store and written to a temporary `.env` file at deploy time. Two new top-level commands — `davit logs` and `davit diagnose` — give operators and AI agents real-time visibility into running applications.

### New commands

| Command | Description |
|---|---|
| `davit app stop <name>` | Stop containers and remove Caddy route |
| `davit app start <name>` | Start containers and re-register Caddy route |
| `davit app restart <name>` | In-place container restart with health check |
| `davit app remove <name>` | Tear down and soft-delete (add `--purge-data` to wipe disk) |
| `davit app env set <app> <KEY> <VALUE>` | Store an encrypted env var (add `--redeploy` to redeploy immediately) |
| `davit app env get <app> <KEY>` | Retrieve a decrypted env var |
| `davit app env list <app>` | List all env var keys for an app |
| `davit app env unset <app> <KEY>` | Remove an env var |
| `davit logs <name>` | Tail or stream Docker Compose logs (`--tail`, `--follow`) |
| `davit diagnose <name>` | JSON snapshot of app health, container state, Caddy route, and recent deployments |

### Changes

- **Encrypted env vars** — AES-256-GCM, per-installation key auto-generated on first use and stored in the SQLite `system_info` table. The `.env` file written to disk at deploy time is removed automatically if no vars are set.
- **Caddy `RouteExists`** — new helper on the Caddy client used by `diagnose` to report whether a route is active.
- **Deploy now writes `.env`** — `davit app deploy` decrypts all stored env vars and writes them to `<appDir>/.env` before running `docker compose up`.
- **`RecentDeployments` DB method** — returns the last N deployment records for an app; used by `diagnose`.

### Error codes added

| Code | Exit | Meaning |
|---|---|---|
| `DOCKER_STOP_FAILED` | 35 | `docker compose down` returned non-zero |
| `ENV_KEY_NOT_FOUND` | 26 | Requested env key does not exist for this app |
