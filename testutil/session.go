package testutil

import (
	"claude-squad/session"
	"fmt"
)

// SessionWaiter provides utilities for waiting on session operations
type SessionWaiter struct {
	instance *session.Instance
}

// NewSessionWaiter creates a new session waiter
func NewSessionWaiter(instance *session.Instance) *SessionWaiter {
	return &SessionWaiter{instance: instance}
}

// WaitForStarted waits for the session to be started
func (w *SessionWaiter) WaitForStarted() error {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("session '%s' to be started", w.getSessionTitle())

	return WaitForCondition(func() bool {
		return w.instance.Started()
	}, config)
}

// WaitForTmuxAlive waits for the tmux session to be alive
func (w *SessionWaiter) WaitForTmuxAlive() error {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("session '%s' tmux to be alive", w.getSessionTitle())

	return WaitForCondition(func() bool {
		return w.instance.TmuxAlive()
	}, config)
}

// WaitForPreview waits for the session to have preview content
func (w *SessionWaiter) WaitForPreview(validator ContentValidator) (string, error) {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("session '%s' to have preview content", w.getSessionTitle())

	return WaitForContent(
		func() (string, error) {
			return w.instance.Preview()
		},
		validator,
		config,
	)
}

// WaitForPreviewWithConfig waits for preview with custom config
func (w *SessionWaiter) WaitForPreviewWithConfig(validator ContentValidator, config WaitConfig) (string, error) {
	if config.Description == "condition" {
		config.Description = fmt.Sprintf("session '%s' to have preview content", w.getSessionTitle())
	}

	return WaitForContent(
		func() (string, error) {
			return w.instance.Preview()
		},
		validator,
		config,
	)
}

// WaitForNonEmptyPreview waits for any non-empty preview content
func (w *SessionWaiter) WaitForNonEmptyPreview() (string, error) {
	return w.WaitForPreview(NonEmptyContent)
}

// WaitForPreviewContaining waits for preview containing specific text
func (w *SessionWaiter) WaitForPreviewContaining(text string) (string, error) {
	return w.WaitForPreview(ContainsText(text))
}

// WaitForFullyReady waits for session to be started, tmux alive, and have content
func (w *SessionWaiter) WaitForFullyReady() error {
	// Wait for session to be started
	if err := w.WaitForStarted(); err != nil {
		return fmt.Errorf("session never started: %v", err)
	}

	// Wait for tmux to be alive
	if err := w.WaitForTmuxAlive(); err != nil {
		return fmt.Errorf("tmux never became alive: %v", err)
	}

	// Wait for content to be available
	_, err := w.WaitForNonEmptyPreview()
	if err != nil {
		return fmt.Errorf("preview never became available: %v", err)
	}

	return nil
}

// WaitForFullyReadyWithTimeout waits for session with custom timeout
func (w *SessionWaiter) WaitForFullyReadyWithTimeout(config WaitConfig) error {
	// Wait for session to be started
	startedConfig := config
	startedConfig.Description = fmt.Sprintf("session '%s' to be started", w.getSessionTitle())
	if err := WaitForCondition(func() bool { return w.instance.Started() }, startedConfig); err != nil {
		return fmt.Errorf("session never started: %v", err)
	}

	// Wait for tmux to be alive
	aliveConfig := config
	aliveConfig.Description = fmt.Sprintf("session '%s' tmux to be alive", w.getSessionTitle())
	if err := WaitForCondition(func() bool { return w.instance.TmuxAlive() }, aliveConfig); err != nil {
		return fmt.Errorf("tmux never became alive: %v", err)
	}

	// Wait for content to be available
	_, err := w.WaitForPreviewWithConfig(NonEmptyContent, config)
	if err != nil {
		return fmt.Errorf("preview never became available: %v", err)
	}

	return nil
}

// getSessionTitle safely gets the session title for error messages
func (w *SessionWaiter) getSessionTitle() string {
	if w.instance == nil {
		return "<nil>"
	}
	return w.instance.Title
}

// Package-level convenience functions

// WaitForSession waits for a session to be fully ready
func WaitForSession(instance *session.Instance) error {
	return NewSessionWaiter(instance).WaitForFullyReady()
}

// WaitForSessionWithTimeout waits for session with custom timeout
func WaitForSessionWithTimeout(instance *session.Instance, config WaitConfig) error {
	return NewSessionWaiter(instance).WaitForFullyReadyWithTimeout(config)
}

// WaitForSessionPreview waits for session to have specific preview content
func WaitForSessionPreview(instance *session.Instance, validator ContentValidator) (string, error) {
	return NewSessionWaiter(instance).WaitForPreview(validator)
}

// WaitForSessionPreviewContaining waits for session preview containing text
func WaitForSessionPreviewContaining(instance *session.Instance, text string) (string, error) {
	return NewSessionWaiter(instance).WaitForPreviewContaining(text)
}
