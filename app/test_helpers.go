package app

import (
	appsession "claude-squad/app/session"
	appui "claude-squad/app/ui"
	"claude-squad/app/state"
	"claude-squad/cmd"
	"claude-squad/config"
	"claude-squad/executor"
	"claude-squad/session"
	"claude-squad/session/tmux"
	"claude-squad/terminal"
	"claude-squad/ui"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/stretchr/testify/require"
)

// TestHomeBuilder provides a fluent interface for building test home instances
// This follows the builder pattern to avoid Law of Demeter violations
type TestHomeBuilder struct {
	ctx       context.Context
	program   string
	autoYes   bool
	withBridge bool
}

// NewTestHomeBuilder creates a new test home builder
func NewTestHomeBuilder() *TestHomeBuilder {
	return &TestHomeBuilder{
		ctx:     context.Background(),
		program: "claude",
		autoYes: false,
		withBridge: false,
	}
}

// WithContext sets the context for the test home
func (b *TestHomeBuilder) WithContext(ctx context.Context) *TestHomeBuilder {
	b.ctx = ctx
	return b
}

// WithProgram sets the program for the test home
func (b *TestHomeBuilder) WithProgram(program string) *TestHomeBuilder {
	b.program = program
	return b
}

// WithAutoYes enables auto-yes mode for the test home
func (b *TestHomeBuilder) WithAutoYes(autoYes bool) *TestHomeBuilder {
	b.autoYes = autoYes
	return b
}

// WithBridge enables bridge initialization for tests that need command handling
func (b *TestHomeBuilder) WithBridge() *TestHomeBuilder {
	b.withBridge = true
	return b
}

// Build creates the test home instance using dependency injection
// This ensures tests use clean architecture patterns
func (b *TestHomeBuilder) Build(t *testing.T) *home {
	t.Helper()

	// Create production dependencies
	deps := NewProductionDependencies(b.ctx, b.program, b.autoYes)

	// Use the dependency injection constructor
	h := newHomeWithDependencies(deps)

	// Initialize bridge if requested (for tests that need command handling)
	if b.withBridge {
		bridge := deps.GetBridge()
		bridge.Initialize(nil, nil, nil, nil, nil)
		bridge.SetContext(cmd.ContextList)
	}

	return h
}

// BuildWithMockDependencies creates a test home with mock dependencies for unit testing
func (b *TestHomeBuilder) BuildWithMockDependencies(t *testing.T, setupMocks func(*MockDependencies)) *home {
	t.Helper()

	// Create mock dependencies with isolated storage to avoid loading production data
	mockDeps := NewMockDependenciesWithIsolatedStorage(t)

	// Allow test to customize mock dependencies
	if setupMocks != nil {
		setupMocks(mockDeps)
	}

	// Use the dependency injection constructor
	return newHomeWithDependencies(mockDeps)
}

// BuildWithMockDependenciesNoInit creates a test home without initializing saved data
func (b *TestHomeBuilder) BuildWithMockDependenciesNoInit(t *testing.T, setupMocks func(*MockDependencies)) *home {
	t.Helper()

	// Create mock dependencies with isolated storage to avoid loading production data
	mockDeps := NewMockDependenciesWithIsolatedStorage(t)

	// Allow test to customize mock dependencies
	if setupMocks != nil {
		setupMocks(mockDeps)
	}

	// Create home instance with injected dependencies (without initialization)
	h := &home{
		ctx:                  mockDeps.GetContext(),
		deps:                 mockDeps, // Store deps for shared state updates
		program:              mockDeps.GetProgram(),
		autoYes:              mockDeps.GetAutoYes(),
		storage:              mockDeps.GetStorage(),
		appConfig:            mockDeps.GetAppConfig(),
		appState:             mockDeps.GetAppState(),
		stateManager:         mockDeps.GetStateManager(),
		terminalManager:      mockDeps.GetTerminalManager(),
		signalManager:        mockDeps.GetSignalManager(),
		list:                 mockDeps.GetList(),
		menu:                 mockDeps.GetMenu(),
		tabbedWindow:         mockDeps.GetTabbedWindow(),
		errBox:               mockDeps.GetErrBox(),
		spinner:              mockDeps.GetSpinner(),
		statusBar:            mockDeps.GetStatusBar(),
		uiCoordinator:        mockDeps.GetUICoordinator(),
		bridge:               mockDeps.GetBridge(),

		// Initialize PTY management to match production initialization
		viewMode:     ViewModeSessions,
		ptyDiscovery: session.NewPTYDiscovery(),
		ptyList:      ui.NewPTYList(),
		ptyPreview:   ui.NewPTYPreview(),

		// Initialize review queue
		reviewQueue: session.NewReviewQueue(),
	}

	// Initialize session controller with dependencies - only in test mode
	h.sessionController = appsession.NewController(appsession.Dependencies{
		List:                h.list,
		Storage:             h.storage,
		AutoYes:             h.autoYes,
		GlobalInstanceLimit: GlobalInstanceLimit,
		ErrorHandler:        h.handleError,
		StateTransition: func(to string) error {
			// Simple state transitions for tests
			switch to {
			case "Default":
				return h.transitionToDefault()
			default:
				return fmt.Errorf("unknown state transition: %s", to)
			}
		},
		InstanceChanged: h.instanceChanged,
		ConfirmAction:   h.confirmAction,
		ShowHelpScreen: func(helpType interface{}, onComplete func()) {
			// Mock help screen for tests
		},
		SetSessionSetupOverlay: func(overlay *overlay.SessionSetupOverlay) {
			// Mock overlay setup for tests
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

	// Initialize the command bridge in test mode (old command registry removed)
	h.initializeCommandBridge()

	return h
}

// CreateTestSession creates a test session instance for use in tests
func CreateTestSession(t *testing.T, title string) *session.Instance {
	t.Helper()

	instance, err := session.NewInstance(session.InstanceOptions{
		Title:            title,
		Path:             t.TempDir(),
		Program:          "claude",
		AutoYes:          false,
		TmuxServerSocket: "teatest_" + t.Name(), // Isolated tmux server for this test
	})
	require.NoError(t, err)

	return instance
}

// CreateTestSessionWithOptions creates a test session with custom options
func CreateTestSessionWithOptions(t *testing.T, opts session.InstanceOptions) *session.Instance {
	t.Helper()

	// Set defaults if not provided
	if opts.Path == "" {
		opts.Path = t.TempDir()
	}
	if opts.Program == "" {
		opts.Program = "claude"
	}
	if opts.TmuxServerSocket == "" {
		opts.TmuxServerSocket = "teatest_" + t.Name() // Isolated tmux server for this test
	}

	instance, err := session.NewInstance(opts)
	require.NoError(t, err)

	return instance
}

// SetupTestHomeWithSession creates a test home with a single session
func SetupTestHomeWithSession(t *testing.T, sessionTitle string) *home {
	t.Helper()

	// Use mock dependencies without loading saved data
	h := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Use minimal required dependencies for testing
		mocks.SetMockAppConfig(config.DefaultConfig())
		mocks.SetMockAppState(config.LoadState())
	})

	session := CreateTestSession(t, sessionTitle)

	// Add session to list
	_ = h.list.AddInstance(session)
	h.list.SetSelectedInstance(0)

	return h
}

// SetupTestHomeWithMultipleSessions creates a test home with multiple sessions
func SetupTestHomeWithMultipleSessions(t *testing.T, sessionTitles []string) *home {
	t.Helper()

	// Use mock dependencies without loading saved data
	h := NewTestHomeBuilder().BuildWithMockDependenciesNoInit(t, func(mocks *MockDependencies) {
		// Use minimal required dependencies for testing
		mocks.SetMockAppConfig(config.DefaultConfig())
		mocks.SetMockAppState(config.LoadState())
	})

	for _, title := range sessionTitles {
		session := CreateTestSession(t, title)
		_ = h.list.AddInstance(session)
	}

	// Select first session
	if len(sessionTitles) > 0 {
		h.list.SetSelectedInstance(0)
	}

	return h
}

// SetupMinimalTestHome creates a minimal test home for unit tests that don't need full initialization
func SetupMinimalTestHome(t *testing.T) *home {
	t.Helper()

	// Use dependency injection with minimal setup
	return NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
		// Set up mock command executor to prevent external command execution
		setupMockCommandExecutor()

		// Set up minimal required dependencies
		mocks.SetMockAppConfig(config.DefaultConfig())
		mocks.SetMockAppState(config.LoadState())

		// Create minimal UI components
		spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
		list := ui.NewList(&spinner, false, nil)
		menu := ui.NewMenu()

		mocks.SetMockList(list)
		mocks.spinner = spinner
		mocks.menu = menu
		mocks.stateManager = state.NewManager()
		mocks.uiCoordinator = appui.NewCoordinator()
	})
}

// SetupTestHomeWithMockedStorage creates a test home with mocked storage for unit tests
func SetupTestHomeWithMockedStorage(t *testing.T, mockStorage *session.Storage) *home {
	t.Helper()

	return NewTestHomeBuilder().BuildWithMockDependencies(t, func(mocks *MockDependencies) {
		// Set up full dependencies with mocked storage
		mocks.SetMockAppConfig(config.DefaultConfig())
		mocks.SetMockAppState(config.LoadState())
		mocks.storage = mockStorage

		// Set up other required dependencies
		spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
		list := ui.NewList(&spinner, false, nil)

		mocks.SetMockList(list)
		mocks.spinner = spinner
		mocks.menu = ui.NewMenu()
		mocks.stateManager = state.NewManager()
		mocks.uiCoordinator = appui.NewCoordinator()
	})
}

// CleanupTestTmuxServer cleans up the isolated tmux server for a specific test
// This should be deferred in tests that create sessions with tmux backends
func CleanupTestTmuxServer(t *testing.T) {
	t.Helper()

	serverSocket := "teatest_" + t.Name()
	cmdExec := executor.MakeExecutor()

	// Clean up any tmux sessions on the isolated server
	if err := tmux.CleanupSessionsOnServer(cmdExec, serverSocket); err != nil {
		// Log warning but don't fail test - cleanup is best effort
		t.Logf("Warning: failed to cleanup tmux server '%s': %v", serverSocket, err)
	}
}

// CreateTestStateInTempDir creates a test state with isolated storage in a unique temporary directory
func CreateTestStateInTempDir(t *testing.T) *config.State {
	t.Helper()

	// Create a unique temporary directory for this test
	testDir := t.TempDir()
	return config.NewTestState(testDir)
}

// NewMockDependenciesWithIsolatedStorage creates mock dependencies with test-specific isolated storage
func NewMockDependenciesWithIsolatedStorage(t *testing.T) *MockDependencies {
	t.Helper()

	// Create test-specific state to avoid loading production data
	appState := CreateTestStateInTempDir(t)
	storage, _ := session.NewStorage(appState) // Ignore error in tests

	// Create UI components
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := ui.NewList(&spinner, false, appState)
	menu := ui.NewMenu()
	tabbedWindow := ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane())
	errBox := ui.NewErrBox()
	terminalManager := terminal.NewManager()
	signalManager := terminal.NewSignalManager(terminalManager)
	bridge := cmd.GetGlobalBridge()

	return &MockDependencies{
		ctx:             context.Background(),
		appConfig:       config.DefaultConfig(),
		appState:        appState,
		storage:         storage,
		stateManager:    state.NewManager(), // Each instance gets a new manager
		uiCoordinator:   appui.NewCoordinator(),
		statusBar:       ui.NewStatusBar(),
		spinner:         spinner,
		list:            list,
		menu:            menu,
		tabbedWindow:    tabbedWindow,
		errBox:          errBox,
		terminalManager: terminalManager,
		signalManager:   signalManager,
		bridge:          bridge,
	}
}

// setupMockCommandExecutor sets up a mock command executor to prevent external command execution during tests
func setupMockCommandExecutor() {
	// Create a mock command executor that returns predictable results
	mockExecutor := &mockCommandExecutor{
		OutputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			// Provide default responses for common commands
			cmdStr := cmd.String()
			if cmd.Args != nil && len(cmd.Args) > 0 {
				switch {
				case strings.Contains(cmdStr, "which claude"):
					return []byte("/usr/local/bin/claude"), nil
				case strings.Contains(cmdStr, "git"):
					return []byte("mock git output"), nil
				default:
					return []byte("mock command output"), nil
				}
			}
			return []byte(""), nil
		},
		LookPathFunc: func(file string) (string, error) {
			// Return mock paths for common executables
			switch file {
			case "claude":
				return "/usr/local/bin/claude", nil
			case "git":
				return "/usr/bin/git", nil
			default:
				return "/usr/bin/" + file, nil
			}
		},
	}

	config.SetCommandExecutor(mockExecutor)
}

// mockCommandExecutor implements config.CommandExecutor for testing
type mockCommandExecutor struct {
	CommandFunc  func(name string, args ...string) *exec.Cmd
	OutputFunc   func(cmd *exec.Cmd) ([]byte, error)
	LookPathFunc func(file string) (string, error)
}

func (m *mockCommandExecutor) Command(name string, args ...string) *exec.Cmd {
	if m.CommandFunc != nil {
		return m.CommandFunc(name, args...)
	}
	return exec.Command("echo", "mock")
}

func (m *mockCommandExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.OutputFunc != nil {
		return m.OutputFunc(cmd)
	}
	return []byte("mock output"), nil
}

func (m *mockCommandExecutor) LookPath(file string) (string, error) {
	if m.LookPathFunc != nil {
		return m.LookPathFunc(file)
	}
	return "/usr/bin/" + file, nil
}

