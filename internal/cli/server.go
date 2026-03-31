package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/osdetect"
	"github.com/getdavit/davit/internal/output"
	"github.com/getdavit/davit/internal/pkgmgr"
	"github.com/getdavit/davit/internal/provisioner"
	"github.com/getdavit/davit/internal/state"
	"github.com/getdavit/davit/internal/version"
)

func init() {
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Server provisioning and status",
	}
	serverCmd.AddCommand(serverInitCmd(), serverStatusCmd())
	rootCmd.AddCommand(serverCmd)
}

func serverInitCmd() *cobra.Command {
	var timezone, email, skipSteps string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Provision this server (idempotent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				e := rootWriter.Error(output.ErrConfigInvalid, "load config: "+err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			db, err := openDB()
			if err != nil {
				e := rootWriter.Error(output.ErrStateDBError, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}
			defer db.Close()

			profile, err := osdetect.Detect()
			if err != nil {
				e := rootWriter.Error(output.ErrUnsupportedOS, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			pm, err := pkgmgr.New(profile.PkgMgr)
			if err != nil {
				e := rootWriter.Error(output.ErrUnsupportedOS, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			var skip []provisioner.StepName
			for _, s := range strings.Split(skipSteps, ",") {
				if s = strings.TrimSpace(s); s != "" {
					skip = append(skip, provisioner.StepName(s))
				}
			}

			if email == "" {
				email = cfg.Server.AdminEmail
			}
			if timezone == "" {
				timezone = cfg.Server.Timezone
			}

			emit := func(sr provisioner.StepResult) {
				_ = rootWriter.JSON(sr)
			}

			prov := provisioner.New(db, profile, pm, emit)
			res, _ := prov.Run(provisioner.Options{
				Timezone:  timezone,
				Email:     email,
				SkipSteps: skip,
				DryRun:    dryRun,
			})

			_ = rootWriter.JSON(res)
			if res.Status != "ok" {
				os.Exit(output.ExitCode(output.ErrProvisionStepFailed))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&timezone, "timezone", "", "timezone (e.g. Europe/London); default: UTC")
	cmd.Flags().StringVar(&email, "email", "", "admin email for Let's Encrypt (required)")
	cmd.Flags().StringVar(&skipSteps, "skip-steps", "", "comma-separated list of step names to skip")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without applying changes")
	return cmd
}

func serverStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				e := rootWriter.Error(output.ErrConfigInvalid, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			db, err := openDB()
			if err != nil {
				e := rootWriter.Error(output.ErrStateDBError, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}
			defer db.Close()

			hostname, _ := os.Hostname()
			provisioned, _ := db.GetSystemInfo("provisioned_at")

			memUsed, memTotal := readMeminfo()
			diskPercent := readDiskUsage("/")

			apps, _ := db.ListApps()
			runningApps := 0
			for _, a := range apps {
				if a.Status == "running" {
					runningApps++
				}
			}

			statusMap := map[string]any{
				"hostname":           hostname,
				"os":                 readPrettyName(),
				"arch":               runtime.GOARCH,
				"davit_version":      version.Version,
				"provisioned":        provisioned != "",
				"provisioned_at":     provisioned,
				"uptime_seconds":     readUptime(),
				"disk_usage_percent": diskPercent,
				"memory_used_mb":     memUsed,
				"memory_total_mb":    memTotal,
				"docker_running":     svcRunning("docker"),
				"caddy_running":      svcRunning("caddy"),
				"daemon_running":     svcRunning("davit-daemon"),
				"fail2ban_running":   svcRunning("fail2ban"),
				"firewall_active":    firewallActive(),
				"apps_total":         len(apps),
				"apps_running":       runningApps,
				"caddy_api":          cfg.Caddy.AdminAPI,
			}

			return rootWriter.JSON(map[string]any{"status": "ok", "server": statusMap})
		},
	}
}

// --- helpers ---

func readMeminfo() (used, total int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	defer f.Close()
	fields := make(map[string]int64)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 2 {
			continue
		}
		v, _ := strconv.ParseInt(parts[1], 10, 64)
		fields[strings.TrimSuffix(parts[0], ":")] = v
	}
	totalKB := fields["MemTotal"]
	availKB := fields["MemAvailable"]
	total = totalKB / 1024
	used = (totalKB - availKB) / 1024
	return
}

func readDiskUsage(path string) int {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0
	}
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	if total == 0 {
		return 0
	}
	return int((total - free) * 100 / total)
}

func readUptime() int64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	parts := strings.Fields(string(data))
	if len(parts) == 0 {
		return 0
	}
	secs, _ := strconv.ParseFloat(parts[0], 64)
	return int64(secs)
}

func readPrettyName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
	}
	return "Linux"
}

func svcRunning(name string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil
}

func firewallActive() bool {
	var buf bytes.Buffer
	cmd := exec.Command("ufw", "status")
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if cmd.Run() == nil && strings.Contains(buf.String(), "Status: active") {
		return true
	}
	return exec.Command("systemctl", "is-active", "--quiet", "firewalld").Run() == nil
}

// openDB opens the state database at the default path.
func openDB() (*state.DB, error) {
	if err := os.MkdirAll("/var/lib/davit", 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return state.Open("/var/lib/davit/davit.db")
}

var _ = time.Now // suppress unused import if timestamp() is removed
