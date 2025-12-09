package services

import (
	appsession "claude-squad/app/session"
	"claude-squad/session"
	"claude-squad/ui"
	"errors"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// SessionManagementService handles all session lifecycle operations
// This service is responsible for creating, killing, attaching, and resuming sessions
type SessionManagementService interface {
	// Core session operations
	CreateSession() SessionResult
	KillSession() SessionResult
	AttachSession() SessionResult
	ResumeSession() SessionResult
	CheckoutSession() SessionResult

	// Validation and queries
	ValidateNewSession() error
	GetActiveSessionsCount() int
	GetSelectedSession() *session.Instance
	CanPerformOperation(op appsession.SessionOperationType) bool
}

// SessionResult represents the result of a session operation
type SessionResult struct {
	Success bool
	Error   error
	Model   tea.Model
	Cmd     tea.Cmd
}

// sessionManagementService implements SessionManagementService
type sessionManagementService struct {
	mu                sync.Mutex
	storage           *session.Storage
	sessionController appsession.Controller
	list              *ui.List
	errorHandler      func(error) tea.Cmd
	instanceLimit     int
}

// NewSessionManagementService creates a new session management service
func NewSessionManagementService(
	storage *session.Storage,
	controller appsession.Controller,
	list *ui.List,
	errorHandler func(error) tea.Cmd,
	instanceLimit int,
) SessionManagementService {
	return &sessionManagementService{
		storage:           storage,
		sessionController: controller,
		list:              list,
		errorHandler:      errorHandler,
		instanceLimit:     instanceLimit,
	}
}

// CreateSession creates a new session
// This method is protected with a mutex to prevent race conditions during concurrent creation attempts
func (s *sessionManagementService) CreateSession() SessionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Validate before creation
	if err := s.ValidateNewSession(); err != nil {
		return SessionResult{
			Success: false,
			Error:   err,
		}
	}

	// Delegate to session controller
	result := s.sessionController.NewSession()
	return SessionResult{
		Success: result.Success,
		Error:   result.Error,
		Model:   result.Model,
		Cmd:     result.Cmd,
	}
}

// KillSession kills the currently selected session
func (s *sessionManagementService) KillSession() SessionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.sessionController.KillSession()
	return SessionResult{
		Success: result.Success,
		Error:   result.Error,
		Model:   result.Model,
		Cmd:     result.Cmd,
	}
}

// AttachSession attaches to the currently selected session
func (s *sessionManagementService) AttachSession() SessionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.sessionController.AttachSession()
	return SessionResult{
		Success: result.Success,
		Error:   result.Error,
		Model:   result.Model,
		Cmd:     result.Cmd,
	}
}

// ResumeSession resumes a paused session
func (s *sessionManagementService) ResumeSession() SessionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.sessionController.ResumeSession()
	return SessionResult{
		Success: result.Success,
		Error:   result.Error,
		Model:   result.Model,
		Cmd:     result.Cmd,
	}
}

// CheckoutSession performs a git checkout operation on the selected session
func (s *sessionManagementService) CheckoutSession() SessionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.sessionController.CheckoutSession()
	return SessionResult{
		Success: result.Success,
		Error:   result.Error,
		Model:   result.Model,
		Cmd:     result.Cmd,
	}
}

// ValidateNewSession checks if a new session can be created
// This includes checking the instance limit
func (s *sessionManagementService) ValidateNewSession() error {
	// Check instance limit (already locked by caller)
	if s.list.NumInstances() >= s.instanceLimit {
		return errors.New("instance limit reached")
	}

	// Delegate to session controller for additional validation
	return s.sessionController.ValidateNewSession()
}

// GetActiveSessionsCount returns the current number of active sessions
func (s *sessionManagementService) GetActiveSessionsCount() int {
	return s.list.NumInstances()
}

// GetSelectedSession returns the currently selected session instance
func (s *sessionManagementService) GetSelectedSession() *session.Instance {
	return s.sessionController.GetSelectedSession()
}

// CanPerformOperation checks if a session operation can be performed
func (s *sessionManagementService) CanPerformOperation(op appsession.SessionOperationType) bool {
	return s.sessionController.CanPerformOperation(op)
}
