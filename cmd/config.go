package cmd

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type varsConfig struct {
	AgentTTL string `yaml:"agent_ttl"`
}

// loadConfig reads ~/.config/vars/config.yaml (XDG_CONFIG_HOME/vars/config.yaml).
// Missing file or unknown fields are silently ignored — all fields are optional.
func loadConfig() varsConfig {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	data, err := os.ReadFile(filepath.Join(dir, "vars", "config.yaml"))
	if err != nil {
		return varsConfig{}
	}
	var cfg varsConfig
	yaml.Unmarshal(data, &cfg) //nolint:errcheck // malformed config → zero value → defaults
	return cfg
}

// defaultTTL returns the agent TTL from config, falling back to 8 hours.
func defaultTTL() int64 {
	cfg := loadConfig()
	if cfg.AgentTTL == "" {
		return 8 * 60 * 60
	}
	ttl, err := parseTTLSeconds(cfg.AgentTTL)
	if err != nil {
		return 8 * 60 * 60
	}
	return ttl
}
