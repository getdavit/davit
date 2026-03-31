# Davit

> *The crane arm that lifts containers into place.*

Davit is a self-hosted deployment manager for containerised applications. Install it once on a Linux server and it handles the full lifecycle: OS hardening, Docker and Caddy installation, TLS certificate automation, application deployment from any Git repository, and continuous monitoring for automatic redeployment.

It ships as a **single static binary** with two interaction surfaces:

- A **JSON CLI** designed to be driven by AI agents and automation over SSH.
- An **interactive TUI** for human operators (planned for v0.4).

## Current status

**v0.1 — Foundation** (current)

The foundation milestone delivers a fully provisionable server and a working application deployment pipeline.

## Quick start

```sh
# On a fresh Linux server
curl -fsSL https://raw.githubusercontent.com/getdavit/davit/main/scripts/install.sh | sudo bash
```

Or manually:

```sh
# Download the binary
curl -Lo /usr/local/bin/davit https://github.com/getdavit/davit/releases/latest/download/davit-linux-amd64
chmod +x /usr/local/bin/davit

# Provision the server
davit server init --email admin@example.com --timezone Europe/London

# Deploy an application
davit app create myapi --repo https://github.com/org/myapi --domain api.example.com
davit app deploy myapi
```

## Commands (v0.1)

All commands produce JSON by default when stdout is not a terminal, and human-readable output when it is. Use `--json` or `--pretty` to override.

### Server

```
davit server init     Provision this server (idempotent)
davit server status   Show current server health snapshot
```

`server init` runs 11 hardening steps in sequence. Each step is idempotent — re-running it is always safe.

| Step | What it does |
|---|---|
| `system_update` | `apt-get update && upgrade` (or distro equivalent) |
| `install_core_utils` | curl, git, wget, gnupg, vim, htop, unzip |
| `configure_timezone` | Sets timezone via timedatectl or /etc/localtime |
| `ssh_hardening` | Appends a managed block to sshd_config; validates before restarting |
| `configure_firewall` | Opens 22/80/443 tcp, 443 udp; default deny inbound |
| `install_fail2ban` | Installs fail2ban with SSH jail; 5 retries, 1h ban |
| `install_docker` | Docker Engine from official repositories, verified with hello-world |
| `install_caddy` | Caddy from official repositories; writes minimal Caddyfile |
| `create_dir_structure` | `/etc/davit`, `/var/lib/davit`, `/var/log/davit`, `/run/davit` |
| `init_state_db` | Initialises the SQLite state database |
| `install_daemon_unit` | Writes `davit-daemon.service` systemd unit |

Supported flags:

```
--email string       Admin email for Let's Encrypt (required)
--timezone string    Timezone (e.g. Europe/London). Default: UTC
--skip-steps string  Comma-separated list of step names to skip
--dry-run            Print what would happen without applying changes
```

### Applications

```
davit app create <name>   Register an application (does not deploy)
davit app deploy <name>   Deploy (git sync + docker compose + Caddy route)
davit app list            List all applications
```

`app create` flags:

```
--repo string          Git repository URL (required)
--domain string        Domain to serve on (required)
--branch string        Branch to track. Default: main
--port int             Container port. Default: 3000
--compose-file string  Path to docker-compose.yml in repo. Default: docker-compose.yml
--build-context string Docker build context. Default: .
--deploy-key string    SSH private key path for private repos
```

`app deploy` flags:

```
--timeout int   Seconds to wait for health check. Default: 60
--force         Deploy even if commit is unchanged
```

### Agent keys

```
davit agent key create   Generate an Ed25519 SSH keypair for agent access
```

Writes `davit-agent.pem` and `davit-agent.pub` to the output directory and installs the public key into `/root/.ssh/authorized_keys` with a forced-command restriction — the key can only run `davit` commands, never an interactive shell.

```
--label string    Human-readable label. Default: agent
--output string   Directory for key files. Default: .
```

## Agent access

An AI agent (e.g. Claude Code) connects over SSH using the agent key:

```sh
# Check status
ssh -i davit-agent.pem root@your-server davit app list

# Deploy
ssh -i davit-agent.pem root@your-server davit app deploy myapi

# Read logs
ssh -i davit-agent.pem root@your-server davit logs myapi --lines 200
```

All output is JSON. The forced-command entry in `authorized_keys` prevents the agent from accessing a shell.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    davit binary (Go)                     │
│                                                          │
│   ┌──────────────┐      ┌───────────────────────────┐   │
│   │  Core Engine │      │  State Store              │   │
│   │  Provisioner │      │  SQLite + TOML config     │   │
│   │  App Manager │      └───────────────────────────┘   │
│   │  Caddy Client│                                       │
│   └──────┬───────┘                                       │
│          │                                               │
│   ┌──────▼──────┐   ┌──────────────────────────────┐    │
│   │  JSON CLI   │   │  Interactive TUI (v0.4)       │    │
│   │  (agents)   │   │  Bubble Tea                  │    │
│   └─────────────┘   └──────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

**State store:** SQLite at `/var/lib/davit/davit.db`. Config at `/etc/davit/davit.toml`.

**Web layer:** Caddy handles all inbound traffic (80/443). Applications are never exposed directly — they bind to `127.0.0.1:{auto-port}` and Caddy reverse-proxies to them. TLS certificates are issued automatically via ACME HTTP-01.

**Supported Linux distributions:** Ubuntu, Debian, Fedora, RHEL/Rocky/AlmaLinux, openSUSE, Arch Linux, Alpine Linux.

## Building from source

No local Go installation required. Builds run entirely inside Docker.

```sh
# Build linux/amd64
make build

# Build linux/arm64
make build-arm64

# Run tests
make test

# Run linter
make lint
```

The output binary lands in `dist/`.

To build manually:

```sh
docker run --rm \
  -v "$(pwd)":/app -w /app \
  -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=amd64 \
  golang:1.22-alpine \
  go build -o dist/davit-linux-amd64 ./cmd/davit
```

## Project layout

```
cmd/davit/          Entry point
internal/
  agent/            Ed25519 SSH key generation + authorized_keys management
  app/              Application lifecycle (create, deploy, list)
  caddy/            Caddy Admin API client
  cli/              Cobra command definitions
  config/           TOML configuration loading
  firewall/         Firewall abstraction (ufw, firewalld, iptables)
  osdetect/         OS and package manager detection
  output/           JSON/pretty output writer, error codes
  pkgmgr/           Package manager abstraction (apt, dnf, yum, zypper, pacman, apk)
  provisioner/      Server provisioning steps
  state/            SQLite state store and migrations
  version/          Build-time version string
docker/             Dockerfile for reproducible builds
scripts/            build.sh, test.sh
```

## Roadmap

| Version | Scope |
|---|---|
| **v0.1** | **Foundation** — provisioner, app deploy, agent keys ✓ |
| v0.2 | App lifecycle (stop/start/restart/remove), env vars, health check, logs, diagnose |
| v0.3 | Git automation — polling daemon, webhook receiver |
| v0.4 | Interactive TUI (Bubble Tea) |
| v0.5 | Hardening, cert commands, audit log, install script |
| v1.0 | ARM64 builds, test suite, public docs |

## Error codes

All errors return a JSON envelope with a stable `error_code` field:

```json
{
  "status": "error",
  "error_code": "APP_NOT_FOUND",
  "message": "application 'myapi' does not exist",
  "docs_url": "https://github.com/getdavit/davit/wiki/errors/APP_NOT_FOUND"
}
```

Exit codes follow a structured convention: `0` success, `1–9` usage errors, `10–19` provisioning, `20–39` application/deployment, `40–49` web server/TLS, `50–59` state/config, `99` internal.

## License

MIT
