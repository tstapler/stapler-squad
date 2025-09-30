package state

import (
	"fmt"
)

// TransitionContext contains data associated with state transitions
type TransitionContext struct {
	// Menu state synchronization
	MenuState string

	// Overlay management
	ShouldCloseOverlay bool
	OverlayName        string

	// Session management
	ShouldKillSession bool
	SessionActive     bool

	// Error handling
	ErrorMessage string

	// Custom actions to execute during transition
	PreTransitionAction  func() error
	PostTransitionAction func() error
}

// TransitionResult contains the result of a state transition
type TransitionResult struct {
	Success      bool
	PreviousState State
	NewState     State
	Context      TransitionContext
	Error        string
}

// StateTransitionHandler handles state transitions with side effects
type StateTransitionHandler interface {
	// TransitionWithContext performs state transition with associated context
	TransitionWithContext(to State, ctx TransitionContext) (*TransitionResult, error)

	// TransitionToDefault transitions to default state with cleanup
	TransitionToDefault(ctx TransitionContext) (*TransitionResult, error)

	// TransitionToOverlay transitions to an overlay state with setup
	TransitionToOverlay(to State, ctx TransitionContext) (*TransitionResult, error)

	// GetTransitionContext gets the current transition context
	GetTransitionContext() TransitionContext
}

// Manager provides centralized state management for the application
type Manager interface {
	// Current returns the current application state
	Current() State

	// Transition attempts to change state from current to target state
	Transition(to State) error

	// CanTransition returns true if transition from current state to target is valid
	CanTransition(to State) bool

	// IsValid returns true if the given state is a valid state value
	IsValid(state State) bool

	// Reset returns to the default state
	Reset()

	// IsOverlay returns true if current state is an overlay/modal state
	IsOverlay() bool

	// IsAsync returns true if current state is an asynchronous operation
	IsAsync() bool

	// StateTransitionHandler methods
	StateTransitionHandler

	// ViewManager methods
	ViewManager

	// Common transition pattern methods
	TransitionToConfirm(message string) (*TransitionResult, error)
	TransitionToHelp(helpType string) (*TransitionResult, error)
	TransitionToCreatingSession() (*TransitionResult, error)
	TransitionWithCleanup(overlayName string) (*TransitionResult, error)
}

// manager implements the Manager interface
type manager struct {
	current           State
	transitionContext TransitionContext
}

// NewManager creates a new state manager initialized to Default state
func NewManager() Manager {
	return &manager{
		current:           Default,
		transitionContext: TransitionContext{},
	}
}

// Current returns the current application state
func (m *manager) Current() State {
	return m.current
}

// Transition attempts to change state from current to target state
func (m *manager) Transition(to State) error {
	if !m.IsValid(to) {
		return fmt.Errorf("invalid target state: %v", to)
	}

	if !m.CanTransition(to) {
		return fmt.Errorf("invalid state transition from %s to %s", m.current.String(), to.String())
	}

	m.current = to
	return nil
}

// CanTransition returns true if transition from current state to target is valid
func (m *manager) CanTransition(to State) bool {
	// Validate target state first
	if !to.IsValid() {
		return false
	}

	// All states can transition to Default (escape/cancel behavior)
	if to == Default {
		return true
	}

	// From Default, can transition to any overlay or async state
	if m.current == Default {
		return to.IsOverlayState() || to.IsAsyncState()
	}

	// From overlay states, can transition to Default or CreatingSession
	if m.current.IsOverlayState() {
		return to == Default || to == CreatingSession
	}

	// From async states, can transition to Default or Prompt (for post-creation actions)
	if m.current.IsAsyncState() {
		return to == Default || to == Prompt
	}

	// Default: allow the transition (permissive approach)
	return true
}

// IsValid returns true if the given state is a valid state value
func (m *manager) IsValid(state State) bool {
	return state.IsValid()
}

// Reset returns to the default state
func (m *manager) Reset() {
	m.current = Default
}

// IsOverlay returns true if current state is an overlay/modal state
func (m *manager) IsOverlay() bool {
	return m.current.IsOverlayState()
}

// IsAsync returns true if current state is an asynchronous operation
func (m *manager) IsAsync() bool {
	return m.current.IsAsyncState()
}

// TransitionValidation provides detailed validation for state transitions
type TransitionValidation struct {
	From    State
	To      State
	Valid   bool
	Reason  string
}

// ValidateTransition provides detailed validation information for a potential transition
func (m *manager) ValidateTransition(to State) TransitionValidation {
	validation := TransitionValidation{
		From:  m.current,
		To:    to,
		Valid: true,
		Reason: "Valid transition",
	}

	if !to.IsValid() {
		validation.Valid = false
		validation.Reason = fmt.Sprintf("Invalid target state: %v", to)
		return validation
	}

	if !m.CanTransition(to) {
		validation.Valid = false
		validation.Reason = fmt.Sprintf("Invalid transition from %s to %s", m.current.String(), to.String())
		return validation
	}

	return validation
}

// TransitionWithContext performs state transition with associated context
func (m *manager) TransitionWithContext(to State, ctx TransitionContext) (*TransitionResult, error) {
	result := &TransitionResult{
		PreviousState: m.current,
		NewState:      to,
		Context:       ctx,
	}

	// Validate the transition
	if !m.CanTransition(to) {
		result.Success = false
		result.Error = fmt.Sprintf("Invalid transition from %s to %s", m.current.String(), to.String())
		return result, fmt.Errorf("%s", result.Error)
	}

	// Execute pre-transition action if provided
	if ctx.PreTransitionAction != nil {
		if err := ctx.PreTransitionAction(); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("Pre-transition action failed: %v", err)
			return result, err
		}
	}

	// Perform the state transition
	previousState := m.current
	m.current = to
	m.transitionContext = ctx

	// Execute post-transition action if provided
	if ctx.PostTransitionAction != nil {
		if err := ctx.PostTransitionAction(); err != nil {
			// Rollback the state change on post-transition failure
			m.current = previousState
			result.Success = false
			result.Error = fmt.Sprintf("Post-transition action failed: %v", err)
			return result, err
		}
	}

	result.Success = true
	return result, nil
}

// TransitionToDefault transitions to default state with cleanup
func (m *manager) TransitionToDefault(ctx TransitionContext) (*TransitionResult, error) {
	// Always allow transitions to Default (escape behavior)
	defaultCtx := ctx
	defaultCtx.MenuState = "Default" // Ensure menu is reset to default

	return m.TransitionWithContext(Default, defaultCtx)
}

// TransitionToOverlay transitions to an overlay state with setup
func (m *manager) TransitionToOverlay(to State, ctx TransitionContext) (*TransitionResult, error) {
	if !to.IsOverlayState() {
		return nil, fmt.Errorf("Cannot transition to non-overlay state %s using TransitionToOverlay", to.String())
	}

	return m.TransitionWithContext(to, ctx)
}

// GetTransitionContext gets the current transition context
func (m *manager) GetTransitionContext() TransitionContext {
	return m.transitionContext
}

// Common transition patterns

// TransitionToConfirm creates a confirmation dialog state transition
func (m *manager) TransitionToConfirm(message string) (*TransitionResult, error) {
	ctx := TransitionContext{
		MenuState:   "Confirm",
		OverlayName: "confirmation",
	}
	return m.TransitionToOverlay(Confirm, ctx)
}

// TransitionToHelp creates a help dialog state transition
func (m *manager) TransitionToHelp(helpType string) (*TransitionResult, error) {
	ctx := TransitionContext{
		MenuState:   "Help",
		OverlayName: "help",
	}
	return m.TransitionToOverlay(Help, ctx)
}

// TransitionToCreatingSession creates an async session creation state transition
func (m *manager) TransitionToCreatingSession() (*TransitionResult, error) {
	ctx := TransitionContext{
		MenuState:     "CreatingInstance",
		SessionActive: true,
	}
	return m.TransitionWithContext(CreatingSession, ctx)
}

// TransitionWithCleanup transitions to default state with overlay cleanup
func (m *manager) TransitionWithCleanup(overlayName string) (*TransitionResult, error) {
	ctx := TransitionContext{
		MenuState:          "Default",
		ShouldCloseOverlay: true,
		OverlayName:        overlayName,
	}
	return m.TransitionToDefault(ctx)
}