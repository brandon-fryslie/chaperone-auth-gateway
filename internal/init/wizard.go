package init

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// WizardConfig configures the init wizard behavior.
type WizardConfig struct {
	// Address is the proxy listen address (default: 127.0.0.1)
	Address string
	// Port is the proxy listen port (default: 4010)
	Port int
	// SentinelValue is an optional exact value to search for
	SentinelValue string
	// NonInteractive mode accepts defaults without prompting
	NonInteractive bool
	// DryRun shows config without saving
	DryRun bool
}

// DefaultWizardConfig returns the default wizard configuration.
func DefaultWizardConfig() WizardConfig {
	return WizardConfig{
		Address: "127.0.0.1",
		Port:    4010,
	}
}

// Wizard orchestrates the init wizard flow.
type Wizard struct {
	config   WizardConfig
	reader   *bufio.Reader
	writer   io.Writer
	evidence *Evidence
	detector *Detector
	stopChan chan struct{}
}

// NewWizard creates a new wizard instance.
func NewWizard(config WizardConfig) *Wizard {
	return &Wizard{
		config:   config,
		reader:   bufio.NewReader(os.Stdin),
		writer:   os.Stdout,
		stopChan: make(chan struct{}),
	}
}

// PrintIntroduction displays the welcome message and overview.
func (w *Wizard) PrintIntroduction() {
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "=== Welcome to Chaperone ===")
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "Chaperone is a local proxy that automatically attaches authentication")
	fmt.Fprintln(w.writer, "credentials to your outgoing API requests. Your applications connect")
	fmt.Fprintln(w.writer, "through Chaperone using standard proxy settings, and Chaperone injects")
	fmt.Fprintln(w.writer, "the appropriate API keys before forwarding requests. This means your")
	fmt.Fprintln(w.writer, "applications never have direct access to your API keys—they stay safely")
	fmt.Fprintln(w.writer, "stored and managed by Chaperone.")
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "This wizard will:")
	fmt.Fprintln(w.writer, "  • Detect which APIs your application calls and how they authenticate")
	fmt.Fprintln(w.writer, "  • Let you select and configure each service")
	fmt.Fprintln(w.writer, "  • Securely store your API credentials (Keychain, file, or .env)")
	fmt.Fprintln(w.writer, "  • Generate a chaperone.toml configuration file")
	fmt.Fprintln(w.writer, "")
}

// Step1ConfigureProxy prompts for proxy configuration.
// Returns updated WizardConfig.
func (w *Wizard) Step1ConfigureProxy() (WizardConfig, error) {
	cfg := w.config

	if cfg.NonInteractive {
		return cfg, nil
	}

	fmt.Fprintln(w.writer, "\n=== Step 1: Configure Proxy ===")

	// Address
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "The IP address where the proxy server will listen.")
	fmt.Fprintf(w.writer, "Listen address [%s]: ", cfg.Address)
	line, err := w.reader.ReadString('\n')
	if err != nil {
		return cfg, err
	}
	line = strings.TrimSpace(line)
	if line != "" {
		cfg.Address = line
	}

	// Port
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "The port number for proxy connections.")
	fmt.Fprintf(w.writer, "Listen port [%d]: ", cfg.Port)
	line, err = w.reader.ReadString('\n')
	if err != nil {
		return cfg, err
	}
	line = strings.TrimSpace(line)
	if line != "" {
		port, err := strconv.Atoi(line)
		if err != nil {
			return cfg, fmt.Errorf("invalid port: %w", err)
		}
		cfg.Port = port
	}

	// Sentinel value
	if cfg.SentinelValue == "" {
		fmt.Fprintln(w.writer, "")
		fmt.Fprintln(w.writer, "An exact string to search for in requests (100% confidence detection).")
		fmt.Fprintf(w.writer, "Sentinel value (optional, press Enter to skip): ")
		line, err = w.reader.ReadString('\n')
		if err != nil {
			return cfg, err
		}
		line = strings.TrimSpace(line)
		if line != "" {
			cfg.SentinelValue = line
		}
	} else {
		fmt.Fprintf(w.writer, "Using sentinel value: %s\n", cfg.SentinelValue)
	}

	w.config = cfg
	return cfg, nil
}

// InitializeDetection sets up the evidence store and detector.
func (w *Wizard) InitializeDetection() {
	w.evidence = NewEvidence()
	w.detector = NewDetector(DetectorConfig{
		SentinelValue: w.config.SentinelValue,
	}, w.evidence)
}

// GetDetector returns the detector for use by the proxy.
func (w *Wizard) GetDetector() *Detector {
	return w.detector
}

// GetEvidence returns the evidence store.
func (w *Wizard) GetEvidence() *Evidence {
	return w.evidence
}

// PrintDetectionInstructions prints instructions for detection mode.
func (w *Wizard) PrintDetectionInstructions() {
	fmt.Fprintln(w.writer, "\n=== Step 2: Detection Mode ===")
	fmt.Fprintln(w.writer)
	fmt.Fprintf(w.writer, "Proxy listening on http://%s:%d\n\n", w.config.Address, w.config.Port)
	fmt.Fprintln(w.writer, "Configure your application to use this proxy:")
	fmt.Fprintf(w.writer, "  export HTTP_PROXY=http://%s:%d\n", w.config.Address, w.config.Port)
	fmt.Fprintf(w.writer, "  export HTTPS_PROXY=http://%s:%d\n\n", w.config.Address, w.config.Port)
	fmt.Fprintln(w.writer, "Send requests through the proxy. Detected auth patterns will appear below.")
	fmt.Fprintln(w.writer, "Press Ctrl+C when done to proceed to review.")
	fmt.Fprintln(w.writer)
	fmt.Fprintln(w.writer, "---")
}

// ReportFinding prints a finding in real-time during detection.
func (w *Wizard) ReportFinding(host string, finding *Finding) {
	confidence := fmt.Sprintf("%.0f%%", finding.Confidence*100)
	fmt.Fprintf(w.writer, "[%s] %s - %s: %s (confidence: %s)\n",
		host,
		finding.Heuristic,
		finding.HeaderName,
		redactValue(finding.HeaderValue),
		confidence,
	)
}

// Step3ReviewFindings allows user to review and select detected hosts.
// Returns the selected host or empty string if user quits.
func (w *Wizard) Step3ReviewFindings() (string, error) {
	hosts := w.evidence.GetAllHosts()

	if len(hosts) == 0 {
		fmt.Fprintln(w.writer, "\nNo services detected. Exiting.")
		return "", nil
	}

	fmt.Fprintln(w.writer, "\n=== Step 3: Review Findings ===")
	fmt.Fprintln(w.writer)
	fmt.Fprintln(w.writer, "Detected services:")
	fmt.Fprintln(w.writer)

	// Sort hosts for consistent display
	sort.Strings(hosts)

	for i, host := range hosts {
		findings := w.evidence.GetFindings(host)
		topFinding := w.evidence.GetTopFinding(host)

		methodCount := len(findings.Policy.Methods)
		pathCount := len(findings.Policy.Paths)

		fmt.Fprintf(w.writer, "  %d. %s\n", i+1, host)
		if topFinding != nil {
			fmt.Fprintf(w.writer, "     Auth: %s (%.0f%% confidence, %s)\n",
				topFinding.HeaderName,
				topFinding.Confidence*100,
				topFinding.Heuristic,
			)
		}
		fmt.Fprintf(w.writer, "     Policy: %d methods, %d paths observed\n\n", methodCount, pathCount)
	}

	if w.config.NonInteractive {
		// In non-interactive mode, return the first host with highest confidence auth
		var bestHost string
		var bestConfidence float64
		for _, host := range hosts {
			topFinding := w.evidence.GetTopFinding(host)
			if topFinding != nil && topFinding.Confidence > bestConfidence {
				bestConfidence = topFinding.Confidence
				bestHost = host
			}
		}
		return bestHost, nil
	}

	// Prompt user to select one service
	fmt.Fprintf(w.writer, "Enter the number of the service to configure (or 'q' to quit): ")
	line, err := w.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)

	if line == "q" || line == "Q" {
		return "", nil
	}

	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > len(hosts) {
		return "", fmt.Errorf("invalid selection: %s", line)
	}

	return hosts[idx-1], nil
}

// Step4ConfigureService prompts for service name and credential storage.
// Returns the GeneratedService or nil if user quits.
func (w *Wizard) Step4ConfigureService(ctx context.Context, host string) (*GeneratedService, error) {
	findings := w.evidence.GetFindings(host)
	topFinding := w.evidence.GetTopFinding(host)

	if findings == nil || topFinding == nil {
		return nil, fmt.Errorf("no findings for host: %s", host)
	}

	fmt.Fprintln(w.writer, "\n=== Step 4: Configure Service ===")
	fmt.Fprintln(w.writer)
	fmt.Fprintf(w.writer, "Host: %s\n", host)
	fmt.Fprintf(w.writer, "Detected auth header: %s\n", topFinding.HeaderName)
	fmt.Fprintf(w.writer, "Inferred strategy: %s\n\n", InferAuthStrategy(topFinding.HeaderName))

	var serviceName string
	var credentialRef string

	if w.config.NonInteractive {
		// Generate default service name from host
		serviceName = generateServiceName(host)
		// In non-interactive mode, we can't prompt for credential
		return nil, fmt.Errorf("credential storage requires interactive mode")
	}

	// Prompt for service name
	defaultName := generateServiceName(host)
	fmt.Fprintln(w.writer, "A short identifier for this service in your config.")
	fmt.Fprintf(w.writer, "Service name [%s]: ", defaultName)
	line, err := w.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		serviceName = defaultName
	} else {
		serviceName = line
	}

	// Prompt for credential value
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "The actual API key or token that Chaperone will inject into requests.")
	fmt.Fprintf(w.writer, "Enter the API key/credential value: ")
	line, err = w.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	credentialValue := strings.TrimSpace(line)
	if credentialValue == "" {
		return nil, fmt.Errorf("credential value cannot be empty")
	}

	// Prompt for storage type
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "Where do you want to store this credential?")
	fmt.Fprintln(w.writer, "  1. macOS Keychain - Most secure, uses macOS secure storage (recommended)")
	fmt.Fprintln(w.writer, "  2. File - Stored with restricted permissions (~/.config/chaperone/secrets/)")
	fmt.Fprintln(w.writer, "  3. .env file - Environment file in current directory")
	fmt.Fprintf(w.writer, "\nChoice [1]: ")

	line, err = w.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}

	var storageType CredentialStorageType
	switch line {
	case "1":
		storageType = StorageKeychain
	case "2":
		storageType = StorageFile
	case "3":
		storageType = StorageEnvFile
	default:
		return nil, fmt.Errorf("invalid storage choice: %s", line)
	}

	// Write credential and get reference
	credentialRef, err = WriteCredential(storageType, serviceName, credentialValue)
	if err != nil {
		return nil, fmt.Errorf("failed to store credential: %w", err)
	}

	fmt.Fprintf(w.writer, "\nCredential stored: %s\n", credentialRef)

	// Build generated service
	svc := BuildGeneratedService(serviceName, findings, topFinding, credentialRef)
	return svc, nil
}

// Step5SaveConfig generates and saves the configuration.
func (w *Wizard) Step5SaveConfig(svc *GeneratedService) error {
	fmt.Fprintln(w.writer, "\n=== Step 5: Save Configuration ===")
	fmt.Fprintln(w.writer)

	// Generate TOML
	tomlContent := GenerateTOMLConfig(svc)

	if w.config.DryRun {
		fmt.Fprintln(w.writer, "Generated configuration (dry-run mode):")
		fmt.Fprintln(w.writer)
		fmt.Fprintln(w.writer, tomlContent)
		return nil
	}

	// Get default config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	defaultPath := filepath.Join(homeDir, ".config", "chaperone", "chaperone.toml")

	if w.config.NonInteractive {
		return w.saveConfig(defaultPath, tomlContent)
	}

	// Prompt for save location
	fmt.Fprintln(w.writer, "Where do you want to save the configuration?")
	fmt.Fprintf(w.writer, "  1. Default location (%s)\n", defaultPath)
	fmt.Fprintln(w.writer, "  2. Current directory (./chaperone.toml)")
	fmt.Fprintln(w.writer, "  3. Custom path")
	fmt.Fprintln(w.writer, "  4. Print only (don't save)")
	fmt.Fprintf(w.writer, "\nChoice [1]: ")

	line, err := w.reader.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}

	var savePath string
	switch line {
	case "1":
		savePath = defaultPath
	case "2":
		savePath = "chaperone.toml"
	case "3":
		fmt.Fprintf(w.writer, "Enter path: ")
		line, err = w.reader.ReadString('\n')
		if err != nil {
			return err
		}
		savePath = strings.TrimSpace(line)
	case "4":
		fmt.Fprintln(w.writer, "\nGenerated configuration:")
		fmt.Fprintln(w.writer)
		fmt.Fprintln(w.writer, tomlContent)
		return nil
	default:
		return fmt.Errorf("invalid choice: %s", line)
	}

	return w.saveConfig(savePath, tomlContent)
}

// saveConfig saves or appends the configuration to a file.
func (w *Wizard) saveConfig(path, content string) error {
	// Create parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file exists
	_, err := os.Stat(path)
	fileExists := err == nil

	if fileExists {
		// Append to existing file
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open config file: %w", err)
		}
		defer f.Close()

		// Add newline separator
		if _, err := f.WriteString("\n" + content); err != nil {
			return fmt.Errorf("failed to append to config file: %w", err)
		}

		fmt.Fprintf(w.writer, "\nConfiguration appended to: %s\n", path)
	} else {
		// Create new file with header
		header := `# Chaperone configuration
# Generated by chaperone init

[server]
address = "127.0.0.1"
port = 4010

[logging]
level = "info"

`
		fullContent := header + content

		if err := os.WriteFile(path, []byte(fullContent), 0644); err != nil {
			return fmt.Errorf("failed to write config file: %w", err)
		}

		fmt.Fprintf(w.writer, "\nConfiguration saved to: %s\n", path)
	}

	w.printNextSteps()
	return nil
}

// printNextSteps prints post-wizard instructions.
func (w *Wizard) printNextSteps() {
	fmt.Fprintln(w.writer, "\n=== Next Steps ===")
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "1. Trust the CA certificate in your browser/system:")
	fmt.Fprintln(w.writer, "   chaperone ca-cert")
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "2. Start the proxy:")
	fmt.Fprintln(w.writer, "   chaperone inject")
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "3. Configure your application to use the proxy:")
	fmt.Fprintf(w.writer, "   export HTTP_PROXY=http://%s:%d\n", w.config.Address, w.config.Port)
	fmt.Fprintf(w.writer, "   export HTTPS_PROXY=http://%s:%d\n", w.config.Address, w.config.Port)
	fmt.Fprintln(w.writer, "")
	fmt.Fprintln(w.writer, "Run 'chaperone init' again to add more services.")
}

// generateServiceName creates a default service name from a hostname.
func generateServiceName(host string) string {
	// Remove common prefixes and TLD
	name := host
	name = strings.TrimPrefix(name, "api.")
	name = strings.TrimPrefix(name, "www.")

	// Take the first part before the first dot
	parts := strings.Split(name, ".")
	if len(parts) > 0 {
		name = parts[0]
	}

	// Sanitize for use as service name
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", "_")

	return name
}

// redactValue partially hides a credential value for display.
func redactValue(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
