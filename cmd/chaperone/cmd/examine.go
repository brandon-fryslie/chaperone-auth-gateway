package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/defaults"
	"github.com/bmf/chaperone/internal/examine"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/recorder"
	"github.com/bmf/chaperone/internal/run"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	cyan    = "\033[36m"
	green   = "\033[32m"
	blue    = "\033[34m"
	yellow  = "\033[33m"
	magenta = "\033[35m"
)

var (
	// Examine flags
	showBody        bool
	showParams      bool
	showCookies     bool
	showResponse    bool
	outputFile      string
	enableHAR       bool
	harOutputFile   string
	enableJSONL     bool
	jsonlOutputFile string
	allHeaders      bool
	sentinelValue   string
	envVars         []string // Can be passed multiple times with -e/--env
)

// formatCommand formats a command slice as a space-separated string
func formatCommand(cmd []string) string {
	result := ""
	for i, arg := range cmd {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}

// setEnvVar parses an environment variable in the format "VAR=value" and sets it
// Splits on the first '=' character
func setEnvVar(builder *run.EnvBuilder, envVar string) error {
	idx := strings.Index(envVar, "=")
	if idx == -1 {
		return fmt.Errorf("missing '=' in environment variable: %s", envVar)
	}
	if idx == 0 {
		return fmt.Errorf("empty variable name in environment variable: %s", envVar)
	}
	varName := envVar[:idx]
	varValue := envVar[idx+1:]
	builder.Set(varName, varValue)
	return nil
}

// printCompleteConfig prints a complete, ready-to-use TOML config based on discoveries
func printCompleteConfig(ctx context.Context, examineLogger *examine.Logger) {
	discoveries := examineLogger.GetDiscoveryTracker().GetDiscoveries()

	// Find sentinel discovery or standard discovery
	var headerDisc *examine.AuthHeaderDiscovery
	for _, disc := range discoveries {
		if disc.FoundSentinel {
			headerDisc = disc
			break
		}
	}
	if headerDisc == nil && len(discoveries) > 0 {
		// Use first standard auth header if no sentinel
		for _, disc := range discoveries {
			if disc.IsStandardAuthKey {
				headerDisc = disc
				break
			}
		}
	}
	if headerDisc == nil && len(discoveries) > 0 {
		// Use first discovery as fallback
		headerDisc = discoveries[0]
	}

	if headerDisc == nil {
		fmt.Fprintf(os.Stderr, "\n%sNo auth headers discovered.%s\n", yellow, reset)
		return
	}

	// Extract hostname from URL
	hostPattern := examine.ExtractHostFromURL(headerDisc.URL)
	serviceName := examineLogger.GetCommandName()
	if serviceName == "" {
		serviceName = "myservice"
	}

	// Determine auth strategy
	strategy := examine.GuessAuthStrategy(headerDisc.HeaderName)

	// Print complete config
	fmt.Fprintf(os.Stderr, "\n%s=== Complete Configuration ===%s\n\n", cyan+bold, reset)
	fmt.Fprintf(os.Stderr, "[services.%s]\n", serviceName)
	fmt.Fprintf(os.Stderr, "host_pattern = \"%s\"\n", hostPattern)
	fmt.Fprintf(os.Stderr, "auth_strategy = \"%s\"\n", strategy)

	// Print credential_ref options - keychain/file first (secure options), then env (quick testing only)
	fmt.Fprintf(os.Stderr, "# Recommended: Use keychain or file for secure credential storage\n")

	// macOS keychain option
	if examine.IsOSMacOS() {
		keychainService := "chaperone/" + serviceName
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "# macOS Keychain (Recommended):\n")
		fmt.Fprintf(os.Stderr, "# security add-generic-password -s \"%s\" -a \"api_key\" -w \"YOUR_API_KEY\"\n", keychainService)
		fmt.Fprintf(os.Stderr, "# credential_ref = \"keychain:%s/api_key\"\n", keychainService)
	} else {
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "# Secure file storage (Recommended):\n")
		fmt.Fprintf(os.Stderr, "# credential_ref = \"file:/path/to/secret\"\n")
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "# credential_ref = \"env:YOUR_API_KEY\"\n")
	fmt.Fprintf(os.Stderr, "\n")
}

var examineCmd = &cobra.Command{
	Use:   "examine [-- <command> <arg1> <arg2> ...]",
	Short: "Passthrough proxy to examine requests for auth discovery",
	Long: `Examine mode runs a MITM proxy that logs requests without modifying them.

Use this to discover where authentication credentials are passed in requests
before writing your chaperone configuration.

The proxy will log:
- Request method, URL, and host
- Headers that may contain authentication (excluding Content-Type, Accept, etc.)

Optional flags control additional output:
- Query parameters (--show-params)
- Cookies (--show-cookies)
- Request/response bodies (--show-body)
- Server responses (--show-response)
- All headers (--all-headers) - disable header filtering heuristics

HAR Recording:
- Capture traffic in HAR format with --har flag
- HAR files can be imported into Chrome DevTools, Firefox, and other tools
- Default filename: chaperone-<timestamp>.har
- Custom path: --har-output <path>

Command Execution:
- Launch a command with proxy configuration: chaperone examine -- <command>
- The command inherits the current terminal's stdin/stdout/stderr
- Proxy logs are written to a temporary file
- Example: chaperone examine -- curl https://api.example.com

Example:
  chaperone examine
  chaperone examine --show-params --show-cookies
  chaperone examine --output results.txt  # Enables all flags and saves to file
  chaperone examine --har                  # Capture HAR
  chaperone examine --har --har-output traffic.har
  chaperone examine -- curl https://api.openai.com/v1/models`,
	RunE: runExamine,
}

func init() {
	rootCmd.AddCommand(examineCmd)

	// Add flags
	examineCmd.Flags().BoolVarP(&showBody, "show-body", "b", false, "Show request and response bodies (truncated at 4KB)")
	examineCmd.Flags().BoolVarP(&showParams, "show-params", "p", false, "Show query parameters")
	examineCmd.Flags().BoolVar(&showCookies, "show-cookies", false, "Show cookies")
	examineCmd.Flags().BoolVarP(&showResponse, "show-response", "r", false, "Show server responses")
	examineCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Save results to file (enables all flags)")
	examineCmd.Flags().BoolVar(&enableHAR, "har", false, "Enable HAR recording (HTTP Archive format)")
	examineCmd.Flags().StringVar(&harOutputFile, "har-output", "", "Custom HAR output file path (implies --har)")
	examineCmd.Flags().BoolVar(&enableJSONL, "jsonl", false, "Enable JSONL recording (JSON Lines format for API analysis)")
	examineCmd.Flags().StringVar(&jsonlOutputFile, "jsonl-output", "", "Custom JSONL output file path (implies --jsonl)")
	examineCmd.Flags().BoolVar(&allHeaders, "all-headers", false, "Show all headers (disable filtering heuristics)")
	examineCmd.Flags().StringVar(&sentinelValue, "sentinel", "chaperone-sentinel", "Sentinel value to look for in auth headers to confirm correct header")
	examineCmd.Flags().StringSliceVarP(&envVars, "env", "e", []string{}, "Set environment variable for command (format: VAR=value, can be used multiple times)")
}

func runExamine(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse command arguments (support "examine -- command args" syntax)
	// Cobra strips the "--" separator, so args will just be the command and its arguments
	var cliCommand []string
	if len(args) > 0 {
		// Command mode: everything in args is the command and its arguments
		cliCommand = args
	}

	// If output file is specified, enable all flags
	if outputFile != "" {
		showBody = true
		showParams = true
		showCookies = true
		showResponse = true
	}

	// If har-output is specified, enable HAR recording
	if harOutputFile != "" {
		enableHAR = true
	}

	// If jsonl-output is specified, enable JSONL recording
	if jsonlOutputFile != "" {
		enableJSONL = true
	}

	// Minimal config - just need server address/port
	// Examine mode doesn't need services configured
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    0,
		},
		Logging: config.LoggingConfig{
			Level: "info",
		},
	}

	// Check if config exists to override logging (but NOT server config for examine mode)
	configPath, err := getConfigPath()
	if err == nil && configPath != "" {
		loadedCfg, loadErr := config.Load(configPath)
		if loadErr == nil {
			// Use logging settings from config if available
			// But keep examine mode's default TCP server config
			cfg.Logging = loadedCfg.Logging

			// Allow config to override server settings if explicitly set
			if loadedCfg.Server.Port != 0 {
				cfg.Server.Port = loadedCfg.Server.Port
			}
			if loadedCfg.Server.Address != "" {
				cfg.Server.Address = loadedCfg.Server.Address
			}
			// Note: Ignore Socket setting - examine mode uses TCP
		}
	}

	cfg.SetDefaults()

	// Create temp log file if running a command (logs won't interfere with stdout)
	var logFile *os.File
	var logPath string
	if len(cliCommand) > 0 {
		var err error
		logFile, logPath, err = run.CreateTempLogFile()
		if err != nil {
			return fmt.Errorf("failed to create temporary log file: %w", err)
		}
		// Set up logging to the temporary file
		run.SetupLoggingToFile(cfg, logFormat, logFile)

		// Create symlink at /tmp/chaperone-examine.latest.log for easy access
		symlinkPath := "/tmp/chaperone-examine.latest.log"
		// Remove old symlink if it exists (ignore errors)
		os.Remove(symlinkPath)
		// Create new symlink
		if err := os.Symlink(logPath, symlinkPath); err != nil {
			// Warn but don't fail - this is a convenience feature
			log.Warn(ctx, "failed to create log symlink (non-fatal)",
				"symlink", symlinkPath,
				"target", logPath,
				"error", err.Error(),
			)
		} else {
			log.Debug(ctx, "created log symlink", "symlink", symlinkPath, "target", logPath)
		}
	} else {
		// Setup logging based on config and format flag (stdout/stderr)
		setupLogging(cfg)
	}

	// Setup examine logger configuration
	examineConfig := examine.Config{
		ShowBody:      showBody,
		ShowParams:    showParams,
		ShowCookies:   showCookies,
		ShowResponse:  showResponse,
		MaxBodyBytes:  defaults.DefaultExamineBodyBytes,
		AllHeaders:    allHeaders,
		SentinelValue: sentinelValue,
	}

	// Setup CA
	caDir, caKeyPath, caCertPath, err := getCAPath()
	if err != nil {
		return fmt.Errorf("failed to get CA path: %w", err)
	}

	if err := os.MkdirAll(caDir, 0700); err != nil {
		return fmt.Errorf("failed to create CA directory: %w", err)
	}

	// Check if CA files already exist
	_, keyErr := os.Stat(caKeyPath)
	_, certErr := os.Stat(caCertPath)
	isNewCA := keyErr != nil || certErr != nil

	ca, err := mitm.LoadOrGenerateCA(caKeyPath, caCertPath)
	if err != nil {
		return fmt.Errorf("failed to initialize CA: %w", err)
	}

	if isNewCA {
		log.Info(ctx, "generated new CA certificate",
			"cert_path", caCertPath,
			"key_path", caKeyPath,
		)
		log.Info(ctx, "trust this CA in your browser/system to avoid certificate warnings")
	}

	certCache := mitm.NewCertCache(ca, slog.Default())
	shutdownMgr := shutdown.NewManager(slog.Default())
	examineLogger := examine.NewLogger(examineConfig)

	// Set command name for config generation if running with a command
	if len(cliCommand) > 0 {
		examineLogger.SetCommandName(cliCommand[0])
	}

	// Create HAR recorder if enabled
	var rec *recorder.Recorder
	var harPath string
	if enableHAR {
		rec = recorder.NewRecorder()

		// Determine HAR output path
		if harOutputFile != "" {
			harPath = harOutputFile
		} else {
			// Default: chaperone-<timestamp>.har
			timestamp := time.Now().Format("20060102-150405")
			harPath = fmt.Sprintf("chaperone-%s.har", timestamp)
		}

		// Register shutdown handler to write HAR file
		shutdownMgr.Register(func(ctx context.Context) error {
			log.Info(ctx, "writing HAR file", "path", harPath)
			if err := rec.WriteToFile(harPath); err != nil {
				return fmt.Errorf("failed to write HAR file: %w", err)
			}
			log.Info(ctx, "HAR file written successfully", "path", harPath)
			return nil
		})

		log.Info(ctx, "HAR recording enabled", "output_path", harPath)
	}

	// Register shutdown handler to print discovery summary
	shutdownMgr.Register(func(ctx context.Context) error {
		examineLogger.PrintSummaryReport(ctx)
		return nil
	})

	// Log startup info
	log.Info(ctx, "chaperone examine mode starting",
		"address", cfg.Server.Address,
		"port", cfg.Server.Port,
	)

	// Print startup info based on execution mode
	if len(cliCommand) > 0 {
		// Command mode: print info and wait for user
		fmt.Fprintf(os.Stderr, "\n%s=== Chaperone Examine Mode ===%s\n\n", cyan+bold, reset)
		fmt.Fprintf(os.Stderr, "Chaperone will start a proxy server and launch your command.\n")
		fmt.Fprintf(os.Stderr, "The proxy will log all requests to help you discover authentication patterns.\n\n")
		fmt.Fprintf(os.Stderr, "%sCommand:%s %s\n", blue+bold, reset, formatCommand(cliCommand))
		fmt.Fprintf(os.Stderr, "%sLog file:%s %s\n", blue+bold, reset, logPath)
		fmt.Fprintf(os.Stderr, "%sTo print logs from latest run:%s tail -F /tmp/chaperone-examine.latest.log\n\n", blue+bold, reset)

		if enableHAR {
			fmt.Fprintf(os.Stderr, "%sHAR Recording:%s %sENABLED%s\n", blue+bold, reset, green+bold, reset)
			fmt.Fprintf(os.Stderr, "%sHAR file:%s %s\n\n", blue+bold, reset, harPath)
		}

		fmt.Fprintf(os.Stderr, "%sPress return to continue...%s", magenta, reset)

		// Wait for user input from /dev/tty (not stdin, to avoid interfering with child process)
		tty, err := os.Open("/dev/tty")
		if err == nil {
			reader := bufio.NewReader(tty)
			_, _ = reader.ReadByte()
			tty.Close()
		} else {
			// Fallback: just wait a moment if /dev/tty not available
			time.Sleep(2 * time.Second)
		}
		fmt.Fprintf(os.Stderr, "\n\n")
	} else {
		// Manual mode: print proxy configuration placeholder (will update after server starts)
		fmt.Printf("\nStarting proxy server...\n")

		if enableHAR {
			fmt.Printf("HAR Recording: ENABLED\n")
			fmt.Printf("Output file: %s\n\n", harPath)
		}
	}

	// Create sentinel channel for detecting auth discovery (only used in command mode)
	var sentinelChan chan struct{}
	if len(cliCommand) > 0 {
		sentinelChan = make(chan struct{})
	}

	server := proxy.NewExamineProxy(cfg, slog.Default(), shutdownMgr, certCache, examineLogger, rec, sentinelChan)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start examine proxy: %w", err)
	}

	// Get the actual listening address (OS-allocated port)
	actualAddr := server.Addr()
	proxyURL := fmt.Sprintf("http://%s", actualAddr)

	// Update manual mode output with actual address
	if len(cliCommand) == 0 {
		fmt.Printf("\nProxy listening on: %s\n", proxyURL)
		fmt.Printf("Configure your application to use this as proxy\n")
		fmt.Printf("Example:\n")
		fmt.Printf("  export HTTP_PROXY=%s\n", proxyURL)
		fmt.Printf("  export HTTPS_PROXY=%s\n\n", proxyURL)
	}

	// If command is provided, launch it with proxy environment
	if len(cliCommand) > 0 {
		// Build environment with proxy vars
		envBuilder := run.NewEnvBuilder()
		envBuilder.InheritParent()
		envBuilder.Set("HTTP_PROXY", proxyURL)
		envBuilder.Set("HTTPS_PROXY", proxyURL)
		envBuilder.Set("CHAPERONE_SERVICE", "examine")

		// Get CA paths for environment
		_, _, caCertPath, err := getCAPath()
		if err != nil {
			return fmt.Errorf("failed to get CA path: %w", err)
		}
		envBuilder.SetCAEnvVars(caCertPath, []string{"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE", "NODE_EXTRA_CA_CERTS"})

		// Set user-provided environment variables
		for _, envVar := range envVars {
			if err := setEnvVar(envBuilder, envVar); err != nil {
				return fmt.Errorf("invalid environment variable: %w", err)
			}
		}

		childEnv := envBuilder.Build()

		// Create command - DO NOT use ProcessManager to avoid process group isolation
		// We want the child to receive signals directly from the terminal
		cmd := exec.Command(cliCommand[0], cliCommand[1:]...)
		cmd.Env = childEnv
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// No Setpgid - child stays in same process group for proper signal handling

		log.Info(ctx, "starting child process", "command", cliCommand[0], "args", cliCommand[1:])

		// Start child process
		if err := cmd.Start(); err != nil {
			shutdownMgr.Shutdown(5 * time.Second)
			return fmt.Errorf("failed to start child process: %w", err)
		}

		log.Info(ctx, "child process started successfully")

		// Wait for child to exit in a goroutine
		childExitChan := make(chan int, 1)
		go func() {
			err := cmd.Wait()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					childExitChan <- exitErr.ExitCode()
				} else {
					childExitChan <- 1
				}
			} else {
				childExitChan <- 0
			}
		}()

		// Wait for child to exit or sentinel to be found
		var exitCode int
		select {
		case <-sentinelChan:
			// Sentinel found - terminate child and print config
			log.Info(ctx, "sentinel value detected, terminating child process")
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				_ = cmd.Process.Kill()
			}
			// Wait for graceful shutdown (up to 10 seconds), then kill if still running
			select {
			case <-time.After(10 * time.Second):
				_ = cmd.Process.Kill()
				// Wait for exit
				exitCode = <-childExitChan
			case exitCode = <-childExitChan:
				// Child exited gracefully
			}
			exitCode = 0

			// Cleanup and print complete config
			log.Info(ctx, "stopping proxy server")
			shutdownMgr.Shutdown(10 * time.Second)
			if logFile != nil {
				log.Info(ctx, "closing temporary log file", "path", logPath)
				logFile.Close()
			}

			// Print complete config to stdout
			printCompleteConfig(ctx, examineLogger)
			os.Exit(exitCode)

		case exitCode = <-childExitChan:
			// Child exited normally
			log.Info(ctx, "child process exited", "exit_code", exitCode)

			// Cleanup
			log.Info(ctx, "stopping proxy server")
			shutdownMgr.Shutdown(10 * time.Second)

			if logFile != nil {
				log.Info(ctx, "closing temporary log file", "path", logPath)
				logFile.Close()
			}

			os.Exit(exitCode)
		}
		return nil // Never reached
	}

	// Manual mode: wait for shutdown signal
	if err := shutdownMgr.WaitForShutdown(); err != nil {
		return err
	}

	return shutdownMgr.Shutdown(30 * time.Second)
}
