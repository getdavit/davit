package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/getdavit/davit/internal/agent"
	"github.com/getdavit/davit/internal/output"
	"github.com/getdavit/davit/internal/state"
)

func init() {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Agent SSH key management",
	}
	keyCmd := &cobra.Command{
		Use:   "key",
		Short: "SSH key operations",
	}
	keyCmd.AddCommand(agentKeyCreateCmd())
	agentCmd.AddCommand(keyCmd)
	rootCmd.AddCommand(agentCmd)
}

func agentKeyCreateCmd() *cobra.Command {
	var label, outputDir string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Generate an Ed25519 SSH keypair for agent access",
		RunE: func(cmd *cobra.Command, args []string) error {
			if label == "" {
				label = "agent"
			}
			if outputDir == "" {
				outputDir = "."
			}

			kp, err := agent.GenerateEd25519(label)
			if err != nil {
				e := rootWriter.Error(output.ErrInternalError,
					"generate keypair: "+err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			// Write private key
			privPath := filepath.Join(outputDir, "davit-agent.pem")
			if err := os.WriteFile(privPath, []byte(kp.PrivateKeyPEM), 0600); err != nil {
				e := rootWriter.Error(output.ErrInternalError,
					"write private key: "+err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			// Write public key
			pubPath := filepath.Join(outputDir, "davit-agent.pub")
			if err := os.WriteFile(pubPath, []byte(kp.PublicKeySSH+"\n"), 0644); err != nil {
				e := rootWriter.Error(output.ErrInternalError,
					"write public key: "+err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			// Install into authorized_keys
			authorizedKeysPath := "/root/.ssh/authorized_keys"
			if err := os.MkdirAll(filepath.Dir(authorizedKeysPath), 0700); err != nil {
				e := rootWriter.Error(output.ErrInternalError,
					"create .ssh dir: "+err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}
			if err := agent.InstallPublicKey(authorizedKeysPath, kp.AuthorizedKeyEntry); err != nil {
				e := rootWriter.Error(output.ErrInternalError,
					"install public key: "+err.Error(), nil)
				os.Exit(output.ExitCode(e.Code))
			}

			// Persist to state store
			db, err := openDB()
			if err == nil {
				defer db.Close()
				_ = db.InsertAgentKey(state.AgentKey{
					Fingerprint: kp.Fingerprint,
					Label:       label,
					PublicKey:   kp.PublicKeySSH,
				})
			}

			return rootWriter.JSON(map[string]any{
				"status":                "ok",
				"label":                 label,
				"public_key":            kp.PublicKeySSH,
				"fingerprint":           kp.Fingerprint,
				"authorized_keys_entry": kp.AuthorizedKeyEntry,
				"private_key_path":      privPath,
				"public_key_path":       pubPath,
			})
		},
	}

	cmd.Flags().StringVar(&label, "label", "agent", "human-readable label for this key")
	cmd.Flags().StringVar(&outputDir, "output", ".", "directory to write key files")
	return cmd
}
