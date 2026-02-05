package config

import (
	"fmt"
	"os"
	"strings"
)

// expandVars expands environment variables in a string.
// Supports $VAR, ${VAR}, and \$VAR (literal) formats.
// Returns error if a referenced variable is undefined.
func expandVars(s string) (string, error) {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '$' {
			// Escaped dollar sign: \$VAR → $VAR (literal)
			result.WriteByte('$')
			i += 2
			continue
		}

		if s[i] == '$' {
			// Start of variable reference
			i++ // Skip $
			if i >= len(s) {
				return "", fmt.Errorf("incomplete variable reference at end of string")
			}

			var varName string
			if s[i] == '{' {
				// ${VAR} format
				i++ // Skip {
				start := i
				for i < len(s) && s[i] != '}' {
					i++
				}
				if i >= len(s) {
					return "", fmt.Errorf("unclosed variable reference ${%s", s[start:])
				}
				varName = s[start:i]
				i++ // Skip }
			} else {
				// $VAR format (alphanumeric + underscore)
				start := i
				for i < len(s) && (isAlphaNumericUnderscore(s[i])) {
					i++
				}
				varName = s[start:i]
			}

			if varName == "" {
				return "", fmt.Errorf("empty variable name")
			}

			// Look up variable
			value, exists := os.LookupEnv(varName)
			if !exists {
				return "", fmt.Errorf("undefined environment variable: %s", varName)
			}
			result.WriteString(value)
		} else {
			result.WriteByte(s[i])
			i++
		}
	}

	return result.String(), nil
}

// isAlphaNumericUnderscore checks if a byte is alphanumeric or underscore.
func isAlphaNumericUnderscore(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// ExpandRunConfig expands all variables in a RunConfig.
// Exported for use by cmd package.
func ExpandRunConfig(rc *RunConfig) error {
	if rc == nil {
		return nil
	}

	var err error

	// Expand command
	if rc.Command, err = expandVars(rc.Command); err != nil {
		return fmt.Errorf("command: %w", err)
	}

	// Expand args
	for i := range rc.Args {
		if rc.Args[i], err = expandVars(rc.Args[i]); err != nil {
			return fmt.Errorf("args[%d]: %w", i, err)
		}
	}

	// Expand env_file
	if rc.EnvFile != "" {
		if rc.EnvFile, err = expandVars(rc.EnvFile); err != nil {
			return fmt.Errorf("env_file: %w", err)
		}
	}

	// Expand stdout
	if rc.Stdout != "" {
		if rc.Stdout, err = expandVars(rc.Stdout); err != nil {
			return fmt.Errorf("stdout: %w", err)
		}
	}

	// Expand stderr
	if rc.Stderr != "" {
		if rc.Stderr, err = expandVars(rc.Stderr); err != nil {
			return fmt.Errorf("stderr: %w", err)
		}
	}

	return nil
}
