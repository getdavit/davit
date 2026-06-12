package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
				
			// Get app manager and database
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
				
			var action string
			if webhook {
				action = "webhook"
			} else {
				action = fmt.Sprintf("polling (every %ds)", pollInterval)
			}
				
			return rootWriter.JSON(map[string]any{
				"status":  "success",
				"message": fmt.Sprintf("Watching enabled for app %s (%s)", name, action),
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
			// TODO: Implement watch disable logic
			return rootWriter.JSON(map[string]any{
				"status":  "not_implemented",
				"message": "Watch disable not yet implemented for app: " + name,
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
			if len(args) == 1 {
				// Status for specific app
				name := args[0]
				// TODO: Implement watch status for specific app
				return rootWriter.JSON(map[string]any{
					"status":  "not_implemented",
					"message": "Watch status not yet implemented for app: " + name,
				})
			}
			// Status for all apps
			// TODO: Implement watch status for all apps
			return rootWriter.JSON(map[string]any{
				"status":  "not_implemented",
				"message": "Watch status not yet implemented",
			})
		},
	}
	return cmd
}

// TODO: Implement daemon command and actual watching logic