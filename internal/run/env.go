package run

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// EnvBuilder builds environment variables for a child process.
type EnvBuilder struct {
	env map[string]string
}

// NewEnvBuilder creates a new environment builder.
func NewEnvBuilder() *EnvBuilder {
	return &EnvBuilder{
		env: make(map[string]string),
	}
}

// InheritParent copies the parent process's environment.
func (eb *EnvBuilder) InheritParent() *EnvBuilder {
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			eb.env[parts[0]] = parts[1]
		}
	}
	return eb
}

// LoadEnvFile loads environment variables from a .env file.
// Format: KEY=value (one per line, # for comments)
// Supports quoted values: KEY="value with spaces"
func (eb *EnvBuilder) LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open env file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("line %d: invalid format (expected KEY=value): %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		eb.env[key] = value
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading env file: %w", err)
	}

	return nil
}

// Set sets a single environment variable.
func (eb *EnvBuilder) Set(key, value string) *EnvBuilder {
	eb.env[key] = value
	return eb
}

// SetProxyVars sets the proxy-related environment variables.
// Adds: HTTP_PROXY, HTTPS_PROXY, CHAPERONE_SERVICE
func (eb *EnvBuilder) SetProxyVars(proxyAddress string, serviceName string) *EnvBuilder {
	eb.env["HTTP_PROXY"] = proxyAddress
	eb.env["HTTPS_PROXY"] = proxyAddress
	eb.env["CHAPERONE_SERVICE"] = serviceName
	return eb
}

// SetCAEnvVars sets CA certificate environment variables.
// If caEnvVars is empty, sets all standard CA environment variables.
// Always sets CHAPERONE_CA_CERT regardless of caEnvVars.
func (eb *EnvBuilder) SetCAEnvVars(caCertPath string, caEnvVars []string) *EnvBuilder {
	if len(caEnvVars) == 0 {
		// Default: set all standard CA environment variables
		caEnvVars = []string{
			"SSL_CERT_FILE",
			"NODE_EXTRA_CA_CERTS",
			"REQUESTS_CA_BUNDLE",
			"CURL_CA_BUNDLE",
		}
	}

	for _, envVar := range caEnvVars {
		eb.env[envVar] = caCertPath
	}

	// Always set CHAPERONE_CA_CERT
	eb.env["CHAPERONE_CA_CERT"] = caCertPath

	return eb
}

// Build returns the environment as a slice of "KEY=value" strings.
func (eb *EnvBuilder) Build() []string {
	result := make([]string, 0, len(eb.env))
	for k, v := range eb.env {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}
