package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/caddy"
	"github.com/getdavit/davit/internal/output"
	"github.com/getdavit/davit/internal/tui"
)

func init() {
	rootCmd.AddCommand(tuiCmd())
}

func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive TUI",
		Long: `Launch the interactive terminal user interface.

The TUI provides a keyboard-navigable dashboard for managing
applications, viewing server health, and running common operations.

If the server has not been provisioned yet, the setup wizard
will automatically guide you through the process.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				e := rootWriter.Error(output.ErrConfigInvalid, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			db, err := openDB()
			if err != nil {
				// If DB can't be opened, we can still launch TUI in setup mode
				db = nil
			}

			var mgr *app.Manager
			if db != nil {
				caddyClient := caddy.NewClient(cfg.Caddy.AdminAPI)
				mgr = app.NewManager(db, caddyClient,
					cfg.Ports.AutoAssignRangeStart,
					cfg.Ports.AutoAssignRangeEnd)
			}

			model := tui.NewModel(db, mgr, cfg.Caddy.AdminAPI)
			if err := tui.Run(model); err != nil {
				e := rootWriter.Error(output.ErrInternalError, err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			return nil
		},
	}
}