// Package manifest parses .vars.yaml and .vars.local.yaml
// and resolves variable names to store keys.
package manifest

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest represents a parsed .vars.yaml file (committed, team-owned).
type Manifest struct {
	Keys     []string                     `yaml:"keys"`
	Profiles map[string]map[string]string `yaml:"profiles"`
}

// LocalManifest represents a parsed .vars.local.yaml file (personal, git-ignored).
type LocalManifest struct {
	Profiles map[string]map[string]string `yaml:"profiles"`
}

// ResolvedVar is a variable name mapped to its store key, or an inline/default value.
type ResolvedVar struct {
	EnvName      string // the env var name to export
	StoreKey     string // the key to look up in the store (empty when IsInline)
	InlineValue  string // literal value when IsInline is true
	IsInline     bool   // true when value is a literal (= syntax)
	DefaultValue string // fallback value when HasDefault is true
	HasDefault   bool   // true when ?= syntax: use store if present, else DefaultValue
}

// Load parses a .vars.yaml file.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("manifest not found: %s", path)
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	m.Keys = dedup(m.Keys)
	return &m, nil
}

// LoadLocal parses a .vars.local.yaml file (personal overrides, git-ignored).
// Returns an empty LocalManifest if the file does not exist.
func LoadLocal(path string) (*LocalManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &LocalManifest{}, nil
		}
		return nil, fmt.Errorf("reading local manifest: %w", err)
	}

	var m LocalManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing local manifest: %w", err)
	}
	return &m, nil
}

// Resolve maps each manifest key to a store key (or inline/default value).
// localPath is the path to .vars.local.yaml (may not exist).
// profile is the active profile name (empty = auto-detect "default" if present).
//
// Reserved profile names:
//   - "default"  auto-applied when no profile is specified
//   - "global"   always-applied fallback layer (cannot be selected via --profile)
//
// Returns profileFound=false when an explicit profile was requested but does not
// exist in either manifest. Resolution still continues using global and identity.
//
// Resolution priority for each key:
//  1. Active profile, local file
//  2. Active profile, committed manifest
//  3. global profile, local file
//  4. global profile, committed manifest
//  5. Bare key (identity)
func Resolve(manifestPath, localPath, profile string) (vars []ResolvedVar, profileFound bool, err error) {
	if profile == "global" {
		return nil, false, fmt.Errorf("\"global\" is a reserved profile name and cannot be selected with --profile")
	}

	m, loadErr := Load(manifestPath)
	if loadErr != nil {
		return nil, false, loadErr
	}

	local, loadErr := LoadLocal(localPath)
	if loadErr != nil {
		return nil, false, loadErr
	}

	explicit := profile != ""

	// Auto-apply "default" profile when no profile is specified and one exists.
	if !explicit {
		if _, ok := m.Profiles["default"]; ok {
			profile = "default"
		} else if _, ok := local.Profiles["default"]; ok {
			profile = "default"
		}
	}

	// An explicitly-requested profile is "found" only if it exists in either manifest.
	profileFound = !explicit ||
		(m.Profiles != nil && m.Profiles[profile] != nil) ||
		(local.Profiles != nil && local.Profiles[profile] != nil)

	vars = make([]ResolvedVar, 0, len(m.Keys))
	for _, key := range m.Keys {
		resolved := resolveKey(key, profile, m, local)
		switch {
		case strings.HasPrefix(resolved, "?="):
			vars = append(vars, ResolvedVar{
				EnvName:      key,
				StoreKey:     key,
				DefaultValue: parseInlineValue(resolved[2:]),
				HasDefault:   true,
			})
		case strings.HasPrefix(resolved, "="):
			vars = append(vars, ResolvedVar{
				EnvName:     key,
				InlineValue: parseInlineValue(resolved[1:]),
				IsInline:    true,
			})
		default:
			vars = append(vars, ResolvedVar{EnvName: key, StoreKey: resolved})
		}
	}
	return vars, profileFound, nil
}

// parseInlineValue extracts the value from a = or ?= entry.
// Trims leading whitespace, then strips one surrounding matched quote pair
// to mirror how YAML itself handles quoted scalars.
func parseInlineValue(s string) string {
	v := strings.TrimLeft(s, " \t")
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
	}
	return v
}

// resolveKey returns the store key (or inline sigil) for a given env var name.
// Priority: active profile (local → committed) → global profile (local → committed) → identity.
func resolveKey(key, profile string, m *Manifest, local *LocalManifest) string {
	// Active profile
	if profile != "" && profile != "global" {
		if local.Profiles != nil {
			if v, ok := local.Profiles[profile][key]; ok {
				return v
			}
		}
		if m.Profiles != nil {
			if v, ok := m.Profiles[profile][key]; ok {
				return v
			}
		}
	}
	// Global profile (always-applied fallback)
	if local.Profiles != nil {
		if v, ok := local.Profiles["global"][key]; ok {
			return v
		}
	}
	if m.Profiles != nil {
		if v, ok := m.Profiles["global"][key]; ok {
			return v
		}
	}
	return key
}

// dedup removes duplicate strings while preserving order.
func dedup(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
