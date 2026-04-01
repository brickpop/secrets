package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
}

// warnIfLocalNotGitignored prints a warning when .vars.local.yaml exists but
// .gitignore is present and doesn't cover it — risk of accidental commit.
func warnIfLocalNotGitignored() {
	if _, err := os.Stat(".vars.local.yaml"); err != nil {
		return // no local file yet, nothing to warn about
	}
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		return // no .gitignore, nothing to check
	}
	if !strings.Contains(string(data), ".vars.local.yaml") {
		fmt.Fprintln(os.Stderr, "warning: .vars.local.yaml exists but is not in .gitignore — add it to avoid committing personal overrides.")
	}
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a .vars.yaml manifest in the current directory",
	Long: `Scaffold a .vars.yaml file for this project.

Commit .vars.yaml to version control. Never commit .vars.local.yaml.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		const path = ".vars.yaml"
		if _, err := os.Stat(path); err == nil {
			return UserError(path + " already exists")
		}

		const scaffold = `# .vars.yaml — declare what env vars this project needs.
# Commit this file. Never commit .vars.local.yaml.

keys:
  - MY_KEY
  # - OTHER_KEY

# profiles:      # optional: named sets of var→storeKey mappings (select with --profile)
#   global:      # always-applied fallback (team-wide aliases)
#     MY_KEY: prod/MY_KEY              # resolve to a store key
#     LOG_LEVEL: = info                # emit this value
#     RPC_URL: ?= http://localhost     # use store value, fall back to this value
#   default:     # auto-applied when no --profile is given
#     MY_KEY: dev/MY_KEY
#   mainnet:
#     MY_KEY: prod/MY_KEY
`

		if err := os.WriteFile(path, []byte(scaffold), 0644); err != nil {
			return InternalError(fmt.Sprintf("writing %s: %v", path, err))
		}

		fmt.Fprintf(os.Stderr, "Created %s\n", path)
		fmt.Fprintf(os.Stderr, "Edit it to list your project's keys, then run `vars resolve`.\n")
		return nil
	},
}
