package run

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/log"
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
		// Default: set all standard CA environment variables for comprehensive coverage
		caEnvVars = []string{
			// OpenSSL / LibSSL (used by many tools)
			"SSL_CERT_FILE",

			// cURL / libcurl
			"CURL_CA_BUNDLE",

			// Node.js
			"NODE_EXTRA_CA_CERTS",

			// Python (requests, httpx, others)
			"REQUESTS_CA_BUNDLE",
			"HTTPX_CA_BUNDLE",

			// Git
			"GIT_SSL_CAINFO",

			// Perl (LWP, HTTPS)
			"PERL_LWP_SSL_CA_FILE",
			"HTTPS_CA_FILE",

			// AWS CLI
			"AWS_CA_BUNDLE",

			// Homebrew
			"HOMEBREW_CERTIFICATE_AUTHORITY",
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

// PrepareRunConfig validates and prepares the RunConfig for execution.
// Handles CLI command override and variable expansion.
func PrepareRunConfig(cfg *config.Config, serviceName string, cliCommand []string) (*config.ServiceConfig, error) {
	svc, ok := cfg.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %q not found in config", serviceName)
	}

	// Handle CLI command override or require config RunConfig
	if len(cliCommand) > 0 {
		// CLI command provided: create RunConfig if needed
		if svc.Run == nil {
			svc.Run = &config.RunConfig{}
		}
		// Override with CLI command
		svc.Run.Command = cliCommand[0]
		svc.Run.Args = cliCommand[1:]
	} else {
		// No CLI command: require RunConfig from config file
		if svc.Run == nil {
			return nil, fmt.Errorf("service %q does not have a [services.%s.run] section and no command provided via CLI", serviceName, serviceName)
		}
	}

	// Expand variables in RunConfig
	if err := config.ExpandRunConfig(svc.Run); err != nil {
		return nil, fmt.Errorf("failed to expand variables in run config: %w", err)
	}

	return &svc, nil
}

// BuildChildEnvironment creates the environment for the child process.
// Sets proxy vars, loads env file, and sets CA environment variables.
func BuildChildEnvironment(ctx context.Context, svc *config.ServiceConfig, serviceName, proxyAddress, caCertPath string) ([]string, error) {
	envBuilder := NewEnvBuilder()
	envBuilder.InheritParent()

	// Load env_file if specified
	if svc.Run.EnvFile != "" {
		log.Info(ctx, "loading env file", "path", svc.Run.EnvFile)
		if err := envBuilder.LoadEnvFile(svc.Run.EnvFile); err != nil {
			return nil, fmt.Errorf("failed to load env file: %w", err)
		}
	}

	// Set proxy environment variables
	envBuilder.SetProxyVars(proxyAddress, serviceName)

	// Set CA environment variables
	log.Info(ctx, "setting CA environment variables",
		"ca_cert", caCertPath,
		"ca_env_vars", svc.Run.CAEnvVars,
	)
	envBuilder.SetCAEnvVars(caCertPath, svc.Run.CAEnvVars)

	return envBuilder.Build(), nil
}

// CreateTempLogFile creates a temporary log file for run mode logging.
// Returns the open file and its path. The caller is responsible for closing the file.
func CreateTempLogFile() (*os.File, string, error) {
	f, err := os.CreateTemp("", "chaperone-run-*.log")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temporary log file: %w", err)
	}
	return f, f.Name(), nil
}

// SetupLoggingToFile sets up logging to a specific file.
func SetupLoggingToFile(cfg *config.Config, logFormat string, writer *os.File) {
	log.Setup(log.Config{
		Level:  cfg.Logging.Level,
		Format: log.Format(logFormat),
		Output: writer,
	})
}
