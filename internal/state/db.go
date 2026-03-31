package state

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

// DB wraps an SQLite connection and exposes high-level state operations.
type DB struct {
	conn *sql.DB
	path string
}

// Open opens (or creates) the SQLite database at path and applies all pending
// migrations. The caller must call Close() when done.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_fk=true")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db := &DB{conn: conn, path: path}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate applies all SQL migration files in lexicographic order.
func (db *DB) migrate() error {
	entries, err := fs.ReadDir(migrationFiles, "sql")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		data, err := migrationFiles.ReadFile("sql/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := db.conn.Exec(string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

// --- system_info ---

func (db *DB) SetSystemInfo(key, value string) error {
	_, err := db.conn.Exec(
		`INSERT INTO system_info (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC(),
	)
	return err
}

func (db *DB) GetSystemInfo(key string) (string, error) {
	var value string
	err := db.conn.QueryRow(`SELECT value FROM system_info WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// --- apps ---

func (db *DB) CreateApp(a App) error {
	_, err := db.conn.Exec(
		`INSERT INTO apps
		 (name, repo_url, branch, domain, container_port, internal_port,
		  compose_file, build_context, deploy_key_path, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, a.RepoURL, a.Branch, a.Domain, a.ContainerPort, a.InternalPort,
		a.ComposeFile, a.BuildContext, a.DeployKeyPath, a.Status, time.Now().UTC(),
	)
	return err
}

func (db *DB) GetApp(name string) (App, error) {
	row := db.conn.QueryRow(
		`SELECT name, repo_url, branch, domain, container_port, internal_port,
		        compose_file, build_context, COALESCE(deploy_key_path,''), status,
		        created_at, removed_at
		 FROM apps WHERE name = ? AND removed_at IS NULL`, name,
	)
	a, err := scanApp(row)
	if err == sql.ErrNoRows {
		return App{}, nil // caller checks app.Name == ""
	}
	return a, err
}

func (db *DB) ListApps() ([]App, error) {
	rows, err := db.conn.Query(
		`SELECT name, repo_url, branch, domain, container_port, internal_port,
		        compose_file, build_context, COALESCE(deploy_key_path,''), status,
		        created_at, removed_at
		 FROM apps WHERE removed_at IS NULL ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (db *DB) UpdateAppStatus(name, status string) error {
	_, err := db.conn.Exec(`UPDATE apps SET status = ? WHERE name = ?`, status, name)
	return err
}

func (db *DB) AppExists(name string) (bool, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM apps WHERE name = ? AND removed_at IS NULL`, name).Scan(&count)
	return count > 0, err
}

// AllocatePort returns the next free internal port in [rangeStart, rangeEnd].
func (db *DB) AllocatePort(rangeStart, rangeEnd int) (int, error) {
	rows, err := db.conn.Query(`SELECT internal_port FROM apps WHERE removed_at IS NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	used := make(map[int]bool)
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return 0, err
		}
		used[p] = true
	}
	for p := rangeStart; p <= rangeEnd; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port in range %d-%d", rangeStart, rangeEnd)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanApp(s scanner) (App, error) {
	var a App
	var createdAt string
	var removedAt *string
	err := s.Scan(
		&a.Name, &a.RepoURL, &a.Branch, &a.Domain,
		&a.ContainerPort, &a.InternalPort,
		&a.ComposeFile, &a.BuildContext, &a.DeployKeyPath,
		&a.Status, &createdAt, &removedAt,
	)
	if err != nil {
		return a, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if removedAt != nil {
		t, _ := time.Parse(time.RFC3339, *removedAt)
		a.RemovedAt = &t
	}
	return a, nil
}

// --- deployments ---

func (db *DB) InsertDeployment(d Deployment) (int64, error) {
	res, err := db.conn.Exec(
		`INSERT INTO deployments
		 (app_name, commit_sha, commit_message, status, error_code, duration_ms, deployed_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.AppName, d.CommitSHA, d.CommitMessage, d.Status, d.ErrorCode, d.DurationMS, time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (db *DB) LastDeployment(appName string) (Deployment, error) {
	row := db.conn.QueryRow(
		`SELECT id, app_name, commit_sha, COALESCE(commit_message,''), status,
		        COALESCE(error_code,''), COALESCE(duration_ms,0), deployed_at
		 FROM deployments WHERE app_name = ? ORDER BY deployed_at DESC LIMIT 1`,
		appName,
	)
	var d Deployment
	var deployedAt string
	err := row.Scan(&d.ID, &d.AppName, &d.CommitSHA, &d.CommitMessage,
		&d.Status, &d.ErrorCode, &d.DurationMS, &deployedAt)
	if err == sql.ErrNoRows {
		return d, nil
	}
	d.DeployedAt, _ = time.Parse(time.RFC3339, deployedAt)
	return d, err
}

// --- agent_keys ---

func (db *DB) InsertAgentKey(k AgentKey) error {
	_, err := db.conn.Exec(
		`INSERT INTO agent_keys (fingerprint, label, public_key, created_at)
		 VALUES (?, ?, ?, ?)`,
		k.Fingerprint, k.Label, k.PublicKey, time.Now().UTC(),
	)
	return err
}

func (db *DB) ListAgentKeys() ([]AgentKey, error) {
	rows, err := db.conn.Query(
		`SELECT fingerprint, label, public_key, created_at, last_used_at, revoked_at
		 FROM agent_keys ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []AgentKey
	for rows.Next() {
		var k AgentKey
		var createdAt string
		var lastUsed, revoked *string
		if err := rows.Scan(&k.Fingerprint, &k.Label, &k.PublicKey,
			&createdAt, &lastUsed, &revoked); err != nil {
			return nil, err
		}
		k.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if lastUsed != nil {
			t, _ := time.Parse(time.RFC3339, *lastUsed)
			k.LastUsedAt = &t
		}
		if revoked != nil {
			t, _ := time.Parse(time.RFC3339, *revoked)
			k.RevokedAt = &t
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// --- operation_log ---

func (db *DB) AppendLog(e OperationLog) error {
	_, err := db.conn.Exec(
		`INSERT INTO operation_log
		 (operation, subject, status, error_code, message, invoked_by, duration_ms, logged_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Operation, e.Subject, e.Status, e.ErrorCode, e.Message,
		e.InvokedBy, e.DurationMS, time.Now().UTC(),
	)
	return err
}
