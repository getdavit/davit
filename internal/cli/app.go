package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/caddy"
	"github.com/getdavit/davit/internal/output"
	"github.com/getdavit/davit/internal/state"
)

func init() {
	appCmd := &cobra.Command{
		Use:   "app",
		Short: "Application lifecycle management",
	}
	appCmd.AddCommand(
		appCreateCmd(),
		appDeployCmd(),
		appListCmd(),
		appStopCmd(),
		appStartCmd(),
		appRestartCmd(),
		appRemoveCmd(),
		appEnvCmd(),
	)
	rootCmd.AddCommand(appCmd)
}

func appCreateCmd() *cobra.Command {
	var repo, branch, domain, composeFile, buildContext, deployKey string
	var port int

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Register a new application (does not deploy)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			a, err := mgr.Create(app.CreateOptions{
				Name:          name,
				RepoURL:       repo,
				Branch:        branch,
				Domain:        domain,
				ContainerPort: port,
				ComposeFile:   composeFile,
				BuildContext:  buildContext,
				DeployKeyPath: deployKey,
			})
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"name": name})
				os.Exit(output.ExitCode(e.Code))
			}

			return rootWriter.JSON(map[string]any{
				"status": "ok",
				"app": map[string]any{
					"name":           a.Name,
					"repo":           a.RepoURL,
					"branch":         a.Branch,
					"domain":         a.Domain,
					"port":           a.ContainerPort,
					"internal_port":  a.InternalPort,
					"created_at":     a.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				},
			})
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "git repository URL (required)")
	cmd.Flags().StringVar(&branch, "branch", "main", "branch to track")
	cmd.Flags().StringVar(&domain, "domain", "", "domain to serve the app on (required)")
	cmd.Flags().IntVar(&port, "port", 3000, "internal container port")
	cmd.Flags().StringVar(&composeFile, "compose-file", "docker-compose.yml",
		"path to docker-compose.yml within repo")
	cmd.Flags().StringVar(&buildContext, "build-context", ".",
		"Docker build context within repo")
	cmd.Flags().StringVar(&deployKey, "deploy-key", "",
		"path to SSH private key for private repos")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func appDeployCmd() *cobra.Command {
	var timeout int
	var force bool

	cmd := &cobra.Command{
		Use:   "deploy <name>",
		Short: "Deploy an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			res, err := mgr.Deploy(app.DeployOptions{
				AppName: name,
				Timeout: timeout,
				Force:   force,
			})
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}

			return rootWriter.JSON(res)
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 60, "seconds to wait for health check")
	cmd.Flags().BoolVar(&force, "force", false, "deploy even if commit is unchanged")
	return cmd
}

func appListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered applications",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			apps, err := mgr.List()
			if err != nil {
				e := rootWriter.Error(output.ErrStateDBError, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			return rootWriter.JSON(map[string]any{"apps": apps})
		},
	}
}

func appStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Stop a running application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			res, err := mgr.Stop(name)
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(res)
		},
	}
}

func appStartCmd() *cobra.Command {
	var timeout int
	cmd := &cobra.Command{
		Use:   "start <name>",
		Short: "Start a stopped application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			res, err := mgr.Start(name, timeout)
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(res)
		},
	}
	cmd.Flags().IntVar(&timeout, "timeout", 60, "seconds to wait for health check")
	return cmd
}

func appRestartCmd() *cobra.Command {
	var timeout int
	cmd := &cobra.Command{
		Use:   "restart <name>",
		Short: "Restart a running application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			res, err := mgr.Restart(name, timeout)
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(res)
		},
	}
	cmd.Flags().IntVar(&timeout, "timeout", 60, "seconds to wait for health check")
	return cmd
}

func appRemoveCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an application (stops containers and soft-deletes from DB)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			res, err := mgr.Remove(name, purge)
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(res)
		},
	}
	cmd.Flags().BoolVar(&purge, "purge-data", false,
		"also delete app data directory and Docker volumes")
	return cmd
}

func appEnvCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage application environment variables (encrypted at rest)",
	}
	cmd.AddCommand(appEnvSetCmd(), appEnvGetCmd(), appEnvListCmd(), appEnvUnsetCmd())
	return cmd
}

func appEnvSetCmd() *cobra.Command {
	var redeploy bool
	var timeout int
	cmd := &cobra.Command{
		Use:   "set <app> <KEY> <VALUE>",
		Short: "Set an encrypted environment variable",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, key, value := args[0], args[1], args[2]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := mgr.EnvSet(appName, key, value); err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": appName, "key": key})
				os.Exit(output.ExitCode(e.Code))
			}

			if redeploy {
				res, err := mgr.Deploy(app.DeployOptions{AppName: appName, Timeout: timeout, Force: true})
				if err != nil {
					code := inferAppErrorCode(err)
					e := rootWriter.Error(code, err.Error(), map[string]any{"app": appName})
					os.Exit(output.ExitCode(e.Code))
				}
				return rootWriter.JSON(map[string]any{
					"status": "ok", "app": appName, "key": key,
					"redeployed": true, "deploy": res,
				})
			}
			return rootWriter.JSON(map[string]any{"status": "ok", "app": appName, "key": key})
		},
	}
	cmd.Flags().BoolVar(&redeploy, "redeploy", false, "trigger a redeploy after setting the var")
	cmd.Flags().IntVar(&timeout, "timeout", 60, "deploy timeout in seconds (used with --redeploy)")
	return cmd
}

func appEnvGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <app> <KEY>",
		Short: "Get the value of an environment variable",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, key := args[0], args[1]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			value, err := mgr.EnvGet(appName, key)
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": appName, "key": key})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(map[string]any{
				"status": "ok", "app": appName, "key": key, "value": value,
			})
		},
	}
}

func appEnvListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <app>",
		Short: "List all environment variable keys for an app (values are not shown)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName := args[0]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			vars, err := mgr.EnvList(appName)
			if err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": appName})
				os.Exit(output.ExitCode(e.Code))
			}
			entries := make([]map[string]any, 0, len(vars))
			for _, v := range vars {
				entries = append(entries, map[string]any{
					"key":        v.Key,
					"updated_at": v.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				})
			}
			return rootWriter.JSON(map[string]any{"status": "ok", "app": appName, "vars": entries})
		},
	}
}

func appEnvUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <app> <KEY>",
		Short: "Remove an environment variable",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			appName, key := args[0], args[1]
			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			if err := mgr.EnvUnset(appName, key); err != nil {
				code := inferAppErrorCode(err)
				e := rootWriter.Error(code, err.Error(), map[string]any{"app": appName, "key": key})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(map[string]any{"status": "ok", "app": appName, "key": key})
		},
	}
}

// newAppManager is a helper that loads config, opens DB, and constructs an app.Manager.
// The caller is responsible for calling db.Close().
func newAppManager() (*app.Manager, *state.DB, error) {
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
	caddyClient := caddy.NewClient(cfg.Caddy.AdminAPI)
	mgr := app.NewManager(db, caddyClient,
		cfg.Ports.AutoAssignRangeStart,
		cfg.Ports.AutoAssignRangeEnd)
	return mgr, db, nil
}

// inferAppErrorCode maps error message prefixes to error codes.
func inferAppErrorCode(err error) output.ErrorCode {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "APP_ALREADY_EXISTS"):
		return output.ErrAppAlreadyExists
	case strings.HasPrefix(msg, "APP_NOT_FOUND"):
		return output.ErrAppNotFound
	case strings.HasPrefix(msg, "APP_TYPE_UNKNOWN"):
		return output.ErrAppTypeUnknown
	case strings.HasPrefix(msg, "GIT_UNREACHABLE"):
		return output.ErrGitUnreachable
	case strings.HasPrefix(msg, "GIT_CLONE_FAILED"):
		return output.ErrGitCloneFailed
	case strings.HasPrefix(msg, "GIT_PULL_FAILED"):
		return output.ErrGitPullFailed
	case strings.HasPrefix(msg, "DOCKER_BUILD_FAILED"):
		return output.ErrDockerBuildFailed
	case strings.HasPrefix(msg, "DOCKER_START_FAILED"):
		return output.ErrDockerStartFailed
	case strings.HasPrefix(msg, "CONTAINER_EXITED"):
		return output.ErrContainerExited
	case strings.HasPrefix(msg, "HEALTH_CHECK_TIMEOUT"):
		return output.ErrHealthCheckTimeout
	case strings.HasPrefix(msg, "PORT_EXHAUSTED"):
		return output.ErrPortExhausted
	case strings.HasPrefix(msg, "CADDY_API_UNREACHABLE"):
		return output.ErrCaddyAPIUnreachable
	case strings.HasPrefix(msg, "CADDY_CONFIG_FAILED"):
		return output.ErrCaddyConfigFailed
	case strings.HasPrefix(msg, "DOCKER_STOP_FAILED"):
		return output.ErrDockerStopFailed
	case strings.HasPrefix(msg, "ENV_KEY_NOT_FOUND"):
		return output.ErrEnvKeyNotFound
	case strings.HasPrefix(msg, "STATE_DB_ERROR"):
		return output.ErrStateDBError
	default:
		return output.ErrInternalError
	}
}

