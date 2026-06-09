package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
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
		fmt.Fprintf(os.Stderr, "\n%sNo auth headers discovered.%s\n", run.Yellow, run.Reset)
		return
	}

	// Extract hostname from URL (use first URL)
	var hostPattern string
	if len(headerDisc.URLs) > 0 {
		hostPattern = examine.ExtractHostFromURL(headerDisc.URLs[0])
	} else {
		hostPattern = "api.example.com"
	}
	serviceName := examineLogger.GetCommandName()
	if serviceName == "" {
		serviceName = "myservice"
	}

	// Determine auth strategy
	strategy := examine.GuessAuthStrategy(headerDisc.HeaderName)

	// Print complete config
	fmt.Fprintf(os.Stderr, "\n%s=== Complete Configuration ===%s\n\n", run.Cyan+run.Bold, run.Reset)
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
		// Remove old symlink if it exists (absence is fine — this is a convenience link)
		_ = os.Remove(symlinkPath)
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
		fmt.Fprintf(os.Stderr, "\n%s=== Chaperone Examine Mode ===%s\n\n", run.Cyan+run.Bold, run.Reset)
		fmt.Fprintf(os.Stderr, "Chaperone will start a proxy server and launch your command.\n")
		fmt.Fprintf(os.Stderr, "The proxy will log all requests to help you discover authentication patterns.\n\n")

		run.PrintExamineBanner(run.ExamineBannerConfig{
			Command:    formatCommand(cliCommand),
			LogPath:    logPath,
			HAREnabled: enableHAR,
			HARPath:    harPath,
		})

		fmt.Fprintf(os.Stderr, "%sPress return to continue...%s", run.Magenta, run.Reset)

		// Wait for user input from /dev/tty (not stdin, to avoid interfering with child process)
		tty, err := os.Open("/dev/tty")
		if err == nil {
			reader := bufio.NewReader(tty)
			_, _ = reader.ReadByte()
			_ = tty.Close()
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

	// Generate proxy secret for authentication
	proxySecret, err := proxy.GenerateProxySecret()
	if err != nil {
		return fmt.Errorf("failed to generate proxy secret: %w", err)
	}

	server := proxy.NewExamineProxy(cfg, slog.Default(), shutdownMgr, certCache, examineLogger, rec, sentinelChan, proxySecret)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start examine proxy: %w", err)
	}

	// Get the proxy URL with embedded credentials
	proxyURL := server.ProxyURL()

	// Update manual mode output with actual address
	if len(cliCommand) == 0 {
		fmt.Printf("\nProxy listening on: %s\n", server.Addr())
		fmt.Printf("Configure your application to use this as proxy\n")
		fmt.Printf("Example:\n")
		fmt.Printf("  export HTTP_PROXY=%s\n", proxyURL)
		fmt.Printf("  export HTTPS_PROXY=%s\n\n", proxyURL)
	}

	// If command is provided, launch it with proxy environment
	if len(cliCommand) > 0 {
		// Get CA paths for environment
		_, _, caCertPath, err := getCAPath()
		if err != nil {
			return fmt.Errorf("failed to get CA path: %w", err)
		}

		// Parse user environment variables into map
		userEnvVars := make(map[string]string)
		for _, envVar := range envVars {
			idx := strings.Index(envVar, "=")
			if idx == -1 {
				return fmt.Errorf("invalid environment variable (missing '='): %s", envVar)
			}
			if idx == 0 {
				return fmt.Errorf("invalid environment variable (empty name): %s", envVar)
			}
			userEnvVars[envVar[:idx]] = envVar[idx+1:]
		}

		// Configure process using unified ProcessConfig
		processCfg := run.ProcessConfig{
			Command:     cliCommand,
			ProxyURL:    proxyURL,
			CACertPath:  caCertPath,
			CAEnvVars:   nil, // Use comprehensive defaults (10+ vars)
			UserEnvVars: userEnvVars,
		}

		// Spawn child process
		childProcess, err := run.SpawnChild(ctx, processCfg)
		if err != nil {
			if shutErr := shutdownMgr.Shutdown(5 * time.Second); shutErr != nil {
				log.Error(ctx, "proxy shutdown failed during spawn-error cleanup", shutErr)
			}
			return err
		}

		// Wait for child to exit or sentinel to be found
		exitCode := childProcess.WaitWithSentinel(ctx, sentinelChan)

		// Check if sentinel was triggered (exit code 0 from WaitWithSentinel means sentinel)
		// We need to check if sentinelChan was closed to know if it was sentinel-triggered
		select {
		case <-sentinelChan:
			// Sentinel was triggered - print complete config
			log.Info(ctx, "stopping proxy server")
			if shutErr := shutdownMgr.Shutdown(10 * time.Second); shutErr != nil {
				log.Error(ctx, "proxy shutdown failed", shutErr)
			}
			if logFile != nil {
				log.Info(ctx, "closing temporary log file", "path", logPath)
				if closeErr := logFile.Close(); closeErr != nil {
					log.Error(ctx, "failed to close temporary log file", closeErr, "path", logPath)
				}
			}

			// Print complete config to stdout
			printCompleteConfig(ctx, examineLogger)
			os.Exit(exitCode)

		default:
			// Child exited normally (sentinel wasn't triggered)
			log.Info(ctx, "stopping proxy server")
			if shutErr := shutdownMgr.Shutdown(10 * time.Second); shutErr != nil {
				log.Error(ctx, "proxy shutdown failed", shutErr)
			}

			if logFile != nil {
				log.Info(ctx, "closing temporary log file", "path", logPath)
				if closeErr := logFile.Close(); closeErr != nil {
					log.Error(ctx, "failed to close temporary log file", closeErr, "path", logPath)
				}
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
