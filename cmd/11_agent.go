package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/vars-cli/vars/internal/agent"
	agebackend "github.com/vars-cli/vars/internal/crypto/age"
	"github.com/vars-cli/vars/internal/store"
)

var agentTTL string

func init() {
	agentCmd.Flags().StringVar(&agentTTL, "ttl", "8h", "Agent lifetime (e.g. 30m, 5h, 10d, 0 for unlimited)")
	agentCmd.AddCommand(agentStopCmd)
	rootCmd.AddCommand(agentCmd)
}

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the background agent",
	Long: `Start a background agent that holds the decrypted store in memory.
Most commands auto-start the agent transparently. Use this command
to set an explicit TTL, or to update the TTL of a running agent.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sockPath := agentSocketPath()

		if agent.IsRunning(sockPath) {
			if !cmd.Flags().Changed("ttl") {
				fmt.Fprintln(os.Stderr, "vars: agent already running")
				return nil
			}
			ttl, err := parseTTLSeconds(agentTTL)
			if err != nil {
				return UserError(fmt.Sprintf("invalid TTL: %v", err))
			}
			if err := agent.SetAgentTTL(sockPath, ttl); err != nil {
				return InternalError(fmt.Sprintf("updating agent TTL: %v", err))
			}
			fmt.Fprintln(os.Stderr, "vars: agent TTL updated")
			return nil
		}

		// Internal daemon mode (re-exec'd child)
		if os.Getenv("_VARS_AGENT_DAEMON") == "1" {
			return runDaemon(sockPath)
		}

		ttl, err := parseTTLSeconds(agentTTL)
		if err != nil {
			return UserError(fmt.Sprintf("invalid TTL: %v", err))
		}

		if _, err := startAgent(ttl); err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "vars: agent started")
		return nil
	},
}

var agentStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running agent",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sockPath := agentSocketPath()
		if !agent.IsRunning(sockPath) {
			fmt.Fprintln(os.Stderr, "vars: no agent running")
			return nil
		}

		if err := agent.Stop(sockPath); err != nil {
			return InternalError(fmt.Sprintf("stopping agent: %v", err))
		}

		fmt.Fprintln(os.Stderr, "vars: agent stopped")
		return nil
	},
}

// startAgent validates the passphrase (if any), then spawns the daemon.
// The daemon receives only the passphrase via stdin and decrypts the store independently —
// decrypted data never crosses the process boundary.
// Used by both `vars agent` and ensureAgent().
func startAgent(ttl int64) (string, error) {
	if !store.Exists() {
		passphrase, err := createStore()
		if err != nil {
			return "", err
		}
		return launchDaemon(passphrase, ttl)
	}

	ciphertext, err := os.ReadFile(store.FilePath())
	if err != nil {
		return "", InternalError(fmt.Sprintf("reading store: %v", err))
	}

	// Print permission warnings
	for _, w := range store.CheckPermissions() {
		fmt.Fprintf(os.Stderr, "vars: %s\n", w)
	}

	// Validate passphrase: trial empty first, then prompt.
	// Plaintext is zeroed immediately — the daemon decrypts the store independently.
	var passphrase string
	if _, ok := agebackend.TrialDecryptEmpty(ciphertext); !ok {
		pass, err := stdinPrompter().Passphrase("Passphrase: ")
		if err != nil {
			return "", UserError(err.Error())
		}
		plaintext, err := agebackend.New(pass).Decrypt(ciphertext)
		if err != nil {
			return "", UserError("incorrect passphrase")
		}
		for i := range plaintext {
			plaintext[i] = 0
		}
		passphrase = pass
	}

	return launchDaemon(passphrase, ttl)
}

// launchDaemon re-execs the binary as a background daemon, passing only the passphrase
// via stdin. The daemon reads the store file from disk and decrypts it independently.
func launchDaemon(passphrase string, ttl int64) (string, error) {
	sockPath := agentSocketPath()

	self, err := os.Executable()
	if err != nil {
		return "", InternalError(fmt.Sprintf("finding executable: %v", err))
	}

	// Re-exec as daemon
	daemonCmd := exec.Command(self, "agent", "--ttl", strconv.FormatInt(ttl, 10))
	daemonCmd.Env = append(os.Environ(), "_VARS_AGENT_DAEMON=1")
	daemonCmd.Stdin = strings.NewReader(passphrase)
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil

	if err := daemonCmd.Start(); err != nil {
		return "", InternalError(fmt.Sprintf("starting daemon: %v", err))
	}

	// Wait for socket
	for i := 0; i < 100; i++ {
		if agent.IsRunning(sockPath) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	daemonCmd.Process.Release()
	return sockPath, nil
}

// runDaemon is called by the re-exec'd child process.
// Reads the passphrase from stdin, decrypts the store itself, and serves.
func runDaemon(sockPath string) error {
	passphraseBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		return InternalError("daemon: reading passphrase")
	}
	passphrase := string(passphraseBytes)
	for i := range passphraseBytes {
		passphraseBytes[i] = 0
	}

	ciphertext, err := os.ReadFile(store.FilePath())
	if err != nil {
		return InternalError(fmt.Sprintf("daemon: reading store: %v", err))
	}

	backend := agebackend.New(passphrase)
	plaintext, err := backend.Decrypt(ciphertext)
	if err != nil {
		return InternalError("daemon: decrypting store")
	}

	var data map[string]string
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return InternalError("daemon: corrupt store data")
	}
	// Zero plaintext after parse
	for i := range plaintext {
		plaintext[i] = 0
	}

	ttl, err := parseTTLSeconds(agentTTL)
	if err != nil {
		ttl = defaultTTL()
	}

	srv := agent.NewServer(data, sockPath, passphrase, backend, agebackend.NewBackend, store.Dir())
	return srv.Start(time.Duration(ttl) * time.Second)
}

// parseTTLSeconds parses a TTL string into seconds.
// Accepts: plain integer (seconds), or suffixed values: s, m, h (via time.ParseDuration), d (days).
// 0 means infinite. Negative values are not allowed (use agent stop to stop).
func parseTTLSeconds(s string) (int64, error) {
	if s == "0" {
		return 0, nil
	}
	// Days suffix (not supported by time.ParseDuration)
	if strings.HasSuffix(s, "d") {
		n, err := strconv.ParseInt(strings.TrimSuffix(s, "d"), 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid TTL %q", s)
		}
		return n * 86400, nil
	}
	// Plain integer = seconds
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("TTL must be >= 0")
		}
		return n, nil
	}
	// Standard duration (s, m, h)
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid TTL %q", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("TTL must be >= 0")
	}
	return int64(d / time.Second), nil
}
