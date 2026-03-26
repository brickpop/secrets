package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
)

func init() {
	rootCmd.AddCommand(historyCmd)
}

var historyCmd = &cobra.Command{
	Use:   "history <key>",
	Short: "Show value history for a key (newest first)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()

		if _, err := agent.Get(sockPath, args[0]); err != nil {
			return UserError(fmt.Sprintf("key %q not found in store", args[0]))
		}

		keys, values, err := agent.History(sockPath, args[0])
		if err != nil {
			return InternalError(err.Error())
		}

		for i, k := range keys {
			fmt.Fprintf(os.Stdout, "%s:\t%s\n", k, values[i])
		}
		return nil
	},
}
