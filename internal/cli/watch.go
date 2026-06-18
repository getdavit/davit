package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/osdetect"
	"github.com/getdavit/davit/internal/state"
	"github.com/getdavit/davit/internal/watch"
)

func init() {
	watchCmd := &cobra.Command{
		Use:   "watch",
		Short: "Git automation for automatic redeployment",
		Long: `Manage Git watching for automatic application redeployment.
Enable polling daemon or webhook receiver to watch for repository changes.`,
	}
	watchCmd.AddCommand(
		watchEnableCmd(),
		watchDisableCmd(),
		watchStatusCmd(),
		daemonCmd(),
	)
	rootCmd.AddCommand(watchCmd)
}

func watchEnableCmd() *cobra.Command {
	var pollInterval int
	var webhook bool

	cmd := &cobra.Command{
		Use:   "enable <app-name>",
		Short: "Enable watching for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Get database
			_, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			// Check if app exists
			exists, err := db.AppExists(name)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("app %s not found", name)
			}

			// Update watch settings
			err = db.UpdateAppWatch(name, true, pollInterval, webhook)
			if err != nil {
				return err
			}

			// Check if we need to start the watcher service
			watchedCount, err := db.CountWatchedApps()
			if err != nil {
				return fmt.Errorf("failed to count watched apps: %w", err)
			}

			// If this is the first app being watched, start the service
			if watchedCount == 1 {
				if err := startWatcherService(); err != nil {
					return fmt.Errorf("failed to start watcher service: %w", err)
				}
			}

			var action string
			if webhook {
				action = "webhook"
			} else {
				action = fmt.Sprintf("polling (every %ds)", pollInterval)
			}

			msg := fmt.Sprintf("Watching enabled for app %s (%s)", name, action)

			// If webhook mode, include setup instructions
			if webhook {
				msg += "\n\n" + webhookSetupInstructions(name, db)
			}

			return rootWriter.JSON(map[string]any{
				"status":  "success",
				"message": msg,
			})
		},
	}

	cmd.Flags().IntVar(&pollInterval, "poll-interval", 30, "Polling interval in seconds")
	cmd.Flags().BoolVar(&webhook, "webhook", false, "Use webhook instead of polling")

	return cmd
}

func watchDisableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable <app-name>",
		Short: "Disable watching for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Get database
			_, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			// Check if app exists
			exists, err := db.AppExists(name)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("app %s not found", name)
			}

			// Disable watch settings
			err = db.UpdateAppWatch(name, false, 0, false)
			if err != nil {
				return err
			}

			// Check if we need to stop the watcher service
			watchedCount, err := db.CountWatchedApps()
			if err != nil {
				return fmt.Errorf("failed to count watched apps: %w", err)
			}

			// If no more apps are being watched, stop the service
			if watchedCount == 0 {
				if err := stopWatcherService(); err != nil {
					return fmt.Errorf("failed to stop watcher service: %w", err)
				}
			}

			return rootWriter.JSON(map[string]any{
				"status":  "success",
				"message": fmt.Sprintf("Watching disabled for app %s", name),
			})
		},
	}

	return cmd
}

func watchStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [app-name]",
		Short: "Show watching status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get database
			_, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			if len(args) == 1 {
				// Status for specific app
				name := args[0]

				app, err := db.GetApp(name)
				if err != nil {
					return err
				}
				if app.Name == "" {
					return fmt.Errorf("app %s not found", name)
				}

				response := map[string]any{
					"app":           name,
					"watching":      app.WatchEnabled,
					"poll_interval": app.WatchPollInterval,
					"last_checked":  app.LastCheckedAt,
					"last_commit":   app.LastCommitSHA,
				}

				if app.WatchUseWebhook {
					response["method"] = "webhook"
				} else {
					response["method"] = "polling"
				}

				return rootWriter.JSON(response)
			}

			// Status for all apps
			watcherServiceStatus, err := getWatcherServiceStatus()
			if err != nil {
				watcherServiceStatus = "unknown"
			}

			watchedCount, err := db.CountWatchedApps()
			if err != nil {
				return fmt.Errorf("failed to count watched apps: %w", err)
			}

			apps, err := db.ListApps()
			if err != nil {
				return err
			}

			var watchedApps []string
			for _, app := range apps {
				if app.WatchEnabled {
					watchedApps = append(watchedApps, app.Name)
				}
			}

			return rootWriter.JSON(map[string]any{
				"service_status": watcherServiceStatus,
				"watched_count":  watchedCount,
				"watched_apps":   watchedApps,
			})
		},
	}

	return cmd
}

// webhookSetupInstructions returns provider-specific setup instructions for webhook mode.
func webhookSetupInstructions(appName string, db *state.DB) string {
	watcher, err := db.GetWatcher(appName)
	if err != nil || watcher == nil {
		return "Unable to retrieve webhook configuration."
	}

	webhookURL := fmt.Sprintf("http://<server-hostname>/.davit/webhook/%s/%s", appName, watcher.WebhookToken)

	var instructions strings.Builder
	instructions.WriteString("Webhook setup instructions:\n\n")

	// GitHub
	instructions.WriteString("--- GitHub ---\n")
	instructions.WriteString("1. Go to your repository on GitHub\n")
	instructions.WriteString("2. Settings → Webhooks → Add webhook\n")
	instructions.WriteString(fmt.Sprintf("3. Payload URL: %s\n", webhookURL))
	instructions.WriteString("4. Content type: application/json\n")
	instructions.WriteString("5. Secret: (leave blank unless you set a webhook secret)\n")
	instructions.WriteString("6. Events: Just the push event\n")
	instructions.WriteString("7. Active: ✅\n\n")

	// GitLab
	instructions.WriteString("--- GitLab ---\n")
	instructions.WriteString("1. Go to your repository on GitLab\n")
	instructions.WriteString("2. Settings → Webhooks\n")
	instructions.WriteString(fmt.Sprintf("3. URL: %s\n", webhookURL))
	instructions.WriteString("4. Secret token: (leave blank unless you set one)\n")
	instructions.WriteString("5. Trigger: Push events\n")
	instructions.WriteString("6. Enable SSL verification: ✅\n\n")

	// Generic
	instructions.WriteString("--- Generic / Custom ---\n")
	instructions.WriteString(fmt.Sprintf("POST JSON payload to: %s\n", webhookURL))
	instructions.WriteString("Payload format: {\"ref\": \"refs/heads/<branch>\", \"after\": \"<commit-sha>\"}\n")

	return instructions.String()
}

// getOSProfile detects the operating system characteristics.
func getOSProfile() (*osdetect.Profile, error) {
	return osdetect.Detect()
}

// isSystemd returns true if the system uses systemd.
func isSystemd() bool {
	profile, err := getOSProfile()
	if err != nil {
		return false
	}
	return profile.InitSystem == osdetect.InitSystemd
}

// createWatcherServiceFile creates the systemd service file for the Davit watcher daemon.
func createWatcherServiceFile() error {
	serviceContent := `[Unit]
Description=Davit Git Watcher and Webhook Receiver
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/davit daemon
Restart=always
RestartSec=10
StartLimitInterval=60
StartLimitBurst=3

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
PrivateDevices=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX

[Install]
WantedBy=multi-user.target
`

	return os.WriteFile("/etc/systemd/system/davit-watcher.service", []byte(serviceContent), 0644)
}

// startWatcherService starts the Davit watcher systemd service.
func startWatcherService() error {
	if !isSystemd() {
		return fmt.Errorf("systemd not detected")
	}

	if err := createWatcherServiceFile(); err != nil {
		return fmt.Errorf("failed to create service file: %w", err)
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	if err := exec.Command("systemctl", "start", "davit-watcher").Run(); err != nil {
		return fmt.Errorf("failed to start davit-watcher service: %w", err)
	}

	return nil
}

// stopWatcherService stops the Davit watcher systemd service.
func stopWatcherService() error {
	if !isSystemd() {
		return fmt.Errorf("systemd not detected")
	}

	if err := exec.Command("systemctl", "stop", "davit-watcher").Run(); err != nil {
		return fmt.Errorf("failed to stop davit-watcher service: %w", err)
	}

	return nil
}

// getWatcherServiceStatus returns the status of the Davit watcher service.
func getWatcherServiceStatus() (string, error) {
	if !isSystemd() {
		return "inactive", fmt.Errorf("systemd not detected")
	}

	outputBytes, err := exec.Command("systemctl", "is-active", "davit-watcher").CombinedOutput()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			if exitError.ExitCode() == 3 || exitError.ExitCode() == 4 {
				return "inactive", nil
			}
		}
		return "", fmt.Errorf("failed to get service status: %w", err)
	}

	return strings.TrimSpace(string(outputBytes)), nil
}

// runDaemon runs the Git watcher and webhook receiver daemon.
func runDaemon() error {
	fmt.Println("Starting Davit watcher daemon...")

	// Load config and open database
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	db, err := openDB()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create and run the daemon
	daemon := watch.NewDaemon(db, cfg)
	return daemon.Run()
}

// daemonCmd defines the `davit daemon` command.
func daemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the Git watcher and webhook receiver daemon",
		Long:  `Run the background daemon that watches Git repositories for changes and handles webhook requests for automatic redeployment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon()
		},
	}
}