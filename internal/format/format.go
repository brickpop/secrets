// Package format produces shell-safe output lines for different shell formats.
//
// Each format function takes a key and value and returns a single line
// that is safe to eval/source in the target shell. Values are properly
// escaped for the target format.
package format

import (
	"fmt"
	"strings"
)

// Posix returns an export line for bash/zsh: export KEY='value'
// Uses single-quote wrapping. Embedded single quotes are escaped as '\''
// (end single quote, escaped literal single quote, restart single quote).
func Posix(key string, value string) string {
	escaped := strings.ReplaceAll(value, "'", "'\\''")
	return fmt.Sprintf("export %s='%s'", key, escaped)
}

// Fish returns a set line for fish: set -x KEY 'value'
// Fish single quotes allow no escapes except \\ and \'.
func Fish(key string, value string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")
	return fmt.Sprintf("set -x %s '%s'", key, escaped)
}

// Dotenv returns a bare dotenv line: KEY=value
// No quoting or escaping — compatible with docker --env-file and similar tools
// that read KEY=value literally.
func Dotenv(key string, value string) string {
	return key + "=" + value
}

// FormatFunc is the type for a format function.
type FormatFunc func(key, value string) string
