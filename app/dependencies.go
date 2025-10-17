package app

import (
	appui "claude-squad/app/ui"
	"claude-squad/app/state"
	"claude-squad/cmd"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/terminal"
	"claude-squad/ui"
	"context"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// Dependencies defines the external dependencies needed by the home struct
// This interface allows for easy mocking and testing
type Dependencies interface {
	// Configuration dependencies
	GetAppConfig() *config.Config
	GetAppState() *config.State
	GetDiscoveryConfig() *config.DiscoveryConfig

	// Storage dependencies
	GetStorage() *session.Storage

	// UI dependencies
	GetUICoordinator() appui.Coordinator
	GetList() *ui.List
	GetMenu() *ui.Menu
	GetTabbedWindow() *ui.TabbedWindow
	GetErrBox() *ui.ErrBox
	GetStatusBar() *ui.StatusBar
	GetSpinner() spinner.Model
	// UpdateSpinner updates the shared spinner instance that all components use
	// CRITICAL: This ensures spinner updates propagate to List renderer
	UpdateSpinner(newSpinner spinner.Model)

	// System dependencies
	GetTerminalManager() *terminal.Manager
	GetSignalManager() *terminal.SignalManager
	GetBridge() *cmd.Bridge
	GetStateManager() state.Manager

	// Context and runtime parameters
	GetContext() context.Context
	GetProgram() string
	GetAutoYes() bool
}

// ProductionDependencies implements Dependencies for production use
// This creates all real dependencies
type ProductionDependencies struct {
	ctx       context.Context
	program   string
	autoYes   bool

	// Lazy-initialized dependencies
	appConfig       *config.Config
	appState        *config.State
	discoveryConfig *config.DiscoveryConfig
	storage         *session.Storage
	uiCoordinator   appui.Coordinator
	list            *ui.List
	menu            *ui.Menu
	tabbedWindow    *ui.TabbedWindow
	errBox          *ui.ErrBox
	statusBar       *ui.StatusBar
	spinner         spinner.Model
	terminalManager *terminal.Manager
	signalManager   *terminal.SignalManager
	bridge          *cmd.Bridge
	stateManager    state.Manager
}

// NewProductionDependencies creates a production dependencies instance
func NewProductionDependencies(ctx context.Context, program string, autoYes bool) *ProductionDependencies {
	return &ProductionDependencies{
		ctx:     ctx,
		program: program,
		autoYes: autoYes,
	}
}

// GetContext returns the application context
func (p *ProductionDependencies) GetContext() context.Context {
	return p.ctx
}

// GetProgram returns the program name
func (p *ProductionDependencies) GetProgram() string {
	return p.program
}

// GetAutoYes returns the auto-yes flag
func (p *ProductionDependencies) GetAutoYes() bool {
	return p.autoYes
}

// GetAppConfig returns the application configuration
func (p *ProductionDependencies) GetAppConfig() *config.Config {
	if p.appConfig == nil {
		p.appConfig = config.DefaultConfig()
	}
	return p.appConfig
}

// GetAppState returns the application state
func (p *ProductionDependencies) GetAppState() *config.State {
	if p.appState == nil {
		p.appState = config.LoadState()
	}
	return p.appState
}

// GetDiscoveryConfig returns the discovery configuration
func (p *ProductionDependencies) GetDiscoveryConfig() *config.DiscoveryConfig {
	if p.discoveryConfig == nil {
		p.discoveryConfig = config.LoadDiscoveryConfig()
	}
	return p.discoveryConfig
}

// GetStorage returns the session storage
func (p *ProductionDependencies) GetStorage() *session.Storage {
	if p.storage == nil {
		appState := p.GetAppState()
		storage, err := session.NewStorage(appState)
		if err != nil {
			panic(err) // In production, this is a fatal error
		}
		p.storage = storage
	}
	return p.storage
}

// GetUICoordinator returns the UI coordinator
func (p *ProductionDependencies) GetUICoordinator() appui.Coordinator {
	if p.uiCoordinator == nil {
		p.uiCoordinator = appui.NewCoordinator()
	}
	return p.uiCoordinator
}

// GetList returns the session list component
func (p *ProductionDependencies) GetList() *ui.List {
	if p.list == nil {
		// CRITICAL: Pass pointer to the actual spinner field, not a local copy
		// GetSpinner() ensures the spinner is initialized before we take its address
		_ = p.GetSpinner() // Initialize spinner first
		appState := p.GetAppState()
		p.list = ui.NewList(&p.spinner, p.autoYes, appState)
	}
	return p.list
}

// GetMenu returns the menu component
func (p *ProductionDependencies) GetMenu() *ui.Menu {
	if p.menu == nil {
		p.menu = ui.NewMenu()
	}
	return p.menu
}

// GetTabbedWindow returns the tabbed window component
func (p *ProductionDependencies) GetTabbedWindow() *ui.TabbedWindow {
	if p.tabbedWindow == nil {
		p.tabbedWindow = ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane())
	}
	return p.tabbedWindow
}

// GetErrBox returns the error box component
func (p *ProductionDependencies) GetErrBox() *ui.ErrBox {
	if p.errBox == nil {
		p.errBox = ui.NewErrBox()
	}
	return p.errBox
}

// GetStatusBar returns the status bar component
func (p *ProductionDependencies) GetStatusBar() *ui.StatusBar {
	if p.statusBar == nil {
		p.statusBar = ui.NewStatusBar()
	}
	return p.statusBar
}

// GetSpinner returns the spinner component
func (p *ProductionDependencies) GetSpinner() spinner.Model {
	if p.spinner.View() == "" { // Check if spinner is initialized
		// CRITICAL: Initialize spinner with functional options to avoid showing "(error)"
		// The spinner.View() method returns "(error)" when frame is uninitialized
		p.spinner = spinner.New(
			spinner.WithSpinner(spinner.MiniDot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("205"))),
		)
	}
	return p.spinner
}

// UpdateSpinner updates the shared spinner instance
func (p *ProductionDependencies) UpdateSpinner(newSpinner spinner.Model) {
	p.spinner = newSpinner
}

// GetTerminalManager returns the terminal manager
func (p *ProductionDependencies) GetTerminalManager() *terminal.Manager {
	if p.terminalManager == nil {
		p.terminalManager = terminal.NewManager()
	}
	return p.terminalManager
}

// GetSignalManager returns the signal manager
func (p *ProductionDependencies) GetSignalManager() *terminal.SignalManager {
	if p.signalManager == nil {
		terminalManager := p.GetTerminalManager()
		p.signalManager = terminal.NewSignalManager(terminalManager)
	}
	return p.signalManager
}

// GetBridge returns the command bridge
func (p *ProductionDependencies) GetBridge() *cmd.Bridge {
	if p.bridge == nil {
		p.bridge = cmd.GetGlobalBridge()
	}
	return p.bridge
}

// GetStateManager returns the state manager
func (p *ProductionDependencies) GetStateManager() state.Manager {
	if p.stateManager == nil {
		p.stateManager = state.NewManager()
	}
	return p.stateManager
}

// MockDependencies implements Dependencies for testing
// This allows for easy mocking of all dependencies
type MockDependencies struct {
	ctx             context.Context
	program         string
	autoYes         bool
	appConfig       *config.Config
	appState        *config.State
	discoveryConfig *config.DiscoveryConfig
	storage         *session.Storage
	uiCoordinator   appui.Coordinator
	list            *ui.List
	menu            *ui.Menu
	tabbedWindow    *ui.TabbedWindow
	errBox          *ui.ErrBox
	statusBar       *ui.StatusBar
	spinner         spinner.Model
	terminalManager *terminal.Manager
	signalManager   *terminal.SignalManager
	bridge          *cmd.Bridge
	stateManager    state.Manager
}

// NewMockDependencies creates a mock dependencies instance for testing
func NewMockDependencies() *MockDependencies {
	// Create default mock dependencies to avoid nil pointer issues
	appState := config.LoadState()
	storage, _ := session.NewStorage(appState) // Ignore error in tests

	// Create UI components
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spinner, false, appState)

	return &MockDependencies{
		ctx:             context.Background(),
		appConfig:       config.DefaultConfig(),
		appState:        appState,
		discoveryConfig: config.DefaultDiscoveryConfig(),
		storage:         storage,
		stateManager:    state.NewManager(), // Each instance gets a new manager
		uiCoordinator:   appui.NewCoordinator(),
		statusBar:       ui.NewStatusBar(),
		spinner:         spinner,
		list:            list,
	}
}

// SetMockAppConfig allows tests to set a custom app config
func (m *MockDependencies) SetMockAppConfig(cfg *config.Config) *MockDependencies {
	m.appConfig = cfg
	return m
}

// SetMockAppState allows tests to set a custom app state
func (m *MockDependencies) SetMockAppState(state *config.State) *MockDependencies {
	m.appState = state
	return m
}

// SetMockList allows tests to set a custom list
func (m *MockDependencies) SetMockList(list *ui.List) *MockDependencies {
	m.list = list
	return m
}

// SetMockStorage allows tests to set a custom storage
func (m *MockDependencies) SetMockStorage(storage *session.Storage) *MockDependencies {
	m.storage = storage
	return m
}

// Implementation of Dependencies interface for MockDependencies
func (m *MockDependencies) GetContext() context.Context { return m.ctx }
func (m *MockDependencies) GetProgram() string { return m.program }
func (m *MockDependencies) GetAutoYes() bool { return m.autoYes }
func (m *MockDependencies) GetAppConfig() *config.Config { return m.appConfig }
func (m *MockDependencies) GetAppState() *config.State { return m.appState }
func (m *MockDependencies) GetDiscoveryConfig() *config.DiscoveryConfig { return m.discoveryConfig }
func (m *MockDependencies) GetStorage() *session.Storage { return m.storage }
func (m *MockDependencies) GetUICoordinator() appui.Coordinator { return m.uiCoordinator }
func (m *MockDependencies) GetList() *ui.List { return m.list }
func (m *MockDependencies) GetMenu() *ui.Menu { return m.menu }
func (m *MockDependencies) GetTabbedWindow() *ui.TabbedWindow { return m.tabbedWindow }
func (m *MockDependencies) GetErrBox() *ui.ErrBox { return m.errBox }
func (m *MockDependencies) GetStatusBar() *ui.StatusBar { return m.statusBar }
func (m *MockDependencies) GetSpinner() spinner.Model { return m.spinner }
func (m *MockDependencies) UpdateSpinner(newSpinner spinner.Model) { m.spinner = newSpinner }
func (m *MockDependencies) GetTerminalManager() *terminal.Manager { return m.terminalManager }
func (m *MockDependencies) GetSignalManager() *terminal.SignalManager { return m.signalManager }
func (m *MockDependencies) GetBridge() *cmd.Bridge { return m.bridge }
func (m *MockDependencies) GetStateManager() state.Manager { return m.stateManager }