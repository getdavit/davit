// Package app implements application lifecycle management.
package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/getdavit/davit/internal/caddy"
	"github.com/getdavit/davit/internal/state"
)

const appsBaseDir = "/var/lib/davit/apps"

var validNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$`)

// CreateOptions holds parameters for davit app create.
type CreateOptions struct {
	Name          string
	RepoURL       string
	Branch        string
	Domain        string
	ContainerPort int
	ComposeFile   string
	BuildContext  string
	DeployKeyPath string
}

// DeployOptions holds parameters for davit app deploy.
type DeployOptions struct {
	AppName string
	Timeout int  // seconds; default 60
	Force   bool // deploy even if commit unchanged
}

// DeployResult is returned after a successful deployment.
type DeployResult struct {
	Status     string `json:"status"`
	App        string `json:"app"`
	DeployedAt string `json:"deployed_at"`
	CommitSHA  string `json:"commit"`
	Duration   int64  `json:"duration_ms"`
	URL        string `json:"url"`
}

// AppSummary is one entry in the app list output.
type AppSummary struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Domain     string `json:"domain"`
	Branch     string `json:"branch"`
	CommitSHA  string `json:"commit"`
	DeployedAt string `json:"deployed_at,omitempty"`
	URL        string `json:"url"`
}

// Manager handles application lifecycle operations.
type Manager struct {
	db      *state.DB
	caddy   *caddy.Client
	appsDir string
	ports   struct{ start, end int }
}

// NewManager creates a Manager.
func NewManager(db *state.DB, caddyClient *caddy.Client, portStart, portEnd int) *Manager {
	return &Manager{
		db:      db,
		caddy:   caddyClient,
		appsDir: appsBaseDir,
		ports:   struct{ start, end int }{portStart, portEnd},
	}
}

// Create registers a new application in the state store. It does not deploy.
func (m *Manager) Create(opts CreateOptions) (state.App, error) {
	// Validate name
	if !validNameRE.MatchString(opts.Name) {
		return state.App{}, fmt.Errorf("APP_NAME_INVALID: name must match ^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$")
	}

	// Check for duplicates
	exists, err := m.db.AppExists(opts.Name)
	if err != nil {
		return state.App{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if exists {
		return state.App{}, fmt.Errorf("APP_ALREADY_EXISTS: %s already registered", opts.Name)
	}

	// Validate git URL reachability
	if err := checkGitReachable(opts.RepoURL, opts.DeployKeyPath); err != nil {
		return state.App{}, fmt.Errorf("GIT_UNREACHABLE: %w", err)
	}

	// Allocate internal port
	port, err := m.db.AllocatePort(m.ports.start, m.ports.end)
	if err != nil {
		return state.App{}, fmt.Errorf("PORT_EXHAUSTED: %w", err)
	}

	// Defaults
	branch := opts.Branch
	if branch == "" {
		branch = "main"
	}
	composeFile := opts.ComposeFile
	if composeFile == "" {
		composeFile = "docker-compose.yml"
	}
	buildContext := opts.BuildContext
	if buildContext == "" {
		buildContext = "."
	}
	containerPort := opts.ContainerPort
	if containerPort == 0 {
		containerPort = 3000
	}

	a := state.App{
		Name:          opts.Name,
		RepoURL:       opts.RepoURL,
		Branch:        branch,
		Domain:        opts.Domain,
		ContainerPort: containerPort,
		InternalPort:  port,
		ComposeFile:   composeFile,
		BuildContext:  buildContext,
		DeployKeyPath: opts.DeployKeyPath,
		Status:        "created",
	}

	if err := m.db.CreateApp(a); err != nil {
		return state.App{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	return a, nil
}

// Deploy runs the full deploy sequence for an existing application.
func (m *Manager) Deploy(opts DeployOptions) (DeployResult, error) {
	start := time.Now()
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60
	}

	// Load app record
	app, err := m.db.GetApp(opts.AppName)
	if err != nil {
		return DeployResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if app.Name == "" {
		return DeployResult{}, fmt.Errorf("APP_NOT_FOUND: %s", opts.AppName)
	}

	appDir := filepath.Join(m.appsDir, app.Name)
	repoDir := filepath.Join(appDir, "repo")

	// Clone or update repository
	commitSHA, err := syncRepo(app.RepoURL, app.Branch, app.DeployKeyPath, repoDir)
	if err != nil {
		return DeployResult{}, err
	}

	// Check if already at this commit (unless --force)
	if !opts.Force {
		last, _ := m.db.LastDeployment(app.Name)
		if last.CommitSHA == commitSHA && last.Status == "ok" {
			return DeployResult{
				Status:    "ok",
				App:       app.Name,
				CommitSHA: commitSHA,
				Duration:  time.Since(start).Milliseconds(),
				URL:       "https://" + app.Domain,
			}, nil
		}
	}

	// Write .env file from encrypted DB vars (idempotent; empty file is fine).
	if err := m.writeEnvFile(app.Name, appDir); err != nil {
		return DeployResult{}, err
	}

	// Resolve compose file path
	composeFilePath := filepath.Join(repoDir, app.ComposeFile)
	if _, err := os.Stat(composeFilePath); os.IsNotExist(err) {
		// Generate a docker-compose.yml
		composeFilePath = filepath.Join(appDir, "docker-compose.yml")
		if err := generateCompose(repoDir, composeFilePath, app); err != nil {
			return DeployResult{}, fmt.Errorf("APP_TYPE_UNKNOWN: %w", err)
		}
	}

	// docker compose up
	if err := composeUp(appDir, composeFilePath); err != nil {
		_, _ = m.db.InsertDeployment(state.Deployment{
			AppName:    app.Name,
			CommitSHA:  commitSHA,
			Status:     "failed",
			ErrorCode:  "DOCKER_START_FAILED",
			DurationMS: time.Since(start).Milliseconds(),
		})
		return DeployResult{}, fmt.Errorf("DOCKER_START_FAILED: %w", err)
	}

	// Health check
	containerName := app.Name + "_app_1"
	if err := waitHealthy(containerName, timeout); err != nil {
		_, _ = m.db.InsertDeployment(state.Deployment{
			AppName:    app.Name,
			CommitSHA:  commitSHA,
			Status:     "failed",
			ErrorCode:  "HEALTH_CHECK_TIMEOUT",
			DurationMS: time.Since(start).Milliseconds(),
		})
		return DeployResult{}, fmt.Errorf("HEALTH_CHECK_TIMEOUT: %w", err)
	}

	// Register route in Caddy
	upstreamAddr := fmt.Sprintf("127.0.0.1:%d", app.InternalPort)
	if err := m.caddy.AddRoute(caddy.Route{
		AppName:      app.Name,
		Domain:       app.Domain,
		UpstreamAddr: upstreamAddr,
	}); err != nil {
		return DeployResult{}, fmt.Errorf("CADDY_CONFIG_FAILED: %w", err)
	}

	// Persist deployment record and update app status
	dur := time.Since(start).Milliseconds()
	_, _ = m.db.InsertDeployment(state.Deployment{
		AppName:    app.Name,
		CommitSHA:  commitSHA,
		Status:     "ok",
		DurationMS: dur,
	})
	_ = m.db.UpdateAppStatus(app.Name, "running")

	return DeployResult{
		Status:     "ok",
		App:        app.Name,
		DeployedAt: time.Now().UTC().Format(time.RFC3339),
		CommitSHA:  commitSHA,
		Duration:   dur,
		URL:        "https://" + app.Domain,
	}, nil
}

// List returns a summary of all active applications.
func (m *Manager) List() ([]AppSummary, error) {
	apps, err := m.db.ListApps()
	if err != nil {
		return nil, err
	}
	out := make([]AppSummary, 0, len(apps))
	for _, a := range apps {
		last, _ := m.db.LastDeployment(a.Name)
		s := AppSummary{
			Name:      a.Name,
			Status:    a.Status,
			Domain:    a.Domain,
			Branch:    a.Branch,
			CommitSHA: last.CommitSHA,
			URL:       "https://" + a.Domain,
		}
		if !last.DeployedAt.IsZero() {
			s.DeployedAt = last.DeployedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, s)
	}
	return out, nil
}

// --- git helpers ---

func checkGitReachable(repoURL, deployKeyPath string) error {
	args := []string{"ls-remote", repoURL}
	cmd := buildGitCmd(deployKeyPath, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git ls-remote %s: %w", repoURL, err)
	}
	return nil
}

func syncRepo(repoURL, branch, deployKeyPath, repoDir string) (string, error) {
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		cmd := buildGitCmd(deployKeyPath, "clone", "--branch", branch, "--depth", "1", repoURL, repoDir)
		var buf bytes.Buffer
		cmd.Stderr = &buf
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("GIT_CLONE_FAILED: %w\n%s", err, buf.String())
		}
	} else {
		cmds := [][]string{
			{"fetch", "origin", branch},
			{"reset", "--hard", "origin/" + branch},
		}
		for _, args := range cmds {
			cmd := buildGitCmd(deployKeyPath, append([]string{"-C", repoDir}, args...)...)
			var buf bytes.Buffer
			cmd.Stderr = &buf
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("GIT_PULL_FAILED: %w\n%s", err, buf.String())
			}
		}
	}

	// Get current commit SHA
	out, err := exec.Command("git", "-C", repoDir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func buildGitCmd(deployKeyPath string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	if deployKeyPath != "" {
		cmd.Env = append(os.Environ(),
			"GIT_SSH_COMMAND=ssh -i "+deployKeyPath+" -o StrictHostKeyChecking=no")
	}
	return cmd
}

// --- docker compose helpers ---

func composeUp(appDir, composeFilePath string) error {
	cmd := exec.Command("docker", "compose",
		"-f", composeFilePath,
		"--project-directory", appDir,
		"up", "-d", "--build", "--remove-orphans")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

func waitHealthy(containerName string, timeoutSec int) error {
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "inspect",
			"--format", "{{.State.Health.Status}}",
			containerName).Output()
		if err == nil {
			status := strings.TrimSpace(string(out))
			if status == "healthy" {
				return nil
			}
			if status == "unhealthy" {
				return fmt.Errorf("container %s is unhealthy", containerName)
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("container %s did not become healthy within %ds", containerName, timeoutSec)
}

// --- compose generation ---

func generateCompose(repoDir, outputPath string, app state.App) error {
	appType := detectAppType(repoDir)
	if appType == "" {
		return fmt.Errorf("cannot detect app type in %s — add a Dockerfile", repoDir)
	}

	content := buildComposeContent(app, repoDir, appType)
	return os.WriteFile(outputPath, []byte(content), 0644)
}

func detectAppType(repoDir string) string {
	checks := []struct {
		file    string
		appType string
	}{
		{"Dockerfile", "dockerfile"},
		{"package.json", "nodejs"},
		{"requirements.txt", "python"},
		{"pyproject.toml", "python"},
		{"go.mod", "go"},
		{"Gemfile", "ruby"},
		{"composer.json", "php"},
	}
	for _, c := range checks {
		if _, err := os.Stat(filepath.Join(repoDir, c.file)); err == nil {
			return c.appType
		}
	}
	return ""
}

func buildComposeContent(app state.App, repoDir, appType string) string {
	internalPort := app.InternalPort
	containerPort := app.ContainerPort

	var buildSection string
	switch appType {
	case "dockerfile":
		buildSection = fmt.Sprintf("    build:\n      context: %s\n", repoDir)
	case "nodejs":
		buildSection = fmt.Sprintf("    image: node:lts-alpine\n    working_dir: /app\n    volumes:\n      - %s:/app\n    command: sh -c \"npm ci && npm start\"\n", repoDir)
	case "python":
		buildSection = fmt.Sprintf("    image: python:3.12-slim\n    working_dir: /app\n    volumes:\n      - %s:/app\n    command: sh -c \"pip install -r requirements.txt && python -m flask run --host=0.0.0.0 --port=%d\"\n", repoDir, containerPort)
	case "go":
		buildSection = fmt.Sprintf("    build:\n      context: %s\n      dockerfile: Dockerfile.generated\n", repoDir)
	case "ruby":
		buildSection = fmt.Sprintf("    image: ruby:3.3-slim\n    working_dir: /app\n    volumes:\n      - %s:/app\n    command: sh -c \"bundle install && bundle exec ruby app.rb\"\n", repoDir)
	case "php":
		buildSection = fmt.Sprintf("    image: php:8.3-fpm-alpine\n    working_dir: /app\n    volumes:\n      - %s:/app\n", repoDir)
	default:
		buildSection = fmt.Sprintf("    build:\n      context: %s\n", repoDir)
	}

	return fmt.Sprintf(`services:
  app:
%s    restart: unless-stopped
    env_file:
      - .env
    ports:
      - "127.0.0.1:%d:%d"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:%d/"]
      interval: 10s
      timeout: 5s
      retries: 6
      start_period: 30s
    logging:
      driver: "json-file"
      options:
        max-size: "50m"
        max-file: "5"
`, buildSection, internalPort, containerPort, containerPort)
}
