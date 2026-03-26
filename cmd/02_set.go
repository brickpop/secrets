package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vars-cli/vars/internal/agent"
)

var (
	setForce bool
	setSkip  bool
)

func init() {
	setCmd.Flags().BoolVarP(&setForce, "force", "f", false, "Overwrite existing key without confirmation")
	setCmd.Flags().BoolVar(&setSkip, "skip", false, "Skip if key already exists")
	rootCmd.AddCommand(setCmd)
}

var setCmd = &cobra.Command{
	Use:   "set <key> [value]",
	Short: "Add or update a key in the store",
	Long: `Write a key-value pair to the store. If value is omitted, prompts
interactively with echo disabled (preferred — inline values appear in
shell history).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if setForce && setSkip {
			return UserError("--force and --skip are mutually exclusive")
		}

		key := args[0]

		if strings.ContainsRune(key, '~') {
			return UserError("key names may not contain '~' (reserved for history entries)")
		}

		var value string
		if len(args) == 2 {
			value = args[1]
		} else {
			v, err := stdinPrompter().Value("Value: ")
			if err != nil {
				return UserError(err.Error())
			}
			value = v
		}

		if err := ensureAgent(); err != nil {
			return err
		}

		sockPath := agentSocketPath()
		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		// Track whether this ends up being an overwrite (existing key with different value)
		isOverwrite := false

		// Conflict resolution loop (handles rename re-checks)
		for {
			existing, getErr := agent.Get(sockPath, key)

			if getErr != nil {
				// New key — no conflict
				break
			}

			if existing == value {
				fmt.Fprintln(os.Stderr, "Already set, nothing to do.")
				return nil
			}

			// Key exists with a different value
			if setSkip {
				fmt.Fprintln(os.Stderr, "Skipped.")
				return nil
			}

			isOverwrite = true

			if setForce {
				break
			}

			if !isTTY {
				return UserError("key already exists; use --force or --skip")
			}

			fmt.Fprintf(os.Stderr, "\n%s already exists. New value will replace it.\n", key)
			choice, err := stdinPrompter().Line("[o]verwrite  [r]ename  [s]kip > ")
			if err != nil {
				return UserError(err.Error())
			}

			switch c := strings.ToLower(strings.TrimSpace(choice)); {
			case strings.HasPrefix(c, "o"):
				// proceed to set below
			case strings.HasPrefix(c, "r"):
				sfx, err := stdinPrompter().Line(fmt.Sprintf("Suffix (saved as %s_<suffix>): ", key))
				if err != nil {
					return UserError(err.Error())
				}
				sfx = strings.TrimSpace(strings.TrimPrefix(sfx, "_"))
				if sfx == "" {
					fmt.Fprintln(os.Stderr, "Suffix cannot be empty, skipping.")
					return nil
				}
				key = key + "_" + sfx
				isOverwrite = false // renamed key may be new — re-check
				continue
			default: // "s" or unrecognised
				fmt.Fprintln(os.Stderr, "Skipped.")
				return nil
			}
			break
		}

		item := []agent.SetItem{{Key: key, Value: value}}
		var setErr error
		if isOverwrite {
			setErr = withPassphrase("Enter passphrase to confirm overwrite: ", func(passphrase string) error {
				return agent.Set(sockPath, item, passphrase)
			})
		} else {
			setErr = agent.Set(sockPath, item, "")
		}
		if setErr != nil {
			return UserError(setErr.Error())
		}

		printManifestHint(key)
		fmt.Fprintln(os.Stderr, "Saved.")
		return nil
	},
}
