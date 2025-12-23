package app

import (
	"claude-squad/app/services"
	appsession "claude-squad/app/session"
	"claude-squad/app/state"
	appui "claude-squad/app/ui"
	"claude-squad/cmd"
	"claude-squad/cmd/commands"
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/vcs"
	"claude-squad/terminal"
	"claude-squad/ui"
	"claude-squad/ui/fuzzy"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 100

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	// Create cancellable context so both BubbleTea and our app can be cancelled together
	// The parent context from main is Background() which can't be cancelled
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // Clean up context when Run() exits

	// Create home model and set its cancel function so handleQuit can cancel BubbleTea
	homeModel := newHome(ctx, program, autoYes)
	homeModel.cancelFunc = cancel

	// Use alt screen for proper rendering, but with corrected size detection
	p := tea.NewProgram(
		homeModel,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
		tea.WithContext(ctx),      // Share cancellable context with BubbleTea
	)
	_, err := p.Run()
	return err
}

type home struct {
	ctx        context.Context
	cancelFunc context.CancelFunc

	// -- Dependencies --

	// deps stores the dependencies to allow updating shared state
	// CRITICAL: Needed for spinner updates to propagate to List renderer
	deps Dependencies

	// -- Services Facade --

	// services provides unified access to all application services
	// This facade reduces coupling and simplifies testing
	// TODO: Gradually migrate direct field access to use this facade
	services services.Facade

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens (now using interface to support SQLite)
	appState config.StateManager
	// bridge provides centralized command and key management
	bridge *cmd.Bridge

	// -- State --

	// stateManager provides centralized state management
	stateManager state.Manager
	// sessionController handles session operations
	sessionController appsession.Controller
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool

	// keySent is used to manage underlining menu items
	keySent bool

	// asyncSessionCreation is used for tracking async session creation
	asyncSessionCreationActive bool
	sessionCreationCancel      context.CancelFunc // Cancel function for ongoing session creation

	// Pending session creation for advanced setup
	pendingSessionInstance *session.Instance
	pendingAutoYes         bool

	// Responsive navigation for instant feedback
	responsiveNav *ResponsiveNavigationManager

	// Terminal management for size detection and signal handling
	terminalManager *terminal.Manager
	signalManager   *terminal.SignalManager

	// Terminal dimensions for viewport-aware rendering
	termWidth  int
	termHeight int

	// -- UI Components --

	// uiCoordinator manages all UI components and their orchestration
	uiCoordinator appui.Coordinator

	// Legacy direct component access (will be removed after migration)
	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// Legacy overlays still used by help system (will be migrated in future tasks)
	// textOverlay displays text information for help system
	textOverlay *overlay.TextOverlay
	// messagesOverlay displays scrollable message history with vim-like navigation
	messagesOverlay *overlay.MessagesOverlay
	// Note: All other overlays now managed by UI coordinator
	// statusBar provides vim-style status bar with :messages command
	statusBar *ui.StatusBar

	// -- PTY Management --

	// viewMode determines if we're showing sessions or PTYs
	viewMode ViewMode
	// ptyDiscovery discovers and monitors PTY connections
	ptyDiscovery *session.PTYDiscovery
	// ptyList displays available PTY connections
	ptyList *ui.PTYList
	// ptyPreview shows output from selected PTY
	ptyPreview *ui.PTYPreview

	// -- Review Queue --

	// reviewQueue tracks sessions needing user attention
	reviewQueue *session.ReviewQueue
	// queueView displays the review queue
	queueView *ui.QueueView
	// statusManager manages instance status information for idle detection
	statusManager *session.InstanceStatusManager
	// reviewQueuePoller automatically adds idle sessions to review queue
	reviewQueuePoller *session.ReviewQueuePoller
}

// ViewMode represents the current view (sessions, PTYs, or review queue)
type ViewMode int

const (
	ViewModeSessions ViewMode = iota
	ViewModePTYs
	ViewModeReviewQueue
)

// newHomeWithDependencies creates a home instance using dependency injection
// This constructor follows clean architecture principles and makes testing easier
func newHomeWithDependencies(deps Dependencies) *home {
	// Use parent context directly - cancel function will be set by caller if needed
	// For production, Run() creates the cancellable context and passes its cancel function
	// For tests, test code can create its own cancellable context
	ctx := deps.GetContext()

	// Create home instance with injected dependencies
	h := &home{
		ctx:             ctx,
		cancelFunc:      nil,  // Will be set by Run() for production use
		deps:            deps, // Store deps for accessing shared state
		program:         deps.GetProgram(),
		autoYes:         deps.GetAutoYes(),
		storage:         deps.GetStorage(),
		appConfig:       deps.GetAppConfig(),
		appState:        deps.GetAppState(),
		stateManager:    deps.GetStateManager(),
		terminalManager: deps.GetTerminalManager(),
		signalManager:   deps.GetSignalManager(),
		list:            deps.GetList(),
		menu:            deps.GetMenu(),
		tabbedWindow:    deps.GetTabbedWindow(),
		errBox:          deps.GetErrBox(),
		spinner:         deps.GetSpinner(),
		statusBar:       deps.GetStatusBar(),
		uiCoordinator:   deps.GetUICoordinator(),
		bridge:          deps.GetBridge(),

		// Initialize PTY management
		viewMode:     ViewModeSessions,
		ptyDiscovery: session.NewPTYDiscovery(),
		ptyList:      ui.NewPTYList(),
		ptyPreview:   ui.NewPTYPreview(),
	}

	// Initialize coordinator with components
	if err := h.uiCoordinator.InitializeComponents(h.autoYes, h.appState); err != nil {
		panic(err) // In production, this is a fatal error
	}

	// Populate coordinator registry with existing components for gradual migration
	h.populateCoordinatorRegistry()

	// Initialize session controller with dependencies - only in production mode
	h.sessionController = appsession.NewController(appsession.Dependencies{
		List:                h.list,
		Storage:             h.storage,
		AutoYes:             h.autoYes,
		GlobalInstanceLimit: GlobalInstanceLimit,
		DiscoveryConfig:     deps.GetDiscoveryConfig(),
		ErrorHandler:        h.handleError,
		StateTransition: func(to string) error {
			// Map string states to enum and transition
			switch to {
			case "Default":
				return h.transitionToDefault()
			case "AdvancedNew":
				return h.transitionToOverlay(state.AdvancedNew, "Default", "sessionSetup")
			case "CreatingSession":
				return h.transitionToCreatingSession()
			default:
				return fmt.Errorf("unknown state transition: %s", to)
			}
		},
		InstanceChanged: h.instanceChanged,
		ConfirmAction:   h.confirmAction,
		ShowHelpScreen: func(helpType interface{}, onComplete func()) {
			// Convert interface{} to helpText and call the actual method
			if helpText, ok := helpType.(helpText); ok {
				h.showHelpScreen(helpText, onComplete)
			}
		},
		SetSessionSetupOverlay: func(overlay *overlay.SessionSetupOverlay) {
			// DEPRECATED: This bridge method is legacy - modern code uses
			// handleAdvancedSessionSetup() which creates the overlay with callbacks
			// directly via uiCoordinator.CreateSessionSetupOverlay(callbacks)
			if overlay != nil {
				// Overlays must now be created with callbacks at construction time
				log.WarningLog.Printf("SetSessionSetupOverlay called with non-nil overlay - this is deprecated")
				// The overlay is already created with callbacks, just store it
				// Note: This code path should not be used in modern code
			} else {
				// Clear the overlay
				h.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)
			}
		},
		GetSessionSetupOverlay: func() *overlay.SessionSetupOverlay {
			return h.uiCoordinator.GetSessionSetupOverlay()
		},
		SetPendingSession: func(inst *session.Instance, autoYes bool) {
			h.pendingSessionInstance = inst
			h.pendingAutoYes = autoYes
		},
		GetNewInstanceFinalizer: func() func() {
			return h.newInstanceFinalizer
		},
		SetNewInstanceFinalizer: func(f func()) {
			h.newInstanceFinalizer = f
		},
	})

	// Initialize services facade for cleaner service access
	// This provides a unified interface to all extracted services
	h.services = services.NewFacade(
		h.storage,
		h.sessionController,
		h.list,
		h.menu,
		h.statusBar,
		h.errBox,
		h.uiCoordinator,
		h.handleError,
		GlobalInstanceLimit,
	)

	// Initialize review queue and status management
	h.reviewQueue = session.NewReviewQueue()
	h.queueView = ui.NewQueueView(h.reviewQueue)
	h.statusManager = session.NewInstanceStatusManager()
	h.reviewQueuePoller = session.NewReviewQueuePoller(h.reviewQueue, h.statusManager, h.storage)

	// Load saved instances and perform initialization
	h.initializeWithSavedData()

	// Start the review queue poller after instances are loaded
	h.reviewQueuePoller.Start(ctx)

	// Old command registry removed - now using bridge system exclusively

	// Initialize the command bridge and setup handlers
	h.initializeCommandBridge()

	return h
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
	// Use dependency injection pattern
	deps := NewProductionDependencies(ctx, program, autoYes)
	return newHomeWithDependencies(deps)
}

// initializeWithSavedData handles loading and initializing saved session data
func (h *home) initializeWithSavedData() {
	// Add startup info message
	h.statusBar.SetInfo(fmt.Sprintf("Claude Squad started - program: %s, autoYes: %v", h.program, h.autoYes))

	// Load saved instances - no synchronous health checking to avoid startup bottleneck
	instances, err := h.storage.LoadInstances()
	if err != nil {
		// Fatal error before TUI starts - log and exit
		log.ErrorLog.Printf("Failed to load instances: %v", err)
		fmt.Fprintf(os.Stderr, "Fatal error: Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Fast startup: Add instances without expensive health checks
	// Health checking is now performed lazily when sessions are accessed
	if len(instances) > 0 {
		log.DebugLog.Printf("Loaded %d sessions - health checks will be performed lazily", len(instances))
	}

	// Add loaded instances using facade method
	for _, instance := range instances {
		h.addInstanceToList(instance)
		if h.autoYes {
			instance.AutoYes = true
		}
		// Wire up review queue to each instance
		instance.SetReviewQueue(h.reviewQueue)
	}

	// Wire up review queue to the list UI
	h.list.SetReviewQueue(h.reviewQueue)

	// Set instances for review queue poller to monitor
	h.reviewQueuePoller.SetInstances(instances)

	// Ensure all loaded instances have controllers started if they're active
	for _, instance := range instances {
		if instance.Started() && !instance.Paused() {
			// Try to start controller for instances that don't have one yet
			// This ensures review queue monitoring works for all loaded sessions
			if err := instance.StartController(); err != nil {
				log.WarningLog.Printf("Failed to start controller for loaded instance '%s': %v", instance.Title, err)
			}
		}
	}

	// Perform startup health check to detect orphaned sessions
	h.performStartupHealthCheck()

	// Restore selection using facade method
	h.restoreLastSelection()

	// Start optional background health checker for maintenance (non-blocking)
	if h.appConfig.PerformBackgroundHealthChecks {
		go h.startBackgroundHealthChecker()
	}
}

// startBackgroundHealthChecker runs health checks in background without blocking UI
func (h *home) startBackgroundHealthChecker() {
	healthChecker := session.NewSessionHealthChecker(h.storage)

	// Create stop channel tied to app context
	stopChan := make(chan struct{})
	go func() {
		<-h.ctx.Done()
		close(stopChan)
	}()

	// Wait 30 seconds after startup before first check to avoid startup contention
	// Use timer that can be interrupted by context cancellation
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Normal startup delay completed
		log.DebugLog.Printf("Starting background health checker")
		healthChecker.ScheduledHealthCheck(5*time.Minute, stopChan)
	case <-h.ctx.Done():
		// Context cancelled during startup delay - exit immediately
		log.DebugLog.Printf("Health checker cancelled during startup delay")
		return
	}
}

// performStartupHealthCheck validates all loaded sessions and marks orphaned ones as Paused
func (h *home) performStartupHealthCheck() {
	orphanedCount := 0
	checkedCount := 0

	for _, instance := range h.getAllInstances() {
		// Skip paused sessions - they're not expected to have tmux backing
		if instance.Paused() {
			continue
		}

		// Skip sessions that haven't been started
		if !instance.Started() {
			continue
		}

		checkedCount++

		// Check if tmux session actually exists
		if !instance.TmuxAlive() {
			log.WarningLog.Printf("Startup health check: Instance '%s' marked as Ready but tmux session doesn't exist, marking as Paused", instance.Title)
			instance.SetStatus(session.Paused)
			orphanedCount++
		}
	}

	if orphanedCount > 0 {
		log.InfoLog.Printf("Startup health check: Found %d orphaned sessions out of %d checked, marked as Paused", orphanedCount, checkedCount)
		// Save the updated state to persist the Paused status
		if err := h.saveAllInstances(); err != nil {
			log.ErrorLog.Printf("Failed to save instances after health check: %v", err)
		}
	} else if checkedCount > 0 {
		log.InfoLog.Printf("Startup health check: All %d active sessions are healthy", checkedCount)
	}
}

// ensureSessionHealthy performs lazy health checking on a specific session when accessed
func (h *home) ensureSessionHealthy(instance *session.Instance) error {
	if instance == nil {
		return fmt.Errorf("instance is nil")
	}

	// Skip health check for paused sessions
	if instance.Paused() {
		return nil
	}

	// Skip if session hasn't been started yet
	if !instance.Started() {
		return nil
	}

	// Quick check if tmux session is alive - this is the most common issue
	if !instance.TmuxAlive() {
		log.DebugLog.Printf("Session '%s' tmux died, attempting recovery", instance.Title)

		// Attempt recovery
		if err := instance.Start(false); err != nil {
			return fmt.Errorf("failed to recover session '%s': %w", instance.Title, err)
		}

		// Verify recovery worked
		if !instance.TmuxAlive() {
			return fmt.Errorf("session '%s' recovery failed - tmux still not alive", instance.Title)
		}

		log.DebugLog.Printf("Session '%s' recovered successfully", instance.Title)
	}

	return nil
}

// initializeCommandBridge sets up the centralized command system
func (m *home) initializeCommandBridge() {
	log.InfoLog.Printf("initializeCommandBridge: starting bridge initialization")
	// Get the global bridge instance
	m.bridge = cmd.GetGlobalBridge()
	log.InfoLog.Printf("initializeCommandBridge: got global bridge, current context: %s", m.bridge.GetCurrentContext())

	// Set up session handlers
	sessionHandlers := &commands.SessionHandlers{
		OnNewSession: func() (tea.Model, tea.Cmd) {
			return m.handleNewSession()
		},
		OnKillSession: func() (tea.Model, tea.Cmd) {
			return m.handleKillSession()
		},
		OnAttachSession: func() (tea.Model, tea.Cmd) {
			return m.handleAttachSession()
		},
		OnCheckout: func() (tea.Model, tea.Cmd) {
			return m.handleCheckoutSession()
		},
		OnResume: func() (tea.Model, tea.Cmd) {
			return m.handleResumeSession()
		},
		OnClaudeSettings: func() (tea.Model, tea.Cmd) {
			return m.handleClaudeSettings()
		},
		OnTagEditor: func() (tea.Model, tea.Cmd) {
			return m.handleTagEditor()
		},
		OnHistoryBrowser: func() (tea.Model, tea.Cmd) {
			return m.handleHistoryBrowser()
		},
		OnConfigEditor: func() (tea.Model, tea.Cmd) {
			return m.handleConfigEditor()
		},
		OnRenameSession: func() (tea.Model, tea.Cmd) {
			return m.handleRenameSession()
		},
		OnRestartSession: func() (tea.Model, tea.Cmd) {
			return m.handleRestartSession()
		},
		OnWorkspaceStatus: func() (tea.Model, tea.Cmd) {
			return m.handleWorkspaceStatus()
		},
		OnWorkspaceSwitch: func() (tea.Model, tea.Cmd) {
			return m.handleWorkspaceSwitch()
		},
	}

	// Set up git handlers
	gitHandlers := &commands.GitHandlers{
		OnGitStatus: func() (tea.Model, tea.Cmd) {
			return m.handleGitStatus()
		},
	}

	// Set up navigation handlers
	navigationHandlers := &commands.NavigationHandlers{
		OnUp: func() (tea.Model, tea.Cmd) {
			return m.handleNavigationUp()
		},
		OnDown: func() (tea.Model, tea.Cmd) {
			return m.handleNavigationDown()
		},
		OnLeft: func() (tea.Model, tea.Cmd) {
			// Left arrow collapses categories - TODO: implement category navigation
			return m, nil
		},
		OnRight: func() (tea.Model, tea.Cmd) {
			// Right arrow expands categories - TODO: implement category navigation
			return m, nil
		},
		OnPageUp: func() (tea.Model, tea.Cmd) {
			// Page up - use facade methods
			for i := 0; i < 10; i++ {
				if m.isAtNavigationStart() {
					break
				}
				m.navigateUp()
			}
			return m, m.instanceChanged()
		},
		OnPageDown: func() (tea.Model, tea.Cmd) {
			// Page down - use facade methods
			for i := 0; i < 10; i++ {
				if m.isAtNavigationEnd() {
					break
				}
				m.navigateDown()
			}
			return m, m.instanceChanged()
		},
		OnSearch: func() (tea.Model, tea.Cmd) {
			return m.handleSearch()
		},
		OnNextReview: func() (tea.Model, tea.Cmd) {
			return m.handleNextReview()
		},
		OnPreviousReview: func() (tea.Model, tea.Cmd) {
			return m.handlePreviousReview()
		},
		OnToggleReviewQueue: func() (tea.Model, tea.Cmd) {
			return m.handleToggleReviewQueue()
		},
	}

	// Set up organization handlers
	organizationHandlers := &commands.OrganizationHandlers{
		OnFilterPaused: func() (tea.Model, tea.Cmd) {
			return m.handleFilterPaused()
		},
		OnClearFilters: func() (tea.Model, tea.Cmd) {
			return m.handleClearFilters()
		},
		OnToggleGroup: func() (tea.Model, tea.Cmd) {
			return m.handleToggleGroup()
		},
		OnCycleGroupingMode: func() (tea.Model, tea.Cmd) {
			return m.handleCycleGroupingMode()
		},
		OnCycleSortMode: func() (tea.Model, tea.Cmd) {
			return m.handleCycleSortMode()
		},
		OnToggleSortDirection: func() (tea.Model, tea.Cmd) {
			return m.handleToggleSortDirection()
		},
	}

	// Set up system handlers
	systemHandlers := &commands.SystemHandlers{
		OnHelp: func() (tea.Model, tea.Cmd) {
			return m.handleHelp()
		},
		OnQuit: func() (tea.Model, tea.Cmd) {
			return m.handleQuit()
		},
		OnEscape: func() (tea.Model, tea.Cmd) {
			return m.handleEscape()
		},
		OnTab: func() (tea.Model, tea.Cmd) {
			return m.handleTab()
		},
		OnConfirm: func() (tea.Model, tea.Cmd) {
			return m.handleConfirm()
		},
		OnCommandMode: func() (tea.Model, tea.Cmd) {
			return m.handleCommandMode()
		},
		OnResize: func() (tea.Model, tea.Cmd) {
			return m.handleResize()
		},
	}

	// Set up PTY handlers
	ptyHandlers := &commands.PTYHandlers{
		OnTogglePTYView: func() (tea.Model, tea.Cmd) {
			return m.handleTogglePTYView()
		},
		OnAttachPTY: func() (tea.Model, tea.Cmd) {
			return m.handleAttachPTY()
		},
		OnSendCommand: func() (tea.Model, tea.Cmd) {
			return m.handleSendCommandToPTY()
		},
		OnDisconnectPTY: func() (tea.Model, tea.Cmd) {
			return m.handleDisconnectPTY()
		},
		OnRefreshPTYs: func() (tea.Model, tea.Cmd) {
			return m.handleRefreshPTYs()
		},
	}

	// Initialize the bridge with all handlers
	m.bridge.Initialize(sessionHandlers, gitHandlers, navigationHandlers, organizationHandlers, systemHandlers)

	// Set PTY handlers separately (they have their own registration method)
	commands.SetPTYHandlers(ptyHandlers)

	// Set up VC handlers for the version control tab
	vcHandlers := &commands.VCHandlers{
		OnVCStageFile: func() (tea.Model, tea.Cmd) {
			return m.handleVCStageFile()
		},
		OnVCUnstageFile: func() (tea.Model, tea.Cmd) {
			return m.handleVCUnstageFile()
		},
		OnVCStageAll: func() (tea.Model, tea.Cmd) {
			return m.handleVCStageAll()
		},
		OnVCUnstageAll: func() (tea.Model, tea.Cmd) {
			return m.handleVCUnstageAll()
		},
		OnVCOpenTerminal: func() (tea.Model, tea.Cmd) {
			return m.handleVCOpenTerminal()
		},
		OnVCToggleHelp: func() (tea.Model, tea.Cmd) {
			return m.handleVCToggleHelp()
		},
		OnVCNavigateUp: func() (tea.Model, tea.Cmd) {
			return m.handleVCNavigateUp()
		},
		OnVCNavigateDown: func() (tea.Model, tea.Cmd) {
			return m.handleVCNavigateDown()
		},
		OnVCCommandPalette: func() (tea.Model, tea.Cmd) {
			return m.handleVCCommandPalette()
		},
	}
	commands.SetVCHandlers(vcHandlers)

	// Start PTY discovery service
	m.ptyDiscovery.Start()

	// Validate bridge setup and log any key conflicts
	if issues := m.bridge.ValidateSetup(); len(issues) > 0 {
		log.InfoLog.Println("Bridge validation issues detected:")
		for _, issue := range issues {
			log.InfoLog.Printf("  - %s", issue)
		}
	} else {
		log.InfoLog.Println("Bridge validation passed - no key conflicts detected")
	}

	// Set initial context to list view
	m.bridge.SetContext(cmd.ContextList)

	// Update menu with available commands for the current context
	m.updateMenuFromContext()
}

// updateMenuFromContext updates the menu with commands available in the current context
// This applies permission-based filtering when an instance is selected
func (m *home) updateMenuFromContext() {
	if m.bridge != nil && m.menu != nil {
		// Get selected instance if available (only in sessions view)
		if m.viewMode == ViewModeSessions && m.list != nil {
			selectedInstance := m.list.GetSelectedInstance()
			if selectedInstance != nil {
				// Use permission-aware filtering for the selected instance
				m.menu.SetAvailableCommands(m.bridge.GetAvailableKeysForInstance(selectedInstance))
				return
			}
		}
		// Fallback to showing all commands if no instance selected
		m.menu.SetAvailableCommands(m.bridge.GetAvailableKeys())
	}
}

// Command handler methods for the centralized command bridge

func (m *home) handleNewSession() (tea.Model, tea.Cmd) {
	// Use the advanced session setup flow with proper callback initialization
	return m.handleAdvancedSessionSetup()
}

func (m *home) handleKillSession() (tea.Model, tea.Cmd) {
	result := m.sessionController.KillSession()
	return m, result.Cmd
}

func (m *home) handleAttachSession() (tea.Model, tea.Cmd) {
	// Get selected instance for lazy health checking
	selected := m.getSelectedInstance()
	if selected != nil {
		// Perform lazy health check before attachment
		if err := m.ensureSessionHealthy(selected); err != nil {
			return m, m.handleError(fmt.Errorf("session health check failed: %w", err))
		}
	}

	result := m.sessionController.AttachSession()
	return m, result.Cmd
}

func (m *home) handleCheckoutSession() (tea.Model, tea.Cmd) {
	result := m.sessionController.CheckoutSession()
	return m, result.Cmd
}

func (m *home) handleResumeSession() (tea.Model, tea.Cmd) {
	result := m.sessionController.ResumeSession()
	return m, result.Cmd
}

func (m *home) handleClaudeSettings() (tea.Model, tea.Cmd) {
	selected := m.getSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for Claude settings"))
	}

	// Get current Claude settings or create defaults
	currentSettings := session.ClaudeSettings{
		AutoReattach:          true,
		PreferredSessionName:  "",
		CreateNewOnMissing:    true,
		ShowSessionSelector:   false,
		SessionTimeoutMinutes: 60, // Default 1 hour timeout
	}

	// If the session already has Claude session data, use its settings
	if claudeSession := selected.GetClaudeSession(); claudeSession != nil {
		currentSettings = claudeSession.Settings
	}

	// Detect available Claude sessions
	sessionManager := session.NewClaudeSessionManager()
	availableSessions, err := sessionManager.DetectAvailableSessions()
	if err != nil {
		log.WarningLog.Printf("Failed to detect Claude sessions: %v", err)
		availableSessions = []session.ClaudeSession{} // Use empty list if detection fails
	}

	// Create the Claude settings overlay using coordinator
	if err := m.uiCoordinator.CreateClaudeSettingsOverlay(currentSettings, availableSessions); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create Claude settings overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	claudeSettingsOverlay := m.uiCoordinator.GetClaudeSettingsOverlay()
	if claudeSettingsOverlay != nil {
		claudeSettingsOverlay.OnComplete = func(settings session.ClaudeSettings, selectedSessionID string) {
			// Update the selected instance's Claude session settings
			if claudeSession := selected.GetClaudeSession(); claudeSession != nil {
				// Update existing session data
				claudeSession.Settings = settings
			} else {
				// Create new Claude session data
				sessionData := &session.ClaudeSessionData{
					SessionID:      selectedSessionID,
					ConversationID: "",
					ProjectName:    selected.Title,
					LastAttached:   time.Now(),
					Settings:       settings,
					Metadata:       make(map[string]string),
				}
				selected.SetClaudeSession(sessionData)
			}

			// Save the updated settings
			if err := m.saveAllInstances(); err != nil {
				m.handleError(fmt.Errorf("failed to save Claude settings: %w", err))
			} else {
				log.InfoLog.Printf("Claude settings updated for session '%s'", selected.Title)
			}

			// Close the overlay using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentClaudeSettingsOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}

		claudeSettingsOverlay.OnCancel = func() {
			// Close the overlay without saving using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentClaudeSettingsOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}
	}

	// Change state
	m.transitionToState(state.ClaudeSettings)

	return m, tea.WindowSize()
}

func (m *home) handleTagEditor() (tea.Model, tea.Cmd) {
	selected := m.getSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for tag editing"))
	}

	// Get current tags from the session
	currentTags := selected.Tags

	// Create the tag editor overlay using coordinator
	if err := m.uiCoordinator.CreateTagEditorOverlay(selected.Title, currentTags); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create tag editor overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	tagEditorOverlay := m.uiCoordinator.GetTagEditorOverlay()
	if tagEditorOverlay != nil {
		tagEditorOverlay.OnComplete = func(tags []string) {
			// Update the selected instance's tags
			selected.Tags = tags

			// Save the updated tags
			if err := m.saveAllInstances(); err != nil {
				m.handleError(fmt.Errorf("failed to save tags: %w", err))
			} else {
				log.InfoLog.Printf("Tags updated for session '%s': %v", selected.Title, tags)
			}

			// Close the overlay using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentTagEditorOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}

		tagEditorOverlay.OnCancel = func() {
			// Close the overlay without saving using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentTagEditorOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}
	}

	// Change state
	m.transitionToState(state.TagEditor)

	return m, tea.WindowSize()
}

func (m *home) handleWorkspaceSwitch() (tea.Model, tea.Cmd) {
	selected := m.getSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for workspace switch"))
	}

	// Get the repository path from the session
	repoPath := selected.Path
	if selected.MainRepoPath != "" {
		repoPath = selected.MainRepoPath
	}

	// Create the workspace switch overlay using coordinator
	if err := m.uiCoordinator.CreateWorkspaceSwitchOverlay(selected.Title, repoPath); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create workspace switch overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	workspaceSwitchOverlay := m.uiCoordinator.GetWorkspaceSwitchOverlay()
	if workspaceSwitchOverlay != nil {
		workspaceSwitchOverlay.OnSwitch = func(target string, switchType int, strategy vcs.ChangeStrategy, createIfMissing bool) {
			// Create the switch request
			req := session.WorkspaceSwitchRequest{
				Type:            session.WorkspaceSwitchType(switchType),
				Target:          target,
				ChangeStrategy:  strategy,
				CreateIfMissing: createIfMissing,
			}

			log.InfoLog.Printf("Switching workspace for '%s': %s -> %s (strategy: %s)",
				selected.Title, req.Type, target, strategy)

			// Perform the switch
			result, err := selected.SwitchWorkspace(req)
			if err != nil {
				m.handleError(fmt.Errorf("workspace switch failed: %w", err))
				return
			}

			if result.Success {
				log.InfoLog.Printf("Workspace switch successful: %s -> %s (changes: %s)",
					result.PreviousRevision, result.CurrentRevision, result.ChangesHandled)
			}

			// Save updated instance state
			if err := m.saveAllInstances(); err != nil {
				m.handleError(fmt.Errorf("failed to save instance after workspace switch: %w", err))
			}

			// Close the overlay
			m.uiCoordinator.HideOverlay(appui.ComponentWorkspaceSwitchOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}

		workspaceSwitchOverlay.OnCancel = func() {
			// Close the overlay without switching
			m.uiCoordinator.HideOverlay(appui.ComponentWorkspaceSwitchOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}
	}

	// Change state
	m.transitionToState(state.Workspace)

	return m, tea.WindowSize()
}

func (m *home) handleHistoryBrowser() (tea.Model, tea.Cmd) {
	// Create history browser overlay using coordinator
	if err := m.uiCoordinator.CreateHistoryBrowserOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create history browser overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	historyBrowserOverlay := m.uiCoordinator.GetHistoryBrowserOverlay()
	if historyBrowserOverlay != nil {
		historyBrowserOverlay.OnSelectEntry = func(entry session.ClaudeHistoryEntry) {
			// Launch a new session based on the history entry
			log.InfoLog.Printf("Creating session from history entry: %s (%s)", entry.Name, entry.Project)

			// Check if we're already at the instance limit
			if m.list.NumInstances() >= GlobalInstanceLimit {
				m.handleError(fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
				return
			}

			// Create InstanceOptions from history entry
			cfg := config.LoadConfig()
			options := session.InstanceOptions{
				Title:       entry.Name,
				Path:        entry.Project,
				WorkingDir:  "",                           // Start at repository root
				Program:     "claude",                     // Default to claude
				AutoYes:     false,                        // Don't auto-accept prompts
				Prompt:      "",                           // No initial prompt
				Category:    "History",                    // Category for organization
				Tags:        []string{"from-history"},     // Tag to indicate origin
				SessionType: session.SessionTypeDirectory, // Use directory-based session
				TmuxPrefix:  cfg.TmuxSessionPrefix,
			}

			// Create the instance
			instance, err := session.NewInstance(options)
			if err != nil {
				m.handleError(fmt.Errorf("failed to create session from history: %w", err))
				return
			}

			// Set pending session for async creation
			m.pendingSessionInstance = instance
			m.pendingAutoYes = false

			// Hide the overlay and transition to creating session state
			m.uiCoordinator.HideOverlay(appui.ComponentHistoryBrowserOverlay)
			m.transitionToCreatingSession()
		}

		historyBrowserOverlay.OnCancel = func() {
			// Close the overlay without action using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentHistoryBrowserOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}
	}

	// Change state
	m.transitionToState(state.HistoryBrowser)

	return m, tea.WindowSize()
}

func (m *home) handleConfigEditor() (tea.Model, tea.Cmd) {
	// Create config editor overlay using coordinator
	if err := m.uiCoordinator.CreateConfigEditorOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create config editor overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	configEditorOverlay := m.uiCoordinator.GetConfigEditorOverlay()
	if configEditorOverlay != nil {
		configEditorOverlay.OnComplete = func() {
			// Close the overlay after successful save
			m.uiCoordinator.HideOverlay(appui.ComponentConfigEditorOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
			m.statusBar.SetInfo("Configuration saved successfully")
		}

		configEditorOverlay.OnCancel = func() {
			// Close the overlay without saving
			m.uiCoordinator.HideOverlay(appui.ComponentConfigEditorOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}
	}

	// Change state
	m.transitionToState(state.ConfigEditor)

	return m, tea.WindowSize()
}

func (m *home) handleWorkspaceStatus() (tea.Model, tea.Cmd) {
	// Create workspace status overlay using coordinator
	if err := m.uiCoordinator.CreateWorkspaceStatusOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create workspace status overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	workspaceStatusOverlay := m.uiCoordinator.GetWorkspaceStatusOverlay()
	if workspaceStatusOverlay != nil {
		workspaceStatusOverlay.OnDismiss = func() {
			m.uiCoordinator.HideOverlay(appui.ComponentWorkspaceStatusOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}

		workspaceStatusOverlay.OnNavigateToSession = func(sessionTitle string) {
			// Navigate to the session in the list
			m.uiCoordinator.HideOverlay(appui.ComponentWorkspaceStatusOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)

			// Find and select the session
			for i, inst := range m.list.GetInstances() {
				if inst.Title == sessionTitle {
					m.list.SetSelectedIdx(i)
					break
				}
			}
		}

		workspaceStatusOverlay.OnRefresh = func() {
			// Refresh workspace data from all sessions
			m.populateWorkspaceStatus(workspaceStatusOverlay)
		}

		// Populate initial workspace data
		m.populateWorkspaceStatus(workspaceStatusOverlay)
	}

	// Change state with overlay context
	m.transitionToOverlay(state.Workspace, "Default", "workspaceStatus")

	return m, tea.WindowSize()
}

func (m *home) handleRenameSession() (tea.Model, tea.Cmd) {
	selected := m.getSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for renaming"))
	}

	// Check if session is started - we can't rename started sessions
	if selected.Started() {
		m.statusBar.SetError("Cannot rename a started session. Please stop the session first.")
		return m, nil
	}

	// Create the rename input overlay
	if err := m.uiCoordinator.CreateRenameInputOverlay(selected.Title, nil); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create rename overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	renameOverlay := m.uiCoordinator.GetRenameInputOverlay()
	if renameOverlay != nil {
		renameOverlay.OnSubmit = func(newTitle string) {
			// Update the session title
			if err := selected.SetTitle(newTitle); err != nil {
				m.statusBar.SetError(fmt.Sprintf("Failed to rename session: %v", err))
			} else {
				m.statusBar.SetInfo(fmt.Sprintf("Session renamed to: %s", newTitle))
				// Save the updated state
				if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
					log.ErrorLog.Printf("Failed to save renamed session: %v", err)
				}
			}
			// Close the overlay
			m.uiCoordinator.HideOverlay(appui.ComponentRenameInputOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		}
	}

	// Change state to Rename
	m.transitionToState(state.Rename)

	return m, tea.WindowSize()
}

func (m *home) handleRestartSession() (tea.Model, tea.Cmd) {
	selected := m.getSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for restart"))
	}

	// Show confirmation dialog
	message := fmt.Sprintf("Are you sure you want to restart '%s'?\nThis will kill the current session and start a new one.", selected.Title)
	if err := m.uiCoordinator.CreateConfirmationOverlay(message); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create confirmation overlay: %w", err))
	}

	// Get the overlay and set up callbacks
	confirmOverlay := m.uiCoordinator.GetConfirmationOverlay()
	if confirmOverlay != nil {
		confirmOverlay.OnConfirm = func() {
			// Restart the session
			m.statusBar.SetInfo(fmt.Sprintf("Restarting session: %s", selected.Title))

			// First, kill the current session
			if err := selected.Kill(); err != nil {
				m.statusBar.SetError(fmt.Sprintf("Failed to stop session: %v", err))
				m.uiCoordinator.HideOverlay(appui.ComponentConfirmationOverlay)
				m.transitionToDefault()
				return
			}

			// Then start it again
			go func() {
				if err := selected.Start(false); err != nil {
					m.statusBar.SetError(fmt.Sprintf("Failed to restart session: %v", err))
				} else {
					m.statusBar.SetInfo(fmt.Sprintf("Session '%s' restarted successfully", selected.Title))
				}
			}()

			// Close the overlay
			m.uiCoordinator.HideOverlay(appui.ComponentConfirmationOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)

			// Update the list to reflect the state change
			m.instanceChanged()
		}

		// The overlay will handle cancel (n key) automatically by closing itself
	}

	// Change state to Confirm
	m.transitionToState(state.Confirm)

	return m, tea.WindowSize()
}

func (m *home) handleNavigationUp() (tea.Model, tea.Cmd) {
	// Handle PTY view navigation
	if m.viewMode == ViewModePTYs {
		m.ptyList.MoveUp()
		// Update preview with selected PTY
		if selected := m.ptyList.GetSelected(); selected != nil {
			m.ptyPreview.SetConnection(selected)
		}
		return m, nil
	}

	// Session view navigation - use facade for cleaner service access
	if m.services.Navigation().GetCurrentIndex() == 0 {
		return m, nil
	}

	if err := m.services.Navigation().NavigateUp(); err != nil {
		return m, m.services.UICoordination().ShowError(err)
	}

	return m, m.instanceChanged()
}

func (m *home) handleNavigationDown() (tea.Model, tea.Cmd) {
	log.DebugLog.Printf("handleNavigationDown called")

	// Handle PTY view navigation
	if m.viewMode == ViewModePTYs {
		m.ptyList.MoveDown()
		// Update preview with selected PTY
		if selected := m.ptyList.GetSelected(); selected != nil {
			m.ptyPreview.SetConnection(selected)
		}
		return m, nil
	}

	// Session view navigation - use facade for cleaner service access
	if err := m.services.Navigation().NavigateDown(); err != nil {
		return m, m.services.UICoordination().ShowError(err)
	}

	log.DebugLog.Printf("Called NavigateDown via facade, triggering instance change")
	return m, m.instanceChanged()
}

func (m *home) handlePageUp() (tea.Model, tea.Cmd) {
	// Handle PTY view navigation
	if m.viewMode == ViewModePTYs {
		for i := 0; i < 10; i++ {
			m.ptyList.MoveUp()
		}
		// Update preview with selected PTY
		if selected := m.ptyList.GetSelected(); selected != nil {
			m.ptyPreview.SetConnection(selected)
		}
		return m, nil
	}

	// Session view - page up - implement with multiple Up calls using facade
	for i := 0; i < 10; i++ {
		if m.isAtNavigationStart() {
			break
		}
		m.navigateUp()
	}
	return m, m.instanceChanged()
}

func (m *home) handlePageDown() (tea.Model, tea.Cmd) {
	// Handle PTY view navigation
	if m.viewMode == ViewModePTYs {
		for i := 0; i < 10; i++ {
			m.ptyList.MoveDown()
		}
		// Update preview with selected PTY
		if selected := m.ptyList.GetSelected(); selected != nil {
			m.ptyPreview.SetConnection(selected)
		}
		return m, nil
	}

	// Session view - page down - implement with multiple Down calls using facade
	for i := 0; i < 10; i++ {
		if m.isAtNavigationEnd() {
			break
		}
		m.navigateDown()
	}
	return m, m.instanceChanged()
}

func (m *home) handleToggleFilter() (tea.Model, tea.Cmd) {
	return m.handleFilterPaused()
}

func (m *home) handleNavigationLeft() (tea.Model, tea.Cmd) {
	// Handle left navigation - collapse category
	// Use existing category toggle logic
	return m.handleToggleGroup()
}

func (m *home) handleNavigationRight() (tea.Model, tea.Cmd) {
	// Handle right navigation - expand category
	// Use existing category toggle logic
	return m.handleToggleGroup()
}

// Note: Navigation boundary checking now handled by facade methods isAtNavigationStart() and isAtNavigationEnd()

func (m *home) handleSearch() (tea.Model, tea.Cmd) {
	// Collect all working directories from active instances for indexing
	var directories []string
	for _, instance := range m.getAllInstances() {
		if instance != nil && !instance.Paused() {
			if workDir := instance.GetWorkingDirectory(); workDir != "" {
				directories = append(directories, workDir)
			}
		}
	}

	// If no active instances, fall back to session search
	if len(directories) == 0 {
		_, previousQuery := m.list.GetSearchState()

		// Create live search overlay using coordinator
		if err := m.uiCoordinator.CreateLiveSearchOverlay("Search Sessions", previousQuery); err != nil {
			return m, m.handleError(fmt.Errorf("failed to create live search overlay: %w", err))
		}

		// Get the overlay and set up callbacks
		liveSearchOverlay := m.uiCoordinator.GetLiveSearchOverlay()
		if liveSearchOverlay != nil {
			// Set up live search callback for instant results
			liveSearchOverlay.SetOnSearchLive(func(query string) {
				m.list.SearchByTitleLive(query)
			})

			// Set up submit callback for when user presses Enter
			liveSearchOverlay.SetOnSubmit(func(query string) {
				if query == "" {
					m.list.ExitSearchMode()
				} else {
					m.list.SearchByTitle(query)
				}
			})

			// Set up cancel callback for when user presses Esc
			liveSearchOverlay.SetOnCancel(func() {
				m.list.ExitSearchMode()
			})
		}

		m.transitionToState(state.Prompt)
		m.menu.SetState(ui.StateSearch)
		return m, nil
	}

	// Create ZF search overlay for file search using coordinator
	if err := m.uiCoordinator.CreateZFSearchOverlay("Search Files", "Enter filename or path...", directories); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create file search overlay: %w", err))
	}

	// Get the overlay and configure callbacks
	zfOverlay := m.uiCoordinator.GetZFSearchOverlay()
	if zfOverlay != nil {
		zfOverlay.SetOnSelect(func(item fuzzy.SearchItem) {
			// Open the selected file (this could be extended to integrate with external editor)
			filePath := item.GetID()
			log.InfoLog.Printf("Selected file: %s", filePath)

			// For now, just show the path in status bar
			m.statusBar.SetInfo(fmt.Sprintf("Selected: %s", filePath))

			// Close overlay using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentZFSearchOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		})

		zfOverlay.SetOnCancel(func() {
			// Close overlay without action using coordinator
			m.uiCoordinator.HideOverlay(appui.ComponentZFSearchOverlay)
			m.transitionToDefault()
			m.menu.SetState(ui.StateDefault)
		})
	}

	m.transitionToState(state.ZFSearch)
	m.menu.SetState(ui.StateSearch)
	return m, tea.WindowSize()
}

func (m *home) handleFilterPaused() (tea.Model, tea.Cmd) {
	// Use facade for cleaner service access
	if err := m.services.Filtering().TogglePausedFilter(); err != nil {
		return m, m.services.UICoordination().ShowError(err)
	}
	return m, nil
}

func (m *home) handleClearFilters() (tea.Model, tea.Cmd) {
	m.list.ClearAllFilters()
	return m, tea.WindowSize()
}

func (m *home) handleCycleGroupingMode() (tea.Model, tea.Cmd) {
	m.list.CycleGroupingStrategy()
	return m, nil
}

func (m *home) handleCycleSortMode() (tea.Model, tea.Cmd) {
	m.list.CycleSortMode()
	return m, nil
}

func (m *home) handleToggleSortDirection() (tea.Model, tea.Cmd) {
	m.list.ToggleSortDirection()
	return m, nil
}

func (m *home) handleToggleGroup() (tea.Model, tea.Cmd) {
	// Handle PTY view category toggle
	if m.viewMode == ViewModePTYs {
		if m.ptyList.IsOnCategoryHeader() {
			category := m.ptyList.GetSelectedCategory()
			m.ptyList.ToggleCategory(category)
		}
		return m, nil
	}

	// Session view category toggle
	selected := m.list.GetSelectedInstance()
	if selected != nil {
		category := selected.Category
		if category == "" {
			category = "Uncategorized"
		}
		m.list.ToggleCategory(category)
	}
	return m, nil
}

func (m *home) handleHelp() (tea.Model, tea.Cmd) {
	return m.showHelpScreen(helpTypeGeneral{}, nil)
}

func (m *home) handleEscape() (tea.Model, tea.Cmd) {
	// Handle escape key for search mode
	if m.isInState(state.Prompt) && m.menu.GetState() == ui.StateSearch {
		m.list.ExitSearchMode()
		m.transitionToDefault()
		m.menu.SetState(ui.StateDefault)
		m.uiCoordinator.HideOverlay(appui.ComponentTextInputOverlay)
		return m, tea.WindowSize()
	}
	return m, nil
}

func (m *home) handleTab() (tea.Model, tea.Cmd) {
	m.tabbedWindow.Toggle()
	m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
	// Update context based on active tab
	m.updateVCContext()
	return m, m.instanceChanged()
}

func (m *home) handleConfirm() (tea.Model, tea.Cmd) {
	// Handle confirmation in dialogs - trigger the confirmation action
	if m.isInState(state.Confirm) {
		if confirmationOverlay := m.uiCoordinator.GetConfirmationOverlay(); confirmationOverlay != nil {
			// Trigger the confirmation callback
			if confirmationOverlay.OnConfirm != nil {
				confirmationOverlay.OnConfirm()
			}
			m.transitionToDefault()
			m.uiCoordinator.HideOverlay(appui.ComponentConfirmationOverlay)
		}
	}
	return m, nil
}

func (m *home) handleCommandMode() (tea.Model, tea.Cmd) {
	// Enter vim-style command mode
	m.statusBar.EnterCommandMode()
	return m, nil
}

func (m *home) handleResize() (tea.Model, tea.Cmd) {
	// Force terminal resize detection using the terminal manager
	if m.terminalManager == nil {
		m.statusBar.SetWarning("Terminal manager not initialized")
		return m, nil
	}

	// Create a window size message with current dimensions
	msg := m.terminalManager.CreateWindowSizeMsg()

	// Show feedback to user
	m.statusBar.SetInfo(fmt.Sprintf("Terminal resized: %dx%d", msg.Width, msg.Height))

	// Return the window size message to trigger the resize handler
	return m, func() tea.Msg { return msg }
}

// handleNextReview navigates to the next session in the review queue
func (m *home) handleNextReview() (tea.Model, tea.Cmd) {
	if m.reviewQueue.Count() == 0 {
		m.statusBar.SetInfo("No sessions need attention")
		return m, nil
	}

	// Get current session
	currentSessionID := ""
	if selected := m.list.GetSelectedInstance(); selected != nil {
		currentSessionID = selected.Title
	}

	// Get next review item
	nextSessionID, found := m.reviewQueue.Next(currentSessionID)
	if !found {
		return m, nil
	}

	// Find and select the session
	instances := m.list.GetInstances()
	for idx, inst := range instances {
		if inst.Title == nextSessionID {
			m.list.SetSelectedIdx(idx)
			if reviewItem, ok := inst.GetReviewItem(); ok {
				m.statusBar.SetInfo(fmt.Sprintf("Review: %s - %s", reviewItem.Reason.String(), reviewItem.Context))
			}
			return m, m.instanceChanged()
		}
	}

	return m, nil
}

// handlePreviousReview navigates to the previous session in the review queue
func (m *home) handlePreviousReview() (tea.Model, tea.Cmd) {
	if m.reviewQueue.Count() == 0 {
		m.statusBar.SetInfo("No sessions need attention")
		return m, nil
	}

	// Get current session
	currentSessionID := ""
	if selected := m.list.GetSelectedInstance(); selected != nil {
		currentSessionID = selected.Title
	}

	// Get previous review item
	prevSessionID, found := m.reviewQueue.Previous(currentSessionID)
	if !found {
		return m, nil
	}

	// Find and select the session
	instances := m.list.GetInstances()
	for idx, inst := range instances {
		if inst.Title == prevSessionID {
			m.list.SetSelectedIdx(idx)
			if reviewItem, ok := inst.GetReviewItem(); ok {
				m.statusBar.SetInfo(fmt.Sprintf("Review: %s - %s", reviewItem.Reason.String(), reviewItem.Context))
			}
			return m, m.instanceChanged()
		}
	}

	return m, nil
}

// handleToggleReviewQueue toggles between normal view and review queue view
func (m *home) handleToggleReviewQueue() (tea.Model, tea.Cmd) {
	if m.viewMode == ViewModeReviewQueue {
		// Switch back to sessions view
		m.viewMode = ViewModeSessions
		m.statusBar.SetInfo("Switched to sessions view")
	} else {
		// Switch to review queue view
		m.viewMode = ViewModeReviewQueue
		queueCount := m.reviewQueue.Count()
		if queueCount == 0 {
			m.statusBar.SetInfo("Review queue is empty - all caught up!")
		} else {
			m.statusBar.SetInfo(fmt.Sprintf("Review queue: %d items need attention", queueCount))
		}
	}
	return m, nil
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// Debug: Compare received dimensions with our comprehensive detection
	// Defensive nil check to prevent panic in tests
	var detectedWidth, detectedHeight int
	if m.terminalManager == nil {
		detectedWidth, detectedHeight = msg.Width, msg.Height
	} else {
		detectedWidth, detectedHeight, _ = m.terminalManager.GetReliableSize()
	}
	// Debug: log.InfoLog.Printf("WindowSizeMsg: received %dx%d, detected %dx%d (method: %s)",
	//	msg.Width, msg.Height, detectedWidth, detectedHeight, method)

	// If there's a significant difference, log a warning
	widthDiff := abs(msg.Width - detectedWidth)
	heightDiff := abs(msg.Height - detectedHeight)
	if widthDiff > 5 || heightDiff > 5 {
		log.WarningLog.Printf("Size mismatch detected! BubbleTea: %dx%d vs Detection: %dx%d (diff: %d,%d)",
			msg.Width, msg.Height, detectedWidth, detectedHeight, widthDiff, heightDiff)
	}

	// Store terminal dimensions for viewport-aware rendering in View()
	m.termWidth = msg.Width
	m.termHeight = msg.Height

	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes space at bottom, error box takes 1 row, content gets remaining space
	// No padding since we removed PaddingTop from the View method
	menuHeight := 3 // Menu typically needs 3 rows for proper display
	errorBoxHeight := 1
	contentHeight := msg.Height - menuHeight - errorBoxHeight

	if contentHeight < 10 { // Minimum content height
		contentHeight = 10
	}

	m.errBox.SetSize(int(float32(msg.Width)*0.9), errorBoxHeight)

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	// Update PTY list and review queue view sizes
	m.ptyList.SetSize(listWidth, contentHeight)
	if m.queueView != nil {
		m.queueView.SetSize(msg.Width, contentHeight)
	}

	// Update overlay sizes using coordinator
	if textInputOverlay := m.uiCoordinator.GetTextInputOverlay(); textInputOverlay != nil {
		textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if messagesOverlay := m.uiCoordinator.GetMessagesOverlay(); messagesOverlay != nil {
		width := int(float32(msg.Width) * 0.8)
		height := int(float32(msg.Height) * 0.8)
		messagesOverlay.SetDimensions(width, height)
	}
	if sessionSetupOverlay := m.uiCoordinator.GetSessionSetupOverlay(); sessionSetupOverlay != nil {
		sessionSetupOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}
	if gitStatusOverlay := m.uiCoordinator.GetGitStatusOverlay(); gitStatusOverlay != nil {
		gitStatusOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}
	if claudeSettingsOverlay := m.uiCoordinator.GetClaudeSettingsOverlay(); claudeSettingsOverlay != nil {
		claudeSettingsOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}
	if zfSearchOverlay := m.uiCoordinator.GetZFSearchOverlay(); zfSearchOverlay != nil {
		zfSearchOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}
	// Handle remaining legacy overlays used by help system
	if m.textOverlay != nil {
		m.textOverlay.SetSize(int(float32(msg.Width)*0.8), int(float32(msg.Height)*0.8))
	}
	if m.messagesOverlay != nil {
		width := int(float32(msg.Width) * 0.8)
		height := int(float32(msg.Height) * 0.8)
		m.messagesOverlay.SetDimensions(width, height)
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		// PTY initialization errors are now handled at the list level as warnings
		// Only log as error if it's a different type of issue
		log.WarningLog.Printf("Preview size update issues: %v", err)
	}
	m.menu.SetSize(msg.Width, menuHeight)

	// Update PTY components
	m.ptyList.SetSize(listWidth, contentHeight)
	m.ptyPreview.SetSize(tabsWidth, contentHeight)

	// Update coordinator layout
	m.uiCoordinator.HandleResize(msg.Width, msg.Height)

	// Propagate window size to all tmux sessions for IntelliJ terminal compatibility
	// This ensures that tmux sessions receive the correct terminal dimensions even when
	// SIGWINCH signals don't work properly in embedded terminals
	for _, instance := range m.list.GetInstances() {
		if instance != nil && instance.Started() {
			instance.SetWindowSize(msg.Width, msg.Height)
		}
	}
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	cmds := []tea.Cmd{
		m.spinner.Tick,
		// CRITICAL: Use proper async pattern to avoid blocking BubbleTea's main event loop
		tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
			return previewTickMsg{}
		}),
		// CRITICAL: Use proper async pattern to avoid blocking BubbleTea's main event loop
		tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
			return previewResultsMsg{}
		}),
		tickUpdateMetadataCmd,
	}

	// Add session detection ticker if enabled
	if m.appConfig.DetectNewSessions {
		cmds = append(cmds, tickSessionDetectionCmd(m.appConfig.SessionDetectionInterval))
	}

	// Terminal size checking disabled - let BubbleTea handle size detection naturally
	// without alt screen, BubbleTea's natural detection should work better with tiling WMs
	// cmds = append(cmds, tickTerminalSizeCheckCmd())

	// Override BubbleTea's initial size detection for tiling window managers
	// Only add terminal size detection if not in test environment
	if m.terminalManager != nil {
		cmds = append(cmds, func() tea.Msg {
			// Use the new terminal manager for size detection
			return m.terminalManager.CreateWindowSizeMsg()
		})
	}

	// SIGWINCH signal handling disabled - let BubbleTea handle resize detection naturally
	// without alt screen, BubbleTea should position content correctly in visible area
	// cmds = append(cmds, setupSIGWINCHHandler())

	return tea.Batch(cmds...)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Ensure categories are organized whenever the model updates
	m.list.OrganizeByCategory()

	// Check for pending session creation from advanced setup
	if m.pendingSessionInstance != nil && !m.asyncSessionCreationActive {
		instance := m.pendingSessionInstance
		autoYes := m.pendingAutoYes

		// Clear pending state
		m.pendingSessionInstance = nil
		m.pendingAutoYes = false

		// Set async flag to prevent double processing
		m.asyncSessionCreationActive = true

		// Start async session creation with timeout protection
		return m, m.createSessionWithTimeout(instance, false, autoYes)
	}

	switch msg := msg.(type) {
	case sessionCreationResultMsg:
		m.asyncSessionCreationActive = false

		// CRITICAL: Clean up the session setup overlay after creation completes
		// This ensures the overlay is properly removed from the UI coordinator
		m.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)

		// Handle error if session creation failed
		if msg.err != nil {
			m.list.Kill()
			m.transitionToDefault()
			return m, m.handleError(msg.err)
		}

		// Save after adding new instance
		if err := m.saveAllInstances(); err != nil {
			return m, m.handleError(err)
		}

		// Instance added successfully, call the finalizer.
		if m.newInstanceFinalizer != nil {
			m.newInstanceFinalizer()
		}
		if msg.autoYes {
			msg.instance.AutoYes = true
		}

		// Reset state
		m.transitionToDefault()
		if msg.promptAfterName {
			m.transitionToState(state.Prompt)
			m.menu.SetState(ui.StatePrompt)
			// Initialize the text input overlay using coordinator
			if err := m.uiCoordinator.CreateTextInputOverlay("Enter prompt", ""); err != nil {
				return m, m.handleError(fmt.Errorf("failed to create text input overlay: %w", err))
			}
			m.promptAfterName = false
		} else {
			m.menu.SetState(ui.StateDefault)
			m.showHelpScreen(helpStart(msg.instance), nil)
		}

		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		// Check if context is cancelled (app is quitting)
		select {
		case <-m.ctx.Done():
			return m, nil
		default:
		}

		// First check if the selected instance exists and isn't paused before updating
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() {
			// CRITICAL: Use proper async pattern to avoid blocking BubbleTea's main event loop
			return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
				return previewTickMsg{}
			})
		}

		// Update the UI with the selected instance
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			// CRITICAL: Use proper async pattern to avoid blocking BubbleTea's main event loop
			tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
				return previewTickMsg{}
			}),
		)
	case previewResultsMsg:
		// Check if context is cancelled (app is quitting)
		select {
		case <-m.ctx.Done():
			return m, nil
		default:
		}

		// Process any pending async results (both preview and diff)
		if err := m.tabbedWindow.ProcessAllResults(); err != nil {
			return m, m.handleError(err)
		}

		// Schedule next result processing
		// CRITICAL: Use proper async pattern to avoid blocking BubbleTea's main event loop
		return m, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
			return previewResultsMsg{}
		})
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case tickUpdateMetadataMessage:
		// Check if context is cancelled (app is quitting)
		select {
		case <-m.ctx.Done():
			return m, nil
		default:
		}

		for _, instance := range m.list.GetInstances() {
			if !instance.Started() {
				continue
			}

			// Check if the instance is paused before updating
			if instance.Paused() {
				continue
			}

			// Track if the instance was already paused before updating diff stats
			wasPaused := instance.Paused()

			// Check if worktree path exists (if it doesn't, the UpdateDiffStats call will mark it as paused)
			if err := instance.UpdateDiffStats(); err != nil {
				log.WarningLog.Printf("could not update diff stats: %v", err)
				continue // Skip the rest of the updates if we can't update diff stats
			}

			// If the instance was newly marked as paused by UpdateDiffStats, trigger UI refresh
			if !wasPaused && instance.Paused() {
				// Force refresh UI layout to handle the changed state
				return m, tea.Sequence(tea.WindowSize(), tickUpdateMetadataCmd)
			}

			// If the instance is now paused, skip the rest of the updates
			if instance.Paused() {
				continue
			}

			updated, prompt := instance.HasUpdated()
			if updated {
				instance.SetStatus(session.Running)
				// Remove from review queue if it was there
				if m.reviewQueue != nil {
					m.reviewQueue.Remove(instance.Title)
				}
			} else {
				if prompt {
					if instance.AutoYes {
						instance.TapEnter()
					} else {
						instance.SetStatus(session.NeedsApproval)
						// Add to review queue when approval is needed
						if m.reviewQueue != nil {
							m.reviewQueue.Add(&session.ReviewItem{
								SessionID:   instance.Title,
								SessionName: instance.Title,
								Reason:      session.ReasonApprovalPending,
								Priority:    session.PriorityHigh,
								DetectedAt:  time.Now(),
								Context:     "Waiting for user approval",
							})
						}
					}
				} else {
					instance.SetStatus(session.Ready)
					// Remove from review queue when ready
					if m.reviewQueue != nil {
						m.reviewQueue.Remove(instance.Title)
					}
				}
			}
		}
		return m, tickUpdateMetadataCmd
	case tickSessionDetectionMessage:
		// Check if context is cancelled (app is quitting)
		select {
		case <-m.ctx.Done():
			return m, nil
		default:
		}

		// Check for new sessions created by other instances
		if m.appConfig.DetectNewSessions {
			if err := m.detectAndLoadNewSessions(); err != nil {
				log.WarningLog.Printf("failed to detect new sessions: %v", err)
			}
		}
		return m, tea.Batch(tickSessionDetectionCmd(m.appConfig.SessionDetectionInterval), tea.WindowSize())
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling the diff/preview pane
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				selected := m.list.GetSelectedInstance()
				if selected == nil || selected.Status == session.Paused {
					return m, nil
				}

				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
				}
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	// case terminalSizeCheckMsg:
	// Terminal size checking disabled - BubbleTea handles this naturally
	// width, height, method := GetReliableTerminalSize()
	// if override_width, override_height := getManualSizeOverride(); override_width > 0 && override_height > 0 {
	//	width = override_width
	//	height = override_height
	// }
	// if width != m.lastTerminalWidth || height != m.lastTerminalHeight {
	//	m.lastTerminalWidth = width
	//	m.lastTerminalHeight = height
	//	return m, tea.Batch(
	//		tickTerminalSizeCheckCmd(),
	//		func() tea.Msg { return tea.WindowSizeMsg{Width: width, Height: height} },
	//	)
	// }
	// return m, tickTerminalSizeCheckCmd()
	// case sigwinchMsg:
	// SIGWINCH handling disabled - BubbleTea handles resize naturally
	// log.InfoLog.Printf("Processing SIGWINCH resize event: %dx%d", msg.width, msg.height)
	// return m, tea.Batch(
	//	func() tea.Msg { return tea.WindowSizeMsg{Width: msg.width, Height: msg.height} },
	//	setupSIGWINCHHandler(),
	// )
	case terminal.ResizeMsg:
		// Handle terminal resize messages from signal manager
		log.InfoLog.Printf("Processing terminal resize event: %dx%d", msg.Width, msg.Height)
		return m, func() tea.Msg { return tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height} }
	case terminal.SizeCheckMsg:
		// Handle periodic size checking for IntelliJ compatibility
		if m.terminalManager.HasSizeChanged() {
			return m, func() tea.Msg { return m.terminalManager.CreateWindowSizeMsg() }
		}
		return m, m.signalManager.CreateSizeCheckCmd()
	case gitStatusLoadedMsg:
		// Handle git status data loaded
		if gitStatusOverlay := m.uiCoordinator.GetGitStatusOverlay(); gitStatusOverlay != nil {
			gitStatusOverlay.SetFiles(msg.files, msg.branchName)
		}
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	// instanceExpensiveUpdateMsg handler removed - now handled by ResponsiveNavigationManager
	case spinner.TickMsg:
		// Check if context is cancelled (app is quitting)
		select {
		case <-m.ctx.Done():
			return m, nil
		default:
		}

		var spinnerCmd tea.Cmd
		m.spinner, spinnerCmd = m.spinner.Update(msg)
		// CRITICAL: Update the shared spinner in dependencies so List renderer sees the update
		m.deps.UpdateSpinner(m.spinner)
		return m, spinnerCmd
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	quitStart := time.Now()
	log.DebugLog.Printf("handleQuit: Starting quit sequence")

	// Cancel context to stop background goroutines
	if m.cancelFunc != nil {
		m.cancelFunc()
		log.DebugLog.Printf("handleQuit: Context cancelled")
	}

	// Perform final synchronous save before quitting to ensure no data loss
	saveStart := time.Now()
	instances := m.getAllInstances()
	log.DebugLog.Printf("handleQuit: Retrieved %d instances to save", len(instances))

	if err := m.storage.SaveInstancesSync(instances); err != nil {
		log.WarningLog.Printf("Failed final save on quit: %v", err)
		// Continue with quit anyway - we tried our best
	}
	log.DebugLog.Printf("handleQuit: SaveInstancesSync took %v", time.Since(saveStart))

	// Output session log paths to stderr before exit
	logPathsStart := time.Now()
	log.LogSessionPathsToStderr()
	log.DebugLog.Printf("handleQuit: LogSessionPathsToStderr took %v", time.Since(logPathsStart))

	// Close storage - this flushes any pending async saves and releases locks
	closeStart := time.Now()
	if err := m.storage.Close(); err != nil {
		log.WarningLog.Printf("Failed to close storage: %v", err)
		// Continue with quit anyway
	}
	log.DebugLog.Printf("handleQuit: storage.Close took %v", time.Since(closeStart))

	log.DebugLog.Printf("handleQuit: Total quit sequence took %v", time.Since(quitStart))
	return m, tea.Quit
}

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.isInState(state.Prompt) || m.isInState(state.Help) || m.isInState(state.Confirm) {
		return nil, false
	}
	// Check if the key is bound to any command in the current context
	if !m.bridge.IsKeyBound(msg.String()) {
		return nil, false
	}

	// Skip highlighting for paused instances on enter key
	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && msg.String() == "enter" {
		return nil, false
	}
	// Skip highlighting for shift navigation keys
	if msg.String() == "shift+up" || msg.String() == "shift+down" {
		return nil, false
	}

	m.keySent = true

	// Highlight the pressed key in the menu
	m.menu.Keydown(msg.String())

	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(msg.String())), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, teaCmd tea.Cmd) {
	menuCmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, menuCmd
	}

	if m.isInState(state.Help) {
		return m.handleHelpState(msg)
	}

	if m.isInState(state.AdvancedNew) {
		return m.handleAdvancedSessionSetupUpdate(msg)
	}

	if m.isInState(state.Git) {
		return m.handleGitState(msg)
	}

	if m.isInState(state.ClaudeSettings) {
		return m.handleClaudeSettingsState(msg)
	}

	if m.isInState(state.ZFSearch) {
		return m.handleZFSearchState(msg)
	}

	if m.isInState(state.TagEditor) {
		return m.handleTagEditorState(msg)
	}

	if m.isInState(state.Workspace) {
		return m.handleWorkspaceState(msg)
	}

	if m.isInState(state.HistoryBrowser) {
		return m.handleHistoryBrowserState(msg)
	}

	if m.isInState(state.ConfigEditor) {
		return m.handleConfigEditorState(msg)
	}

	if m.isInState(state.Rename) {
		return m.handleRenameState(msg)
	}

	if m.isInState(state.New) {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.transitionToDefault()
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance (enable previews etc) and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// Set state to creating session and update menu
			m.transitionToCreatingSession()

			// Start session creation in a goroutine
			// Store important values before the goroutine starts
			promptAfterName := m.promptAfterName
			autoYes := m.autoYes

			// Return the tick command to send a message after session is created
			return m, m.createSessionWithTimeout(instance, promptAfterName, autoYes)
		case tea.KeyRunes:
			if len(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			if len(instance.Title) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(instance.Title[:len(instance.Title)-1]); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.transitionToDefault()
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.isInState(state.Prompt) {
		var shouldClose bool

		// Handle search overlay (live search)
		liveSearchOverlay := m.uiCoordinator.GetLiveSearchOverlay()
		if liveSearchOverlay != nil {
			shouldClose = liveSearchOverlay.HandleKeyPress(msg)
			if shouldClose {
				// Close the overlay and reset state
				m.uiCoordinator.HideOverlay(appui.ComponentLiveSearchOverlay)
				m.transitionToDefault()
				m.menu.SetState(ui.StateDefault)
				return m, tea.WindowSize()
			}
			return m, nil
		}

		// Handle regular text input overlay (prompts)
		textInputOverlay := m.uiCoordinator.GetTextInputOverlay()
		if textInputOverlay != nil {
			shouldClose = textInputOverlay.HandleKeyPress(msg)

			// Check if the form was submitted or canceled
			if shouldClose {
				// Get the current menu state
				// Handle search mode differently than prompt mode
				if m.menu.GetState() == ui.StateSearch {
					// Close the overlay and reset state
					m.uiCoordinator.HideOverlay(appui.ComponentTextInputOverlay)
					m.transitionToDefault()
					m.menu.SetState(ui.StateDefault)
					return m, tea.WindowSize()
				}

				// Regular prompt handling
				selected := m.list.GetSelectedInstance()
				// TODO: this should never happen since we set the instance in the previous state.
				if selected == nil {
					return m, nil
				}
				if textInputOverlay.IsSubmitted() {
					if err := selected.SendPrompt(textInputOverlay.GetValue()); err != nil {
						// TODO: we probably end up in a bad state here.
						return m, m.handleError(err)
					}
				}

				// Close the overlay and reset state
				m.uiCoordinator.HideOverlay(appui.ComponentTextInputOverlay)
				m.transitionToDefault()
				return m, tea.Sequence(
					tea.WindowSize(),
					func() tea.Msg {
						m.menu.SetState(ui.StateDefault)
						m.showHelpScreen(helpStart(selected), nil)
						return nil
					},
				)
			}
		}

		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle status bar command mode
	if m.statusBar.IsInCommandMode() {
		m.statusBar.HandleCommandInput(msg)

		if msg.String() == "enter" {
			command := m.statusBar.GetCommand()
			m.statusBar.ExitCommandMode()

			// Handle commands
			switch command {
			case "messages":
				m.transitionToState(state.Help)
				uiMessages := m.statusBar.GetMessageHistory()

				// Convert ui.StatusMessage to overlay.StatusMessage to avoid circular imports
				overlayMessages := make([]overlay.StatusMessage, len(uiMessages))
				for i, msg := range uiMessages {
					overlayMessages[i] = overlay.StatusMessage{
						Timestamp: msg.Timestamp,
						Level:     msg.Level,
						Message:   msg.Message,
					}
				}

				// Create messages overlay using coordinator
				if err := m.uiCoordinator.CreateMessagesOverlay(overlayMessages); err != nil {
					return m, m.handleError(fmt.Errorf("failed to create messages overlay: %w", err))
				}

				// Get the overlay and set dimensions
				messagesOverlay := m.uiCoordinator.GetMessagesOverlay()
				if messagesOverlay != nil {
					// Set default dimensions - will be updated on next window resize
					messagesOverlay.SetDimensions(100, 30)
				}
				m.menu.SetState(ui.StateDefault)
				return m, nil
			default:
				m.statusBar.SetWarning(fmt.Sprintf("Unknown command: %s", command))
			}
		} else if msg.String() == "esc" {
			m.statusBar.ExitCommandMode()
		}

		return m, nil
	}

	// Colon key now handled by bridge system

	// Handle quit commands - now handled by bridge, but keep special session creation handling
	// ALSO handle Escape key for cancelling session creation
	if (msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc") && m.isInState(state.CreatingSession) {
		// Special handling for session creation cancellation
		log.InfoLog.Printf("User cancelled session creation with %s", msg.String())

		// Cancel the ongoing session creation goroutines
		if m.sessionCreationCancel != nil {
			log.InfoLog.Printf("Cancelling session creation goroutines")
			m.sessionCreationCancel()
			m.sessionCreationCancel = nil
		}

		m.asyncSessionCreationActive = false

		// Clean up the session setup overlay
		m.uiCoordinator.HideOverlay(appui.ComponentSessionSetupOverlay)

		m.transitionToDefault()
		m.menu.SetState(ui.StateDefault)

		// Clean up the pending instance if it was partially created
		if m.pendingSessionInstance != nil {
			log.InfoLog.Printf("Cleaning up pending session instance: %s", m.pendingSessionInstance.Title)
			if err := m.pendingSessionInstance.Kill(); err != nil {
				log.ErrorLog.Printf("Failed to cleanup pending session: %v", err)
			}
			m.pendingSessionInstance = nil
		}

		m.statusBar.SetWarning("Session creation cancelled")
		return m, tea.WindowSize()
	}

	// Handle confirmation state BEFORE command registry to prevent key conflicts
	if m.isInState(state.Confirm) {
		confirmationOverlay := m.uiCoordinator.GetConfirmationOverlay()
		if confirmationOverlay != nil {
			shouldClose := confirmationOverlay.HandleKeyPress(msg)
			if shouldClose {
				m.transitionToDefault()
				// Note: overlay cleanup is handled by transitionToDefault()
				// Force screen refresh after modal dismissal to ensure UI updates correctly
				return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
			}
		}
		return m, nil
	}

	// Route through the new bridge command system
	log.DebugLog.Printf("Handling key: %s", msg.String())

	// Check if bridge is nil (should never happen)
	if m.bridge == nil {
		log.ErrorLog.Printf("CRITICAL: bridge is nil when handling key %s", msg.String())
		return m, m.handleError(fmt.Errorf("internal error: command bridge not initialized"))
	}

	// Use the bridge to handle the key
	model, teaCmd, err := m.bridge.HandleKeyString(msg.String())
	if err != nil {
		log.ErrorLog.Printf("Bridge key handling error: %v", err)
		return m, m.handleError(err)
	}

	if model != nil {
		log.DebugLog.Printf("Bridge handled key successfully: %s", msg.String())
		return model, teaCmd
	}

	// All keys should be handled by the bridge system now
	// If we reach here, the key is not registered in the bridge
	log.InfoLog.Printf("Unhandled key: %s", msg.String())
	return m, nil

}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance.
// It provides instant UI feedback and debounces expensive operations.
func (m *home) instanceChanged() tea.Cmd {
	// Use the optimized responsive navigation manager
	if m.responsiveNav == nil {
		m.responsiveNav = NewResponsiveNavigationManager()
	}

	selected := m.list.GetSelectedInstance()
	m.responsiveNav.HandleInstanceChange(m, selected)

	return nil // All expensive operations are now handled asynchronously
}

// Legacy methods removed - functionality moved to ResponsiveNavigationManager
// This eliminates blocking expensive operations during navigation

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(keyString string) tea.Cmd {
	// TODO: Update menu to use string-based key highlighting when menu system is migrated
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type previewResultsMsg struct{}

type tickUpdateMetadataMessage struct{}

type tickSessionDetectionMessage struct{}

type instanceChangedMsg struct{}

// terminalSizeCheckMsg removed - now handled by terminal.SizeCheckMsg

// instanceExpensiveUpdateMsg removed - handled by ResponsiveNavigationManager

// sessionCreationResultMsg is sent when async session creation completes
type sessionCreationResultMsg struct {
	instance        *session.Instance
	err             error
	promptAfterName bool
	autoYes         bool
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 500ms. Note that we iterate
// overall the instances and capture their output. It's a pretty expensive operation. Let's do it 2x a second only.
// CRITICAL: Uses proper async pattern to avoid blocking BubbleTea's main event loop
var tickUpdateMetadataCmd = tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
	return tickUpdateMetadataMessage{}
})

// tickSessionDetectionCmd creates a ticker for detecting new sessions created by other instances
// CRITICAL: Uses proper async pattern to avoid blocking BubbleTea's main event loop
func tickSessionDetectionCmd(intervalMs int) tea.Cmd {
	return tea.Tick(time.Duration(intervalMs)*time.Millisecond, func(time.Time) tea.Msg {
		return tickSessionDetectionMessage{}
	})
}

// tickTerminalSizeCheckCmd removed - now handled by terminal.SignalManager.CreateSizeCheckCmd()

// detectAndLoadNewSessions checks for new sessions created by other instances and adds them to the current list
func (m *home) detectAndLoadNewSessions() error {
	// Load all instances from storage (this will get the latest state including new sessions)
	allStorageInstances, err := m.storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances from storage: %w", err)
	}

	// Create a map of current instances by title for quick lookup
	currentInstances := make(map[string]*session.Instance)
	for _, instance := range m.list.GetInstances() {
		currentInstances[instance.Title] = instance
	}

	// Check for new instances that aren't in our current list
	newInstancesFound := false
	for _, storageInstance := range allStorageInstances {
		if _, exists := currentInstances[storageInstance.Title]; !exists {
			// This is a new instance we don't have yet
			log.InfoLog.Printf("Detected new session created by another instance: %s", storageInstance.Title)

			// Add the new instance to our list
			finalizer := m.list.AddInstance(storageInstance)
			finalizer() // Call finalizer to complete setup

			newInstancesFound = true
		}
	}

	// If we found new instances, save our updated list and trigger UI refresh
	if newInstancesFound {
		if err := m.saveAllInstances(); err != nil {
			log.WarningLog.Printf("failed to save instances after detecting new sessions: %v", err)
		}
		log.InfoLog.Printf("Successfully loaded new sessions from other instances")
	}

	return nil
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)

	// Also add to status bar for vim-style error visibility
	m.statusBar.SetError(err.Error())

	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.transitionToState(state.Confirm)

	// Create and show the confirmation overlay using coordinator
	if err := m.uiCoordinator.CreateConfirmationOverlay(message); err != nil {
		return m.handleError(fmt.Errorf("failed to create confirmation overlay: %w", err))
	}

	// Get the overlay and set callbacks
	confirmationOverlay := m.uiCoordinator.GetConfirmationOverlay()
	if confirmationOverlay != nil {
		// Set callbacks for confirmation and cancellation
		confirmationOverlay.OnConfirm = func() {
			m.transitionToDefault()
			// Execute the action if it exists
			if action != nil {
				_ = action()
			}
		}

		confirmationOverlay.SetOnCancel(func() {
			m.transitionToDefault()
		})
	}

	return nil
}

// handleGitStatus opens the git status overlay interface
func (m *home) handleGitStatus() (tea.Model, tea.Cmd) {
	selected := m.list.GetSelectedInstance()
	if selected == nil {
		return m, m.handleError(fmt.Errorf("no session selected for git operations"))
	}

	// Check if session has git integration
	if !selected.Started() || selected.Paused() {
		return m, m.handleError(fmt.Errorf("session must be started and active for git operations"))
	}

	// Create git status overlay using coordinator
	if err := m.uiCoordinator.CreateGitStatusOverlay(); err != nil {
		return m, m.handleError(fmt.Errorf("failed to create git status overlay: %w", err))
	}

	// Get the overlay and configure it
	gitStatusOverlay := m.uiCoordinator.GetGitStatusOverlay()
	if gitStatusOverlay != nil {
		gitStatusOverlay.SetSize(int(float32(m.list.NumInstances())*0.8), int(float32(m.list.NumInstances())*0.8))
		// Set up git operation callbacks
		m.setupGitCallbacks(selected, gitStatusOverlay)
	}

	// Change to git state
	m.transitionToState(state.Git)

	// Load git status data
	return m, m.loadGitStatus(selected)
}

// setupGitCallbacks configures the git operation callbacks for the overlay
func (m *home) setupGitCallbacks(instance *session.Instance, gitStatusOverlay *overlay.GitStatusOverlay) {
	gitStatusOverlay.OnCancel = func() {
		m.transitionToDefault()
		m.uiCoordinator.HideOverlay(appui.ComponentGitStatusOverlay)
	}

	// TODO: Implement these callbacks with actual git operations
	gitStatusOverlay.OnStageFile = func(path string) error {
		// Placeholder - will implement in next task
		return fmt.Errorf("staging not yet implemented")
	}

	gitStatusOverlay.OnUnstageFile = func(path string) error {
		// Placeholder - will implement in next task
		return fmt.Errorf("unstaging not yet implemented")
	}

	gitStatusOverlay.OnCommit = func() error {
		// Placeholder - will implement in next task
		return fmt.Errorf("commit not yet implemented")
	}

	gitStatusOverlay.OnPush = func() error {
		// Use existing push functionality
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return err
		}
		commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", instance.Title, time.Now().Format(time.RFC822))
		return worktree.PushChanges(commitMsg, true)
	}
}

// handleGitState processes key events when in git status mode
func (m *home) handleGitState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	gitStatusOverlay := m.uiCoordinator.GetGitStatusOverlay()
	if gitStatusOverlay == nil {
		m.transitionToDefault()
		return m, nil
	}

	shouldClose := gitStatusOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.transitionToDefault()
		m.uiCoordinator.HideOverlay(appui.ComponentGitStatusOverlay)
	}

	return m, nil
}

// handleClaudeSettingsState processes key events when in Claude settings mode
func (m *home) handleClaudeSettingsState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	claudeSettingsOverlay := m.uiCoordinator.GetClaudeSettingsOverlay()
	if claudeSettingsOverlay == nil {
		m.transitionToDefault()
		return m, nil
	}

	shouldClose := claudeSettingsOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.transitionToDefault()
		m.uiCoordinator.HideOverlay(appui.ComponentClaudeSettingsOverlay)
	}

	return m, nil
}

// handleZFSearchState processes key events when in ZF search mode
func (m *home) handleZFSearchState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	zfSearchOverlay := m.uiCoordinator.GetZFSearchOverlay()
	if zfSearchOverlay == nil {
		m.transitionToDefault()
		return m, nil
	}

	return m, zfSearchOverlay.Update(msg)
}

// handleTagEditorState processes key events when in tag editor mode
func (m *home) handleTagEditorState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tagEditorOverlay := m.uiCoordinator.GetTagEditorOverlay()
	if tagEditorOverlay == nil {
		m.transitionToDefault()
		return m, nil
	}

	shouldClose := tagEditorOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.transitionToDefault()
		m.uiCoordinator.HideOverlay(appui.ComponentTagEditorOverlay)
	}

	return m, nil
}

// handleWorkspaceState processes key events when in workspace mode
// This handles both the workspace switch overlay and workspace status overlay
func (m *home) handleWorkspaceState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Check for workspace status overlay first
	workspaceStatusOverlay := m.uiCoordinator.GetWorkspaceStatusOverlay()
	if workspaceStatusOverlay != nil {
		shouldClose := workspaceStatusOverlay.HandleKeyPress(msg)
		if shouldClose {
			m.transitionToDefault()
			m.uiCoordinator.HideOverlay(appui.ComponentWorkspaceStatusOverlay)
		}
		return m, nil
	}

	// Fall back to workspace switch overlay
	workspaceSwitchOverlay := m.uiCoordinator.GetWorkspaceSwitchOverlay()
	if workspaceSwitchOverlay == nil {
		m.transitionToDefault()
		return m, nil
	}

	shouldClose := workspaceSwitchOverlay.HandleKeyPress(msg)
	if shouldClose {
		m.transitionToDefault()
		m.uiCoordinator.HideOverlay(appui.ComponentWorkspaceSwitchOverlay)
	}

	return m, nil
}

func (m *home) handleHistoryBrowserState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	historyBrowserOverlay := m.uiCoordinator.GetHistoryBrowserOverlay()
	if historyBrowserOverlay == nil {
		// Overlay disappeared, return to default
		m.transitionToDefault()
		return m, nil
	}

	// Let overlay handle the key press
	shouldClose := historyBrowserOverlay.HandleKeyPress(msg)
	if shouldClose {
		// Overlay requested close
		m.uiCoordinator.HideOverlay(appui.ComponentHistoryBrowserOverlay)
		m.transitionToDefault()
	}

	return m, nil
}

func (m *home) handleConfigEditorState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	configEditorOverlay := m.uiCoordinator.GetConfigEditorOverlay()
	if configEditorOverlay == nil {
		// Overlay disappeared, return to default
		m.transitionToDefault()
		return m, nil
	}

	// Let overlay handle the key press
	shouldClose := configEditorOverlay.HandleKeyPress(msg)
	if shouldClose {
		// Overlay requested close
		m.uiCoordinator.HideOverlay(appui.ComponentConfigEditorOverlay)
		m.transitionToDefault()
	}

	return m, nil
}

// handleRenameState processes key events when in rename mode
func (m *home) handleRenameState(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	renameOverlay := m.uiCoordinator.GetRenameInputOverlay()
	if renameOverlay == nil {
		// Overlay disappeared, return to default
		m.transitionToDefault()
		return m, nil
	}

	// Let overlay handle the key press
	shouldClose := renameOverlay.HandleKeyPress(msg)
	if shouldClose {
		// Overlay requested close
		m.uiCoordinator.HideOverlay(appui.ComponentRenameInputOverlay)
		m.transitionToDefault()
	}

	return m, nil
}

// loadGitStatus loads git status information for the overlay
func (m *home) loadGitStatus(instance *session.Instance) tea.Cmd {
	return func() tea.Msg {
		// TODO: Implement actual git status loading
		// For now, return some mock data to test the interface
		files := []overlay.GitFileStatus{
			{Path: "app/app.go", Staged: false, Modified: true, StatusChar: "M"},
			{Path: "keys/keys.go", Staged: true, Modified: false, StatusChar: "A"},
			{Path: "ui/overlay/gitStatusOverlay.go", Staged: false, Modified: false, Untracked: true, StatusChar: "??"},
		}

		return gitStatusLoadedMsg{
			files:      files,
			branchName: "feature/fugitive-git-integration",
		}
	}
}

// gitStatusLoadedMsg carries git status data
type gitStatusLoadedMsg struct {
	files      []overlay.GitFileStatus
	branchName string
}

// createSessionWithTimeout creates a session with a timeout to prevent hanging
func (m *home) createSessionWithTimeout(instance *session.Instance, promptAfterName bool, autoYes bool) tea.Cmd {
	return func() tea.Msg {
		log.InfoLog.Printf("Starting session creation for '%s' with timeout monitoring", instance.Title)

		// Create a cancellable context for this session creation
		ctx, cancel := context.WithCancel(m.ctx)
		m.sessionCreationCancel = cancel // Store cancel function for Ctrl+C handler

		// Channel to receive the result
		resultChan := make(chan sessionCreationResultMsg, 1)

		// Start timestamp for duration tracking
		startTime := time.Now()

		// Start the session creation in a goroutine
		go func() {
			defer cancel() // Clean up context when done
			log.InfoLog.Printf("Session creation goroutine started for '%s'", instance.Title)

			// Add periodic logging to track progress
			progressTicker := time.NewTicker(10 * time.Second)
			defer progressTicker.Stop()

			// Channel to signal completion
			done := make(chan bool, 1)

			// Start the actual session creation in another goroutine
			go func() {
				defer func() { done <- true }()
				log.InfoLog.Printf("Calling instance.Start(true) for '%s'", instance.Title)
				err := instance.Start(true)
				if err != nil {
					log.ErrorLog.Printf("Session creation failed for '%s': %v", instance.Title, err)
				} else {
					log.InfoLog.Printf("Session creation completed successfully for '%s' (duration: %v)", instance.Title, time.Since(startTime))
				}

				// Check if context was cancelled before sending result
				select {
				case <-ctx.Done():
					log.InfoLog.Printf("Session creation for '%s' was cancelled, not sending result", instance.Title)
					return
				default:
					resultChan <- sessionCreationResultMsg{
						instance:        instance,
						err:             err,
						promptAfterName: promptAfterName,
						autoYes:         autoYes,
					}
				}
			}()

			// Monitor progress with periodic logging
			for {
				select {
				case <-ctx.Done():
					log.InfoLog.Printf("Session creation monitoring cancelled for '%s'", instance.Title)
					return
				case <-done:
					return
				case <-progressTicker.C:
					elapsed := time.Since(startTime)
					log.DebugLog.Printf("Session creation for '%s' still in progress (elapsed: %v)", instance.Title, elapsed)
				}
			}
		}()

		// Wait for either completion, cancellation, or timeout
		select {
		case <-ctx.Done():
			log.InfoLog.Printf("Session creation for '%s' cancelled by user", instance.Title)
			return sessionCreationResultMsg{
				instance:        instance,
				err:             fmt.Errorf("session creation cancelled by user"),
				promptAfterName: promptAfterName,
				autoYes:         autoYes,
			}
		case result := <-resultChan:
			totalDuration := time.Since(startTime)
			if result.err != nil {
				log.ErrorLog.Printf("Session creation for '%s' completed with error after %v: %v", instance.Title, totalDuration, result.err)
			} else {
				log.InfoLog.Printf("Session creation for '%s' completed successfully after %v", instance.Title, totalDuration)
			}
			return result
		case <-time.After(60 * time.Second): // 60 second timeout
			// Session creation timed out - clean up and return error
			log.ErrorLog.Printf("Session creation for '%s' timed out after 60 seconds", instance.Title)
			if cleanupErr := instance.Kill(); cleanupErr != nil {
				log.ErrorLog.Printf("Failed to cleanup instance '%s' after timeout: %v", instance.Title, cleanupErr)
			} else {
				log.InfoLog.Printf("Successfully cleaned up timed-out instance '%s'", instance.Title)
			}
			return sessionCreationResultMsg{
				instance:        instance,
				err:             fmt.Errorf("session creation timed out after 60 seconds"),
				promptAfterName: promptAfterName,
				autoYes:         autoYes,
			}
		}
	}
}

func (m *home) View() string {
	// Check if we're in PTY view mode
	if m.viewMode == ViewModePTYs {
		return m.renderPTYView()
	}

	// Check if we're in review queue view mode
	if m.viewMode == ViewModeReviewQueue {
		return m.renderReviewQueueView()
	}

	// Build main view layout (session list)
	listContent := m.list.String()
	tabbedContent := m.tabbedWindow.String()
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listContent, tabbedContent)
	mainView := lipgloss.JoinVertical(
		lipgloss.Left,
		listAndPreview,
		m.menu.String(),
		m.statusBar.View(m.termWidth),
		m.errBox.String(),
	)

	// Get view directive from state manager
	// Defensive nil check to prevent panic in tests or edge cases
	if m.stateManager == nil {
		return "Error: State manager not initialized"
	}
	directive := m.stateManager.GetViewDirective()

	// Handle different view types based on directive
	switch directive.Type {
	case state.ViewMain:
		return mainView

	case state.ViewOverlay:
		return m.renderOverlay(directive, mainView)

	case state.ViewText:
		return m.renderTextOverlay(directive, mainView)

	case state.ViewSpinner:
		return m.renderSpinnerOverlay(directive, mainView)

	default:
		// Fallback to main view for unknown directive types
		return mainView
	}
}

// View rendering helper methods

// renderOverlay renders an overlay component based on the directive
func (m *home) renderOverlay(directive state.ViewDirective, mainView string) string {
	// Map overlay component names to component types
	var componentType appui.ComponentType
	var validComponent bool

	switch directive.OverlayComponent {
	case "textInputOverlay":
		componentType = appui.ComponentTextInputOverlay
		validComponent = true
	case "liveSearchOverlay":
		componentType = appui.ComponentLiveSearchOverlay
		validComponent = true
	case "confirmationOverlay":
		componentType = appui.ComponentConfirmationOverlay
		validComponent = true
	case "sessionSetupOverlay":
		componentType = appui.ComponentSessionSetupOverlay
		validComponent = true
	case "gitStatusOverlay":
		componentType = appui.ComponentGitStatusOverlay
		validComponent = true
	case "claudeSettingsOverlay":
		componentType = appui.ComponentClaudeSettingsOverlay
		validComponent = true
	case "zfSearchOverlay":
		componentType = appui.ComponentZFSearchOverlay
		validComponent = true
	case "messagesOverlay":
		componentType = appui.ComponentMessagesOverlay
		validComponent = true
	case "tagEditorOverlay":
		componentType = appui.ComponentTagEditorOverlay
		validComponent = true
	case "historyBrowserOverlay":
		componentType = appui.ComponentHistoryBrowserOverlay
		validComponent = true
	case "configEditorOverlay":
		componentType = appui.ComponentConfigEditorOverlay
		validComponent = true
	case "renameInputOverlay":
		componentType = appui.ComponentRenameInputOverlay
		validComponent = true
	case "workspaceSwitchOverlay":
		componentType = appui.ComponentWorkspaceSwitchOverlay
		validComponent = true
	case "workspaceStatusOverlay":
		componentType = appui.ComponentWorkspaceStatusOverlay
		validComponent = true
	default:
		validComponent = false
	}

	if !validComponent {
		if directive.ShouldResetOnNil {
			log.ErrorLog.Printf("Unknown overlay component '%s' - resetting to default state", directive.OverlayComponent)
			m.transitionToDefault()
		}
		return mainView
	}

	// Use coordinator to render the overlay
	overlayContent := m.uiCoordinator.RenderOverlay(componentType)
	if overlayContent == "" {
		if directive.ShouldResetOnNil {
			log.ErrorLog.Printf("%s overlay is not active - resetting to default state", directive.OverlayComponent)
			m.transitionToDefault()
		}
		return mainView
	}

	// Place overlay with specified positioning
	return overlay.PlaceOverlay(0, 0, overlayContent, mainView, directive.Centered, directive.Bordered)
}

// renderTextOverlay renders a text-based overlay (legacy text overlay)
func (m *home) renderTextOverlay(directive state.ViewDirective, mainView string) string {
	if m.textOverlay == nil {
		if directive.ShouldResetOnNil {
			log.ErrorLog.Printf("text overlay is nil - resetting to default state")
			m.transitionToDefault()
		}
		return mainView
	}

	return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, directive.Centered, directive.Bordered)
}

// renderSpinnerOverlay renders a progress spinner with message
func (m *home) renderSpinnerOverlay(directive state.ViewDirective, mainView string) string {
	// Create styled message with spinner
	spinnerMsg := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Render(
		m.spinner.View() + " " + directive.Message)

	return overlay.PlaceOverlay(0, 0, spinnerMsg, mainView, directive.Centered, directive.Bordered)
}

// State Manager Integration Helper Methods

// Helper methods for common state transitions

// isInState checks if the current state matches the given state
func (m *home) isInState(state state.State) bool {
	return m.stateManager.Current() == state
}

// setState sets the current state (facade for stateManager.Transition)
func (m *home) setState(targetState state.State) error {
	return m.stateManager.Transition(targetState)
}

// getState returns the current state (facade for stateManager.Current)
func (m *home) getState() state.State {
	return m.stateManager.Current()
}

// transitionToState performs a simple state transition
func (m *home) transitionToState(targetState state.State) error {
	return m.stateManager.Transition(targetState)
}

// transitionToDefault transitions to default state with cleanup actions
func (m *home) transitionToDefault() error {
	ctx := state.TransitionContext{
		MenuState:          "Default",
		ShouldCloseOverlay: true,
		PostTransitionAction: func() error {
			// Set menu state
			m.menu.SetState(ui.StateDefault)
			// Clear any overlays that should be closed
			if ctx := m.stateManager.GetTransitionContext(); ctx.ShouldCloseOverlay {
				m.clearOverlaysForState()
			}
			return nil
		},
	}

	_, err := m.stateManager.TransitionToDefault(ctx)
	return err
}

// transitionToOverlay transitions to an overlay state with setup
func (m *home) transitionToOverlay(targetState state.State, menuState string, overlayName string) error {
	ctx := state.TransitionContext{
		MenuState:   menuState,
		OverlayName: overlayName,
		PostTransitionAction: func() error {
			// Set appropriate menu state
			switch menuState {
			case "CreatingInstance":
				m.menu.SetState(ui.StateCreatingInstance)
			case "Search":
				m.menu.SetState(ui.StateSearch)
			case "Prompt":
				m.menu.SetState(ui.StatePrompt)
			default:
				m.menu.SetState(ui.StateDefault)
			}
			return nil
		},
	}

	_, err := m.stateManager.TransitionToOverlay(targetState, ctx)
	return err
}

// transitionToCreatingSession transitions to session creation state
func (m *home) transitionToCreatingSession() error {
	_, err := m.stateManager.TransitionToCreatingSession()
	if err == nil {
		// Set session flags
		m.asyncSessionCreationActive = true
		m.menu.SetState(ui.StateCreatingInstance)
	}
	return err
}

// clearOverlaysForState clears overlays when transitioning to default
func (m *home) clearOverlaysForState() {
	// Use coordinator to close all overlays
	if err := m.uiCoordinator.CloseAllOverlays(); err != nil {
		log.WarningLog.Printf("Error closing overlays: %v", err)
	}

	// Clear remaining legacy overlay references (help system overlays)
	m.textOverlay = nil
	m.messagesOverlay = nil
	// Note: All other overlays now managed by coordinator.CloseAllOverlays()
}

// populateCoordinatorRegistry populates the coordinator registry with existing components
// This enables gradual migration from direct component access to coordinator-based access
func (m *home) populateCoordinatorRegistry() {
	registry := m.uiCoordinator.GetRegistry()

	// Set existing components in the registry
	if m.list != nil {
		m.uiCoordinator.SetComponent(appui.ComponentList, m.list)
	}
	if m.menu != nil {
		m.uiCoordinator.SetComponent(appui.ComponentMenu, m.menu)
	}
	if m.tabbedWindow != nil {
		m.uiCoordinator.SetComponent(appui.ComponentTabbedWindow, m.tabbedWindow)
	}
	if m.errBox != nil {
		m.uiCoordinator.SetComponent(appui.ComponentErrBox, m.errBox)
	}
	if m.statusBar != nil {
		m.uiCoordinator.SetComponent(appui.ComponentStatusBar, m.statusBar)
	}

	// Set spinner from coordinator (it's initialized there)
	m.spinner = registry.Spinner
}

// Migration helper methods - these will help transition from direct component access to coordinator access

// getCoordinatorComponent returns a component from the coordinator registry
func (m *home) getCoordinatorComponent(componentType appui.ComponentType) interface{} {
	return m.uiCoordinator.GetComponentByType(componentType)
}

// updateCoordinatorOverlay updates an overlay in the coordinator registry
func (m *home) updateCoordinatorOverlay(componentType appui.ComponentType, overlay interface{}) error {
	return m.uiCoordinator.SetComponent(componentType, overlay)
}

// Facade methods for Law of Demeter compliance

// addInstanceToList adds an instance to the list using proper encapsulation
func (h *home) addInstanceToList(instance *session.Instance) {
	// Wire up review queue to new instance
	instance.SetReviewQueue(h.reviewQueue)

	// Wire up status manager for idle detection
	instance.SetStatusManager(h.statusManager)

	// Call the finalizer immediately to complete the setup
	finalizer := h.list.AddInstance(instance)
	finalizer()

	// Add instance to review queue poller for monitoring
	h.reviewQueuePoller.AddInstance(instance)

	// Ensure controller is started for active instances
	if instance.Started() && !instance.Paused() {
		if err := instance.StartController(); err != nil {
			log.WarningLog.Printf("Failed to start controller for new instance '%s': %v", instance.Title, err)
		}
	}
}

// restoreLastSelection restores the last selected index with proper bounds checking
func (h *home) restoreLastSelection() {
	lastSelectedIdx := h.getLastSelectedIndex()
	totalInstances := h.getInstanceCount()

	if lastSelectedIdx >= 0 && lastSelectedIdx < totalInstances {
		// Check if this selection would require excessive scrolling on startup
		// If so, start from the beginning for better UX
		maxReasonableStartIndex := min(totalInstances-1, 10) // Don't start beyond item 10
		if lastSelectedIdx <= maxReasonableStartIndex {
			h.setSelectedIndex(lastSelectedIdx)
		} else {
			// Start from the beginning if the last selection was too far down
			h.setSelectedIndex(0)
		}
	} else {
		// Default to first item if no valid last index
		if totalInstances > 0 {
			h.setSelectedIndex(0)
		}
	}
}

// Facade methods for state access (avoiding direct appState access)

// getLastSelectedIndex returns the last selected index from app state
func (h *home) getLastSelectedIndex() int {
	return h.appState.GetSelectedIndex()
}

// Facade methods for list operations (avoiding direct list access)

// getInstanceCount returns the number of instances in the list
func (h *home) getInstanceCount() int {
	return h.list.NumInstances()
}

// setSelectedIndex sets the selected index in the list
func (h *home) setSelectedIndex(index int) {
	h.list.SetSelectedIdx(index)
}

// navigateUp moves list selection up
func (h *home) navigateUp() {
	h.list.Up()
}

// navigateDown moves list selection down
func (h *home) navigateDown() {
	h.list.Down()
}

// getSelectedInstance returns the currently selected instance
func (h *home) getSelectedInstance() *session.Instance {
	return h.list.GetSelectedInstance()
}

// getAllInstances returns all instances from the list
func (h *home) getAllInstances() []*session.Instance {
	return h.list.GetInstances()
}

// isAtNavigationStart checks if we're at the start of the list
func (h *home) isAtNavigationStart() bool {
	selected := h.getSelectedInstance()
	return selected == nil || h.getInstanceCount() == 0
}

// isAtNavigationEnd checks if we're at the end of the list
func (h *home) isAtNavigationEnd() bool {
	if h.getInstanceCount() == 0 {
		return true
	}

	// Get current selection index and compare with list size
	instances := h.getAllInstances()
	selectedIndex := -1
	selected := h.getSelectedInstance()

	if selected == nil {
		return true
	}

	// Find the index of the selected instance
	for i, instance := range instances {
		if instance == selected {
			selectedIndex = i
			break
		}
	}

	// We're at the end if we're at the last index
	return selectedIndex >= len(instances)-1
}

// saveAllInstances saves all instances to storage using proper encapsulation
func (h *home) saveAllInstances() error {
	return h.storage.SaveInstances(h.getAllInstances())
}

// populateWorkspaceStatus populates the workspace status overlay with data from all sessions
func (m *home) populateWorkspaceStatus(overlayRef *overlay.WorkspaceStatusOverlay) {
	var workspaces []overlay.WorkspaceInfo
	var summary overlay.WorkspaceSummary

	sessions := m.getAllInstances()
	repoSet := make(map[string]bool)

	for _, inst := range sessions {
		if inst == nil {
			continue
		}

		// Get VCS info for the session
		vcsInfo, err := inst.GetVCSInfo()
		if err != nil {
			log.WarningLog.Printf("Failed to get VCS info for session %s: %v", inst.Title, err)
			continue
		}

		// Create workspace info from VCS info
		info := overlay.WorkspaceInfo{
			Path:           inst.Path,
			SessionTitle:   inst.Title,
			SessionStatus:  inst.Status,
			IsWorktree:     inst.HasGitWorktree(),
			Branch:         vcsInfo.CurrentBookmark,
			ModifiedCount:  vcsInfo.ModifiedFileCount,
			UntrackedCount: 0, // Not available from VCSInfo
			StagedCount:    0, // Not available from VCSInfo
			ConflictCount:  0, // Not available from VCSInfo
			IsClean:        !vcsInfo.HasUncommittedChanges,
			IsOrphaned:     false,
			RepositoryRoot: vcsInfo.RepoPath,
		}

		workspaces = append(workspaces, info)

		// Track unique repositories
		repoRoot := info.RepositoryRoot
		if repoRoot == "" {
			repoRoot = inst.Path
		}
		repoSet[repoRoot] = true

		// Update summary counts
		summary.TotalUncommitted += info.ModifiedCount
		summary.TotalUntracked += info.UntrackedCount
		summary.TotalStaged += info.StagedCount
		summary.TotalConflicts += info.ConflictCount
		if !info.IsClean {
			summary.WorkspacesWithWork++
		}
		if info.IsOrphaned {
			summary.OrphanedWorkspaces++
		}
	}

	summary.TotalRepositories = len(repoSet)
	summary.TotalWorkspaces = len(workspaces)

	overlayRef.SetWorkspaces(workspaces, summary)
}
