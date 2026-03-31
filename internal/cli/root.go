// Package cli wires together the Cobra command tree.
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/config"
	"github.com/getdavit/davit/internal/output"
	"github.com/getdavit/davit/internal/version"
)

// globalFlags holds the values bound to persistent root-level flags.
type globalFlags struct {
	configPath string
	json       bool
	pretty     bool
	quiet      bool
	noColor    bool
}

var gf globalFlags

// rootWriter is the shared output writer, set up in PersistentPreRunE.
var rootWriter *output.Writer

// rootCmd is the top-level cobra command.
var rootCmd = &cobra.Command{
	Use:   "davit",
	Short: "Davit — the crane arm for your containers",
	Long: `Davit is a self-hosted server deployment manager.
It provisions Linux servers, manages Docker applications, handles TLS
via Caddy, and exposes a structured JSON API for AI agent automation.`,
	Version:          version.Version,
	SilenceUsage:     true,
	SilenceErrors:    true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		isTTY := output.IsTTY()
		rootWriter = output.New(os.Stdout, isTTY, gf.pretty, gf.json, gf.quiet)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&gf.configPath, "config", config.DefaultPath,
		"path to global config file")
	rootCmd.PersistentFlags().BoolVar(&gf.json, "json", false,
		"force JSON output (default when stdout is not a TTY)")
	rootCmd.PersistentFlags().BoolVar(&gf.pretty, "pretty", false,
		"force human-readable output (default when stdout is a TTY)")
	rootCmd.PersistentFlags().BoolVar(&gf.quiet, "quiet", false,
		"suppress all output except errors")
	rootCmd.PersistentFlags().BoolVar(&gf.noColor, "no-color", false,
		"disable ANSI colour codes")
}

// Execute runs the root command. Called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// loadConfig reads the config file; falls back to defaults on missing file.
func loadConfig() (*config.Config, error) {
	return config.Load(gf.configPath)
}
