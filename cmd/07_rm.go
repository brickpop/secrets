package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
)

func init() {
	rmCmd.Flags().BoolVarP(&rmForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(rmCmd)
}

var rmForce bool

var rmCmd = &cobra.Command{
	Use:   "rm <key> [key...]",
	Short: "Remove one or more keys from the store",
	Long:  `Delete keys from the store. Prompts for confirmation unless --force is used.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		// Verify all keys exist before prompting
		for _, key := range args {
			if _, err := agent.Get(sockPath, key); err != nil {
				return UserError(fmt.Sprintf("key %q not found in store", key))
			}
		}

		if !rmForce {
			if len(args) == 1 {
				histKeys, _, _ := agent.History(sockPath, args[0])
				if len(histKeys) == 0 {
					fmt.Fprintf(os.Stderr, "Removing %s.\n", args[0])
				} else {
					fmt.Fprintf(os.Stderr, "Removing %s and its %s.\n", args[0], backupCount(len(histKeys)))
				}
			} else {
				fmt.Fprintf(os.Stderr, "Removing %d keys:\n", len(args))
				for _, key := range args {
					histKeys, _, _ := agent.History(sockPath, key)
					if len(histKeys) == 0 {
						fmt.Fprintf(os.Stderr, "  %s\n", key)
					} else {
						fmt.Fprintf(os.Stderr, "  %s (+ %s)\n", key, backupCount(len(histKeys)))
					}
				}
			}
		}

		err := withPassphrase("Enter passphrase to confirm: ", func(passphrase string) error {
			return agent.Delete(sockPath, args, passphrase)
		})
		if err != nil {
			return UserError(err.Error())
		}

		if len(args) == 1 {
			fmt.Fprintln(os.Stderr, "Removed.")
		} else {
			fmt.Fprintf(os.Stderr, "Removed %d keys.\n", len(args))
		}
		return nil
	},
}

func backupCount(n int) string {
	if n == 1 {
		return "1 backup"
	}
	return fmt.Sprintf("%d backups", n)
}
