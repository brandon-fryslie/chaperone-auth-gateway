// Package examine provides functionality for discovering authentication patterns in HTTP traffic.
// This file contains discovery tracking for auth headers found during examination.
package examine

import (
	"strings"
	"sync"
)

// AuthHeaderDiscovery tracks a discovered auth header
type AuthHeaderDiscovery struct {
	HeaderName        string
	URLs              []string // Changed from single URL to list
	TruncatedValue    string
	FoundSentinel     bool
	IsStandardAuthKey bool
}

// DiscoveryTracker collects auth header discoveries across all requests
type DiscoveryTracker struct {
	mu          sync.Mutex
	discoveries map[string]*AuthHeaderDiscovery // key: headerName
	seenHeaders map[string]bool                 // Track if we've logged this header before
}

// NewDiscoveryTracker creates a new discovery tracker
func NewDiscoveryTracker() *DiscoveryTracker {
	return &DiscoveryTracker{
		discoveries: make(map[string]*AuthHeaderDiscovery),
		seenHeaders: make(map[string]bool),
	}
}

// TrackHeader records a discovered auth header
func (dt *DiscoveryTracker) TrackHeader(headerName, url, truncatedValue string, foundSentinel, isStandardAuth bool) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	key := strings.ToLower(headerName)
	if existing, exists := dt.discoveries[key]; exists {
		// Append URL if not already in list
		for _, existingURL := range existing.URLs {
			if existingURL == url {
				return // URL already tracked
			}
		}
		existing.URLs = append(existing.URLs, url)
	} else {
		dt.discoveries[key] = &AuthHeaderDiscovery{
			HeaderName:        headerName,
			URLs:              []string{url},
			TruncatedValue:    truncatedValue,
			FoundSentinel:     foundSentinel,
			IsStandardAuthKey: isStandardAuth,
		}
	}
}

// HasSeenHeader returns true if we've already logged this header during the session
func (dt *DiscoveryTracker) HasSeenHeader(headerName string) bool {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	key := strings.ToLower(headerName)
	return dt.seenHeaders[key]
}

// MarkHeaderSeen marks a header as having been logged
func (dt *DiscoveryTracker) MarkHeaderSeen(headerName string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	key := strings.ToLower(headerName)
	dt.seenHeaders[key] = true
}

// GetDiscoveries returns all discovered headers (sorted for consistency)
func (dt *DiscoveryTracker) GetDiscoveries() []*AuthHeaderDiscovery {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	result := make([]*AuthHeaderDiscovery, 0, len(dt.discoveries))
	for _, disc := range dt.discoveries {
		result = append(result, disc)
	}
	return result
}
