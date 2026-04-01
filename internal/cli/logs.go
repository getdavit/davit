package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/output"
)

func init() {
	rootCmd.AddCommand(logsCmd())
}

func logsCmd() *cobra.Command {
	var lines int
	var follow bool

	cmd := &cobra.Command{
		Use:   "logs <app>",
		Short: "Stream or tail log output for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			// Logs streams directly to stdout — no JSON envelope.
			if err := mgr.Logs(name, lines, follow, os.Stdout); err != nil {
				e := rootWriter.Error(inferAppErrorCode(err), err.Error(),
					map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&lines, "tail", 100, "number of log lines to show from the end")
	cmd.Flags().BoolVar(&follow, "follow", false, "stream new log lines as they arrive")
	return cmd
}
