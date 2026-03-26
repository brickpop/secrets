package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vars-cli/vars/internal/agent"
	"github.com/vars-cli/vars/internal/envfile"
)

var (
	importForce bool
	importSkip  bool
)

func init() {
	importCmd.Flags().BoolVarP(&importForce, "force", "f", false, "Overwrite conflicting keys without confirmation")
	importCmd.Flags().BoolVar(&importSkip, "skip", false, "Skip conflicting keys without prompting")
	rootCmd.AddCommand(importCmd)
}

var importCmd = &cobra.Command{
	Use:   "import [scope] <file>",
	Short: "Import keys from a .env file",
	Long: `Import key-value pairs from a .env file into the store.

Without a scope, keys are imported into the default scope.
With a scope, keys are prefixed: vars import prod .env → prod/KEY.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if importForce && importSkip {
			return UserError("--force and --skip are mutually exclusive")
		}

		var scope, filePath string
		if len(args) == 2 {
			scope = args[0]
			filePath = args[1]
		} else {
			filePath = args[0]
		}

		f, err := os.Open(filePath)
		if err != nil {
			return UserError(fmt.Sprintf("opening file: %v", err))
		}
		defer f.Close()

		entries, err := envfile.Parse(f)
		if err != nil {
			return UserError(fmt.Sprintf("parsing file: %v", err))
		}
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "No entries found.")
			return nil
		}

		// Apply scope prefix
		if scope != "" {
			for i := range entries {
				entries[i].Key = scope + "/" + entries[i].Key
			}
		}

		if err := ensureAgent(); err != nil {
			return err
		}
		sockPath := agentSocketPath()

		isTTY := term.IsTerminal(int(os.Stdin.Fd()))

		type pendingItem struct {
			key         string
			value       string
			isOverwrite bool
		}
		var pending []pendingItem
		var imported, overwritten, skipped int

	entryLoop:
		for _, e := range entries {
			key := e.Key
			value := e.Value

			for {
				existing, getErr := agent.Get(sockPath, key)

				if getErr != nil {
					// New key
					pending = append(pending, pendingItem{key, value, false})
					imported++
					continue entryLoop
				}

				if existing == value {
					// Same value — idempotent, skip silently
					skipped++
					continue entryLoop
				}

				// Conflict: key exists with a different value
				if importSkip {
					fmt.Fprintf(os.Stderr, "Skipped %s\n", key)
					skipped++
					continue entryLoop
				}

				if importForce {
					pending = append(pending, pendingItem{key, value, true})
					overwritten++
					continue entryLoop
				}

				// Interactive mode
				if !isTTY {
					return UserError("conflicting keys found; use --force or --skip to resolve non-interactively")
				}

				fmt.Fprintf(os.Stderr, "\n%s already exists.\n  current:  %s\n  imported: %s\n", key, existing, value)
				choice, err := stdinPrompter().Line("[o]verwrite  [r]ename  [s]kip > ")
				if err != nil {
					return UserError(err.Error())
				}

				switch c := strings.ToLower(strings.TrimSpace(choice)); {
				case strings.HasPrefix(c, "o"):
					pending = append(pending, pendingItem{key, value, true})
					overwritten++
					continue entryLoop

				case strings.HasPrefix(c, "r"):
					sfx, err := stdinPrompter().Line(fmt.Sprintf("Suffix (saved as %s_<suffix>): ", key))
					if err != nil {
						return UserError(err.Error())
					}
					sfx = strings.TrimSpace(strings.TrimPrefix(sfx, "_"))
					if sfx == "" {
						fmt.Fprintln(os.Stderr, "Suffix cannot be empty, skipping.")
						skipped++
						continue entryLoop
					}
					key = key + "_" + sfx
					// Re-check the renamed key for conflicts

				default: // includes "s" and anything unrecognised
					fmt.Fprintf(os.Stderr, "Skipped %s\n", key)
					skipped++
					continue entryLoop
				}
			}
		}

		if len(pending) > 0 {
			items := make([]agent.SetItem, len(pending))
			for i, p := range pending {
				items[i] = agent.SetItem{Key: p.key, Value: p.value}
			}

			hasOverwrite := false
			for _, p := range pending {
				if p.isOverwrite {
					hasOverwrite = true
					break
				}
			}

			var setErr error
			if hasOverwrite {
				setErr = withPassphrase("Enter passphrase to confirm overwrite: ", func(passphrase string) error {
					return agent.Set(sockPath, items, passphrase)
				})
			} else {
				setErr = agent.Set(sockPath, items, "")
			}
			if setErr != nil {
				return UserError(setErr.Error())
			}
		}

		fmt.Fprintf(os.Stderr, "Imported %d, overwritten %d, skipped %d.\n", imported, overwritten, skipped)
		return nil
	},
}
