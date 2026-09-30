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
// config.Timeout is scaled by ScaleTimeout before use, so the effective bound
// stretches under real machine load instead of assuming an idle CPU.
func WaitForCondition(condition func() bool, config WaitConfig) error {
	timeout := ScaleTimeout(config.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	// Check immediately first.
	if condition() {
		return nil
	}

	tr := timeoutReport{description: config.Description, baseTimeout: config.Timeout, scaledTimeout: timeout, pollInterval: config.PollInterval, start: time.Now()}
	for {
		select {
		case <-ctx.Done():
			return tr.err()
		case <-ticker.C:
			tr.polls++
			if condition() {
				return nil
			}
		}
	}
}

// WaitForConditionWithError polls a condition that can return an error.
// config.Timeout is scaled by ScaleTimeout before use, so the effective bound
// stretches under real machine load instead of assuming an idle CPU.
func WaitForConditionWithError(condition func() (bool, error), config WaitConfig) error {
	timeout := ScaleTimeout(config.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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

	tr := timeoutReport{description: config.Description, baseTimeout: config.Timeout, scaledTimeout: timeout, pollInterval: config.PollInterval, start: time.Now()}
	for {
		select {
		case <-ctx.Done():
			tr.lastErr = lastErr
			return tr.err()
		case <-ticker.C:
			tr.polls++
			if ok, err := condition(); err != nil {
				lastErr = err
			} else if ok {
				return nil
			}
		}
	}
}

// timeoutReport builds a timeout error that reports enough for a human to
// tell "this was scheduler contention" from "this condition genuinely never
// became true" without re-running anything: the scaled vs. base timeout
// (non-1.0 scaling implicates load), and how many polls actually happened
// vs. how many the poll interval alone would predict -- a poll count far
// below that expectation means ticks themselves were delayed by scheduler
// contention, not that the condition was checked and found false repeatedly.
type timeoutReport struct {
	description                string
	baseTimeout, scaledTimeout time.Duration
	pollInterval               time.Duration
	start                      time.Time
	polls                      int
	lastErr                    error
}

func (tr timeoutReport) err() error {
	elapsed := time.Since(tr.start)
	expectedPolls := int(elapsed / tr.pollInterval)
	scaleNote := ""
	if tr.scaledTimeout != tr.baseTimeout {
		scaleNote = fmt.Sprintf(" (base %v, scaled to %v for load)", tr.baseTimeout, tr.scaledTimeout)
	}
	contentionNote := ""
	if expectedPolls > 0 && tr.polls < expectedPolls/2 {
		contentionNote = fmt.Sprintf(" -- only %d of ~%d expected polls ran, indicating scheduler contention delayed ticks rather than the condition being repeatedly false", tr.polls, expectedPolls)
	}
	if tr.lastErr != nil {
		return fmt.Errorf("timeout waiting for %s after %v%s (last error: %v)%s", tr.description, elapsed, scaleNote, tr.lastErr, contentionNote)
	}
	return fmt.Errorf("timeout waiting for %s after %v%s%s", tr.description, elapsed, scaleNote, contentionNote)
}
