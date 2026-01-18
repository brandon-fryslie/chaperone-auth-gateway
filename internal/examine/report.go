// Package examine provides functionality for discovering authentication patterns in HTTP traffic.
// This file contains summary report generation and configuration suggestion logic.
package examine

import (
	"context"
	"runtime"
	"strings"

	"github.com/bmf/chaperone/internal/log"
)

// PrintSummaryReport prints a summary of all discovered auth headers at exit
func (l *Logger) PrintSummaryReport(ctx context.Context) {
	discoveries := l.discovery.GetDiscoveries()
	if len(discoveries) == 0 {
		log.Info(ctx, "No auth headers discovered during examination")
		return
	}

	// Log summary as a detail message
	log.Detail(ctx, "detail_text", "=== Auth Discovery Summary ===", "detail_color", "blue")

	// Separate discoveries by type
	sentinelHeaders := make([]*AuthHeaderDiscovery, 0)
	standardHeaders := make([]*AuthHeaderDiscovery, 0)
	possibleHeaders := make([]*AuthHeaderDiscovery, 0)

	for _, disc := range discoveries {
		if disc.FoundSentinel {
			sentinelHeaders = append(sentinelHeaders, disc)
		} else if disc.IsStandardAuthKey {
			standardHeaders = append(standardHeaders, disc)
		} else {
			possibleHeaders = append(possibleHeaders, disc)
		}
	}

	// Log sentinel matches
	if len(sentinelHeaders) > 0 {
		log.Detail(ctx, "detail_text", "✓ Sentinel value found in:", "detail_color", "green")
		for _, disc := range sentinelHeaders {
			log.Detail(ctx, "detail_text", "  - "+disc.HeaderName+" sent to "+disc.URL)
		}
	}

	// Log standard auth headers
	if len(standardHeaders) > 0 {
		log.Detail(ctx, "detail_text", "✓ Standard auth headers found:", "detail_color", "green")
		for _, disc := range standardHeaders {
			log.Detail(ctx, "detail_text", "  - "+disc.HeaderName+" sent to "+disc.URL)
		}
	}

	// Log possible headers
	if len(possibleHeaders) > 0 {
		log.Detail(ctx, "detail_text", "? Possible auth headers:", "detail_color", "yellow")
		for _, disc := range possibleHeaders {
			log.Detail(ctx, "detail_text", "  - "+disc.HeaderName+" sent to "+disc.URL)
		}
	}

	// Log example config
	l.printExampleConfig(ctx, sentinelHeaders, standardHeaders, possibleHeaders)
}

// printExampleConfig generates an example config block based on discoveries
func (l *Logger) printExampleConfig(ctx context.Context, sentinels, standards, possible []*AuthHeaderDiscovery) {
	log.Detail(ctx, "detail_text", "=== Example Config Block ===", "detail_color", "blue")

	var exampleHeader *AuthHeaderDiscovery

	if len(sentinels) > 0 {
		exampleHeader = sentinels[0]
	} else if len(standards) > 0 {
		exampleHeader = standards[0]
	} else if len(possible) > 0 {
		exampleHeader = possible[0]
	} else {
		log.Detail(ctx, "detail_text", "# No auth headers discovered. Check --all-headers flag or examine your requests.")
		return
	}

	// Extract hostname from URL
	hostPattern := ExtractHostFromURL(exampleHeader.URL)

	// Determine service name
	serviceName := "myservice"
	if l.commandName != "" {
		serviceName = l.commandName
	}

	// Generate example config
	strategy := GuessAuthStrategy(exampleHeader.HeaderName)

	log.Detail(ctx, "detail_text", "")
	log.Detail(ctx, "detail_text", "[services."+serviceName+"]")
	log.Detail(ctx, "detail_text", "host_pattern = \""+hostPattern+"\"")
	log.Detail(ctx, "detail_text", "auth_strategy = \""+strategy+"\"")

	if IsOSMacOS() {
		// macOS: Show keychain command and credential_ref
		credentialName := "chaperone/" + serviceName
		keychainCmd := "security add-generic-password -s \"" + credentialName + "\" -a \"\" -w \"<YOUR_API_KEY>\""
		log.Detail(ctx, "detail_text", "")
		log.Detail(ctx, "detail_text", "# to add to MacOS keychain, run: "+keychainCmd+" # MacOS Only")
		log.Detail(ctx, "detail_text", "# credential_ref = \"keychain:"+credentialName+"\" # MacOS Only")
	}

	// Always show env and file options
	log.Detail(ctx, "detail_text", "# credential_ref = \"env:YOUR_API_KEY\"")
	log.Detail(ctx, "detail_text", "# credential_ref = \"file:/path/to/secret\"")
}

// ExtractHostFromURL extracts the hostname from a URL
func ExtractHostFromURL(urlStr string) string {
	// Simple extraction - just get the host part
	// e.g., "https://api.openai.com/v1/chat" -> "api.openai.com"
	if urlStr == "" {
		return "api.example.com"
	}

	// Find the scheme
	schemeEnd := strings.Index(urlStr, "://")
	if schemeEnd == -1 {
		return "api.example.com"
	}

	urlStr = urlStr[schemeEnd+3:] // Skip "://"

	// Find the end of the host (port, path, etc.)
	hostEnd := strings.IndexAny(urlStr, ":/?")
	if hostEnd == -1 {
		return urlStr
	}
	return urlStr[:hostEnd]
}

// IsOSMacOS checks if the current OS is macOS
func IsOSMacOS() bool {
	return runtime.GOOS == "darwin"
}

// GuessAuthStrategy determines likely auth strategy based on header name
func GuessAuthStrategy(headerName string) string {
	nameLower := strings.ToLower(headerName)
	if nameLower == "authorization" {
		return "bearer"
	}
	return "header:" + headerName
}
