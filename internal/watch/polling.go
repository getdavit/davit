package watch

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/config"
	"github.com/getdavit/davit/internal/state"
)

// PollingScheduler manages polling goroutines for apps configured for polling mode.
type PollingScheduler struct {
	db      *state.DB
	manager *app.Manager
	cfg     *config.Config
}

// NewPollingScheduler creates a new PollingScheduler.
func NewPollingScheduler(db *state.DB, manager *app.Manager, cfg *config.Config) *PollingScheduler {
	return &PollingScheduler{
		db:      db,
		manager: manager,
		cfg:     cfg,
	}
}

// Start launches polling goroutines for each polling-mode watcher.
// It blocks until ctx is cancelled; each polling loop runs in its own goroutine.
func (ps *PollingScheduler) Start(ctx context.Context, watchers []state.Watcher) {
	for _, w := range watchers {
		if w.Method != "polling" {
			continue
		}
		w := w // capture loop variable
		interval := time.Duration(w.PollIntervalSeconds) * time.Second
		if interval < 10*time.Second {
			interval = 30 * time.Second // minimum 30s to avoid hammering
		}

		go func() {
			log.Printf("Starting poll watcher for app %s (interval: %v)\n", w.AppName, interval)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			// Do an initial check immediately
			ps.checkAndDeploy(w)

			for {
				select {
				case <-ctx.Done():
					log.Printf("Stopping poll watcher for app %s\n", w.AppName)
					return
				case <-ticker.C:
					ps.checkAndDeploy(w)
				}
			}
		}()
	}
}

// checkAndDeploy performs a single poll cycle for a watcher.
func (ps *PollingScheduler) checkAndDeploy(w state.Watcher) {
	appRecord, err := ps.db.GetApp(w.AppName)
	if err != nil {
		log.Printf("Poll error [%s]: failed to get app: %v\n", w.AppName, err)
		return
	}
	if appRecord.Name == "" {
		log.Printf("Poll error [%s]: app not found\n", w.AppName)
		_ = ps.deactivateWatcher(w.AppName)
		return
	}

	// Get latest remote commit
	latestSHA, err := fetchLatestCommit(appRecord.RepoURL, appRecord.Branch, appRecord.DeployKeyPath)
	if err != nil {
		log.Printf("Poll error [%s]: failed to fetch latest commit: %v\n", w.AppName, err)
		_ = ps.db.UpdateWatcherCommit(w.AppName, "")
		return
	}

	// Compare with stored commit
	if latestSHA == w.LastCommitSHA && w.LastCommitSHA != "" {
		// No change — just update timestamp
		_ = ps.db.UpdateWatcherCommit(w.AppName, w.LastCommitSHA)
		return
	}

	log.Printf("Poll [%s]: new commit detected %s (was %s)\n", w.AppName, latestSHA, w.LastCommitSHA)

	// Trigger async deploy
	go func() {
		result, err := ps.manager.Deploy(app.DeployOptions{
			AppName: w.AppName,
			Timeout: 120,
			Force:   true,
		})
		if err != nil {
			log.Printf("Poll deploy error [%s]: %v\n", w.AppName, err)
		} else {
			log.Printf("Poll deploy success [%s]: commit=%s duration=%dms\n",
				w.AppName, result.CommitSHA, result.Duration)
		}
	}()

	// Update stored commit SHA and timestamp
	_ = ps.db.UpdateWatcherCommit(w.AppName, latestSHA)
}

// fetchLatestCommit gets the latest commit SHA from the remote repository.
func fetchLatestCommit(repoURL, branch, deployKeyPath string) (string, error) {
	args := []string{"ls-remote", repoURL, "refs/heads/" + branch}
	cmd := exec.Command("git", args...)
	if deployKeyPath != "" {
		cmd.Env = append(cmd.Environ(),
			"GIT_SSH_COMMAND=ssh -i "+deployKeyPath+" -o StrictHostKeyChecking=no")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Output format: "<SHA>\trefs/heads/<branch>"
	parts := strings.Fields(string(output))
	if len(parts) < 1 {
		return "", nil
	}
	return parts[0], nil
}

// deactivateWatcher marks a watcher as inactive (e.g., app was removed).
func (ps *PollingScheduler) deactivateWatcher(appName string) error {
	return ps.db.UpdateAppWatch(appName, false, 0, false)
}