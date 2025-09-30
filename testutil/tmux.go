package testutil

import (
	"claude-squad/session/tmux"
	"fmt"
)

// TmuxWaiter provides utilities for waiting on tmux operations
type TmuxWaiter struct {
	session *tmux.TmuxSession
}

// NewTmuxWaiter creates a new tmux waiter
func NewTmuxWaiter(session *tmux.TmuxSession) *TmuxWaiter {
	return &TmuxWaiter{session: session}
}

// WaitForSessionExists waits for the tmux session to exist
func (w *TmuxWaiter) WaitForSessionExists() error {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("tmux session '%s' to exist", w.getSessionName())

	return WaitForCondition(func() bool {
		return w.session.DoesSessionExist()
	}, config)
}

// WaitForSessionExistsWithConfig waits for session with custom config
func (w *TmuxWaiter) WaitForSessionExistsWithConfig(config WaitConfig) error {
	if config.Description == "condition" {
		config.Description = fmt.Sprintf("tmux session '%s' to exist", w.getSessionName())
	}

	return WaitForCondition(func() bool {
		return w.session.DoesSessionExist()
	}, config)
}

// WaitForContent waits for tmux session to have specific content
func (w *TmuxWaiter) WaitForContent(validator ContentValidator) (string, error) {
	config := DefaultWaitConfig()
	config.Description = fmt.Sprintf("tmux session '%s' to have expected content", w.getSessionName())

	return WaitForContent(
		func() (string, error) {
			return w.session.CapturePaneContent()
		},
		validator,
		config,
	)
}

// WaitForContentWithConfig waits for content with custom config
func (w *TmuxWaiter) WaitForContentWithConfig(validator ContentValidator, config WaitConfig) (string, error) {
	if config.Description == "condition" {
		config.Description = fmt.Sprintf("tmux session '%s' to have expected content", w.getSessionName())
	}

	return WaitForContent(
		func() (string, error) {
			return w.session.CapturePaneContent()
		},
		validator,
		config,
	)
}

// WaitForNonEmptyContent waits for any non-empty content
func (w *TmuxWaiter) WaitForNonEmptyContent() (string, error) {
	return w.WaitForContent(NonEmptyContent)
}

// WaitForContentContaining waits for content containing specific text
func (w *TmuxWaiter) WaitForContentContaining(text string) (string, error) {
	return w.WaitForContent(ContainsText(text))
}

// WaitForSessionReady waits for session to exist and have content
func (w *TmuxWaiter) WaitForSessionReady() error {
	// First wait for session to exist
	if err := w.WaitForSessionExists(); err != nil {
		return fmt.Errorf("session never became available: %v", err)
	}

	// Then wait for it to have content (indicating it's running)
	_, err := w.WaitForNonEmptyContent()
	if err != nil {
		return fmt.Errorf("session never produced content: %v", err)
	}

	return nil
}

// WaitForSessionReadyWithTimeout waits for session with custom timeout
func (w *TmuxWaiter) WaitForSessionReadyWithTimeout(config WaitConfig) error {
	// First wait for session to exist
	existsConfig := config
	existsConfig.Description = fmt.Sprintf("tmux session '%s' to exist", w.getSessionName())
	if err := w.WaitForSessionExistsWithConfig(existsConfig); err != nil {
		return fmt.Errorf("session never became available: %v", err)
	}

	// Then wait for it to have content
	contentConfig := config
	contentConfig.Description = fmt.Sprintf("tmux session '%s' to have content", w.getSessionName())
	_, err := w.WaitForContentWithConfig(NonEmptyContent, contentConfig)
	if err != nil {
		return fmt.Errorf("session never produced content: %v", err)
	}

	return nil
}

// getSessionName safely gets the session name for error messages
func (w *TmuxWaiter) getSessionName() string {
	if w.session == nil {
		return "<nil>"
	}
	// We can't access the private sanitizedName field, so use a generic name
	return "session"
}

// Package-level convenience functions

// WaitForTmuxSession waits for a tmux session to be ready
func WaitForTmuxSession(session *tmux.TmuxSession) error {
	return NewTmuxWaiter(session).WaitForSessionReady()
}

// WaitForTmuxSessionWithTimeout waits for tmux session with custom timeout
func WaitForTmuxSessionWithTimeout(session *tmux.TmuxSession, config WaitConfig) error {
	return NewTmuxWaiter(session).WaitForSessionReadyWithTimeout(config)
}

// WaitForTmuxContent waits for tmux session to have specific content
func WaitForTmuxContent(session *tmux.TmuxSession, validator ContentValidator) (string, error) {
	return NewTmuxWaiter(session).WaitForContent(validator)
}

// WaitForTmuxContentContaining waits for tmux content containing text
func WaitForTmuxContentContaining(session *tmux.TmuxSession, text string) (string, error) {
	return NewTmuxWaiter(session).WaitForContentContaining(text)
}
