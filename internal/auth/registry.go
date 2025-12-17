package auth

import (
	"fmt"
	"sync"
)

// Registry manages authentication strategies by name.
// It is safe for concurrent use.
type Registry struct {
	mu         sync.RWMutex
	strategies map[string]AuthStrategy
}

// NewRegistry creates a new authentication strategy registry.
func NewRegistry() *Registry {
	return &Registry{
		strategies: make(map[string]AuthStrategy),
	}
}

// Register adds or replaces an authentication strategy.
// If a strategy with the same name already exists, it will be replaced.
func (r *Registry) Register(name string, strategy AuthStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategies[name] = strategy
}

// Get retrieves an authentication strategy by name.
// Returns an error if the strategy is not found.
func (r *Registry) Get(name string) (AuthStrategy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	strategy, found := r.strategies[name]
	if !found {
		return nil, fmt.Errorf("authentication strategy not found: %s", name)
	}

	return strategy, nil
}
