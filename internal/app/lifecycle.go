package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/getdavit/davit/internal/caddy"
	"github.com/getdavit/davit/internal/state"
)

// StopResult is returned by a successful Stop operation.
type StopResult struct {
	Status string `json:"status"`
	App    string `json:"app"`
}

// StartResult is returned by a successful Start operation.
type StartResult struct {
	Status string `json:"status"`
	App    string `json:"app"`
	URL    string `json:"url"`
}

// RestartResult is returned by a successful Restart operation.
type RestartResult struct {
	Status string `json:"status"`
	App    string `json:"app"`
	URL    string `json:"url"`
}

// RemoveResult is returned by a successful Remove operation.
type RemoveResult struct {
	Status string `json:"status"`
	App    string `json:"app"`
}

// DiagnoseResult contains full diagnostic information about an app.
type DiagnoseResult struct {
	Status      string        `json:"status"`
	App         AppDiagInfo   `json:"app"`
	Container   ContainerInfo `json:"container"`
	Caddy       CaddyInfo     `json:"caddy"`
	Deployments []DeployInfo  `json:"recent_deployments"`
	DiskBytes   int64         `json:"disk_bytes"`
}

// AppDiagInfo holds the app metadata section of a diagnosis.
type AppDiagInfo struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	Domain        string `json:"domain"`
	Branch        string `json:"branch"`
	Repo          string `json:"repo"`
	ContainerPort int    `json:"container_port"`
	InternalPort  int    `json:"internal_port"`
	CreatedAt     string `json:"created_at"`
}

// ContainerInfo holds Docker container state for a diagnosis.
type ContainerInfo struct {
	Name      string `json:"name"`
	Running   bool   `json:"running"`
	Health    string `json:"health"`
	StartedAt string `json:"started_at,omitempty"`
	Image     string `json:"image,omitempty"`
}

// CaddyInfo holds Caddy reverse-proxy state for a diagnosis.
type CaddyInfo struct {
	Reachable   bool `json:"reachable"`
	RouteActive bool `json:"route_active"`
}

// DeployInfo is a condensed deployment record for diagnostics.
type DeployInfo struct {
	ID         int64  `json:"id"`
	CommitSHA  string `json:"commit"`
	Status     string `json:"status"`
	ErrorCode  string `json:"error_code,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	DeployedAt string `json:"deployed_at"`
}

// findComposeFile returns the compose file path for an app.
// It prefers the file inside the repo; falls back to the generated file in appDir.
func findComposeFile(appDir string, a state.App) string {
	repoPath := filepath.Join(appDir, "repo", a.ComposeFile)
	if _, err := os.Stat(repoPath); err == nil {
		return repoPath
	}
	return filepath.Join(appDir, "docker-compose.yml")
}

// Stop stops the Docker containers for an app and removes its Caddy route.
// Idempotent: stopping an already-stopped app returns success.
func (m *Manager) Stop(appName string) (StopResult, error) {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return StopResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return StopResult{}, fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	if a.Status != "stopped" {
		appDir := filepath.Join(m.appsDir, a.Name)
		composeFile := findComposeFile(appDir, a)

		if err := composeDown(appDir, composeFile, false); err != nil {
			return StopResult{}, fmt.Errorf("DOCKER_STOP_FAILED: %w", err)
		}
		_ = m.caddy.RemoveRoute(a.Name)
		_ = m.db.UpdateAppStatus(a.Name, "stopped")
	}

	return StopResult{Status: "ok", App: a.Name}, nil
}

// Start starts previously stopped Docker containers and re-registers the Caddy route.
func (m *Manager) Start(appName string, timeout int) (StartResult, error) {
	if timeout == 0 {
		timeout = 60
	}
	a, err := m.db.GetApp(appName)
	if err != nil {
		return StartResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return StartResult{}, fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	appDir := filepath.Join(m.appsDir, a.Name)
	composeFile := findComposeFile(appDir, a)

	if err := composeStart(appDir, composeFile); err != nil {
		return StartResult{}, fmt.Errorf("DOCKER_START_FAILED: %w", err)
	}

	containerName := a.Name + "_app_1"
	if err := waitHealthy(containerName, timeout); err != nil {
		return StartResult{}, fmt.Errorf("HEALTH_CHECK_TIMEOUT: %w", err)
	}

	upstreamAddr := fmt.Sprintf("127.0.0.1:%d", a.InternalPort)
	if err := m.caddy.AddRoute(caddy.Route{
		AppName:      a.Name,
		Domain:       a.Domain,
		UpstreamAddr: upstreamAddr,
	}); err != nil {
		return StartResult{}, fmt.Errorf("CADDY_CONFIG_FAILED: %w", err)
	}

	_ = m.db.UpdateAppStatus(a.Name, "running")
	return StartResult{Status: "ok", App: a.Name, URL: "https://" + a.Domain}, nil
}

// Restart restarts an app's containers in-place and waits for a healthy state.
func (m *Manager) Restart(appName string, timeout int) (RestartResult, error) {
	if timeout == 0 {
		timeout = 60
	}
	a, err := m.db.GetApp(appName)
	if err != nil {
		return RestartResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return RestartResult{}, fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	appDir := filepath.Join(m.appsDir, a.Name)
	composeFile := findComposeFile(appDir, a)

	if err := composeRestart(appDir, composeFile); err != nil {
		return RestartResult{}, fmt.Errorf("DOCKER_START_FAILED: %w", err)
	}

	containerName := a.Name + "_app_1"
	if err := waitHealthy(containerName, timeout); err != nil {
		return RestartResult{}, fmt.Errorf("HEALTH_CHECK_TIMEOUT: %w", err)
	}

	_ = m.db.UpdateAppStatus(a.Name, "running")
	return RestartResult{Status: "ok", App: a.Name, URL: "https://" + a.Domain}, nil
}

// Remove tears down an app: stops containers, removes Caddy route, soft-deletes from DB.
// If purgeData is true, the app directory on disk is also removed.
func (m *Manager) Remove(appName string, purgeData bool) (RemoveResult, error) {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return RemoveResult{}, fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	appDir := filepath.Join(m.appsDir, a.Name)
	composeFile := findComposeFile(appDir, a)

	// Bring down containers (and volumes if purging).
	_ = composeDown(appDir, composeFile, purgeData)

	// Remove Caddy route (ignore error — route may not exist if app was stopped).
	_ = m.caddy.RemoveRoute(a.Name)

	// Soft-delete app record.
	if err := m.db.RemoveApp(a.Name); err != nil {
		return RemoveResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}

	// Optionally wipe app directory.
	if purgeData {
		_ = os.RemoveAll(appDir)
	}

	return RemoveResult{Status: "ok", App: a.Name}, nil
}

// Logs streams Docker Compose log output for an app to w.
func (m *Manager) Logs(appName string, lines int, follow bool, w io.Writer) error {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	appDir := filepath.Join(m.appsDir, a.Name)
	composeFile := findComposeFile(appDir, a)

	tail := fmt.Sprintf("%d", lines)
	args := []string{
		"compose", "-f", composeFile, "--project-directory", appDir,
		"logs", "--no-color", "--tail", tail,
	}
	if follow {
		args = append(args, "--follow")
	}

	cmd := exec.Command("docker", args...)
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

// Diagnose returns a comprehensive snapshot of an app's health and history.
func (m *Manager) Diagnose(appName string) (DiagnoseResult, error) {
	a, err := m.db.GetApp(appName)
	if err != nil {
		return DiagnoseResult{}, fmt.Errorf("STATE_DB_ERROR: %w", err)
	}
	if a.Name == "" {
		return DiagnoseResult{}, fmt.Errorf("APP_NOT_FOUND: %s", appName)
	}

	result := DiagnoseResult{Status: "ok"}

	// App metadata.
	result.App = AppDiagInfo{
		Name:          a.Name,
		Status:        a.Status,
		Domain:        a.Domain,
		Branch:        a.Branch,
		Repo:          a.RepoURL,
		ContainerPort: a.ContainerPort,
		InternalPort:  a.InternalPort,
		CreatedAt:     a.CreatedAt.UTC().Format(time.RFC3339),
	}

	// Container state.
	containerName := a.Name + "_app_1"
	result.Container = inspectContainer(containerName)

	// Caddy state.
	result.Caddy = checkCaddyRoute(m.caddy, a.Name)

	// Recent deployments (last 5).
	deployments, _ := m.db.RecentDeployments(a.Name, 5)
	for _, d := range deployments {
		result.Deployments = append(result.Deployments, DeployInfo{
			ID:         d.ID,
			CommitSHA:  d.CommitSHA,
			Status:     d.Status,
			ErrorCode:  d.ErrorCode,
			DurationMS: d.DurationMS,
			DeployedAt: d.DeployedAt.UTC().Format(time.RFC3339),
		})
	}

	// Disk usage of app directory.
	appDir := filepath.Join(m.appsDir, a.Name)
	result.DiskBytes = dirSize(appDir)

	return result, nil
}

// --- docker compose helpers ---

func composeDown(appDir, composeFilePath string, removeVolumes bool) error {
	args := []string{
		"compose", "-f", composeFilePath, "--project-directory", appDir,
		"down", "--remove-orphans",
	}
	if removeVolumes {
		args = append(args, "--volumes")
	}
	cmd := exec.Command("docker", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

func composeStart(appDir, composeFilePath string) error {
	cmd := exec.Command("docker", "compose",
		"-f", composeFilePath, "--project-directory", appDir,
		"start")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

func composeRestart(appDir, composeFilePath string) error {
	cmd := exec.Command("docker", "compose",
		"-f", composeFilePath, "--project-directory", appDir,
		"restart")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w\n%s", err, buf.String())
	}
	return nil
}

// --- inspect helpers ---

type dockerInspectState struct {
	Running   bool   `json:"Running"`
	StartedAt string `json:"StartedAt"`
	Health    *struct {
		Status string `json:"Status"`
	} `json:"Health"`
	Image string `json:"Image"`
}

func inspectContainer(containerName string) ContainerInfo {
	out, err := exec.Command("docker", "inspect",
		"--format", "{{json .State}}", containerName).Output()
	if err != nil {
		return ContainerInfo{Name: containerName, Running: false, Health: "unknown"}
	}

	// docker inspect --format "{{json .State}}" returns the State object.
	// We also need the image; get it separately.
	var s dockerInspectState
	if err := json.Unmarshal(bytes.TrimSpace(out), &s); err != nil {
		return ContainerInfo{Name: containerName, Running: false, Health: "unknown"}
	}

	health := "none"
	if s.Health != nil {
		health = s.Health.Status
	}

	imgOut, _ := exec.Command("docker", "inspect",
		"--format", "{{.Config.Image}}", containerName).Output()

	return ContainerInfo{
		Name:      containerName,
		Running:   s.Running,
		Health:    health,
		StartedAt: s.StartedAt,
		Image:     strings.TrimSpace(string(imgOut)),
	}
}

func checkCaddyRoute(c *caddy.Client, appName string) CaddyInfo {
	err := c.Ping()
	if err != nil {
		return CaddyInfo{Reachable: false, RouteActive: false}
	}
	// A route exists if RemoveRoute returns successfully on a non-existent name
	// or if we can GET the route id. We use a lightweight check: try to GET
	// /id/davit_<appName>. If Caddy returns 200, the route is active.
	active := c.RouteExists(appName)
	return CaddyInfo{Reachable: true, RouteActive: active}
}

// dirSize returns the total byte size of a directory tree.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total
}
