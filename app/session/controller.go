package session

import (
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"

	tea "github.com/charmbracelet/bubbletea"
)

// SessionStorage defines the storage operations needed by the session controller
type SessionStorage interface {
	DeleteInstance(title string) error
	LoadInstances() ([]*session.Instance, error)
	SaveInstances([]*session.Instance) error
	GetStateManager() interface{}
}

// SessionOperation represents the result of a session operation
type SessionOperation struct {
	Type    SessionOperationType
	Success bool
	Error   error
	Model   tea.Model
	Cmd     tea.Cmd
}

// SessionOperationType defines the type of session operation
type SessionOperationType int

const (
	OpNewSession SessionOperationType = iota
	OpKillSession
	OpAttachSession
	OpCheckoutSession
	OpResumeSession
)

// String returns the string representation of the operation type
func (op SessionOperationType) String() string {
	switch op {
	case OpNewSession:
		return "NewSession"
	case OpKillSession:
		return "KillSession"
	case OpAttachSession:
		return "AttachSession"
	case OpCheckoutSession:
		return "CheckoutSession"
	case OpResumeSession:
		return "ResumeSession"
	default:
		return "Unknown"
	}
}

// DiscoveryConfig represents discovery configuration interface
type DiscoveryConfig interface {
	IsExternalDiscoveryEnabled() bool
	IsManagedDiscoveryEnabled() bool
	ShouldConfirmOperation(isExternal bool) bool
	CanAttachToExternal() bool
}

// Dependencies represents the external dependencies needed by the session controller
type Dependencies struct {
	// Core components
	List    *ui.List
	Storage SessionStorage

	// UI state management
	ErrorHandler    func(error) tea.Cmd
	StateTransition func(to string) error
	InstanceChanged func() tea.Cmd
	ConfirmAction   func(message string, action tea.Cmd) tea.Cmd
	ShowHelpScreen  func(helpType interface{}, onComplete func())

	// Configuration
	AutoYes             bool
	GlobalInstanceLimit int
	TmuxPrefix          string
	DiscoveryConfig     DiscoveryConfig

	// Overlay management
	SetSessionSetupOverlay func(*overlay.SessionSetupOverlay)
	GetSessionSetupOverlay func() *overlay.SessionSetupOverlay

	// Async session creation
	SetPendingSession  func(*session.Instance, bool)
	GetNewInstanceFinalizer func() func()
	SetNewInstanceFinalizer func(func())
}

// Controller defines the interface for session operations
type Controller interface {
	// Core session operations
	NewSession() SessionOperation
	KillSession() SessionOperation
	AttachSession() SessionOperation
	CheckoutSession() SessionOperation
	ResumeSession() SessionOperation

	// Helper methods
	ValidateNewSession() error
	GetSelectedSession() *session.Instance
	CanPerformOperation(op SessionOperationType) bool

	// State management
	SetDependencies(deps Dependencies)
	GetDependencies() Dependencies
}

// controller implements the Controller interface
type controller struct {
	deps Dependencies
}

// NewController creates a new session controller with the provided dependencies
func NewController(deps Dependencies) Controller {
	return &controller{
		deps: deps,
	}
}

// SetDependencies updates the controller dependencies
func (c *controller) SetDependencies(deps Dependencies) {
	c.deps = deps
}

// GetDependencies returns the current controller dependencies
func (c *controller) GetDependencies() Dependencies {
	return c.deps
}

// ValidateNewSession checks if a new session can be created
func (c *controller) ValidateNewSession() error {
	if c.deps.List.NumInstances() >= c.deps.GlobalInstanceLimit {
		return &SessionError{
			Operation: OpNewSession,
			Message:   "instance limit reached",
			Details:   map[string]interface{}{"limit": c.deps.GlobalInstanceLimit},
		}
	}
	return nil
}

// GetSelectedSession returns the currently selected session instance
func (c *controller) GetSelectedSession() *session.Instance {
	return c.deps.List.GetSelectedInstance()
}

// CanPerformOperation checks if the specified operation can be performed
func (c *controller) CanPerformOperation(op SessionOperationType) bool {
	switch op {
	case OpNewSession:
		return c.ValidateNewSession() == nil

	case OpKillSession, OpCheckoutSession, OpResumeSession:
		selected := c.GetSelectedSession()
		return selected != nil

	case OpAttachSession:
		if c.deps.List.NumInstances() == 0 {
			return false
		}
		selected := c.GetSelectedSession()
		return selected != nil && !selected.Paused() && selected.TmuxAlive()

	default:
		return false
	}
}

// SessionError represents an error that occurred during a session operation
type SessionError struct {
	Operation SessionOperationType
	Message   string
	Details   map[string]interface{}
	Cause     error
}

// Error implements the error interface
func (e *SessionError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Unwrap implements the unwrap interface for error chaining
func (e *SessionError) Unwrap() error {
	return e.Cause
}