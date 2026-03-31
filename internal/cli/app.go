package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/caddy"
	"github.com/getdavit/davit/internal/output"
)

func init() {
	appCmd := &cobra.Command{
		Use:   "app",
		Short: "Application lifecycle management",
	}
	appCmd.AddCommand(appCreateCmd(), appDeployCmd(), appListCmd())
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

			caddyClient := caddy.NewClient(cfg.Caddy.AdminAPI)
			mgr := app.NewManager(db, caddyClient,
				cfg.Ports.AutoAssignRangeStart,
				cfg.Ports.AutoAssignRangeEnd)

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

			caddyClient := caddy.NewClient(cfg.Caddy.AdminAPI)
			mgr := app.NewManager(db, caddyClient,
				cfg.Ports.AutoAssignRangeStart,
				cfg.Ports.AutoAssignRangeEnd)

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

			caddyClient := caddy.NewClient(cfg.Caddy.AdminAPI)
			mgr := app.NewManager(db, caddyClient,
				cfg.Ports.AutoAssignRangeStart,
				cfg.Ports.AutoAssignRangeEnd)

			apps, err := mgr.List()
			if err != nil {
				e := rootWriter.Error(output.ErrStateDBError, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			return rootWriter.JSON(map[string]any{"apps": apps})
		},
	}
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
	case strings.HasPrefix(msg, "STATE_DB_ERROR"):
		return output.ErrStateDBError
	default:
		return output.ErrInternalError
	}
}

var _ = fmt.Sprintf // keep fmt imported
