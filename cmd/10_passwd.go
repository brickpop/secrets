package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
)

func init() {
	rootCmd.AddCommand(passwdCmd)
}

var passwdCmd = &cobra.Command{
	Use:   "passwd",
	Short: "Change the store passphrase",
	Long: `Re-encrypt the store with a new passphrase. An empty passphrase
is allowed. The agent updates its internal state — no restart needed.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		// Prompt for current passphrase first (leave empty if none was set)
		oldPass, err := stdinPrompter().Passphrase("Current passphrase (leave empty if none): ")
		if err != nil {
			return UserError(err.Error())
		}

		// Then prompt for new passphrase
		newPass, err := stdinPrompter().PassphraseConfirm(
			"New passphrase (leave empty for no passphrase): ",
			"Confirm new passphrase: ",
		)
		if err != nil {
			return UserError(err.Error())
		}

		if err := agent.Passwd(sockPath, oldPass, newPass); err != nil {
			if strings.Contains(err.Error(), agent.ErrPassphraseRequired) {
				return UserError("incorrect passphrase")
			}
			return UserError(err.Error())
		}

		fmt.Fprintln(os.Stderr, "Passphrase updated.")
		return nil
	},
}
