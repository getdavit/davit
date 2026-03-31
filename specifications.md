# Davit — Project Specification
> *The crane arm that lifts containers into place.*

**Version:** 0.1-draft  
**Status:** Pre-development specification  
**Domain:** davit.sh  

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Design Philosophy](#2-design-philosophy)
3. [Architecture Overview](#3-architecture-overview)
4. [Technology Stack](#4-technology-stack)
5. [Installation & Bootstrap](#5-installation--bootstrap)
6. [Server Provisioning Engine](#6-server-provisioning-engine)
7. [Core CLI Reference](#7-core-cli-reference)
8. [Interactive TUI](#8-interactive-tui)
9. [Application Lifecycle](#9-application-lifecycle)
10. [Web Server & TLS](#10-web-server--tls)
11. [Git Integration & Auto-Deploy](#11-git-integration--auto-deploy)
12. [Agent Access Model](#12-agent-access-model)
13. [State & Configuration](#13-state--configuration)
14. [Logging & Diagnostics](#14-logging--diagnostics)
15. [Security Hardening Procedures](#15-security-hardening-procedures)
16. [Error Codes & Machine Contracts](#16-error-codes--machine-contracts)
17. [Directory Layout on Server](#17-directory-layout-on-server)
18. [Development Roadmap](#18-development-roadmap)

---

## 1. Project Overview

Davit is a self-hosted, server-side deployment manager for containerised applications. It is installed once per Linux server and provides two interaction surfaces over a single binary:

- A **structured CLI** designed to be invoked by AI agents and automation over SSH, producing machine-readable JSON output with strict exit codes.
- An **interactive TUI** designed for human operators, providing a guided, keyboard-navigable interface inspired by tools such as Claude Code and OpenClaw.

Davit handles the full lifecycle of a deployment server: initial OS hardening, Docker installation, web server configuration, TLS certificate management, application deployment from any Git repository, and continuous monitoring of branches for automatic redeployment.

It is explicitly **not** a hosted service, a SaaS platform, or a multi-server orchestrator. It is a single binary that lives on the server it manages.

---

## 2. Design Philosophy

### 2.1 Agent-First, Human-Friendly

Every operation in Davit is designed to be safely callable by an AI agent with no human in the loop. This imposes strict requirements:

- All output is **structured JSON** by default. Human-readable formatting is opt-in via `--pretty` or the TUI.
- Every command is **idempotent**: running it twice against an already-correct state does nothing and returns success.
- All errors carry a **machine-readable code** (see §16), never only a prose message.
- The **blast radius** of every command is bounded and documented. No command has implicit side effects outside its declared scope.

### 2.2 Least Privilege at Every Layer

- The agent SSH key is restricted to invoking the `davit` binary only — not a shell.
- Container workloads run as non-root inside Docker.
- Caddy manages TLS and reverse proxying; application containers are never exposed directly to the internet.
- Firewall rules are managed by Davit and documented before application.

### 2.3 Reproducibility

Davit stores enough state in its local SQLite database that a fresh reinstallation of the binary can reconstruct the running configuration of every deployed application. The database is the source of truth; filesystem state is derived from it.

### 2.4 Zero External Dependencies at Runtime

Once installed, Davit operates without any external Davit service, licence server, or cloud API. The only outbound network calls are:
- Git repository polling or webhook receipt
- ACME HTTP-01 challenges to Let's Encrypt
- OS package manager updates

---

## 3. Architecture Overview

```
┌──────────────────────────────────────────────────────────┐
│                     Linux Server                         │
│                                                          │
│  ┌─────────────────────────────────────────────────────┐ │
│  │                  davit binary (Go)                  │ │
│  │                                                     │ │
│  │  ┌──────────────┐      ┌──────────────────────────┐ │ │
│  │  │  Core Engine │      │     State Store           │ │ │
│  │  │  - Provisioner│     │     (SQLite + TOML)       │ │ │
│  │  │  - App Manager│     └──────────────────────────┘ │ │
│  │  │  - Cert Manager│                                  │ │
│  │  │  - Git Watcher│     ┌──────────────────────────┐ │ │
│  │  │  - Firewall   │     │     Operation Log         │ │ │
│  │  └──────┬───────┘      │     (append-only)         │ │ │
│  │         │              └──────────────────────────┘ │ │
│  │  ┌──────▼──────┐  ┌──────────────────────────────┐  │ │
│  │  │  JSON CLI   │  │       Interactive TUI         │  │ │
│  │  │  (agents)   │  │       (Bubble Tea)            │  │ │
│  │  └──────┬──────┘  └──────────────┬───────────────┘  │ │
│  └─────────┼───────────────────────┼───────────────────┘ │
│            │                       │                      │
│     SSH (forced command)     SSH (interactive pty)        │
│            │                       │                      │
└────────────┼───────────────────────┼──────────────────────┘
             │                       │
        AI Agent               Human Operator
```

### Component Responsibilities

| Component | Responsibility |
|---|---|
| **Core Engine** | All business logic. Stateless functions that read/write the state store. |
| **Provisioner** | OS detection, package installation, hardening, firewall management. |
| **App Manager** | Git clone/pull, Docker build, Compose lifecycle, health checking. |
| **Cert Manager** | ACME HTTP-01 flow via Caddy's API or `certbot`, renewal scheduling. |
| **Git Watcher** | Daemon thread: webhook receiver + polling scheduler, triggers deploys. |
| **JSON CLI** | Thin shell over Core Engine. Serialises all output to JSON. |
| **Interactive TUI** | Bubble Tea UI. Calls the same Core Engine functions as the CLI. |
| **State Store** | SQLite database + per-app TOML config files. Never derived; always canonical. |
| **Operation Log** | Append-only structured log of every operation, success or failure. |

---

## 4. Technology Stack

### 4.1 Language

**Go 1.22+**

Rationale:
- Compiles to a single static binary with no runtime dependencies.
- Excellent SSH server/client library support (`golang.org/x/crypto/ssh`).
- Bubble Tea (TUI framework) is Go-native.
- Strong stdlib for subprocess management, file I/O, HTTP.
- Cross-compilation to `linux/amd64` and `linux/arm64` from any dev machine.

### 4.2 TUI Framework

**Charm Bubble Tea** (`github.com/charmbracelet/bubbletea`)

Supporting Charm libraries:
- `lipgloss` — layout, colour, borders
- `bubbles` — pre-built components (list, spinner, text input, viewport)
- `glamour` — markdown rendering in terminal

### 4.3 Web Server

**Caddy 2**

Rationale:
- Automatic ACME HTTP-01 certificate issuance and renewal with zero configuration.
- JSON-driven admin API on `localhost:2019` — Davit can reconfigure Caddy programmatically without touching config files or restarting.
- Built-in reverse proxy.
- No DNS provider dependency for TLS certificates.

Caddy is installed as a system service. Davit never modifies the Caddyfile directly; it exclusively uses the Caddy Admin API to add/remove routes and certificate domains.

### 4.4 Container Runtime

**Docker Engine + Docker Compose v2** (plugin, not standalone binary)

Docker Compose is used for all application deployments. Even single-container applications are deployed via a generated `docker-compose.yml` to maintain a consistent interface.

### 4.5 Database

**SQLite 3** via `github.com/mattn/go-sqlite3` (CGo) or `modernc.org/sqlite` (pure Go, preferred for static binary).

Schema migrations managed with `github.com/golang-migrate/migrate`.

### 4.6 Configuration Format

**TOML** for human-editable configuration files (per-app configs, global settings).  
**SQLite** for runtime state (deployment history, certificate status, watcher state, operation log).

### 4.7 HTTP Client

Go stdlib `net/http`. No external HTTP client library needed.

### 4.8 Process Supervision

Davit's background daemon (the Git watcher and webhook receiver) is managed as a **systemd service** on systems with systemd (all modern distros). On systems without systemd, an OpenRC or SysVinit script is generated. The correct init system is detected during provisioning.

---

## 5. Installation & Bootstrap

### 5.1 Install Script

Davit ships a single `install.sh` script hosted at `https://davit.sh/install.sh`. It must be reviewed before execution. It performs the following:

1. Detect CPU architecture (`uname -m`): `x86_64` → `amd64`, `aarch64` → `arm64`.
2. Fetch the latest release binary from GitHub releases: `https://github.com/getdavit/davit/releases/latest/download/davit-linux-{arch}`.
3. Verify the SHA256 checksum against the published `.sha256` file.
4. Move binary to `/usr/local/bin/davit` and set permissions `0755`.
5. Run `davit server init` to begin provisioning (see §6).

```bash
curl -fsSL https://davit.sh/install.sh | sudo bash
```

The install script must exit cleanly without performing provisioning if `--no-init` is passed:
```bash
curl -fsSL https://davit.sh/install.sh | sudo bash -s -- --no-init
```

### 5.2 First-Run Experience

On the first interactive SSH session after installation, if `davit server init` has not been run, the TUI automatically launches the **Setup Wizard** (see §8.2). The wizard walks the operator through:

1. Confirming server hostname and timezone.
2. Reviewing and applying OS hardening steps (displayed as a checklist).
3. Setting the admin email address for Let's Encrypt notifications.
4. Creating the agent SSH key (see §12).
5. Displaying the final status dashboard.

---

## 6. Server Provisioning Engine

`davit server init` is the entry point. It is fully idempotent: each step checks whether the action is already complete before applying it. Every step emits a structured log entry on completion or failure.

### 6.1 OS Detection

```
detect_os():
  Read /etc/os-release
  Extract ID, ID_LIKE, VERSION_ID
  Determine package_manager:
    apt-get   → Debian, Ubuntu, Raspbian, Linux Mint
    dnf       → Fedora, RHEL 8+, Rocky Linux, AlmaLinux, CentOS Stream
    yum       → RHEL 7, CentOS 7
    zypper    → openSUSE, SLES
    pacman    → Arch Linux, Manjaro
    apk       → Alpine Linux
  If package_manager cannot be determined → EXIT_CODE 10 (unsupported OS)
  Detect init_system:
    Check if /run/systemd/private exists → systemd
    Check if /sbin/openrc exists → openrc
    Else → sysvinit
```

The detected OS profile is stored in the SQLite `system_info` table and used by all subsequent package installation steps.

### 6.2 Package Manager Abstraction

All package operations go through an internal `PackageManager` interface:

```go
type PackageManager interface {
    Update() error                    // Refresh package index
    Upgrade() error                   // Apply security upgrades
    Install(packages ...string) error // Install named packages
    IsInstalled(pkg string) bool      // Check presence without installing
    Purge(packages ...string) error   // Remove and clean config
}
```

Concrete implementations: `AptManager`, `DnfManager`, `YumManager`, `ZypperManager`, `PacmanManager`, `ApkManager`.

### 6.3 OS Hardening Steps

Each step is a named, idempotent function. Steps are executed in the order listed. A failed step halts provisioning and returns an error with the step name and reason.

#### Step 1: System Update
```
package_manager.Update()
package_manager.Upgrade()
```

#### Step 2: Install Core Utilities
Install: `curl`, `git`, `wget`, `gnupg`, `ca-certificates`, `unzip`, `htop`, `vim` (or `nano` if vim unavailable).

#### Step 3: Timezone Configuration
If `--timezone` flag is not provided, detect from `/etc/timezone` or default to `UTC`. Set using `timedatectl set-timezone` (systemd) or by writing `/etc/localtime` symlink directly.

#### Step 4: SSH Hardening

Modify `/etc/ssh/sshd_config`. Davit **appends a Davit-managed block** rather than editing the file wholesale, to avoid conflicts with existing configuration:

```
# BEGIN DAVIT MANAGED BLOCK — do not edit manually
PermitRootLogin no
PasswordAuthentication no
ChallengeResponseAuthentication no
X11Forwarding no
AllowTcpForwarding no
PrintMotd yes
MaxAuthTries 3
LoginGraceTime 30
# END DAVIT MANAGED BLOCK
```

Before writing: check if `PasswordAuthentication no` is already present. If yes, skip.  
After writing: run `sshd -t` to validate config. If validation fails, remove the block and return an error — do not restart sshd with a broken config.  
Restart sshd only after validation passes.

**Critical safety check:** Before disabling password auth, verify that at least one public key exists in `/root/.ssh/authorized_keys` or the invoking user's `~/.ssh/authorized_keys`. If no key is found, skip this step and emit `WARN_NO_SSH_KEY` rather than locking the operator out.

#### Step 5: Firewall Installation and Configuration

Detect existing firewall:
```
Is ufw installed and active?  → use ufw
Is firewalld installed and active? → use firewalld
Is nftables in use? → use nftables
Is iptables in use? → use iptables
None found → install ufw (apt/dnf/zypper/pacman) or nftables (Alpine)
```

Davit implements a `Firewall` interface analogous to `PackageManager`:

```go
type Firewall interface {
    Enable() error
    AllowPort(port int, proto string) error   // proto: "tcp" | "udp" | "any"
    DenyPort(port int, proto string) error
    AllowService(name string) error           // e.g. "ssh", "http", "https"
    Status() ([]FirewallRule, error)
    Reset() error                             // DANGEROUS: drops all rules
}
```

Initial ruleset applied during provisioning:
```
Allow: 22/tcp    (SSH)
Allow: 80/tcp    (HTTP — required for ACME HTTP-01 challenge)
Allow: 443/tcp   (HTTPS)
Allow: 443/udp   (QUIC/HTTP3, optional but recommended)
Default policy: DENY inbound
Default policy: ALLOW outbound
```

When `davit app create` is called, no additional firewall rules are needed because all traffic is proxied through Caddy on 80/443. Application containers bind only to `127.0.0.1` and are never exposed on a public port.

**The agent API port** (see §12.3) is NOT opened in the firewall by default. The agent communicates over the existing SSH port using the forced-command mechanism.

#### Step 6: Install fail2ban

Install `fail2ban` via package manager.  
Write `/etc/fail2ban/jail.d/davit.conf`:

```ini
[sshd]
enabled  = true
port     = ssh
filter   = sshd
logpath  = %(sshd_log)s
maxretry = 5
bantime  = 3600
findtime = 600

[caddy-http-auth]
enabled  = false
; Populated by Davit when apps with auth are deployed
```

Enable and start fail2ban service.

#### Step 7: Install Docker Engine

Do not use distro-packaged Docker (often outdated). Use Docker's official install script or official apt/dnf repositories.

```
If apt-get:
  Add docker.com GPG key to /etc/apt/keyrings/docker.gpg
  Add repository: https://download.docker.com/linux/{distro}
  apt-get install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

If dnf/yum:
  dnf config-manager --add-repo https://download.docker.com/linux/fedora/docker-ce.repo
  (adjust URL for RHEL/Rocky/Alma)
  dnf install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

If pacman:
  pacman -S docker docker-compose

If apk (Alpine):
  apk add docker docker-cli-compose
  rc-update add docker boot
```

After install:
- Enable and start `docker` service.
- Verify: `docker run --rm hello-world` must exit 0.
- **Do not** add any user to the `docker` group. Davit invokes Docker as root via `sudo` with a specific sudoers entry, or runs as root itself if installed system-wide. Adding users to the docker group is equivalent to granting root.

#### Step 8: Install Caddy

Use Caddy's official packages, not distro packages.

```
If apt-get:
  Add caddy stable repository from https://dl.cloudsmith.io/public/caddy/stable/
  apt-get install caddy

If dnf/yum:
  dnf install 'dnf-command(copr)'
  dnf copr enable @caddy/caddy
  dnf install caddy

If pacman:
  pacman -S caddy

If apk:
  apk add caddy
```

Write initial `/etc/caddy/Caddyfile`:

```
{
    admin localhost:2019
    email {ADMIN_EMAIL}
}
```

This is the **only** time Davit writes to the Caddyfile. All subsequent configuration is applied via the Caddy Admin JSON API at `http://localhost:2019`.

Enable and start Caddy service.

Verify: `curl -s http://localhost:2019/config/` must return a JSON object.

#### Step 9: Create Davit Directory Structure

See §17 for the full layout. Create all directories with correct permissions.

#### Step 10: Initialise State Database

Run all SQLite migrations. Schema defined in §13.

#### Step 11: Install Davit Daemon (Git Watcher)

Write a systemd unit (or equivalent) for `davit-daemon`:

```ini
[Unit]
Description=Davit Git Watcher and Webhook Receiver
After=network.target docker.service caddy.service

[Service]
Type=simple
ExecStart=/usr/local/bin/davit daemon start
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=davit-daemon

[Install]
WantedBy=multi-user.target
```

Enable and start. Verify via `davit daemon status`.

#### Step 12: Write MOTD

Write `/etc/motd.d/davit`:

```
╔══════════════════════════════════════════════╗
║          This server is managed by Davit     ║
║          Run: davit tui   to get started     ║
╚══════════════════════════════════════════════╝
```

### 6.4 Provisioning Output Format

Each step emits a JSON object to stdout as it completes:

```json
{
  "step": "install_docker",
  "status": "ok",
  "duration_ms": 4821,
  "message": "Docker 26.1.3 installed"
}
```

On failure:
```json
{
  "step": "ssh_hardening",
  "status": "error",
  "error_code": "SSH_VALIDATION_FAILED",
  "message": "sshd -t reported syntax error after config write; block removed, sshd unchanged",
  "duration_ms": 120
}
```

In `--pretty` mode, steps render as a progress list with green checkmarks, yellow warnings, and red errors.

---

## 7. Core CLI Reference

### 7.1 Global Flags

All commands accept these flags:

| Flag | Default | Description |
|---|---|---|
| `--json` | `true` | Output as JSON (default when stdout is not a TTY) |
| `--pretty` | `true` | Human-readable output (default when stdout is a TTY) |
| `--quiet` | `false` | Suppress all output except errors |
| `--config` | `/etc/davit/davit.toml` | Path to global config file |
| `--no-color` | `false` | Disable ANSI colour codes |

### 7.2 Command Groups

```
davit server   — Server provisioning and status
davit app      — Application lifecycle management
davit cert     — TLS certificate management
davit watch    — Git branch monitoring
davit agent    — Agent SSH key management
davit logs     — Log retrieval
davit diagnose — Structured diagnostics
davit daemon   — Background daemon control
davit tui      — Launch interactive TUI
davit version  — Print version and build info
```

### 7.3 Server Commands

#### `davit server init`

Runs the full provisioning sequence (§6). Safe to re-run.

```
Flags:
  --timezone string     Timezone string (e.g. "Europe/London"). Default: UTC
  --email string        Admin email for Let's Encrypt. Required.
  --skip-steps string   Comma-separated list of step names to skip
  --dry-run             Print what would be done without doing it
```

Output (JSON):
```json
{
  "status": "ok",
  "steps_total": 12,
  "steps_ok": 12,
  "steps_skipped": 0,
  "steps_failed": 0,
  "duration_ms": 48291
}
```

#### `davit server status`

Returns a full JSON snapshot of the server's current state.

Output:
```json
{
  "hostname": "prod-01",
  "os": "Ubuntu 24.04 LTS",
  "arch": "amd64",
  "davit_version": "0.3.1",
  "provisioned": true,
  "provisioned_at": "2025-11-14T10:32:00Z",
  "uptime_seconds": 432900,
  "disk_usage_percent": 34,
  "memory_used_mb": 812,
  "memory_total_mb": 4096,
  "docker_running": true,
  "caddy_running": true,
  "daemon_running": true,
  "fail2ban_running": true,
  "firewall_active": true,
  "apps_total": 3,
  "apps_running": 3,
  "certs_total": 3,
  "certs_expiring_soon": 0
}
```

#### `davit server update`

Runs OS security updates only (equivalent to `apt-get upgrade --only-upgrade` for security packages, or `dnf --security upgrade`). Does not upgrade Davit itself.

---

### 7.4 App Commands

#### `davit app create <name>`

Creates a new application record in the state store. Does not deploy.

```
Flags:
  --repo string       Git repository URL. Required.
                      Supports: https://github.com/org/repo
                                git@github.com:org/repo.git
                                https://user:token@gitlab.com/org/repo.git
                                Any valid Git URL
  --branch string     Branch to track. Default: "main"
  --domain string     Domain to serve the app on. Required.
  --port int          Internal container port to proxy to. Default: 3000
  --env-file string   Path to a .env file to inject at build time
  --compose-file string  Path to docker-compose.yml within repo. Default: "docker-compose.yml"
  --build-context string Path within repo to use as Docker build context. Default: "."
  --deploy-key string    Path to SSH private key for private repo access
```

Validation:
- `name` must match `^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$`.
- `domain` must be a valid FQDN. No wildcards.
- `repo` must be a reachable Git URL (perform a `git ls-remote` check — fail with `GIT_UNREACHABLE` if it times out).
- `name` must not already exist in the state store — fail with `APP_ALREADY_EXISTS`.

Output:
```json
{
  "status": "ok",
  "app": {
    "name": "myapi",
    "repo": "https://github.com/org/myapi.git",
    "branch": "main",
    "domain": "api.example.com",
    "port": 3000,
    "created_at": "2025-11-14T11:00:00Z"
  }
}
```

#### `davit app deploy <name>`

Full deployment sequence for a named application. Idempotent.

Sequence:
1. Look up app record from state store.
2. Clone repository if not present; `git fetch && git reset --hard origin/<branch>` if already present.
3. If `docker-compose.yml` exists at `--compose-file` path: `docker compose pull && docker compose up -d --build --remove-orphans`.
4. If no `docker-compose.yml` exists: generate one (see §9.2) and proceed.
5. Wait for container health check (up to 60 seconds, polling every 2s).
6. Register app with Caddy via Admin API (see §10.2).
7. Issue TLS certificate if not already valid (see §10.3).
8. Write deployment record to state store.

```
Flags:
  --timeout int     Seconds to wait for health check. Default: 60
  --force           Deploy even if current commit matches last deployed commit
```

Output:
```json
{
  "status": "ok",
  "app": "myapi",
  "deployed_at": "2025-11-14T11:05:32Z",
  "commit": "a3f91bc",
  "commit_message": "Fix rate limiting bug",
  "duration_ms": 34210,
  "url": "https://api.example.com"
}
```

#### `davit app stop <name>`

Runs `docker compose stop` for the named application. Does not remove containers or volumes.

#### `davit app start <name>`

Runs `docker compose start` for a stopped application.

#### `davit app restart <name>`

Runs `docker compose restart`.

#### `davit app remove <name>`

Removes the application completely.

```
Flags:
  --keep-data    Do not remove named Docker volumes for this app
  --keep-cert    Do not remove the TLS certificate from Caddy
```

Sequence:
1. `docker compose down --volumes` (unless `--keep-data`).
2. Remove Caddy route via Admin API.
3. Remove TLS cert from Caddy (unless `--keep-cert`).
4. Remove app directory from `/var/lib/davit/apps/<name>`.
5. Mark app as removed in state store (soft delete — record is retained for audit).

#### `davit app list`

```json
{
  "apps": [
    {
      "name": "myapi",
      "status": "running",
      "domain": "api.example.com",
      "branch": "main",
      "commit": "a3f91bc",
      "deployed_at": "2025-11-14T11:05:32Z",
      "url": "https://api.example.com",
      "uptime_seconds": 3600
    }
  ]
}
```

#### `davit app info <name>`

Full detail record for one app: config, deployment history (last 10), cert status, container stats.

#### `davit app env <name>`

Subcommands for managing environment variables:
```
davit app env set <name> KEY=VALUE [KEY=VALUE ...]
davit app env unset <name> KEY [KEY ...]
davit app env list <name>
```

Env vars are stored encrypted (AES-256-GCM) in the SQLite database. The encryption key is derived from a server-local secret written to `/etc/davit/secret.key` during `server init` (mode `0600`, owned by root).

After any `env set` or `env unset`, a redeployment is automatically triggered unless `--no-redeploy` is passed.

---

### 7.5 Certificate Commands

Caddy manages certificates automatically; these commands interact with Caddy's certificate state for inspection and manual intervention.

#### `davit cert status [domain]`

```json
{
  "certs": [
    {
      "domain": "api.example.com",
      "issuer": "Let's Encrypt",
      "valid_from": "2025-11-01T00:00:00Z",
      "valid_until": "2026-01-30T00:00:00Z",
      "days_remaining": 77,
      "auto_renew": true,
      "managed_by": "caddy"
    }
  ]
}
```

#### `davit cert renew <domain>`

Forces immediate renewal via Caddy's `/certificates` API endpoint. Caddy handles the ACME HTTP-01 challenge internally.

---

### 7.6 Watch Commands

#### `davit watch enable <name>`

Enables automatic deployment when the tracked branch updates.

```
Flags:
  --method string    "webhook" | "poll" | "both". Default: "both"
  --interval int     Polling interval in seconds (when polling enabled). Default: 60
```

Webhooks: Davit's daemon listens on `localhost` only (not publicly accessible). The webhook endpoint is exposed through Caddy as:  
`https://<server-hostname>/.davit/webhook/<name>/<token>`

The `token` is a 32-byte random hex string generated at watch creation time, stored in the state store. Displayed once to the operator.

#### `davit watch disable <name>`

Stops all watching for the named app.

#### `davit watch status`

```json
{
  "watchers": [
    {
      "app": "myapi",
      "method": "both",
      "last_checked": "2025-11-14T11:30:00Z",
      "last_commit_seen": "a3f91bc",
      "webhook_url": "https://prod-01.example.com/.davit/webhook/myapi/abc123...",
      "poll_interval_seconds": 60,
      "status": "active"
    }
  ]
}
```

#### `davit watch trigger <name>`

Manually triggers a deployment check (fetches latest commit, deploys if changed). Useful for testing webhook setup.

---

### 7.7 Agent Commands

#### `davit agent key create`

Generates an Ed25519 SSH keypair for agent access.

```
Flags:
  --label string    Human-readable label for this key. Default: "agent"
  --output string   Directory to write key files. Default: current directory
```

Sequence:
1. Generate Ed25519 keypair.
2. Write `davit-agent.pub` and `davit-agent.pem` to `--output` directory.
3. Install the public key into `~root/.ssh/authorized_keys` with the forced-command restriction (see §12.2).

Output:
```json
{
  "status": "ok",
  "label": "agent",
  "public_key": "ssh-ed25519 AAAA...",
  "authorized_keys_entry": "command=\"/usr/local/bin/davit --json\",no-pty,no-port-forwarding,no-agent-forwarding,no-X11-forwarding ssh-ed25519 AAAA...",
  "fingerprint": "SHA256:abc..."
}
```

The private key is displayed once and written to disk. Davit never stores it.

#### `davit agent key list`

Lists all agent keys registered in the state store (label, fingerprint, created date, last used).

#### `davit agent key revoke <fingerprint>`

Removes the matching entry from `authorized_keys` and marks it as revoked in the state store.

---

### 7.8 Log Commands

#### `davit logs <name>`

```
Flags:
  --lines int      Number of lines to return. Default: 100
  --since string   Duration string (e.g. "1h", "30m") or ISO8601 timestamp
  --follow         Stream logs in real time (not available to agents — use polling)
  --json           Output as JSON array of log line objects
```

JSON output format:
```json
{
  "app": "myapi",
  "lines": [
    {
      "ts": "2025-11-14T11:05:40Z",
      "stream": "stdout",
      "text": "Server listening on :3000"
    }
  ]
}
```

#### `davit logs system`

Returns Davit's own operation log (not application logs). Includes all provisioning steps, deployments, cert events, and errors.

---

### 7.9 Diagnose Command

`davit diagnose <name>` is the primary interface for AI-assisted troubleshooting. It collects a structured snapshot of everything relevant to why an application might be misbehaving and returns it in a single JSON object.

```json
{
  "app": "myapi",
  "timestamp": "2025-11-14T11:00:00Z",
  "overall_status": "degraded",
  "checks": {
    "container_running": false,
    "container_exit_code": 1,
    "container_oom_killed": false,
    "port_listening": false,
    "caddy_route_exists": true,
    "cert_valid": true,
    "cert_days_remaining": 77,
    "disk_free_gb": 1.2,
    "disk_warning": true,
    "git_repo_reachable": true,
    "last_deploy_success": true,
    "env_vars_present": true
  },
  "last_100_log_lines": [...],
  "last_deploy": {
    "at": "2025-11-14T10:00:00Z",
    "commit": "a3f91bc",
    "status": "ok"
  },
  "suggested_issues": [
    "Low disk space (1.2 GB free) may be causing container crash",
    "Container exited with code 1 — likely application error, check logs"
  ]
}
```

The `suggested_issues` array is generated by deterministic rules (not AI inference), covering the most common deployment failure patterns. An AI agent receiving this object has everything it needs to reason about the problem.

---

## 8. Interactive TUI

### 8.1 Design Principles

- Keyboard-only navigation. No mouse required.
- Every screen is reachable within 3 keystrokes from the home dashboard.
- Actions that are destructive require a confirmation prompt (type the app name to confirm, not just `y`).
- All operations show a live progress view while running (same underlying engine as the CLI).
- `?` always opens contextual help. `q` or `Esc` goes back. `Ctrl+C` exits to shell.

### 8.2 Setup Wizard

Launched automatically on first run when the server is not yet provisioned.

```
Screen 1: Welcome
  ┌─────────────────────────────────────┐
  │  Welcome to Davit                   │
  │  The crane arm for your containers. │
  │                                     │
  │  This wizard will:                  │
  │  ✓ Harden your Linux server         │
  │  ✓ Install Docker and Caddy         │
  │  ✓ Set up firewall rules            │
  │  ✓ Configure TLS automation         │
  │                                     │
  │  [Press Enter to continue]          │
  └─────────────────────────────────────┘

Screen 2: Confirm settings (hostname, timezone, email)
Screen 3: Review hardening steps (scrollable checklist, all pre-ticked, operator can untick)
Screen 4: Provisioning progress (live step-by-step with timing)
Screen 5: Summary + next steps
```

### 8.3 Main Dashboard

```
╔═══════════════════════════════════════════════════════════════╗
║ davit  prod-01.example.com          ↑ 5d 2h    v0.3.1        ║
╠═══════════════════════════════════════════════════════════════╣
║ APPS                                                          ║
║  ● myapi          api.example.com        running   a3f91bc   ║
║  ● frontend       example.com            running   1bc0421   ║
║  ○ worker         (no domain)            stopped   f4a2200   ║
╠═══════════════════════════════════════════════════════════════╣
║ SERVER HEALTH                                                 ║
║  Docker ✓   Caddy ✓   fail2ban ✓   Daemon ✓   Disk 34%      ║
╠═══════════════════════════════════════════════════════════════╣
║ [n] New app  [d] Deploy  [l] Logs  [s] Server  [?] Help      ║
╚═══════════════════════════════════════════════════════════════╝
```

### 8.4 Key Bindings (Global)

| Key | Action |
|---|---|
| `↑` / `↓` or `j` / `k` | Navigate lists |
| `Enter` | Select / confirm |
| `Esc` / `q` | Back / cancel |
| `?` | Context help |
| `Ctrl+C` | Exit to shell |
| `r` | Refresh current view |
| `n` | New (context-sensitive: new app, new key, etc.) |
| `/` | Filter/search current list |

### 8.5 App Detail Screen

Selected from the main dashboard by pressing `Enter` on an app:

```
myapi                                          running ●
──────────────────────────────────────────────────────
  Domain:    https://api.example.com
  Repo:      https://github.com/org/myapi
  Branch:    main  (watching: poll every 60s + webhook)
  Commit:    a3f91bc — "Fix rate limiting bug"
  Deployed:  14 Nov 2025 11:05  (2h ago)
  Cert:      Valid until 30 Jan 2026 (77 days)

  [d] Deploy now   [l] Logs   [e] Env vars
  [w] Watch config [x] Stop   [!] Remove
```

---

## 9. Application Lifecycle

### 9.1 Directory Structure Per App

```
/var/lib/davit/apps/<name>/
  repo/              Git working directory (full clone)
  docker-compose.yml Symlink to repo/docker-compose.yml (or generated)
  .env               Decrypted env vars written here at deploy time, mode 0600
  deploy.log         Append-only log of all deployments for this app
```

### 9.2 Generated docker-compose.yml

If the repository does not contain a `docker-compose.yml`, Davit generates one. It detects the application type by scanning the repository root:

| Detection signal | Assumed type | Generated config |
|---|---|---|
| `Dockerfile` present | Generic | Build from Dockerfile, expose `--port` |
| `package.json` present, no Dockerfile | Node.js | Use `node:lts-alpine`, `npm ci && npm start` |
| `requirements.txt` or `pyproject.toml`, no Dockerfile | Python | Use `python:3.12-slim`, `pip install -r requirements.txt` |
| `go.mod`, no Dockerfile | Go | Multi-stage build with `golang:1.22-alpine` |
| `Gemfile`, no Dockerfile | Ruby | Use `ruby:3.3-slim` |
| `composer.json`, no Dockerfile | PHP | Use `php:8.3-fpm-alpine` |
| None of the above | Unknown | Fail with `APP_TYPE_UNKNOWN` — require explicit Dockerfile |

Generated `docker-compose.yml` structure:

```yaml
services:
  app:
    build:
      context: ./repo
      dockerfile: Dockerfile    # or generated inline
    restart: unless-stopped
    env_file:
      - .env
    ports:
      - "127.0.0.1:{auto_assigned_port}:{container_port}"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:{container_port}/"]
      interval: 10s
      timeout: 5s
      retries: 6
      start_period: 30s
    logging:
      driver: "json-file"
      options:
        max-size: "50m"
        max-file: "5"
```

The `auto_assigned_port` is a port in the range `40000–49999`, allocated sequentially and recorded in the state store to avoid conflicts.

### 9.3 Health Check

After `docker compose up`, Davit polls the container health status:

```
Poll every 2 seconds for up to --timeout seconds (default 60)
If container status becomes "healthy" → success
If container exits → fail immediately with CONTAINER_EXITED
If timeout reached and status is "starting" → fail with HEALTH_CHECK_TIMEOUT
```

On failure, Davit automatically runs `davit diagnose <name>` and includes the result in the error output. The failed container is left running (not removed) so logs can be inspected.

### 9.4 Zero-Downtime Deploys

For applications with more than one replica (defined in `docker-compose.yml`), Davit uses Docker Compose's built-in rolling update:

```
docker compose up -d --build --no-deps --scale app=2
# Wait for new container healthy
docker compose up -d --scale app=1 --no-deps
```

For single-replica apps, there is a brief downtime during container restart. Zero-downtime for single-replica is outside scope for v1.

---

## 10. Web Server & TLS

### 10.1 Caddy as Sole Ingress

All inbound traffic on ports 80 and 443 is handled by Caddy. Application containers **never** bind to public interfaces. The internal container port is mapped only to `127.0.0.1:{auto_port}`.

### 10.2 Caddy Admin API Integration

Davit configures Caddy exclusively through its Admin API at `http://localhost:2019`. This means Caddy never needs to be restarted — routes are added and removed live.

**Adding a route for a new app:**

```
PUT http://localhost:2019/config/apps/http/servers/srv0/routes/{app_name}
Content-Type: application/json

{
  "match": [{"host": ["api.example.com"]}],
  "handle": [
    {
      "@id": "davit_{app_name}",
      "handler": "reverse_proxy",
      "upstreams": [{"dial": "127.0.0.1:42001"}]
    }
  ],
  "terminal": true
}
```

**Removing a route:**
```
DELETE http://localhost:2019/id/davit_{app_name}
```

Each route is given an `@id` of `davit_{app_name}` to enable targeted deletion.

### 10.3 TLS Certificate Issuance

Caddy handles ACME HTTP-01 automatically when a virtual host is configured. No explicit `davit cert issue` invocation is needed for standard deployments — the certificate is provisioned the first time Caddy proxies a request for the domain.

However, `davit cert status` and `davit cert renew` interact with the Caddy Admin API's certificate management endpoints:

```
GET  http://localhost:2019/config/apps/tls/certificates/automate
POST http://localhost:2019/load   (to force reload after domain changes)
```

**Requirement for HTTP-01:** Port 80 must be reachable from the public internet for the ACME challenge. The firewall ruleset ensures this. If port 80 is blocked by an upstream provider (e.g. some cloud VPCs), Davit detects this during `server init` via an external check (HTTP request to a Davit-operated probe endpoint) and warns the operator.

---

## 11. Git Integration & Auto-Deploy

### 11.1 Polling

The Davit daemon maintains a polling loop per watched app:

```
Every <interval> seconds:
  git fetch origin <branch>
  latest_commit = git rev-parse origin/<branch>
  if latest_commit != stored_commit:
    trigger deploy
    update stored_commit
```

The daemon stores `latest_commit` in the SQLite `watchers` table. Polling runs in goroutines, one per watched app, managed by the daemon's scheduler.

### 11.2 Webhooks

The daemon runs an HTTP server on `127.0.0.1:2020` (not exposed publicly). Caddy routes `/.davit/webhook/<name>/<token>` to this internal server:

```
POST /.davit/webhook/myapi/abc123...
Content-Type: application/json
X-Hub-Signature-256: sha256=...   (GitHub format, optional)
```

On receipt:
1. Validate token matches state store.
2. If `X-Hub-Signature-256` header is present, validate HMAC signature using stored secret.
3. Parse payload: extract branch and commit SHA.
4. If branch matches tracked branch: trigger deploy.
5. Return `200 OK` immediately; deploy runs asynchronously.

GitHub and GitLab webhook payload formats are both supported. Any other provider sending a JSON body with a `ref` field is also supported (generic mode).

### 11.3 Webhook Setup Instructions

`davit watch enable <name> --method webhook` prints setup instructions tailored to the detected Git provider:

```
Webhook URL:   https://prod-01.example.com/.davit/webhook/myapi/abc123...
Content type:  application/json
Events:        Push events only
Secret:        (optional but recommended) abc456...

For GitHub: Settings → Webhooks → Add webhook
For GitLab: Settings → Webhooks
```

---

## 12. Agent Access Model

### 12.1 Threat Model

An AI agent is granted the ability to run Davit commands on the server. The threat model assumes:
- The agent's SSH private key may be compromised.
- The agent may act on incorrect reasoning.
- No interactive shell should ever be accessible via the agent key.
- The agent should not be able to modify Davit's own configuration or the OS outside of declared Davit operations.

### 12.2 Forced SSH Command

The `authorized_keys` entry for an agent key takes the following form:

```
command="/usr/local/bin/davit --json $SSH_ORIGINAL_COMMAND",no-pty,no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-user-rc ssh-ed25519 AAAA... davit-agent-label
```

How this works:
- When the agent SSHes in and runs `davit app list`, the SSH server runs `/usr/local/bin/davit --json app list` regardless of what was actually requested.
- If the agent attempts to run a shell command (e.g. `ls /`), the SSH server runs `/usr/local/bin/davit --json ls /`, which Davit rejects with error code `UNKNOWN_COMMAND`.
- The agent cannot open an interactive session (`no-pty`).
- The agent cannot forward ports (`no-port-forwarding`).

### 12.3 Agent-Safe Commands

Not all Davit commands are available to agent keys. The following commands are **disabled for agent keys** and return `AGENT_FORBIDDEN`:

- `davit server init` — too destructive to allow agent invocation
- `davit server update` — should be human-reviewed
- `davit agent key create` — agents cannot create new agent keys
- `davit agent key revoke` — agents cannot revoke access
- `davit app remove` — irreversible, requires human confirmation
- `davit tui` — TUI requires a PTY which agents don't have

All other commands are available to agents.

### 12.4 Agent Usage Pattern

An AI agent (e.g. Claude Code) integrates with Davit as follows:

```bash
# Check status
ssh -i davit-agent.pem root@prod-01.example.com davit app list

# Deploy
ssh -i davit-agent.pem root@prod-01.example.com davit app deploy myapi

# Diagnose a problem
ssh -i davit-agent.pem root@prod-01.example.com davit diagnose myapi

# Read logs
ssh -i davit-agent.pem root@prod-01.example.com davit logs myapi --lines 200
```

All output is JSON. The agent parses the JSON and reasons about the result. No screen-scraping of human-readable output.

---

## 13. State & Configuration

### 13.1 SQLite Schema

```sql
-- System information, written once during provisioning
CREATE TABLE system_info (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Registered applications
CREATE TABLE apps (
    name TEXT PRIMARY KEY,
    repo_url TEXT NOT NULL,
    branch TEXT NOT NULL DEFAULT 'main',
    domain TEXT NOT NULL,
    container_port INTEGER NOT NULL,
    internal_port INTEGER NOT NULL,
    compose_file TEXT NOT NULL DEFAULT 'docker-compose.yml',
    build_context TEXT NOT NULL DEFAULT '.',
    deploy_key_path TEXT,
    status TEXT NOT NULL DEFAULT 'created',  -- created|running|stopped|removed
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    removed_at DATETIME
);

-- Encrypted environment variables
CREATE TABLE app_env (
    app_name TEXT NOT NULL REFERENCES apps(name),
    key TEXT NOT NULL,
    value_encrypted BLOB NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (app_name, key)
);

-- Deployment history
CREATE TABLE deployments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name TEXT NOT NULL REFERENCES apps(name),
    commit_sha TEXT NOT NULL,
    commit_message TEXT,
    status TEXT NOT NULL,  -- ok|failed
    error_code TEXT,
    duration_ms INTEGER,
    deployed_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Git watchers
CREATE TABLE watchers (
    app_name TEXT PRIMARY KEY REFERENCES apps(name),
    method TEXT NOT NULL DEFAULT 'both',  -- webhook|poll|both
    poll_interval_seconds INTEGER DEFAULT 60,
    webhook_token TEXT NOT NULL,
    webhook_secret TEXT,
    last_checked_at DATETIME,
    last_commit_sha TEXT,
    status TEXT NOT NULL DEFAULT 'active'  -- active|paused
);

-- Agent SSH keys
CREATE TABLE agent_keys (
    fingerprint TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    public_key TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME,
    revoked_at DATETIME
);

-- Append-only operation log
CREATE TABLE operation_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL,
    subject TEXT,       -- app name, domain, etc.
    status TEXT NOT NULL,
    error_code TEXT,
    message TEXT,
    invoked_by TEXT,    -- "human"|"agent:{key_fingerprint}"
    duration_ms INTEGER,
    logged_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 13.2 Global Config File

`/etc/davit/davit.toml`:

```toml
[server]
hostname = "prod-01.example.com"
timezone = "UTC"
admin_email = "admin@example.com"

[caddy]
admin_api = "http://localhost:2019"

[daemon]
poll_default_interval = 60
webhook_listen_addr = "127.0.0.1:2020"

[logging]
level = "info"      # debug | info | warn | error
format = "json"

[ports]
auto_assign_range_start = 40000
auto_assign_range_end   = 49999
```

---

## 14. Logging & Diagnostics

### 14.1 Application Logs

Davit retrieves application logs via `docker logs <container_id>`. Logs are returned as structured JSON when `--json` is set. Docker's `json-file` logging driver is mandated (see §9.2) to ensure reliable retrieval.

Log rotation is handled by Docker's log driver options: `max-size: 50m`, `max-file: 5`.

### 14.2 Davit Operation Log

Every operation — CLI invocation, daemon action, webhook receipt — is appended to the `operation_log` SQLite table. This provides a full audit trail accessible via `davit logs system`.

The log records who invoked the operation: `"human"` for interactive TTY sessions, or `"agent:{fingerprint}"` for agent SSH key invocations.

### 14.3 Structured Error Format

All errors returned by Davit commands (CLI or programmatically) follow this structure:

```json
{
  "status": "error",
  "error_code": "CONTAINER_EXITED",
  "message": "Container exited with code 1 during health check",
  "context": {
    "app": "myapi",
    "exit_code": 1,
    "oom_killed": false
  },
  "docs_url": "https://davit.sh/errors/CONTAINER_EXITED"
}
```

---

## 15. Security Hardening Procedures

This section consolidates the security configuration applied by `davit server init` for audit purposes.

### 15.1 SSH Configuration Summary

| Setting | Value | Reason |
|---|---|---|
| `PermitRootLogin` | `no` | Prevents direct root login |
| `PasswordAuthentication` | `no` | Keys only — prevents brute force |
| `ChallengeResponseAuthentication` | `no` | Disables PAM-based auth |
| `X11Forwarding` | `no` | Reduces attack surface |
| `AllowTcpForwarding` | `no` | Prevents port forwarding abuse |
| `MaxAuthTries` | `3` | Limits brute force attempts per connection |
| `LoginGraceTime` | `30` | Closes unauthenticated connections after 30s |

### 15.2 Firewall Policy Summary

| Port | Protocol | Policy | Reason |
|---|---|---|---|
| 22 | TCP | ALLOW | SSH access |
| 80 | TCP | ALLOW | HTTP + ACME challenges |
| 443 | TCP | ALLOW | HTTPS |
| 443 | UDP | ALLOW | HTTP/3 (QUIC) |
| All other | Any | DENY | Default deny |

### 15.3 fail2ban Jail Summary

| Jail | Service | Max Retries | Ban Time |
|---|---|---|---|
| `sshd` | SSH | 5 | 1 hour |

### 15.4 Docker Security Defaults

- Containers run as non-root (enforced via `user:` directive in generated Compose files, defaulting to `1000:1000` unless overridden by the app's own Dockerfile `USER` directive).
- No containers are run with `--privileged`.
- No containers are run with `--net=host`.
- All containers bind only to `127.0.0.1` on the host.
- No Docker socket is mounted into any application container.

---

## 16. Error Codes & Machine Contracts

All error codes follow `SCREAMING_SNAKE_CASE`. Each code has a stable meaning across versions.

| Code | Exit Code | Meaning |
|---|---|---|
| `OK` | 0 | Operation completed successfully |
| `UNKNOWN_COMMAND` | 1 | Command not recognised |
| `AGENT_FORBIDDEN` | 2 | Command not permitted for agent keys |
| `UNSUPPORTED_OS` | 10 | Could not detect a supported OS or package manager |
| `PROVISION_STEP_FAILED` | 11 | A provisioning step failed (see step name in context) |
| `SSH_VALIDATION_FAILED` | 12 | sshd config validation failed; no changes applied |
| `WARN_NO_SSH_KEY` | 13 | No SSH public key found; password auth hardening skipped |
| `APP_ALREADY_EXISTS` | 20 | App name already registered |
| `APP_NOT_FOUND` | 21 | Named app does not exist |
| `APP_TYPE_UNKNOWN` | 22 | Cannot detect app type; Dockerfile required |
| `GIT_UNREACHABLE` | 23 | Git repository could not be reached |
| `GIT_CLONE_FAILED` | 24 | `git clone` returned non-zero |
| `GIT_PULL_FAILED` | 25 | `git fetch/reset` returned non-zero |
| `DOCKER_BUILD_FAILED` | 30 | `docker compose build` returned non-zero |
| `DOCKER_START_FAILED` | 31 | `docker compose up` returned non-zero |
| `CONTAINER_EXITED` | 32 | Container exited during health check window |
| `HEALTH_CHECK_TIMEOUT` | 33 | Container did not become healthy within timeout |
| `PORT_EXHAUSTED` | 34 | No free port in auto-assign range |
| `CADDY_API_UNREACHABLE` | 40 | Cannot connect to Caddy Admin API |
| `CADDY_CONFIG_FAILED` | 41 | Caddy rejected the configuration payload |
| `CERT_ISSUE_FAILED` | 42 | ACME certificate issuance failed |
| `STATE_DB_ERROR` | 50 | SQLite read/write error |
| `CONFIG_INVALID` | 51 | TOML config file has a syntax or validation error |
| `INTERNAL_ERROR` | 99 | Unexpected error; always includes a stack trace in debug mode |

Exit code conventions:
- `0` — success
- `1–9` — usage errors (wrong arguments, forbidden commands)
- `10–19` — provisioning errors
- `20–39` — application/deployment errors
- `40–49` — web server / TLS errors
- `50–59` — state / configuration errors
- `99` — internal / unexpected errors

---

## 17. Directory Layout on Server

```
/usr/local/bin/davit              Main binary

/etc/davit/
  davit.toml                      Global configuration (human-editable)
  secret.key                      Encryption key for env vars (mode 0600, root only)

/var/lib/davit/
  davit.db                        SQLite state database
  apps/
    <app-name>/
      repo/                       Git working directory
      docker-compose.yml          Symlink or generated file
      .env                        Decrypted env vars at deploy time (mode 0600)
      deploy.log                  Append-only deployment log

/var/log/davit/
  davit.log                       Davit daemon structured log (JSON lines)

/etc/caddy/
  Caddyfile                       Minimal bootstrap config (written once at init)

/etc/fail2ban/jail.d/
  davit.conf                      Davit-managed fail2ban jail config

/etc/motd.d/
  davit                           MOTD banner

/run/davit/
  daemon.pid                      PID file for the background daemon
  webhook.sock                    (future: Unix socket alternative to TCP)
```

---

## 18. Development Roadmap

### v0.1 — Foundation
- [ ] OS detection and package manager abstraction (all major distros)
- [ ] Server provisioning steps 1–9 (update, SSH, firewall, fail2ban, Docker, Caddy)
- [ ] SQLite state store with migrations
- [ ] `davit server init` and `davit server status`
- [ ] `davit app create`, `davit app deploy`, `davit app list`
- [ ] Caddy Admin API integration (add/remove routes)
- [ ] Basic TLS issuance via Caddy
- [ ] JSON CLI output
- [ ] Agent SSH key creation and forced-command installation

### v0.2 — App Lifecycle
- [ ] `davit app stop/start/restart/remove`
- [ ] `davit app env` (encrypted storage + redeploy)
- [ ] Generated `docker-compose.yml` for auto-detected app types
- [ ] Container health check polling
- [ ] `davit logs <name>`
- [ ] `davit diagnose <name>`

### v0.3 — Git Automation
- [ ] `davit watch enable/disable/status`
- [ ] Polling daemon (goroutine scheduler)
- [ ] Webhook receiver (HTTP server via Caddy proxy)
- [ ] GitHub and GitLab payload parsers

### v0.4 — Interactive TUI
- [ ] Setup wizard
- [ ] Main dashboard
- [ ] App detail screen
- [ ] Deploy progress view
- [ ] Log viewer
- [ ] Env var editor

### v0.5 — Hardening & Polish
- [ ] Complete error code coverage
- [ ] `davit server update`
- [ ] Certificate status and manual renewal commands
- [ ] Operation log and audit trail
- [ ] `davit agent key list/revoke`
- [ ] man page generation from CLI definitions
- [ ] Install script with checksum verification

### v1.0 — Production Ready
- [ ] ARM64 binary builds
- [ ] Automated test suite (provisioning tested against Docker-in-Docker for each distro)
- [ ] Public documentation site at davit.sh
- [ ] GitHub Actions CI/CD for Davit's own build pipeline

---

*End of specification — davit v0.1-draft*
