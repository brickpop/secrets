package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brickpop/secrets/internal/agent"
	"github.com/brickpop/secrets/internal/prompt"
	"github.com/brickpop/secrets/internal/store"
)

const defaultAgentTTL = 8 * time.Hour

// stdinPrompt is a lazily-initialized Prompter backed by os.Stdin.
// All code must use this instead of prompt.New(os.Stdin, ...) to avoid
// creating multiple bufio.Readers over the same stdin.
var stdinPrompt *prompt.Prompter

func stdinPrompter() *prompt.Prompter {
	if stdinPrompt == nil {
		stdinPrompt = prompt.New(os.Stdin, os.Stderr)
	}
	return stdinPrompt
}

// ensureAgent ensures a running agent, auto-starting one if needed.
// If no agent is running, it prompts for passphrase if required and starts the daemon.
func ensureAgent() error {
	if agent.IsRunning(agentSocketPath()) {
		return nil
	}
	_, err := startAgent(defaultAgentTTL)
	return err
}

// withPassphrase runs fn with the trial-passphrase approach.
// First tries empty passphrase. If agent returns "passphrase required",
// prompts the user and retries once.
func withPassphrase(fn func(passphrase string) error) error {
	err := fn("")
	if err == nil {
		return nil
	}

	if !strings.Contains(err.Error(), agent.ErrPassphraseRequired) {
		return err
	}

	// Passphrase required — prompt and retry
	pass, promptErr := stdinPrompter().Passphrase("Passphrase: ")
	if promptErr != nil {
		return UserError(promptErr.Error())
	}

	return fn(pass)
}

// agentSocketPath returns the agent socket path.
func agentSocketPath() string {
	if sock := os.Getenv("SECRETS_AGENT_SOCK"); sock != "" {
		return sock
	}
	return store.Dir() + "/agent.sock"
}

// printManifestHint prints a hint if .secrets.yaml exists in cwd
// and the key is not listed in it.
func printManifestHint(key string) {
	data, err := os.ReadFile(".secrets.yaml")
	if err != nil {
		return
	}
	content := string(data)
	if !containsKey(content, key) {
		fmt.Fprintf(os.Stderr, "Hint: %q is not listed in .secrets.yaml. Consider adding it.\n", key)
	}
}

// containsKey checks if a key appears as a YAML list item (- KEY).
func containsKey(yamlContent string, key string) bool {
	needle := "- " + key
	idx := strings.Index(yamlContent, needle)
	if idx < 0 {
		return false
	}
	// Ensure it's at end-of-string or followed by a newline (not a prefix of another key).
	end := idx + len(needle)
	return end == len(yamlContent) || yamlContent[end] == '\n' || yamlContent[end] == '\r'
}
