package executor

import (
	"sync"
)

// CircuitBreakerRegistry maintains a global registry of all CircuitBreakerExecutor instances.
// This enables the debug endpoint to aggregate circuit breaker state from all sessions.
type CircuitBreakerRegistry struct {
	mu        sync.RWMutex
	executors map[string]*CircuitBreakerExecutor
}

// globalRegistry is the singleton registry instance.
var globalRegistry = &CircuitBreakerRegistry{
	executors: make(map[string]*CircuitBreakerExecutor),
}

// GetGlobalRegistry returns the global circuit breaker registry.
func GetGlobalRegistry() *CircuitBreakerRegistry {
	return globalRegistry
}

// Register adds a CircuitBreakerExecutor to the registry with a unique key.
// The key should identify the owner (e.g., "git-<session-title>" or "tmux-<session-title>").
func (r *CircuitBreakerRegistry) Register(key string, cbe *CircuitBreakerExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[key] = cbe
}

// Unregister removes a CircuitBreakerExecutor from the registry.
func (r *CircuitBreakerRegistry) Unregister(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.executors, key)
}

// ResetAll resets all circuit breakers in all registered executors to closed state.
// Call after successfully recovering an external dependency (e.g. tmux server restart).
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cbe := range r.executors {
		cbe.Reset()
	}
}

// AllBreakers returns a combined snapshot of all circuit breakers across all registered executors.
// The returned map keys are prefixed with the executor key for disambiguation (e.g., "git-session1/git-diff").
func (r *CircuitBreakerRegistry) AllBreakers() map[string]CircuitBreakerSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]CircuitBreakerSnapshot)
	for execKey, cbe := range r.executors {
		for breakerKey, snap := range cbe.AllBreakers() {
			compositeKey := execKey + "/" + breakerKey
			result[compositeKey] = snap
		}
	}
	return result
}
