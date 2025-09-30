package ui

import (
	"claude-squad/ui"
	"claude-squad/ui/overlay"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// ComponentType represents the type of UI component
type ComponentType int

const (
	// Core UI components
	ComponentList ComponentType = iota
	ComponentMenu
	ComponentTabbedWindow
	ComponentErrBox
	ComponentSpinner
	ComponentStatusBar

	// Overlay components
	ComponentTextInputOverlay
	ComponentLiveSearchOverlay
	ComponentTextOverlay
	ComponentMessagesOverlay
	ComponentConfirmationOverlay
	ComponentSessionSetupOverlay
	ComponentGitStatusOverlay
	ComponentClaudeSettingsOverlay
	ComponentZFSearchOverlay
)

// String returns the string representation of the component type
func (c ComponentType) String() string {
	switch c {
	case ComponentList:
		return "List"
	case ComponentMenu:
		return "Menu"
	case ComponentTabbedWindow:
		return "TabbedWindow"
	case ComponentErrBox:
		return "ErrBox"
	case ComponentSpinner:
		return "Spinner"
	case ComponentStatusBar:
		return "StatusBar"
	case ComponentTextInputOverlay:
		return "TextInputOverlay"
	case ComponentLiveSearchOverlay:
		return "LiveSearchOverlay"
	case ComponentTextOverlay:
		return "TextOverlay"
	case ComponentMessagesOverlay:
		return "MessagesOverlay"
	case ComponentConfirmationOverlay:
		return "ConfirmationOverlay"
	case ComponentSessionSetupOverlay:
		return "SessionSetupOverlay"
	case ComponentGitStatusOverlay:
		return "GitStatusOverlay"
	case ComponentClaudeSettingsOverlay:
		return "ClaudeSettingsOverlay"
	case ComponentZFSearchOverlay:
		return "ZFSearchOverlay"
	default:
		return "Unknown"
	}
}

// IsOverlay returns true if the component is an overlay component
func (c ComponentType) IsOverlay() bool {
	switch c {
	case ComponentTextInputOverlay, ComponentLiveSearchOverlay, ComponentTextOverlay,
		ComponentMessagesOverlay, ComponentConfirmationOverlay, ComponentSessionSetupOverlay,
		ComponentGitStatusOverlay, ComponentClaudeSettingsOverlay, ComponentZFSearchOverlay:
		return true
	default:
		return false
	}
}

// ComponentState represents the current state of a UI component
type ComponentState struct {
	Type      ComponentType
	Visible   bool
	Active    bool
	Focused   bool
	Error     string
	LastUpdate int64
}

// ComponentRegistry holds references to all UI components
type ComponentRegistry struct {
	// Core UI components
	List         *ui.List
	Menu         *ui.Menu
	TabbedWindow *ui.TabbedWindow
	ErrBox       *ui.ErrBox
	Spinner      spinner.Model
	StatusBar    *ui.StatusBar

	// Overlay components
	TextInputOverlay        *overlay.TextInputOverlay
	LiveSearchOverlay       *overlay.LiveSearchOverlay
	TextOverlay             *overlay.TextOverlay
	MessagesOverlay         *overlay.MessagesOverlay
	ConfirmationOverlay     *overlay.ConfirmationOverlay
	SessionSetupOverlay     *overlay.SessionSetupOverlay
	GitStatusOverlay        *overlay.GitStatusOverlay
	ClaudeSettingsOverlay   *overlay.ClaudeSettingsOverlay
	ZFSearchOverlay         *overlay.ZFSearchOverlay

	// Component states
	states map[ComponentType]*ComponentState
}

// NewComponentRegistry creates a new component registry
func NewComponentRegistry() *ComponentRegistry {
	return &ComponentRegistry{
		states: make(map[ComponentType]*ComponentState),
	}
}

// GetState returns the state of a component
func (r *ComponentRegistry) GetState(componentType ComponentType) *ComponentState {
	if state, exists := r.states[componentType]; exists {
		return state
	}

	// Create default state if not exists
	state := &ComponentState{
		Type:    componentType,
		Visible: true,
		Active:  false,
		Focused: false,
	}
	r.states[componentType] = state
	return state
}

// SetState updates the state of a component
func (r *ComponentRegistry) SetState(componentType ComponentType, state *ComponentState) {
	r.states[componentType] = state
}

// MessageRouter handles routing BubbleTea messages to appropriate components
type MessageRouter interface {
	// RouteMessage routes a message to the appropriate component
	RouteMessage(msg tea.Msg) (tea.Model, tea.Cmd)

	// RouteKeyMessage routes key messages with component-specific handling
	RouteKeyMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd)

	// RouteUpdateMessage routes update messages to components
	RouteUpdateMessage(msg tea.Msg, componentType ComponentType) (tea.Model, tea.Cmd)
}

// LifecycleManager handles component lifecycle operations
type LifecycleManager interface {
	// Initialize sets up all UI components
	Initialize() error

	// Cleanup performs cleanup for all components
	Cleanup() error

	// InitializeComponent initializes a specific component
	InitializeComponent(componentType ComponentType) error

	// CleanupComponent cleans up a specific component
	CleanupComponent(componentType ComponentType) error

	// ResetComponent resets a component to its initial state
	ResetComponent(componentType ComponentType) error
}

// OverlayManager handles overlay-specific operations
type OverlayManager interface {
	// ShowOverlay displays the specified overlay
	ShowOverlay(componentType ComponentType) error

	// HideOverlay closes the specified overlay
	HideOverlay(componentType ComponentType) error

	// IsOverlayVisible returns true if the overlay is currently visible
	IsOverlayVisible(componentType ComponentType) bool

	// GetActiveOverlay returns the currently active overlay, if any
	GetActiveOverlay() (ComponentType, bool)

	// CloseAllOverlays closes all active overlays
	CloseAllOverlays() error
}

// ViewRenderer handles view rendering coordination
type ViewRenderer interface {
	// RenderMain renders the main application view
	RenderMain() string

	// RenderOverlay renders the specified overlay
	RenderOverlay(componentType ComponentType) string

	// RenderWithLayout renders components with proper layout
	RenderWithLayout() string

	// GetComponentView gets the view for a specific component
	GetComponentView(componentType ComponentType) string
}