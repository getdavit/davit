package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/output"
)

func init() {
	rootCmd.AddCommand(diagnoseCmd())
}

func diagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose <app>",
		Short: "Show a comprehensive health report for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			mgr, db, err := newAppManager()
			if err != nil {
				return err
			}
			defer db.Close()

			result, err := mgr.Diagnose(name)
			if err != nil {
				e := rootWriter.Error(inferAppErrorCode(err), err.Error(),
					map[string]any{"app": name})
				os.Exit(output.ExitCode(e.Code))
			}
			return rootWriter.JSON(result)
		},
	}
}
