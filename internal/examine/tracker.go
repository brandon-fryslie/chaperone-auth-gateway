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
	URL               string
	TruncatedValue    string
	FoundSentinel     bool
	IsStandardAuthKey bool
}

// DiscoveryTracker collects auth header discoveries across all requests
type DiscoveryTracker struct {
	mu          sync.Mutex
	discoveries map[string]*AuthHeaderDiscovery // key: headerName
}

// NewDiscoveryTracker creates a new discovery tracker
func NewDiscoveryTracker() *DiscoveryTracker {
	return &DiscoveryTracker{
		discoveries: make(map[string]*AuthHeaderDiscovery),
	}
}

// TrackHeader records a discovered auth header
func (dt *DiscoveryTracker) TrackHeader(headerName, url, truncatedValue string, foundSentinel, isStandardAuth bool) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	key := strings.ToLower(headerName)
	if _, exists := dt.discoveries[key]; !exists {
		dt.discoveries[key] = &AuthHeaderDiscovery{
			HeaderName:        headerName,
			URL:               url,
			TruncatedValue:    truncatedValue,
			FoundSentinel:     foundSentinel,
			IsStandardAuthKey: isStandardAuth,
		}
	}
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
