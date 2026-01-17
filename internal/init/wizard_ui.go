package init

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// UI functions for the wizard - all prompts and display logic.
// wizard.go contains only coordination logic.

// printIntroduction displays the welcome message and overview.
func printIntroduction(w io.Writer) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "=== Welcome to Chaperone ===")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Chaperone is a local proxy that automatically attaches authentication")
	fmt.Fprintln(w, "credentials to your outgoing API requests. Your applications connect")
	fmt.Fprintln(w, "through Chaperone using standard proxy settings, and Chaperone injects")
	fmt.Fprintln(w, "the appropriate API keys before forwarding requests. This means your")
	fmt.Fprintln(w, "applications never have direct access to your API keys—they stay safely")
	fmt.Fprintln(w, "stored and managed by Chaperone.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "This wizard will:")
	fmt.Fprintln(w, "  • Detect which APIs your application calls and how they authenticate")
	fmt.Fprintln(w, "  • Let you select and configure each service")
	fmt.Fprintln(w, "  • Securely store your API credentials (Keychain, file, or .env)")
	fmt.Fprintln(w, "  • Generate a chaperone.toml configuration file")
	fmt.Fprintln(w, "")
}

// printDetectionInstructions prints instructions for detection mode.
func printDetectionInstructions(w io.Writer, address string, port int) {
	fmt.Fprintln(w, "\n=== Step 2: Detection Mode ===")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Proxy listening on http://%s:%d\n\n", address, port)
	fmt.Fprintln(w, "Configure your application to use this proxy:")
	fmt.Fprintf(w, "  export HTTP_PROXY=http://%s:%d\n", address, port)
	fmt.Fprintf(w, "  export HTTPS_PROXY=http://%s:%d\n\n", address, port)
	fmt.Fprintln(w, "Send requests through the proxy. Detected auth patterns will appear below.")
	fmt.Fprintln(w, "Press Ctrl+C when done to proceed to review.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "---")
}

// reportFinding prints a finding in real-time during detection.
func reportFinding(w io.Writer, host string, finding *Finding) {
	confidence := fmt.Sprintf("%.0f%%", finding.Confidence*100)
	fmt.Fprintf(w, "[%s] %s - %s: %s (confidence: %s)\n",
		host,
		finding.Heuristic,
		finding.HeaderName,
		redactValue(finding.HeaderValue),
		confidence,
	)
}

// printNextSteps prints post-wizard instructions.
func printNextSteps(w io.Writer, address string, port int) {
	fmt.Fprintln(w, "\n=== Next Steps ===")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "1. Trust the CA certificate in your browser/system:")
	fmt.Fprintln(w, "   chaperone ca-cert")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "2. Start the proxy:")
	fmt.Fprintln(w, "   chaperone inject")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "3. Configure your application to use the proxy:")
	fmt.Fprintf(w, "   export HTTP_PROXY=http://%s:%d\n", address, port)
	fmt.Fprintf(w, "   export HTTPS_PROXY=http://%s:%d\n", address, port)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'chaperone init' again to add more services.")
}

// promptAddress prompts for the proxy listen address.
func promptAddress(reader *bufio.Reader, w io.Writer, defaultAddress string) (string, error) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "The IP address where the proxy server will listen.")
	fmt.Fprintf(w, "Listen address [%s]: ", defaultAddress)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultAddress, nil
	}
	return line, nil
}

// promptPort prompts for the proxy listen port.
func promptPort(reader *bufio.Reader, w io.Writer, defaultPort int) (int, error) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "The port number for proxy connections.")
	fmt.Fprintf(w, "Listen port [%d]: ", defaultPort)
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultPort, nil
	}
	port, err := strconv.Atoi(line)
	if err != nil {
		return 0, fmt.Errorf("invalid port: %w", err)
	}
	return port, nil
}

// promptSentinel prompts for an optional sentinel value.
func promptSentinel(reader *bufio.Reader, w io.Writer) (string, error) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "An exact string to search for in requests (100% confidence detection).")
	fmt.Fprintf(w, "Sentinel value (optional, press Enter to skip): ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptServiceName prompts for a service name.
func promptServiceName(reader *bufio.Reader, w io.Writer, defaultName string) (string, error) {
	fmt.Fprintln(w, "A short identifier for this service in your config.")
	fmt.Fprintf(w, "Service name [%s]: ", defaultName)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultName, nil
	}
	return line, nil
}

// promptCredentialValue prompts for the actual credential value.
func promptCredentialValue(reader *bufio.Reader, w io.Writer) (string, error) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "The actual API key or token that Chaperone will inject into requests.")
	fmt.Fprintf(w, "Enter the API key/credential value: ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	credentialValue := strings.TrimSpace(line)
	if credentialValue == "" {
		return "", fmt.Errorf("credential value cannot be empty")
	}
	return credentialValue, nil
}

// promptStorageType prompts for the credential storage type.
func promptStorageType(reader *bufio.Reader, w io.Writer) (CredentialStorageType, error) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Where do you want to store this credential?")
	fmt.Fprintln(w, "  1. macOS Keychain - Most secure, uses macOS secure storage (recommended)")
	fmt.Fprintln(w, "  2. File - Stored with restricted permissions (~/.config/chaperone/secrets/)")
	fmt.Fprintln(w, "  3. .env file - Environment file in current directory")
	fmt.Fprintf(w, "\nChoice [1]: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}

	switch line {
	case "1":
		return StorageKeychain, nil
	case "2":
		return StorageFile, nil
	case "3":
		return StorageEnvFile, nil
	default:
		return 0, fmt.Errorf("invalid storage choice: %s", line)
	}
}

// promptServiceSelection prompts user to select a service from the list.
// Returns the selected index (0-based) or -1 if user quits.
func promptServiceSelection(reader *bufio.Reader, w io.Writer, count int) (int, error) {
	fmt.Fprintf(w, "Enter the number of the service to configure (or 'q' to quit): ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return -1, err
	}
	line = strings.TrimSpace(line)

	if line == "q" || line == "Q" {
		return -1, nil
	}

	idx, err := strconv.Atoi(line)
	if err != nil || idx < 1 || idx > count {
		return -1, fmt.Errorf("invalid selection: %s", line)
	}

	return idx - 1, nil
}

// promptConfigPath prompts for the config save location.
// Returns the selected path or empty string to skip saving.
func promptConfigPath(reader *bufio.Reader, w io.Writer, defaultPath string) (string, error) {
	fmt.Fprintln(w, "Where do you want to save the configuration?")
	fmt.Fprintf(w, "  1. Default location (%s)\n", defaultPath)
	fmt.Fprintln(w, "  2. Current directory (./chaperone.toml)")
	fmt.Fprintln(w, "  3. Custom path")
	fmt.Fprintln(w, "  4. Print only (don't save)")
	fmt.Fprintf(w, "\nChoice [1]: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = "1"
	}

	switch line {
	case "1":
		return defaultPath, nil
	case "2":
		return "chaperone.toml", nil
	case "3":
		fmt.Fprintf(w, "Enter path: ")
		line, err = reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(line), nil
	case "4":
		return "", nil // Empty string means print-only
	default:
		return "", fmt.Errorf("invalid choice: %s", line)
	}
}

// redactValue partially hides a credential value for display.
func redactValue(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
