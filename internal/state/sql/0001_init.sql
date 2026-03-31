-- System information written during provisioning
CREATE TABLE IF NOT EXISTS system_info (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Registered applications
CREATE TABLE IF NOT EXISTS apps (
    name            TEXT PRIMARY KEY,
    repo_url        TEXT NOT NULL,
    branch          TEXT NOT NULL DEFAULT 'main',
    domain          TEXT NOT NULL,
    container_port  INTEGER NOT NULL,
    internal_port   INTEGER NOT NULL,
    compose_file    TEXT NOT NULL DEFAULT 'docker-compose.yml',
    build_context   TEXT NOT NULL DEFAULT '.',
    deploy_key_path TEXT,
    status          TEXT NOT NULL DEFAULT 'created',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    removed_at      DATETIME
);

-- Encrypted environment variables per application
CREATE TABLE IF NOT EXISTS app_env (
    app_name        TEXT NOT NULL REFERENCES apps(name),
    key             TEXT NOT NULL,
    value_encrypted BLOB NOT NULL,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (app_name, key)
);

-- Deployment history
CREATE TABLE IF NOT EXISTS deployments (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    app_name       TEXT NOT NULL REFERENCES apps(name),
    commit_sha     TEXT NOT NULL,
    commit_message TEXT,
    status         TEXT NOT NULL,
    error_code     TEXT,
    duration_ms    INTEGER,
    deployed_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Git watchers
CREATE TABLE IF NOT EXISTS watchers (
    app_name              TEXT PRIMARY KEY REFERENCES apps(name),
    method                TEXT NOT NULL DEFAULT 'both',
    poll_interval_seconds INTEGER DEFAULT 60,
    webhook_token         TEXT NOT NULL,
    webhook_secret        TEXT,
    last_checked_at       DATETIME,
    last_commit_sha       TEXT,
    status                TEXT NOT NULL DEFAULT 'active'
);

-- Agent SSH keys
CREATE TABLE IF NOT EXISTS agent_keys (
    fingerprint  TEXT PRIMARY KEY,
    label        TEXT NOT NULL,
    public_key   TEXT NOT NULL,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME,
    revoked_at   DATETIME
);

-- Append-only operation audit log
CREATE TABLE IF NOT EXISTS operation_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    operation   TEXT NOT NULL,
    subject     TEXT,
    status      TEXT NOT NULL,
    error_code  TEXT,
    message     TEXT,
    invoked_by  TEXT,
    duration_ms INTEGER,
    logged_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);
