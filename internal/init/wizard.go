package init

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
	printIntroduction(w.writer)
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
	address, err := promptAddress(w.reader, w.writer, cfg.Address)
	if err != nil {
		return cfg, err
	}
	cfg.Address = address

	// Port
	port, err := promptPort(w.reader, w.writer, cfg.Port)
	if err != nil {
		return cfg, err
	}
	cfg.Port = port

	// Sentinel value
	if cfg.SentinelValue == "" {
		sentinel, err := promptSentinel(w.reader, w.writer)
		if err != nil {
			return cfg, err
		}
		cfg.SentinelValue = sentinel
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
	printDetectionInstructions(w.writer, w.config.Address, w.config.Port)
}

// ReportFinding prints a finding in real-time during detection.
func (w *Wizard) ReportFinding(host string, finding *Finding) {
	reportFinding(w.writer, host, finding)
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
	idx, err := promptServiceSelection(w.reader, w.writer, len(hosts))
	if err != nil {
		return "", err
	}
	if idx == -1 {
		return "", nil // User quit
	}

	return hosts[idx], nil
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

	// Generate default service name
	defaultName := generateServiceName(host)

	if w.config.NonInteractive {
		// Use default service name
		serviceName = defaultName

		// P1: Default to file storage in non-interactive mode
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}

		// Sanitize host for use as filename
		sanitizedHost := strings.ReplaceAll(host, ".", "_")
		storagePath := filepath.Join(homeDir, ".config", "chaperone", "secrets", sanitizedHost)

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(storagePath), 0700); err != nil {
			return nil, fmt.Errorf("failed to create secrets directory: %w", err)
		}

		// In non-interactive mode, we can't prompt for credential value
		// The credential must be provided via environment or config
		// For now, create a placeholder that user can fill in later
		credentialRef = fmt.Sprintf("file:%s", storagePath)

		fmt.Fprintf(w.writer, "\nNon-interactive mode: credential reference set to %s\n", credentialRef)
		fmt.Fprintf(w.writer, "You must manually write your credential to this file before running Chaperone.\n")
		fmt.Fprintf(w.writer, "Example: echo 'your-api-key' > %s\n", storagePath)
	} else {
		// Interactive mode: prompt for all details

		// Prompt for service name
		name, err := promptServiceName(w.reader, w.writer, defaultName)
		if err != nil {
			return nil, err
		}
		serviceName = name

		// Prompt for credential value
		credentialValue, err := promptCredentialValue(w.reader, w.writer)
		if err != nil {
			return nil, err
		}

		// Prompt for storage type
		storageType, err := promptStorageType(w.reader, w.writer)
		if err != nil {
			return nil, err
		}

		// Write credential and get reference
		credentialRef, err = WriteCredential(storageType, serviceName, credentialValue)
		if err != nil {
			return nil, fmt.Errorf("failed to store credential: %w", err)
		}

		fmt.Fprintf(w.writer, "\nCredential stored: %s\n", credentialRef)
	}

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
	savePath, err := promptConfigPath(w.reader, w.writer, defaultPath)
	if err != nil {
		return err
	}

	if savePath == "" {
		// Print-only mode
		fmt.Fprintln(w.writer, "\nGenerated configuration:")
		fmt.Fprintln(w.writer)
		fmt.Fprintln(w.writer, tomlContent)
		return nil
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

	printNextSteps(w.writer, w.config.Address, w.config.Port)
	return nil
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
