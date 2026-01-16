package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/bmf/chaperone/internal/config"
	"github.com/bmf/chaperone/internal/examine"
	"github.com/bmf/chaperone/internal/log"
	"github.com/bmf/chaperone/internal/mitm"
	"github.com/bmf/chaperone/internal/proxy"
	"github.com/bmf/chaperone/internal/shutdown"
	"github.com/spf13/cobra"
)

var (
	// Examine flags
	showBody     bool
	showParams   bool
	showCookies  bool
	showResponse bool
	outputFile   string
)

var examineCmd = &cobra.Command{
	Use:   "examine",
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

Example:
  chaperone examine
  chaperone examine --show-params --show-cookies
  chaperone examine --output results.txt  # Enables all flags and saves to file`,
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
}

func runExamine(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// If output file is specified, enable all flags
	if outputFile != "" {
		showBody = true
		showParams = true
		showCookies = true
		showResponse = true
	}

	// Minimal config - just need server address/port
	// Examine mode doesn't need services configured
	cfg := &config.Config{
		Server: config.ServerConfig{
			Address: "127.0.0.1",
			Port:    4010,
		},
		Logging: config.LoggingConfig{
			Level: "info",
		},
	}

	// Check if config exists to override defaults
	configPath, err := getConfigPath()
	if err == nil && configPath != "" {
		loadedCfg, loadErr := config.Load(configPath)
		if loadErr == nil {
			// Use server and logging settings from config if available
			cfg.Server = loadedCfg.Server
			cfg.Logging = loadedCfg.Logging
		}
	}

	cfg.SetDefaults()

	// Setup logging based on config and format flag
	setupLogging(cfg)

	// Setup examine logger configuration
	examineConfig := examine.Config{
		ShowBody:     showBody,
		ShowParams:   showParams,
		ShowCookies:  showCookies,
		ShowResponse: showResponse,
		MaxBodyBytes: 4096, // 4KB
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
	examineLogger := examine.NewLogger(nil, examineConfig)

	// Log startup info
	log.Info(ctx, "chaperone examine mode starting",
		"address", cfg.Server.Address,
		"port", cfg.Server.Port,
	)
	fmt.Printf("\nConfigure your application to use: http://%s:%d as proxy\n", cfg.Server.Address, cfg.Server.Port)
	fmt.Printf("Example:\n")
	fmt.Printf("  export HTTP_PROXY=http://%s:%d\n", cfg.Server.Address, cfg.Server.Port)
	fmt.Printf("  export HTTPS_PROXY=http://%s:%d\n\n", cfg.Server.Address, cfg.Server.Port)
	fmt.Printf("Press Ctrl+C to stop.\n\n")
	fmt.Printf("  *** WARNING: SECURITY NOTICE ***\n")
	fmt.Printf("  Examine mode passes ALL traffic through unmodified, including authentication credentials.\n")
	fmt.Printf("  Real API keys, tokens, and passwords will be transmitted to their destinations.\n")
	fmt.Printf("  Use test/development credentials when possible.\n\n")

	server := proxy.NewExamineProxy(cfg, slog.Default(), shutdownMgr, certCache, examineLogger)

	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start examine proxy: %w", err)
	}

	if err := shutdownMgr.WaitForShutdown(); err != nil {
		return err
	}

	return shutdownMgr.Shutdown(30 * time.Second)
}
