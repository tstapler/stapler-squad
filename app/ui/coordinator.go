package ui

import (
	"claude-squad/app/state"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Coordinator provides centralized UI component management and coordination
type Coordinator interface {
	// Component management
	MessageRouter
	LifecycleManager
	OverlayManager
	ViewRenderer

	// Coordinator-specific methods
	GetRegistry() *ComponentRegistry
	SetStateManager(stateManager state.Manager)
	SetContext(ctx context.Context)
	UpdateLayout(width, height int)
	HandleResize(width, height int)

	// Component initialization and management
	InitializeComponents(autoYes bool, appState interface{}) error
	GetComponentByType(componentType ComponentType) interface{}
	SetComponent(componentType ComponentType, component interface{}) error

	// Overlay creation and management methods
	CreateTextInputOverlay(title, initialValue string) error
	CreateLiveSearchOverlay(title, initialQuery string) error
	CreateConfirmationOverlay(message string) error
	CreateSessionSetupOverlay() error
	CreateMessagesOverlay(messages []overlay.StatusMessage) error
	CreateZFSearchOverlay(title, placeholder string, directories []string) error
	CreateGitStatusOverlay() error
	CreateClaudeSettingsOverlay(settings session.ClaudeSettings, availableSessions []session.ClaudeSession) error

	// Overlay accessor methods
	GetTextInputOverlay() *overlay.TextInputOverlay
	GetLiveSearchOverlay() *overlay.LiveSearchOverlay
	GetConfirmationOverlay() *overlay.ConfirmationOverlay
	GetSessionSetupOverlay() *overlay.SessionSetupOverlay
	GetMessagesOverlay() *overlay.MessagesOverlay
	GetZFSearchOverlay() *overlay.ZFSearchOverlay
	GetGitStatusOverlay() *overlay.GitStatusOverlay
	GetClaudeSettingsOverlay() *overlay.ClaudeSettingsOverlay
}

// coordinator implements the Coordinator interface
type coordinator struct {
	registry     *ComponentRegistry
	stateManager state.Manager
	ctx          context.Context
	config       *config.Config

	// Layout information
	width  int
	height int

	// Active overlay tracking
	activeOverlay ComponentType
	overlayActive bool

	// Initialization state
	initialized bool
}

// NewCoordinator creates a new UI coordinator
func NewCoordinator() Coordinator {
	return &coordinator{
		registry:      NewComponentRegistry(),
		width:         80,
		height:        24,
		activeOverlay: ComponentType(-1), // No active overlay
		overlayActive: false,
		initialized:   false,
	}
}

// GetRegistry returns the component registry
func (c *coordinator) GetRegistry() *ComponentRegistry {
	return c.registry
}

// SetStateManager sets the state manager for state-aware UI coordination
func (c *coordinator) SetStateManager(stateManager state.Manager) {
	c.stateManager = stateManager
}

// SetContext sets the application context
func (c *coordinator) SetContext(ctx context.Context) {
	c.ctx = ctx
}

// UpdateLayout updates the layout dimensions
func (c *coordinator) UpdateLayout(width, height int) {
	c.width = width
	c.height = height

	// Update component sizes if initialized
	if c.initialized {
		c.updateComponentSizes()
	}
}

// HandleResize handles terminal resize events
func (c *coordinator) HandleResize(width, height int) {
	c.UpdateLayout(width, height)
}

// Initialize sets up all UI components
func (c *coordinator) Initialize() error {
	if c.initialized {
		return nil
	}

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	c.registry.Spinner = s

	c.initialized = true
	return nil
}

// InitializeComponents initializes all components with their dependencies
func (c *coordinator) InitializeComponents(autoYes bool, appState interface{}) error {
	if !c.initialized {
		if err := c.Initialize(); err != nil {
			return err
		}
	}

	// Initialize core components
	c.registry.Menu = ui.NewMenu()
	c.registry.TabbedWindow = ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane())
	c.registry.ErrBox = ui.NewErrBox()
	c.registry.StatusBar = ui.NewStatusBar()

	// List needs special handling due to spinner dependency and type assertion
	// Note: This will be set from app.go during populateCoordinatorRegistry
	// c.registry.List = ui.NewList(&c.registry.Spinner, autoYes, appState)

	// Initialize component states
	for _, componentType := range []ComponentType{
		ComponentMenu, ComponentTabbedWindow, ComponentErrBox, ComponentStatusBar,
	} {
		c.InitializeComponent(componentType)
	}

	return nil
}

// Cleanup performs cleanup for all components
func (c *coordinator) Cleanup() error {
	// Close all overlays
	if err := c.CloseAllOverlays(); err != nil {
		log.Printf("Error closing overlays during cleanup: %v", err)
	}

	// Reset states
	c.registry.states = make(map[ComponentType]*ComponentState)
	c.activeOverlay = ComponentType(-1)
	c.overlayActive = false
	c.initialized = false

	return nil
}

// InitializeComponent initializes a specific component
func (c *coordinator) InitializeComponent(componentType ComponentType) error {
	state := c.registry.GetState(componentType)
	state.Visible = true
	state.Active = false
	state.LastUpdate = time.Now().Unix()
	c.registry.SetState(componentType, state)

	return nil
}

// CleanupComponent cleans up a specific component
func (c *coordinator) CleanupComponent(componentType ComponentType) error {
	if componentType.IsOverlay() {
		return c.HideOverlay(componentType)
	}

	state := c.registry.GetState(componentType)
	state.Active = false
	state.Visible = false
	c.registry.SetState(componentType, state)

	return nil
}

// ResetComponent resets a component to its initial state
func (c *coordinator) ResetComponent(componentType ComponentType) error {
	return c.InitializeComponent(componentType)
}

// ShowOverlay displays the specified overlay
func (c *coordinator) ShowOverlay(componentType ComponentType) error {
	if !componentType.IsOverlay() {
		return fmt.Errorf("component %s is not an overlay", componentType.String())
	}

	// Close any existing overlay first
	if c.overlayActive {
		if err := c.HideOverlay(c.activeOverlay); err != nil {
			log.Printf("Error closing existing overlay: %v", err)
		}
	}

	// Show the new overlay
	state := c.registry.GetState(componentType)
	state.Visible = true
	state.Active = true
	state.Focused = true
	state.LastUpdate = time.Now().Unix()
	c.registry.SetState(componentType, state)

	c.activeOverlay = componentType
	c.overlayActive = true

	return nil
}

// HideOverlay closes the specified overlay
func (c *coordinator) HideOverlay(componentType ComponentType) error {
	if !componentType.IsOverlay() {
		return fmt.Errorf("component %s is not an overlay", componentType.String())
	}

	state := c.registry.GetState(componentType)
	state.Visible = false
	state.Active = false
	state.Focused = false
	c.registry.SetState(componentType, state)

	// Clear the overlay from the registry
	switch componentType {
	case ComponentTextInputOverlay:
		c.registry.TextInputOverlay = nil
	case ComponentLiveSearchOverlay:
		c.registry.LiveSearchOverlay = nil
	case ComponentTextOverlay:
		c.registry.TextOverlay = nil
	case ComponentMessagesOverlay:
		c.registry.MessagesOverlay = nil
	case ComponentConfirmationOverlay:
		c.registry.ConfirmationOverlay = nil
	case ComponentSessionSetupOverlay:
		c.registry.SessionSetupOverlay = nil
	case ComponentGitStatusOverlay:
		c.registry.GitStatusOverlay = nil
	case ComponentClaudeSettingsOverlay:
		c.registry.ClaudeSettingsOverlay = nil
	case ComponentZFSearchOverlay:
		c.registry.ZFSearchOverlay = nil
	}

	if c.activeOverlay == componentType {
		c.activeOverlay = ComponentType(-1)
		c.overlayActive = false
	}

	return nil
}

// IsOverlayVisible returns true if the overlay is currently visible
func (c *coordinator) IsOverlayVisible(componentType ComponentType) bool {
	if !componentType.IsOverlay() {
		return false
	}

	state := c.registry.GetState(componentType)
	return state.Visible && state.Active
}

// GetActiveOverlay returns the currently active overlay, if any
func (c *coordinator) GetActiveOverlay() (ComponentType, bool) {
	if c.overlayActive {
		return c.activeOverlay, true
	}
	return ComponentType(-1), false
}

// CloseAllOverlays closes all active overlays
func (c *coordinator) CloseAllOverlays() error {
	overlayComponents := []ComponentType{
		ComponentTextInputOverlay,
		ComponentLiveSearchOverlay,
		ComponentTextOverlay,
		ComponentMessagesOverlay,
		ComponentConfirmationOverlay,
		ComponentSessionSetupOverlay,
		ComponentGitStatusOverlay,
		ComponentClaudeSettingsOverlay,
		ComponentZFSearchOverlay,
	}

	for _, component := range overlayComponents {
		if c.IsOverlayVisible(component) {
			if err := c.HideOverlay(component); err != nil {
				log.Printf("Error closing overlay %s: %v", component.String(), err)
			}
		}
	}

	return nil
}

// Overlay creation and management methods

// CreateTextInputOverlay creates and shows a text input overlay
func (c *coordinator) CreateTextInputOverlay(title, initialValue string) error {
	textInputOverlay := overlay.NewTextInputOverlay(title, initialValue)
	c.registry.TextInputOverlay = textInputOverlay
	return c.ShowOverlay(ComponentTextInputOverlay)
}

// CreateLiveSearchOverlay creates and shows a live search overlay
func (c *coordinator) CreateLiveSearchOverlay(title, initialQuery string) error {
	liveSearchOverlay := overlay.NewLiveSearchOverlay(title, initialQuery)
	c.registry.LiveSearchOverlay = liveSearchOverlay
	return c.ShowOverlay(ComponentLiveSearchOverlay)
}

// CreateConfirmationOverlay creates and shows a confirmation overlay
func (c *coordinator) CreateConfirmationOverlay(message string) error {
	confirmationOverlay := overlay.NewConfirmationOverlay(message)
	c.registry.ConfirmationOverlay = confirmationOverlay
	return c.ShowOverlay(ComponentConfirmationOverlay)
}

// CreateSessionSetupOverlay creates and shows a session setup overlay
func (c *coordinator) CreateSessionSetupOverlay() error {
	sessionSetupOverlay := overlay.NewSessionSetupOverlay()
	c.registry.SessionSetupOverlay = sessionSetupOverlay
	return c.ShowOverlay(ComponentSessionSetupOverlay)
}

// CreateMessagesOverlay creates and shows a messages overlay
func (c *coordinator) CreateMessagesOverlay(messages []overlay.StatusMessage) error {
	messagesOverlay := overlay.NewMessagesOverlay(messages)
	c.registry.MessagesOverlay = messagesOverlay
	return c.ShowOverlay(ComponentMessagesOverlay)
}

// CreateZFSearchOverlay creates and shows a ZF search overlay
func (c *coordinator) CreateZFSearchOverlay(title, placeholder string, directories []string) error {
	zfSearchOverlay, err := overlay.NewZFSearchOverlay(title, placeholder, directories)
	if err != nil {
		return fmt.Errorf("failed to create ZF search overlay: %w", err)
	}
	c.registry.ZFSearchOverlay = zfSearchOverlay
	return c.ShowOverlay(ComponentZFSearchOverlay)
}

// CreateGitStatusOverlay creates and shows a Git status overlay
func (c *coordinator) CreateGitStatusOverlay() error {
	gitStatusOverlay := overlay.NewGitStatusOverlay()
	c.registry.GitStatusOverlay = gitStatusOverlay
	return c.ShowOverlay(ComponentGitStatusOverlay)
}

// CreateClaudeSettingsOverlay creates and shows a Claude settings overlay
func (c *coordinator) CreateClaudeSettingsOverlay(settings session.ClaudeSettings, availableSessions []session.ClaudeSession) error {
	claudeSettingsOverlay := overlay.NewClaudeSettingsOverlay(settings, availableSessions)
	c.registry.ClaudeSettingsOverlay = claudeSettingsOverlay
	return c.ShowOverlay(ComponentClaudeSettingsOverlay)
}

// GetTextInputOverlay returns the text input overlay if active
func (c *coordinator) GetTextInputOverlay() *overlay.TextInputOverlay {
	if c.IsOverlayVisible(ComponentTextInputOverlay) {
		return c.registry.TextInputOverlay
	}
	return nil
}

// GetLiveSearchOverlay returns the live search overlay if active
func (c *coordinator) GetLiveSearchOverlay() *overlay.LiveSearchOverlay {
	if c.IsOverlayVisible(ComponentLiveSearchOverlay) {
		return c.registry.LiveSearchOverlay
	}
	return nil
}

// GetConfirmationOverlay returns the confirmation overlay if active
func (c *coordinator) GetConfirmationOverlay() *overlay.ConfirmationOverlay {
	if c.IsOverlayVisible(ComponentConfirmationOverlay) {
		return c.registry.ConfirmationOverlay
	}
	return nil
}

// GetSessionSetupOverlay returns the session setup overlay if active
func (c *coordinator) GetSessionSetupOverlay() *overlay.SessionSetupOverlay {
	if c.IsOverlayVisible(ComponentSessionSetupOverlay) {
		return c.registry.SessionSetupOverlay
	}
	return nil
}

// GetMessagesOverlay returns the messages overlay if active
func (c *coordinator) GetMessagesOverlay() *overlay.MessagesOverlay {
	if c.IsOverlayVisible(ComponentMessagesOverlay) {
		return c.registry.MessagesOverlay
	}
	return nil
}

// GetZFSearchOverlay returns the ZF search overlay if active
func (c *coordinator) GetZFSearchOverlay() *overlay.ZFSearchOverlay {
	if c.IsOverlayVisible(ComponentZFSearchOverlay) {
		return c.registry.ZFSearchOverlay
	}
	return nil
}

// GetGitStatusOverlay returns the Git status overlay if active
func (c *coordinator) GetGitStatusOverlay() *overlay.GitStatusOverlay {
	if c.IsOverlayVisible(ComponentGitStatusOverlay) {
		return c.registry.GitStatusOverlay
	}
	return nil
}

// GetClaudeSettingsOverlay returns the Claude settings overlay if active
func (c *coordinator) GetClaudeSettingsOverlay() *overlay.ClaudeSettingsOverlay {
	if c.IsOverlayVisible(ComponentClaudeSettingsOverlay) {
		return c.registry.ClaudeSettingsOverlay
	}
	return nil
}

// RouteMessage routes a message to the appropriate component
func (c *coordinator) RouteMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return c.RouteKeyMessage(msg)
	case tea.WindowSizeMsg:
		return c.routeWindowSizeMessage(msg)
	case spinner.TickMsg:
		return c.routeSpinnerMessage(msg)
	default:
		// Route to specific components based on message type or current state
		return c.routeGenericMessage(msg)
	}
}

// RouteKeyMessage routes key messages with component-specific handling
func (c *coordinator) RouteKeyMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If an overlay is active, route to that overlay first
	if c.overlayActive {
		return c.routeToOverlay(msg)
	}

	// Otherwise route to main components
	return c.routeToMainComponents(msg)
}

// RouteUpdateMessage routes update messages to components
func (c *coordinator) RouteUpdateMessage(msg tea.Msg, componentType ComponentType) (tea.Model, tea.Cmd) {
	// Component-specific update routing
	// Implementation will be completed in Task 3.2
	return nil, nil
}

// RenderMain renders the main application view
func (c *coordinator) RenderMain() string {
	if !c.initialized {
		return "UI not initialized"
	}

	// Get component views
	listView := ""
	if c.registry.List != nil {
		listView = c.registry.List.String()
	}

	tabbedView := ""
	if c.registry.TabbedWindow != nil {
		tabbedView = c.registry.TabbedWindow.String()
	}

	menuView := ""
	if c.registry.Menu != nil {
		menuView = c.registry.Menu.String()
	}

	statusView := ""
	if c.registry.StatusBar != nil {
		statusView = c.registry.StatusBar.View(c.width)
	}

	errView := ""
	if c.registry.ErrBox != nil {
		errView = c.registry.ErrBox.String()
	}

	// Combine views using lipgloss layout
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listView, tabbedView)
	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		listAndPreview,
		menuView,
		statusView,
		errView,
	)

	return mainView
}

// RenderOverlay renders the specified overlay
func (c *coordinator) RenderOverlay(componentType ComponentType) string {
	if !componentType.IsOverlay() || !c.IsOverlayVisible(componentType) {
		return ""
	}

	// Render the appropriate overlay
	switch componentType {
	case ComponentTextInputOverlay:
		if c.registry.TextInputOverlay != nil {
			return c.registry.TextInputOverlay.Render()
		}
	case ComponentLiveSearchOverlay:
		if c.registry.LiveSearchOverlay != nil {
			return c.registry.LiveSearchOverlay.View()
		}
	case ComponentTextOverlay:
		if c.registry.TextOverlay != nil {
			return c.registry.TextOverlay.Render()
		}
	case ComponentMessagesOverlay:
		if c.registry.MessagesOverlay != nil {
			return c.registry.MessagesOverlay.Render()
		}
	case ComponentConfirmationOverlay:
		if c.registry.ConfirmationOverlay != nil {
			return c.registry.ConfirmationOverlay.Render()
		}
	case ComponentSessionSetupOverlay:
		if c.registry.SessionSetupOverlay != nil {
			return c.registry.SessionSetupOverlay.View()
		}
	case ComponentGitStatusOverlay:
		if c.registry.GitStatusOverlay != nil {
			return c.registry.GitStatusOverlay.View()
		}
	case ComponentClaudeSettingsOverlay:
		if c.registry.ClaudeSettingsOverlay != nil {
			return c.registry.ClaudeSettingsOverlay.View()
		}
	case ComponentZFSearchOverlay:
		if c.registry.ZFSearchOverlay != nil {
			return c.registry.ZFSearchOverlay.View()
		}
	}

	return ""
}

// RenderWithLayout renders components with proper layout
func (c *coordinator) RenderWithLayout() string {
	// Start with main view
	mainView := c.RenderMain()

	// If there's an active overlay, render it on top
	if c.overlayActive {
		overlayContent := c.RenderOverlay(c.activeOverlay)
		if overlayContent != "" {
			// Use the overlay system to place overlay on main view
			return overlay.PlaceOverlay(0, 0, overlayContent, mainView, true, true)
		}
	}

	return mainView
}

// GetComponentView gets the view for a specific component
func (c *coordinator) GetComponentView(componentType ComponentType) string {
	state := c.registry.GetState(componentType)
	if !state.Visible {
		return ""
	}

	// Component view rendering will be implemented in Task 3.2
	return fmt.Sprintf("Component %s view placeholder", componentType.String())
}

// Helper methods for message routing

func (c *coordinator) routeToOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Route key messages to the active overlay based on its type
	switch c.activeOverlay {
	case ComponentTextInputOverlay:
		if c.registry.TextInputOverlay != nil {
			shouldClose := c.registry.TextInputOverlay.HandleKeyPress(msg)
			if shouldClose {
				c.HideOverlay(ComponentTextInputOverlay)
			}
		}
	case ComponentLiveSearchOverlay:
		if c.registry.LiveSearchOverlay != nil {
			shouldClose := c.registry.LiveSearchOverlay.HandleKeyPress(msg)
			if shouldClose {
				c.HideOverlay(ComponentLiveSearchOverlay)
			}
		}
	case ComponentConfirmationOverlay:
		if c.registry.ConfirmationOverlay != nil {
			shouldClose := c.registry.ConfirmationOverlay.HandleKeyPress(msg)
			if shouldClose {
				c.HideOverlay(ComponentConfirmationOverlay)
			}
		}
	case ComponentSessionSetupOverlay:
		if c.registry.SessionSetupOverlay != nil {
			// Session setup overlay uses BubbleTea Update method
			// For now, let it be handled externally in app.go
			// TODO: Migrate session setup overlay handling to coordinator
		}
	case ComponentMessagesOverlay:
		if c.registry.MessagesOverlay != nil {
			shouldClose := c.registry.MessagesOverlay.HandleKeyPress(msg)
			if shouldClose {
				c.HideOverlay(ComponentMessagesOverlay)
			}
		}
	}
	return nil, nil
}

func (c *coordinator) routeToMainComponents(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Route key messages to main components
	// Implementation will be completed in Task 3.2
	return nil, nil
}

func (c *coordinator) updateComponentSizes() {
	// Update component sizes based on current layout
	if c.registry.List != nil {
		listWidth := int(float32(c.width) * 0.3)
		contentHeight := c.height - 4 // Space for menu and status
		c.registry.List.SetSize(listWidth, contentHeight)
	}

	if c.registry.TabbedWindow != nil {
		tabsWidth := c.width - int(float32(c.width)*0.3) // Remaining width after list
		contentHeight := c.height - 4
		c.registry.TabbedWindow.SetSize(tabsWidth, contentHeight)
	}

	if c.registry.Menu != nil {
		c.registry.Menu.SetSize(c.width, 3)
	}

	if c.registry.ErrBox != nil {
		c.registry.ErrBox.SetSize(int(float32(c.width)*0.9), 1)
	}

	// Update overlay sizes if they're active
	c.updateOverlaySizes()
}

// updateOverlaySizes updates the sizes of active overlays
func (c *coordinator) updateOverlaySizes() {
	if c.registry.TextInputOverlay != nil {
		c.registry.TextInputOverlay.SetSize(int(float32(c.width)*0.6), int(float32(c.height)*0.4))
	}
	if c.registry.SessionSetupOverlay != nil {
		c.registry.SessionSetupOverlay.SetSize(int(float32(c.width)*0.8), int(float32(c.height)*0.8))
	}
	// Add other overlays as needed
}

// Message routing helper methods

func (c *coordinator) routeWindowSizeMessage(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	c.UpdateLayout(msg.Width, msg.Height)
	return nil, nil
}

func (c *coordinator) routeSpinnerMessage(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	c.registry.Spinner, cmd = c.registry.Spinner.Update(msg)
	return nil, cmd
}

func (c *coordinator) routeGenericMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Route other message types to appropriate components
	// This can be extended for specific message types as needed
	return nil, nil
}

// GetComponentByType returns the actual component instance by type
func (c *coordinator) GetComponentByType(componentType ComponentType) interface{} {
	switch componentType {
	case ComponentList:
		return c.registry.List
	case ComponentMenu:
		return c.registry.Menu
	case ComponentTabbedWindow:
		return c.registry.TabbedWindow
	case ComponentErrBox:
		return c.registry.ErrBox
	case ComponentSpinner:
		return c.registry.Spinner
	case ComponentStatusBar:
		return c.registry.StatusBar
	case ComponentTextInputOverlay:
		return c.registry.TextInputOverlay
	case ComponentLiveSearchOverlay:
		return c.registry.LiveSearchOverlay
	case ComponentTextOverlay:
		return c.registry.TextOverlay
	case ComponentMessagesOverlay:
		return c.registry.MessagesOverlay
	case ComponentConfirmationOverlay:
		return c.registry.ConfirmationOverlay
	case ComponentSessionSetupOverlay:
		return c.registry.SessionSetupOverlay
	case ComponentGitStatusOverlay:
		return c.registry.GitStatusOverlay
	case ComponentClaudeSettingsOverlay:
		return c.registry.ClaudeSettingsOverlay
	case ComponentZFSearchOverlay:
		return c.registry.ZFSearchOverlay
	default:
		return nil
	}
}

// SetComponent sets a component in the registry
func (c *coordinator) SetComponent(componentType ComponentType, component interface{}) error {
	switch componentType {
	case ComponentList:
		if list, ok := component.(*ui.List); ok {
			c.registry.List = list
		} else {
			return fmt.Errorf("invalid component type for List")
		}
	case ComponentMenu:
		if menu, ok := component.(*ui.Menu); ok {
			c.registry.Menu = menu
		} else {
			return fmt.Errorf("invalid component type for Menu")
		}
	case ComponentTabbedWindow:
		if tabbedWindow, ok := component.(*ui.TabbedWindow); ok {
			c.registry.TabbedWindow = tabbedWindow
		} else {
			return fmt.Errorf("invalid component type for TabbedWindow")
		}
	case ComponentErrBox:
		if errBox, ok := component.(*ui.ErrBox); ok {
			c.registry.ErrBox = errBox
		} else {
			return fmt.Errorf("invalid component type for ErrBox")
		}
	case ComponentStatusBar:
		if statusBar, ok := component.(*ui.StatusBar); ok {
			c.registry.StatusBar = statusBar
		} else {
			return fmt.Errorf("invalid component type for StatusBar")
		}
	case ComponentTextInputOverlay:
		if overlay, ok := component.(*overlay.TextInputOverlay); ok {
			c.registry.TextInputOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for TextInputOverlay")
		}
	case ComponentLiveSearchOverlay:
		if overlay, ok := component.(*overlay.LiveSearchOverlay); ok {
			c.registry.LiveSearchOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for LiveSearchOverlay")
		}
	case ComponentTextOverlay:
		if overlay, ok := component.(*overlay.TextOverlay); ok {
			c.registry.TextOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for TextOverlay")
		}
	case ComponentMessagesOverlay:
		if overlay, ok := component.(*overlay.MessagesOverlay); ok {
			c.registry.MessagesOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for MessagesOverlay")
		}
	case ComponentConfirmationOverlay:
		if overlay, ok := component.(*overlay.ConfirmationOverlay); ok {
			c.registry.ConfirmationOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for ConfirmationOverlay")
		}
	case ComponentSessionSetupOverlay:
		if overlay, ok := component.(*overlay.SessionSetupOverlay); ok {
			c.registry.SessionSetupOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for SessionSetupOverlay")
		}
	case ComponentGitStatusOverlay:
		if overlay, ok := component.(*overlay.GitStatusOverlay); ok {
			c.registry.GitStatusOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for GitStatusOverlay")
		}
	case ComponentClaudeSettingsOverlay:
		if overlay, ok := component.(*overlay.ClaudeSettingsOverlay); ok {
			c.registry.ClaudeSettingsOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for ClaudeSettingsOverlay")
		}
	case ComponentZFSearchOverlay:
		if overlay, ok := component.(*overlay.ZFSearchOverlay); ok {
			c.registry.ZFSearchOverlay = overlay
		} else {
			return fmt.Errorf("invalid component type for ZFSearchOverlay")
		}
	default:
		return fmt.Errorf("unknown component type: %s", componentType.String())
	}

	return nil
}