package testutil

import (
	"context"
	"fmt"
	"time"
)

// Common timeout durations for different test scenarios
var (
	FastTimeout    = 2 * time.Second  // For unit tests and quick operations
	DefaultTimeout = 10 * time.Second // For most integration tests
	SlowTimeout    = 30 * time.Second // For complex operations (file I/O, network)
)

// WaitConfig allows customizing wait behavior
type WaitConfig struct {
	Timeout      time.Duration
	PollInterval time.Duration
	Description  string // For better error messages
}

// DefaultWaitConfig provides sensible defaults
func DefaultWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      DefaultTimeout,
		PollInterval: 100 * time.Millisecond,
		Description:  "condition",
	}
}

// FastWaitConfig for quick operations
func FastWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      FastTimeout,
		PollInterval: 50 * time.Millisecond,
		Description:  "condition",
	}
}

// SlowWaitConfig for complex operations
func SlowWaitConfig() WaitConfig {
	return WaitConfig{
		Timeout:      SlowTimeout,
		PollInterval: 200 * time.Millisecond,
		Description:  "condition",
	}
}

// WaitForCondition polls a condition until it returns true or timeout occurs
func WaitForCondition(condition func() bool, config WaitConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	// Check immediately first
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

// WaitForConditionWithError polls a condition that can return an error
func WaitForConditionWithError(condition func() (bool, error), config WaitConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	ticker := time.NewTicker(config.PollInterval)
	defer ticker.Stop()

	var lastErr error

	// Check immediately first
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

// ContentValidator is a function that validates content
type ContentValidator func(content string) bool

// NonEmptyContent validates that content is not empty
func NonEmptyContent(content string) bool {
	return content != ""
}

// ContainsText validates that content contains specific text
func ContainsText(text string) ContentValidator {
	return func(content string) bool {
		return len(content) > 0 && len(text) > 0 &&
			contains(content, text)
	}
}

// contains is a simple substring check helper
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) &&
		findSubstring(s, substr))
}

// findSubstring implements basic substring search
func findSubstring(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// WaitForContent polls a content getter until the validator returns true
func WaitForContent(getter func() (string, error), validator ContentValidator, config WaitConfig) (string, error) {
	var lastContent string
	var lastErr error

	condition := func() (bool, error) {
		content, err := getter()
		if err != nil {
			lastErr = err
			return false, err
		}
		lastContent = content
		return validator(content), nil
	}

	err := WaitForConditionWithError(condition, config)
	if err != nil {
		if lastErr != nil {
			return "", fmt.Errorf("failed to get content: %v", lastErr)
		}
		return lastContent, fmt.Errorf("content validation failed: %v (last content: %q)", err, lastContent)
	}

	return lastContent, nil
}

// RetryOperation retries an operation with exponential backoff
func RetryOperation(operation func() error, config WaitConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	var lastErr error
	backoff := config.PollInterval

	for {
		err := operation()
		if err == nil {
			return nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return fmt.Errorf("operation failed after %v: %v", config.Timeout, lastErr)
		case <-time.After(backoff):
			// Exponential backoff with max cap
			backoff = time.Duration(float64(backoff) * 1.5)
			if backoff > time.Second {
				backoff = time.Second
			}
		}
	}
}
