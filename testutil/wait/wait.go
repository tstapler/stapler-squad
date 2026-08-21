// Package wait provides polling helpers for tests that cannot import the top-level
// testutil package due to import cycles with the session package.
package wait

import (
	"context"
	"fmt"
	"time"
)

// Common timeout durations for different test scenarios.
var (
	FastTimeout    = 2 * time.Second  // For unit tests and quick operations
	DefaultTimeout = 10 * time.Second // For most integration tests
	SlowTimeout    = 30 * time.Second // For complex operations (file I/O, network)
)

// WaitConfig allows customizing wait behaviour.
type WaitConfig struct {
	Timeout      time.Duration
	PollInterval time.Duration
	Description  string // For better error messages
}

// DefaultWaitConfig provides sensible defaults.
func DefaultWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      DefaultTimeout,
		PollInterval: 100 * time.Millisecond,
		Description:  "condition",
	}
}

// FastWaitConfig is for quick operations.
func FastWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      FastTimeout,
		PollInterval: 50 * time.Millisecond,
		Description:  "condition",
	}
}

// SlowWaitConfig is for complex operations.
func SlowWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      SlowTimeout,
		PollInterval: 200 * time.Millisecond,
		Description:  "condition",
	}
}

// WaitForCondition polls a condition until it returns true or timeout occurs.
func WaitForCondition(condition func() bool, config WaitConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	// Check immediately first.
	if condition() {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s after %v", config.Description, config.Timeout)
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}

// WaitForConditionWithError polls a condition that can return an error.
func WaitForConditionWithError(condition func() (bool, error), config WaitConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	var lastErr error

	// Check immediately first.
	if ok, err := condition(); err != nil {
		lastErr = err
	} else if ok {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("timeout waiting for %s after %v (last error: %v)", config.Description, config.Timeout, lastErr)
			}
			return fmt.Errorf("timeout waiting for %s after %v", config.Description, config.Timeout)
		case <-ticker.C:
			if ok, err := condition(); err != nil {
				lastErr = err
			} else if ok {
				return nil
			}
		}
	}
}
