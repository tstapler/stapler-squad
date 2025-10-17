package overlay

import (
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui/fuzzy"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionSetupStep represents the current step in the session setup wizard
type SessionSetupStep int

const (
	StepBasics   SessionSetupStep = iota // Name + Program in one step
	StepLocation                         // Location choice (streamlined)
	StepAdvanced                         // Category + Tags (optional, combined)
	StepConfirm                          // Final confirmation

	// Legacy steps still referenced in complex flows
	StepRepository
	StepDirectory
	StepWorktree
	StepBranch
)

// BranchChoice represents the branch selection strategy
type BranchChoice string

const (
	BranchChoiceNew      BranchChoice = "new"
	BranchChoiceCurrent  BranchChoice = "current"
	BranchChoiceExisting BranchChoice = "existing"
)

// SessionSetupCallbacks contains the required callbacks for SessionSetupOverlay
// This struct ensures that all necessary callbacks are provided at construction time,
// preventing runtime errors from missing callbacks.
type SessionSetupCallbacks struct {
	// OnComplete is called when the user confirms the session setup
	// This callback is REQUIRED and will cause a panic if nil
	OnComplete func(session.InstanceOptions)

	// OnCancel is called when the user cancels the session setup (presses Esc)
	// This callback is optional - if nil, the overlay will use the base cancel handler
	OnCancel func()
}

// SessionSetupOverlay is a multi-step modal for configuring a new session
type SessionSetupOverlay struct {
	BaseOverlay // Embed base for common overlay functionality

	// Core state
	step    SessionSetupStep
	error   string
	warning string

	// Session configuration being built
	sessionName      string
	repoPath         string
	workingDir       string
	program          string
	branch           string
	existingWorktree string
	category         string

	// Step-specific states
	locationChoice    string // "current", "different", "existing"
	branchChoice      BranchChoice
	showAdvanced      bool // Whether to show advanced options
	locationNavigator *NavigationHandler
	branchNavigator   *NavigationHandler

	// Combined input state for basics step
	basicsFocused string // "name" or "program"
	basicsNavigator *NavigationHandler

	// UI components for different steps
	nameInput           *TextInputOverlay
	programInput        *TextInputOverlay
	categoryInput       *TextInputOverlay
	categorySelector    *FuzzyInputOverlay
	repoSelector        *FuzzyInputOverlay
	dirBrowser          *FuzzyInputOverlay
	fuzzyDirBrowser     *FuzzyDirectoryBrowser // New FZF-style directory browser
	worktreeSelector    *FuzzyInputOverlay
	branchSelector      *FuzzyInputOverlay

	// UI Styles
	titleStyle    lipgloss.Style
	stepStyle     lipgloss.Style
	contentStyle  lipgloss.Style
	errorStyle    lipgloss.Style
	buttonStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	infoStyle     lipgloss.Style

	// Callback when complete
	onComplete func(session.InstanceOptions)
}

// NewSessionSetupOverlay creates a new session setup wizard overlay with required callbacks.
// The OnComplete callback is required and will cause a panic if not provided.
// This design prevents the "completion callback not set" runtime error by requiring
// callbacks at construction time instead of through setter methods.
func NewSessionSetupOverlay(callbacks SessionSetupCallbacks) *SessionSetupOverlay {
	// Validate that the required OnComplete callback is provided
	if callbacks.OnComplete == nil {
		panic("SessionSetupOverlay requires OnComplete callback - use SessionSetupCallbacks{OnComplete: func(opts session.InstanceOptions){...}}")
	}
	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		MarginBottom(1)

	stepStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA")).
		MarginBottom(1)

	contentStyle := lipgloss.NewStyle().
		MarginTop(1).
		MarginBottom(1)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		MarginTop(1)

	buttonStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 3).
		MarginRight(1)

	selectedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF")).
		Padding(0, 3).
		MarginRight(1)

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA")).
		Italic(true).
		MarginTop(1)

	// Load default program from config
	cfg := config.LoadConfig()
	defaultProgram := cfg.DefaultProgram

	// Create input components
	nameInput := NewTextInputOverlay("Session Name", "")
	programInput := NewTextInputOverlay("Program", defaultProgram)
	categoryInput := NewTextInputOverlay("Category", "")

	// Create navigation handlers for steps with multiple options
	basicsNav := NewNavigationHandler(2, true)   // 2 fields: name and program
	locationNav := NewNavigationHandler(3, true) // 3 choices: current, different, existing
	branchNav := NewNavigationHandler(3, true)   // 3 choices: new, current, existing

	// Create an empty session setup overlay
	overlay := &SessionSetupOverlay{
		step:  StepBasics,
		error: "",

		sessionName: "",
		repoPath:    ".", // Default to current directory
		workingDir:  "",  // Default to repository root
		program:     defaultProgram,
		branch:      "", // Will be generated based on session name
		category:    "", // Default to no category

		locationChoice:    "current",
		branchChoice:      BranchChoiceNew,
		showAdvanced:      false,
		basicsFocused:     "name",
		basicsNavigator:   basicsNav,
		locationNavigator: locationNav,
		branchNavigator:   branchNav,

		nameInput:     nameInput,
		programInput:  programInput,
		categoryInput: categoryInput,

		titleStyle:    titleStyle,
		stepStyle:     stepStyle,
		contentStyle:  contentStyle,
		errorStyle:    errorStyle,
		buttonStyle:   buttonStyle,
		selectedStyle: selectedStyle,
		infoStyle:     infoStyle,

		// Set the required callback from construction parameter
		onComplete: callbacks.OnComplete,
	}

	// Set optional cancel callback if provided
	if callbacks.OnCancel != nil {
		overlay.BaseOverlay.SetOnCancel(callbacks.OnCancel)
	}

	// Initialize BaseOverlay
	overlay.BaseOverlay.SetSize(60, 20)
	overlay.BaseOverlay.Focus()

	return overlay
}

// SetSize sets the size of the overlay
func (s *SessionSetupOverlay) SetSize(width, height int) {
	// Update BaseOverlay size
	s.BaseOverlay.SetSize(width, height)

	// Calculate responsive sizes for components
	responsiveWidth := s.GetResponsiveWidth()

	// Update component sizes
	if s.nameInput != nil {
		s.nameInput.SetSize(responsiveWidth-10, 5)
	}

	if s.programInput != nil {
		s.programInput.SetSize(responsiveWidth-10, 5)
	}

	if s.categoryInput != nil {
		s.categoryInput.SetSize(responsiveWidth-10, 5)
	}

	if s.categorySelector != nil {
		s.categorySelector.SetSize(width-10, height-10)
	}

	if s.repoSelector != nil {
		s.repoSelector.SetSize(width-10, height-10)
	}

	if s.fuzzyDirBrowser != nil {
		s.fuzzyDirBrowser.SetSize(width-10, height-10)
	}

	if s.worktreeSelector != nil {
		s.worktreeSelector.SetSize(width-10, height-10)
	}

	if s.branchSelector != nil {
		s.branchSelector.SetSize(width-10, height-10)
	}
}
// Focus gives focus to the overlay
func (s *SessionSetupOverlay) Focus() {
	s.BaseOverlay.Focus()

	// Focus the appropriate input for the current step
	switch s.step {
	case StepBasics:
		// Focus will be managed by basicsFocused state
		break
	case StepAdvanced:
		if s.categoryInput != nil {
			// TextInputOverlay will handle focus internally
		}
		break
	case StepRepository:
		if s.repoSelector != nil {
			s.repoSelector.Focus()
		}
	case StepDirectory:
		// FuzzyDirectoryBrowser manages its own focus state
		// No explicit focus call needed
	case StepWorktree:
		if s.worktreeSelector != nil {
			s.worktreeSelector.Focus()
		}
	case StepBranch:
		if s.branchSelector != nil {
			s.branchSelector.Focus()
		}
	}
}

// Blur removes focus from the overlay
func (s *SessionSetupOverlay) Blur() {
	s.BaseOverlay.Blur()

	// Blur all inputs
	// TextInputOverlay handles focus internally

	if s.categorySelector != nil {
		s.categorySelector.Blur()
	}

	if s.repoSelector != nil {
		s.repoSelector.Blur()
	}

	// FuzzyDirectoryBrowser manages its own focus state
	// No explicit blur call needed

	if s.worktreeSelector != nil {
		s.worktreeSelector.Blur()
	}

	if s.branchSelector != nil {
		s.branchSelector.Blur()
	}
}

// nextStep advances to the next step in the wizard
func (s *SessionSetupOverlay) nextStep() {
	// Clear any previous errors
	s.error = ""

	switch s.step {
	case StepBasics:
		// Validate both name and program
		s.sessionName = s.nameInput.GetValue()
		if s.sessionName == "" {
			s.error = "Session name cannot be empty"
			s.basicsFocused = "name"
			return
		}

		s.program = s.programInput.GetValue()
		if s.program == "" {
			s.program = config.LoadConfig().DefaultProgram
		}

		// Move to location step
		s.step = StepLocation

	case StepLocation:
		if s.locationChoice == "current" {
			// Using current directory, expand and set the path
			currentDir, err := os.Getwd()
			if err != nil {
				s.error = "Failed to get current directory: " + err.Error()
				return
			}
			s.repoPath = currentDir
			// Go to branch selection step
			s.step = StepBranch
		} else if s.locationChoice == "different" {
			// Need to select a different repository
			s.step = StepRepository
			s.initRepositorySelector()
		} else { // "existing"
			// Need to select an existing worktree
			s.step = StepWorktree
			s.initWorktreeSelector()
		}

	case StepAdvanced:
		s.category = s.categoryInput.GetValue()
		// Category is optional, can be empty
		s.step = StepConfirm

	case StepRepository:
		// After selecting a repository, determine next step
		if s.repoPath == "" {
			s.error = "Please select a repository"
			return
		}

		// Skip directory selection if we're going to create a new branch (worktree)
		// since worktrees create their own separate directory
		if s.branchChoice == BranchChoiceNew {
			// When creating a new branch, we use a worktree which creates its own directory
			// so we don't need to ask the user to select a directory within the repo
			s.workingDir = "" // Use repository root as working directory
			s.step = StepBranch
		} else {
			// For existing branches, we might want to work in a specific directory
			s.step = StepDirectory
			s.initDirectoryBrowser()
		}

	case StepDirectory:
		// After selecting a directory, select branch strategy
		s.step = StepBranch

	case StepWorktree:
		// After selecting a worktree, go to confirm
		if s.existingWorktree == "" {
			s.error = "Please select a worktree"
			return
		}
		s.step = StepConfirm

	case StepBranch:
		if s.branchChoice == BranchChoiceNew || s.branchChoice == BranchChoiceCurrent {
			// Using new branch or current branch, go to confirm
			s.step = StepConfirm
		} else {
			// Need to select an existing branch
			s.initBranchSelector()
		}

	case StepConfirm:
		// Complete the setup
		if s.onComplete == nil {
			s.error = "Internal error: completion callback not set. Please cancel and try again."
			return
		}

		// Ensure all paths are properly expanded
		repoPath := s.repoPath
		expandedRepoPath, err := ExpandPath(repoPath)
		if err == nil {
			repoPath = expandedRepoPath
		}

		existingWorktree := s.existingWorktree
		if existingWorktree != "" {
			expandedWorktree, err := ExpandPath(existingWorktree)
			if err == nil {
				existingWorktree = expandedWorktree
			}
		}

		// Determine session type based on location choice
		var sessionType session.SessionType
		switch s.locationChoice {
		case "current", "different":
			// For current directory or different directory, check if we're creating a new branch
			if s.branchChoice == BranchChoiceNew {
				sessionType = session.SessionTypeNewWorktree
			} else {
				sessionType = session.SessionTypeDirectory
			}
		case "existing":
			sessionType = session.SessionTypeExistingWorktree
		default:
			// Default to directory session for backward compatibility
			sessionType = session.SessionTypeDirectory
		}

		instance := session.InstanceOptions{
			Title:            s.sessionName,
			Path:             repoPath,
			WorkingDir:       s.workingDir,
			Program:          s.program,
			ExistingWorktree: existingWorktree,
			Category:         s.category,
			SessionType:      sessionType,
		}

		s.onComplete(instance)
	}

	// Clear any error when advancing steps
	s.error = ""
}

// prevStep goes back to the previous step in the wizard
func (s *SessionSetupOverlay) prevStep() {
	switch s.step {
	case StepBasics:
		// First step, cancel the setup
		if s.onCancel != nil {
			s.onCancel()
		}

	case StepLocation:
		s.step = StepBasics

	case StepAdvanced:
		s.step = StepLocation

	case StepRepository:
		s.step = StepLocation

	case StepDirectory:
		s.step = StepRepository
		if s.repoSelector != nil {
			s.repoSelector.Focus()
		}

	case StepWorktree:
		s.step = StepLocation

	case StepBranch:
		// Branch selection depends on the location choice and branch choice
		if s.locationChoice == "different" {
			// If we're creating a new branch, we skipped StepDirectory, so go back to StepRepository
			if s.branchChoice == BranchChoiceNew {
				s.step = StepRepository
				if s.repoSelector != nil {
					s.repoSelector.Focus()
				}
			} else {
				// For existing branches, we went through StepDirectory
				s.step = StepDirectory
				// FuzzyDirectoryBrowser manages its own focus state
				// No explicit focus call needed
			}
		} else {
			s.step = StepLocation
		}

	case StepConfirm:
		// Going back from confirm depends on the previous choices
		if s.locationChoice == "current" {
			s.step = StepLocation
		} else if s.locationChoice == "different" {
			if s.branchChoice == "new" {
				s.step = StepBranch
			} else {
				s.step = StepBranch
				if s.branchSelector != nil {
					s.branchSelector.Focus()
				}
			}
		} else { // "existing"
			s.step = StepWorktree
			if s.worktreeSelector != nil {
				s.worktreeSelector.Focus()
			}
		}
	}

	// Clear any error when going back
	s.error = ""
}

// initRepositorySelector initializes the repository selector component
func (s *SessionSetupOverlay) initRepositorySelector() {
	if s.repoSelector == nil {
		s.repoSelector = NewFuzzyInputOverlay("Select Repository", "Type path or search repositories (~, /path, relative)")

		// Set up async loader for contextual repository discovery
		s.repoSelector.SetAsyncLoader(func(query string) ([]fuzzy.SearchItem, error) {
			return s.discoverGitRepositoriesContextual(query), nil
		})

		// Discover Git repositories from common locations initially
		items := s.discoverGitRepositories()
		s.repoSelector.SetItems(items)

		// Set up selection callback
		s.repoSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			path := item.GetID()

			// Use enhanced path validation for better error messages
			validation := ValidatePathEnhanced(path)

			if !validation.Valid {
				s.error = validation.ErrorMessage
				return
			}

			// Set the validated expanded path
			s.repoPath = validation.ExpandedPath

			// Display any warnings to the user (non-blocking)
			if len(validation.Warnings) > 0 {
				// For now, we'll show the first warning as a non-error message
				// Could be enhanced with a warning display system later
				s.warning = validation.Warnings[0]
			} else {
				s.warning = ""
			}

			s.error = ""
			s.nextStep()
		})

		s.repoSelector.SetOnCancel(func() {
			s.prevStep()
		})

		s.repoSelector.SetSize(s.width-10, s.height-10)
		s.repoSelector.Focus()
	}
}

// initDirectoryBrowser initializes the FZF-style directory browser component
func (s *SessionSetupOverlay) initDirectoryBrowser() {
	if s.fuzzyDirBrowser == nil {
		// Expand the repository path first
		expandedRepoPath, err := ExpandPath(s.repoPath)
		if err != nil {
			s.error = "Error expanding path: " + err.Error()
			return
		}

		// Create new FZF-style directory browser
		s.fuzzyDirBrowser = NewFuzzyDirectoryBrowser("Select Directory", expandedRepoPath)

		// Set up selection and cancellation callbacks
		s.fuzzyDirBrowser.SetCallbacks(
			func(directoryPath string) {
				// Convert absolute path to relative path for working directory
				relPath, err := filepath.Rel(expandedRepoPath, directoryPath)
				if err != nil {
					// If we can't get relative path, use the directory path as-is
					relPath = directoryPath
				}

				// Handle special case for repository root
				if directoryPath == expandedRepoPath || relPath == "." {
					s.workingDir = ""
				} else {
					s.workingDir = relPath
				}

				// Validate the working directory exists within the repository
				if s.workingDir != "" {
					fullPath := filepath.Join(expandedRepoPath, s.workingDir)
					if !PathExists(fullPath) || !IsDirectory(fullPath) {
						s.error = "Invalid directory: " + fullPath
						return
					}
				}

				s.error = ""
				s.nextStep()
			},
			func() {
				s.prevStep()
			},
		)

		s.fuzzyDirBrowser.SetSize(s.width-10, s.height-10)
	}
}

// getSubdirectories function removed - FuzzyDirectoryBrowser handles directory traversal internally

// initWorktreeSelector initializes the worktree selector component
func (s *SessionSetupOverlay) initWorktreeSelector() {
	if s.worktreeSelector == nil {
		s.worktreeSelector = NewFuzzyInputOverlay("Select Worktree", "Search worktrees")

		// Load actual Git worktrees from the system
		items := s.loadGitWorktrees()
		s.worktreeSelector.SetItems(items)

		// Set up selection callback
		s.worktreeSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			path := item.GetID()

			// Expand the path
			expandedPath, err := ExpandPath(path)
			if err == nil {
				path = expandedPath
			}

			// We don't strictly check existence here as worktrees might be paths
			// that don't exist yet but will be created by git
			// But we do inform the user if the path doesn't exist
			if !PathExists(path) {
				// Just a warning, not an error that prevents proceeding
				s.error = "Warning: Path does not exist (will be created): " + path
			} else {
				s.error = ""
			}

			s.existingWorktree = path
			s.nextStep()
		})

		s.worktreeSelector.SetOnCancel(func() {
			s.prevStep()
		})

		s.worktreeSelector.SetSize(s.width-10, s.height-10)
		s.worktreeSelector.Focus()
	}
}

// initBranchSelector initializes the branch selector component
func (s *SessionSetupOverlay) initBranchSelector() {
	if s.branchSelector == nil {
		s.branchSelector = NewFuzzyInputOverlay("Select Branch", "Search branches")

		// Ensure we have an expanded repo path for branch listing
		_, err := ExpandPath(s.repoPath)
		if err != nil {
			s.error = "Error expanding repository path: " + err.Error()
			// Continue with unexpanded path
		}

		// Load actual branches from the selected repository
		items := s.loadGitBranches(s.repoPath)
		s.branchSelector.SetItems(items)

		// Set up selection callback
		s.branchSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			s.branch = item.GetID()

			// In a real implementation, we would validate that the branch exists in the repository
			// For now, just clear any error and proceed
			s.error = ""
			s.nextStep()
		})

		s.branchSelector.SetOnCancel(func() {
			s.prevStep()
		})

		s.branchSelector.SetSize(s.width-10, s.height-10)
		s.branchSelector.Focus()
	}
}

// Update handles messages for the overlay and its sub-components
func (s *SessionSetupOverlay) Update(msg tea.Msg) tea.Cmd {
	if !s.focused {
		return nil
	}

	// Handle key presses for navigation
	if msg, ok := msg.(tea.KeyMsg); ok {
		// Use BaseOverlay for common keys (Esc)
		if handled, shouldClose := s.HandleCommonKeys(msg); handled {
			if shouldClose {
				return nil
			}
		}

		switch msg.Type {

		case tea.KeyEnter:
			// Handle Enter for different steps
			switch s.step {
			case StepBasics, StepAdvanced, StepLocation, StepConfirm:
				s.nextStep()
				return nil
			case StepBranch:
				if s.branchChoice == BranchChoiceNew || s.branchChoice == BranchChoiceCurrent {
					s.nextStep()
					return nil
				}
			}

		case tea.KeyTab:
			// Tab has special meaning in some steps - use navigation handlers
			if s.step == StepBasics {
				if s.basicsNavigator.HandleTabNavigation(msg) {
					s.basicsFocused = []string{"name", "program"}[s.basicsNavigator.GetCurrentIndex()]
				}
				return nil
			} else if s.step == StepLocation {
				if s.locationNavigator.HandleTabNavigation(msg) {
					s.locationChoice = []string{"current", "different", "existing"}[s.locationNavigator.GetCurrentIndex()]
				}
				return nil
			} else if s.step == StepBranch {
				if s.branchNavigator.HandleTabNavigation(msg) {
					s.branchChoice = []BranchChoice{BranchChoiceNew, BranchChoiceCurrent, BranchChoiceExisting}[s.branchNavigator.GetCurrentIndex()]
				}
				return nil
			}

		case tea.KeyUp, tea.KeyDown:
			// Arrow key navigation - use navigation handlers (includes vim j/k via HandleNavigation)
			if s.step == StepBasics {
				if s.basicsNavigator.HandleNavigation(msg) {
					s.basicsFocused = []string{"name", "program"}[s.basicsNavigator.GetCurrentIndex()]
				}
				return nil
			} else if s.step == StepLocation {
				if s.locationNavigator.HandleNavigation(msg) {
					s.locationChoice = []string{"current", "different", "existing"}[s.locationNavigator.GetCurrentIndex()]
				}
				return nil
			} else if s.step == StepBranch {
				if s.branchNavigator.HandleNavigation(msg) {
					s.branchChoice = []BranchChoice{BranchChoiceNew, BranchChoiceCurrent, BranchChoiceExisting}[s.branchNavigator.GetCurrentIndex()]
				}
				return nil
			}

		case tea.KeyRunes:
			// Vim-style navigation with j/k - ONLY handle these specific keys
			// Don't interfere with text input in text fields
			if len(msg.Runes) == 1 {
				char := string(msg.Runes)
				if char == "j" || char == "k" {
					// Only handle j/k on steps that don't have text input
					if s.step == StepLocation {
						if s.locationNavigator.HandleNavigation(msg) {
							s.locationChoice = []string{"current", "different", "existing"}[s.locationNavigator.GetCurrentIndex()]
						}
						return nil
					} else if s.step == StepBranch {
						if s.branchNavigator.HandleNavigation(msg) {
							s.branchChoice = []BranchChoice{BranchChoiceNew, BranchChoiceCurrent, BranchChoiceExisting}[s.branchNavigator.GetCurrentIndex()]
						}
						return nil
					}
				}
			}
		}
	}

	// Forward messages to the appropriate component for the current step
	var cmd tea.Cmd
	switch s.step {
	case StepBasics:
		if msg, ok := msg.(tea.KeyMsg); ok {
			// Forward to the focused input
			if s.basicsFocused == "name" && s.nameInput != nil {
				s.nameInput.HandleKeyPress(msg)
			} else if s.basicsFocused == "program" && s.programInput != nil {
				s.programInput.HandleKeyPress(msg)
			}
		}

	case StepAdvanced:
		if msg, ok := msg.(tea.KeyMsg); ok && s.categoryInput != nil {
			s.categoryInput.HandleKeyPress(msg)
		}

	case StepRepository:
		if s.repoSelector != nil {
			_ = s.repoSelector.Update(msg)
		}

	case StepDirectory:
		if s.fuzzyDirBrowser != nil {
			_, _ = s.fuzzyDirBrowser.Update(msg)
		}

	case StepWorktree:
		if s.worktreeSelector != nil {
			_ = s.worktreeSelector.Update(msg)
		}

	case StepBranch:
		if s.branchSelector != nil && s.branchChoice == BranchChoiceExisting {
			_ = s.branchSelector.Update(msg)
		}
	}

	return cmd
}

// View renders the overlay based on the current step
func (s *SessionSetupOverlay) View() string {
	// Debug: Log focus state
	log.DebugLog.Printf("SessionSetupOverlay.View(): focused=%v, width=%d, height=%d", s.focused, s.width, s.height)

	if !s.focused {
		log.DebugLog.Printf("SessionSetupOverlay.View(): returning empty (not focused)")
		return ""
	}

	var sb strings.Builder

	// Title with better styling
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginBottom(1).
		Render("🚀 Create New Session")
	sb.WriteString(title)
	sb.WriteString("\n")

	// Progress indicator (simplified)
	progress := s.renderProgress()
	sb.WriteString(progress)
	sb.WriteString("\n\n")

	// Content based on current step
	switch s.step {
	case StepBasics:
		sb.WriteString(s.renderBasicsStep())

	case StepLocation:
		sb.WriteString(s.renderLocationStep())

	case StepAdvanced:
		sb.WriteString(s.categoryInput.View())
		sb.WriteString("\n")
		sb.WriteString(s.infoStyle.Render("Enter a category name to help organize sessions (optional)"))

	case StepRepository:
		if s.repoSelector != nil {
			sb.WriteString(s.repoSelector.View())
		} else {
			sb.WriteString("Loading repositories...")
		}

	case StepDirectory:
		if s.fuzzyDirBrowser != nil {
			sb.WriteString(s.fuzzyDirBrowser.View())
		} else {
			sb.WriteString("Loading directories...")
		}

	case StepWorktree:
		if s.worktreeSelector != nil {
			sb.WriteString(s.worktreeSelector.View())
		} else {
			sb.WriteString("Loading worktrees...")
		}

	case StepBranch:
		// Section title
		sectionStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFCC00")).
			MarginBottom(1)
		sb.WriteString(sectionStyle.Render("🌿 Branch Strategy"))
		sb.WriteString("\n")

		// Branch options with descriptions
		options := []struct {
			choice BranchChoice
			icon   string
			title  string
			desc   string
		}{
			{BranchChoiceNew, "🌱", "Create New Branch", "A new branch will be created based on your session name"},
			{BranchChoiceCurrent, "🔀", "Use Current Branch", "Work on the repository's current branch"},
			{BranchChoiceExisting, "🌿", "Choose Another Branch", "Select from existing branches"},
		}

		// Calculate responsive width for options
		contentWidth := s.GetResponsiveWidth()

		for _, opt := range options {
			isSelected := s.branchChoice == opt.choice

			var style lipgloss.Style
			if isSelected {
				style = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(lipgloss.Color("#00FFFF")).
					Padding(0, 1).
					MarginBottom(1).
					Background(lipgloss.Color("#1a1a2e")).
					MaxWidth(contentWidth)
			} else {
				style = lipgloss.NewStyle().
					Border(lipgloss.NormalBorder()).
					BorderForeground(lipgloss.Color("#555555")).
					Padding(0, 1).
					MarginBottom(1).
					MaxWidth(contentWidth)
			}

			// Shorten description for narrow terminals
			desc := ShortenDescriptionForWidth(opt.desc, s.GetWidth())
			content := fmt.Sprintf("%s %s\n%s", opt.icon, opt.title, desc)
			sb.WriteString(style.Render(content))
			sb.WriteString("\n")
		}

		// Show branch selector if existing branch is selected
		if s.branchChoice == BranchChoiceExisting {
			if s.branchSelector != nil {
				sb.WriteString(s.branchSelector.View())
			} else {
				sb.WriteString("Loading branches...")
			}
		}

		// Help text
		helpStyle := lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#AAAAAA"))
		sb.WriteString(helpStyle.Render("💡 ↑/↓ or j/k to navigate • Tab to cycle • Enter to select"))

	case StepConfirm:
		sb.WriteString(s.renderConfirmStep())
	}

	// Error message if any
	if s.error != "" {
		sb.WriteString("\n\n")
		sb.WriteString(s.errorStyle.Render(s.error))
	}

	// Navigation help at the bottom
	sb.WriteString("\n\n")
	sb.WriteString(s.infoStyle.Render("Esc: Cancel, ↑/↓: Navigate"))

	return sb.String()
}

// renderProgress creates a visual progress indicator
func (s *SessionSetupOverlay) renderProgress() string {
	totalSteps := 4 // StepBasics, StepLocation, StepAdvanced, StepConfirm
	currentStep := int(s.step) + 1

	dots := make([]string, totalSteps)
	for i := 0; i < totalSteps; i++ {
		if i < currentStep-1 {
			dots[i] = "●" // Completed
		} else if i == currentStep-1 {
			dots[i] = "◐" // Current
		} else {
			dots[i] = "○" // Pending
		}
	}

	progressStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#36CFC9"))

	return progressStyle.Render(strings.Join(dots, " "))
}

// renderBasicsStep renders the combined name+program step
func (s *SessionSetupOverlay) renderBasicsStep() string {
	var sb strings.Builder

	// Section title
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFCC00")).
		MarginBottom(1)
	sb.WriteString(sectionStyle.Render("📝 Session Details"))
	sb.WriteString("\n")

	// Name input with focus indication
	nameLabel := "Session Name:"
	if s.basicsFocused == "name" {
		nameLabel = "► " + nameLabel + " (active)"
	}
	sb.WriteString(s.infoStyle.Render(nameLabel))
	sb.WriteString("\n")
	sb.WriteString(s.nameInput.View())
	sb.WriteString("\n\n")

	// Program input with focus indication
	programLabel := "Program:"
	if s.basicsFocused == "program" {
		programLabel = "► " + programLabel + " (active)"
	}
	sb.WriteString(s.infoStyle.Render(programLabel))
	sb.WriteString("\n")
	sb.WriteString(s.programInput.View())
	sb.WriteString("\n\n")

	// Help text
	helpStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#AAAAAA"))
	sb.WriteString(helpStyle.Render("💡 Tab to switch fields • Enter to continue"))

	return sb.String()
}

// renderLocationStep renders the improved location selection
func (s *SessionSetupOverlay) renderLocationStep() string {
	var sb strings.Builder

	// Section title
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFCC00")).
		MarginBottom(1)
	sb.WriteString(sectionStyle.Render("📂 Choose Location"))
	sb.WriteString("\n")

	// Better styled options
	options := []struct {
		key   string
		icon  string
		title string
		desc  string
	}{
		{"current", "🏠", "Current Directory", "Use this repository"},
		{"different", "📁", "Different Repository", "Browse to another location"},
		{"existing", "🌿", "Existing Worktree", "Use git worktree"},
	}

	// Calculate responsive width for options
	contentWidth := s.GetResponsiveWidth()

	for _, opt := range options {
		isSelected := s.locationChoice == opt.key

		var style lipgloss.Style
		if isSelected {
			style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#00FFFF")).
				Padding(0, 1).
				MarginBottom(1).
				Background(lipgloss.Color("#1a1a2e")).
				MaxWidth(contentWidth)
		} else {
			style = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#555555")).
				Padding(0, 1).
				MarginBottom(1).
				MaxWidth(contentWidth)
		}

		// Shorten description for narrow terminals
		desc := ShortenDescriptionForWidth(opt.desc, s.GetWidth())
		content := fmt.Sprintf("%s %s\n%s", opt.icon, opt.title, desc)
		sb.WriteString(style.Render(content))
		sb.WriteString("\n")
	}

	// Help text
	helpStyle := lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color("#AAAAAA"))
	sb.WriteString(helpStyle.Render("💡 Tab to cycle options • Enter to select"))

	return sb.String()
}

// renderConfirmStep renders the improved confirmation step
func (s *SessionSetupOverlay) renderConfirmStep() string {
	var sb strings.Builder

	// Section title
	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FF00")).
		MarginBottom(1)
	sb.WriteString(sectionStyle.Render("✨ Ready to Create"))
	sb.WriteString("\n")

	// Summary box with better styling
	summaryStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#36CFC9")).
		Padding(1).
		MarginBottom(1).
		Width(50)

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("📛 Name: %s\n", s.sessionName))

	// Show friendly program name instead of full path
	programName := filepath.Base(s.program)
	if strings.Contains(programName, "claude") {
		programName = "claude"
	}
	summary.WriteString(fmt.Sprintf("🤖 Program: %s\n", programName))

	if s.category != "" {
		summary.WriteString(fmt.Sprintf("🏷️  Category: %s\n", s.category))
	}

	// Friendly location display
	if s.locationChoice == "current" {
		currentDir, _ := os.Getwd()
		homeDir, _ := os.UserHomeDir()
		displayPath := currentDir
		if strings.HasPrefix(currentDir, homeDir) {
			displayPath = "~" + currentDir[len(homeDir):]
		}
		summary.WriteString(fmt.Sprintf("📍 Location: Current (%s)", filepath.Base(displayPath)))
	} else {
		summary.WriteString(fmt.Sprintf("📍 Location: %s", s.locationChoice))
	}

	sb.WriteString(summaryStyle.Render(summary.String()))
	sb.WriteString("\n")

	// Action buttons
	helpStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#00FF00"))
	sb.WriteString(helpStyle.Render("🚀 Press Enter to create • Esc to cancel"))

	return sb.String()
}

// Git Integration Functions

// discoverGitRepositories finds Git repositories in common locations
func (s *SessionSetupOverlay) discoverGitRepositories() []fuzzy.SearchItem {
	items := []fuzzy.SearchItem{
		fuzzy.BasicStringItem{ID: ".", Text: "Current Directory"},
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return items
	}

	// Add home directory if it's a Git repository
	if s.isGitRepository(home) {
		items = append(items, fuzzy.BasicStringItem{
			ID:   home,
			Text: "Home Directory (~)",
		})
	}

	// Common development directories to scan
	commonDirs := []string{
		filepath.Join(home, "projects"),
		filepath.Join(home, "dev"),
		filepath.Join(home, "code"),
		filepath.Join(home, "workspace"),
		filepath.Join(home, "git"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "Documents"),
		filepath.Join(home, "Desktop"),
	}

	for _, dir := range commonDirs {
		repos := s.findGitRepositoriesInDirectory(dir, 2) // Max depth 2
		items = append(items, repos...)
	}

	return items
}

// discoverGitRepositoriesContextual discovers repositories based on user input
func (s *SessionSetupOverlay) discoverGitRepositoriesContextual(query string) []fuzzy.SearchItem {
	query = strings.TrimSpace(query)

	// Enhanced empty query handling - provide contextual defaults
	if query == "" {
		// Start with current directory context
		items := []fuzzy.SearchItem{}
		if cwd, err := os.Getwd(); err == nil {
			validation := ValidatePathEnhanced(cwd)
			icon := "📍"
			description := "current directory"
			if validation.IsGitRepo {
				icon = "✅"
				description = "current Git repository"
			}
			items = append(items, fuzzy.BasicStringItem{
				ID:   cwd,
				Text: icon + " " + s.getDisplayPath(cwd) + " (" + description + ")",
			})
		}

		// Add common suggestions
		home, err := os.UserHomeDir()
		if err == nil {
			items = append(items, fuzzy.BasicStringItem{
				ID:   home,
				Text: "🏠 " + s.getDisplayPath(home) + " (home directory)",
			})
		}

		// Merge with default discovered repositories but prioritize contextual ones
		defaultRepos := s.discoverGitRepositories()
		items = append(items, defaultRepos...)
		return items
	}

	items := []fuzzy.SearchItem{}

	// Add literal path with enhanced validation preview
	literalValidation := ValidatePathEnhanced(query)
	literalIcon := "📁"
	literalDesc := "use as typed"

	if literalValidation.Valid {
		if literalValidation.IsGitRepo {
			literalIcon = "✅"
			literalDesc = "Git repository"
		} else {
			literalIcon = "📂"
			literalDesc = "directory"
		}
		if len(literalValidation.Warnings) > 0 {
			literalDesc += " ⚠️"
		}
	} else if literalValidation.Error != nil {
		literalIcon = "❌"
		literalDesc = "invalid path"
	}

	items = append(items, fuzzy.SearchItem(fuzzy.BasicStringItem{
		ID:   query,
		Text: literalIcon + " " + query + " (" + literalDesc + ")",
	}))

	// Try to expand and resolve the path for contextual discovery
	expandedQuery, err := ExpandPath(query)
	if err != nil {
		// If expansion fails, still allow the original query
		expandedQuery = query
	}

	// Use quick validation for performance in contextual discovery
	if err := ValidatePathQuick(expandedQuery); err == nil {
		// Path is valid, determine how to display it
		validation := ValidatePathEnhanced(expandedQuery)

		var icon, description string
		if validation.IsGitRepo {
			icon = "✅"
			description = "Git repository"
		} else {
			icon = "📂"
			description = "directory"
		}

		// Add warnings to description if present
		if len(validation.Warnings) > 0 {
			description += " ⚠️"
		}

		items = append(items, fuzzy.BasicStringItem{
			ID:   expandedQuery,
			Text: icon + " " + s.getDisplayPath(expandedQuery) + " (" + description + ")",
		})

		// Scan for Git repositories within this directory
		repos := s.findGitRepositoriesInDirectory(expandedQuery, 2)
		items = append(items, repos...)

		// If it looks like a path prefix, scan parent directories
		parentDir := filepath.Dir(expandedQuery)
		if parentDir != expandedQuery && parentDir != "." && PathExists(parentDir) {
			parentRepos := s.findGitRepositoriesInDirectory(parentDir, 1)
			items = append(items, parentRepos...)
		}
	} else {
		// Path doesn't exist - try to find similar paths or parent directories

		// Try parent directory if the full path doesn't exist
		parentDir := filepath.Dir(expandedQuery)
		if parentDir != expandedQuery && parentDir != "." && PathExists(parentDir) && IsDirectory(parentDir) {
			items = append(items, fuzzy.BasicStringItem{
				ID:   parentDir,
				Text: "📁 " + s.getDisplayPath(parentDir) + " (parent directory)",
			})

			// Scan parent for repositories
			repos := s.findGitRepositoriesInDirectory(parentDir, 2)
			items = append(items, repos...)
		}
	}

	// Limit results to prevent overwhelming the UI
	if len(items) > 20 {
		items = items[:20]
	}

	return items
}

// findGitRepositoriesInDirectory recursively finds Git repositories up to maxDepth with graceful error handling
func (s *SessionSetupOverlay) findGitRepositoriesInDirectory(dir string, maxDepth int) []fuzzy.SearchItem {
	var items []fuzzy.SearchItem

	if maxDepth <= 0 {
		return items
	}

	// Check if directory exists and is accessible
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return items // Directory doesn't exist
		} else if os.IsPermission(err) {
			// Permission denied - add a notice but continue
			items = append(items, fuzzy.BasicStringItem{
				ID:   dir,
				Text: fmt.Sprintf("🔒 %s (permission denied)", s.getDisplayPath(dir)),
			})
			return items
		}
		// Other errors (network timeout, etc.) - skip silently
		return items
	}

	// Check if this directory is a Git repository with enhanced error handling
	if s.isGitRepositoryEnhanced(dir) {
		displayPath := s.getDisplayPath(dir)
		icon := "✅"

		// Add network warning if detected
		if isNetworkPath(dir) {
			icon = "🌐"
		}

		items = append(items, fuzzy.BasicStringItem{
			ID:   dir,
			Text: fmt.Sprintf("%s %s (%s)", icon, filepath.Base(dir), displayPath),
		})
	}

	// Scan subdirectories with graceful error handling
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsPermission(err) {
			// Add a notice about inaccessible directory
			items = append(items, fuzzy.BasicStringItem{
				ID:   dir,
				Text: fmt.Sprintf("🔒 %s (cannot list contents)", s.getDisplayPath(dir)),
			})
		}
		// For other errors (network timeout, temporary issues), fail silently
		return items
	}

	for _, entry := range entries {
		// Skip non-directories and hidden directories
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		subDir := filepath.Join(dir, entry.Name())

		// Skip known problematic directories to avoid hanging
		if s.shouldSkipDirectory(entry.Name()) {
			continue
		}

		subItems := s.findGitRepositoriesInDirectory(subDir, maxDepth-1)
		items = append(items, subItems...)

		// Limit total results to avoid UI overwhelm and improve performance
		if len(items) > 20 {
			break
		}
	}

	return items
}

// isGitRepositoryEnhanced checks if a directory is a Git repository with better error handling
func (s *SessionSetupOverlay) isGitRepositoryEnhanced(path string) bool {
	gitDir := filepath.Join(path, ".git")

	// Use Lstat instead of Stat to avoid following symlinks that might cause issues
	if stat, err := os.Lstat(gitDir); err == nil {
		// .git can be a directory or file (for worktrees)
		return stat.IsDir() || stat.Mode().IsRegular()
	}

	// If we can't access .git, it's not a Git repository we can work with
	return false
}

// shouldSkipDirectory determines if a directory should be skipped during scanning
func (s *SessionSetupOverlay) shouldSkipDirectory(dirName string) bool {
	// Skip directories that are likely to cause performance issues or hangs
	problematicDirs := []string{
		// System directories that might be slow or restricted
		"proc", "sys", "dev", "run", "tmp",
		// macOS-specific directories
		"System", "Library", "Applications", "Network",
		// Virtual filesystems and mount points
		"snap", "flatpak", "AppImage",
		// Package managers and caches
		"node_modules", ".npm", ".yarn", ".cargo", ".go",
		// IDE and editor directories
		".vscode", ".idea", ".eclipse",
		// Version control directories (other than .git)
		".svn", ".hg", ".bzr",
		// Build and cache directories
		"build", "dist", "target", "out", "__pycache__",
	}

	dirLower := strings.ToLower(dirName)
	for _, problematic := range problematicDirs {
		if dirLower == strings.ToLower(problematic) {
			return true
		}
	}

	// Skip directories with certain patterns that indicate they're problematic
	if strings.HasPrefix(dirName, "Backup of ") ||
		strings.HasPrefix(dirName, "RECYCLER") ||
		strings.HasPrefix(dirName, "$RECYCLE.BIN") {
		return true
	}

	return false
}

// isGitRepository checks if a directory contains a .git folder
func (s *SessionSetupOverlay) isGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	if stat, err := os.Stat(gitDir); err == nil {
		return stat.IsDir() || stat.Mode().IsRegular() // .git can be a directory or file (for worktrees)
	}
	return false
}

// getDisplayPath converts absolute path to a display-friendly format
func (s *SessionSetupOverlay) getDisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}

	return path
}

// loadGitWorktrees discovers existing Git worktrees on the system
func (s *SessionSetupOverlay) loadGitWorktrees() []fuzzy.SearchItem {
	var items []fuzzy.SearchItem

	// First, try to find worktrees from the current working directory
	if cwd, err := os.Getwd(); err == nil {
		if s.isGitRepository(cwd) {
			worktrees := s.getWorktreesForRepository(cwd)
			items = append(items, worktrees...)
		}
	}

	// Try to find worktrees from common Git repositories
	home, err := os.UserHomeDir()
	if err != nil {
		return items
	}

	// Scan common directories for repositories with worktrees
	commonDirs := []string{
		filepath.Join(home, "projects"),
		filepath.Join(home, "dev"),
		filepath.Join(home, "code"),
	}

	for _, dir := range commonDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			repoPath := filepath.Join(dir, entry.Name())
			if s.isGitRepository(repoPath) {
				worktrees := s.getWorktreesForRepository(repoPath)
				items = append(items, worktrees...)
			}

			// Limit results
			if len(items) > 15 {
				break
			}
		}

		if len(items) > 15 {
			break
		}
	}

	// Add option to specify custom worktree path
	items = append(items, fuzzy.BasicStringItem{
		ID:   "",
		Text: "📝 Enter custom worktree path...",
	})

	return items
}

// getWorktreesForRepository lists worktrees for a specific Git repository
func (s *SessionSetupOverlay) getWorktreesForRepository(repoPath string) []fuzzy.SearchItem {
	var items []fuzzy.SearchItem

	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return items
	}

	lines := strings.Split(string(output), "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "worktree ") {
			worktreePath := strings.TrimPrefix(line, "worktree ")

			// Skip if it's the main repository (not a separate worktree)
			if worktreePath == repoPath {
				continue
			}

			// Look for the branch name in the next lines
			branchName := "unknown"
			for j := i + 1; j < len(lines) && j < i+5; j++ {
				nextLine := strings.TrimSpace(lines[j])
				if strings.HasPrefix(nextLine, "branch ") {
					branchName = strings.TrimPrefix(nextLine, "branch refs/heads/")
					break
				}
			}

			displayPath := s.getDisplayPath(worktreePath)
			items = append(items, fuzzy.SearchItem(fuzzy.BasicStringItem{
				ID:   worktreePath,
				Text: fmt.Sprintf("%s (%s)", branchName, displayPath),
			}))
		}
	}

	return items
}

// loadGitBranches loads branches from the specified Git repository
func (s *SessionSetupOverlay) loadGitBranches(repoPath string) []fuzzy.SearchItem {
	var items []fuzzy.SearchItem

	// Add debugging
	if repoPath == "" {
		return []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "main", Text: "main (no repository path provided)"},
		}
	}

	// Expand the repository path
	expandedPath, err := ExpandPath(repoPath)
	if err != nil {
		expandedPath = repoPath
	}

	// Check if it's a Git repository
	if !s.isGitRepository(expandedPath) {
		return []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "main", Text: fmt.Sprintf("main (not a Git repository: %s)", expandedPath)},
		}
	}

	// Get local branches
	cmd := exec.Command("git", "-C", expandedPath, "branch", "--format=%(refname:short)")
	output, err := cmd.Output()
	if err == nil {
		branches := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, branch := range branches {
			branch = strings.TrimSpace(branch)
			if branch != "" {
				items = append(items, fuzzy.BasicStringItem{
					ID:   branch,
					Text: fmt.Sprintf("%s (local)", branch),
				})
			}
		}
	}

	// Get remote branches
	cmd = exec.Command("git", "-C", expandedPath, "branch", "-r", "--format=%(refname:short)")
	output, err = cmd.Output()
	if err == nil {
		branches := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, branch := range branches {
			branch = strings.TrimSpace(branch)
			if branch != "" && !strings.Contains(branch, "HEAD") {
				// Remove origin/ prefix for display
				displayBranch := branch
				if strings.HasPrefix(branch, "origin/") {
					displayBranch = strings.TrimPrefix(branch, "origin/")
				}

				items = append(items, fuzzy.BasicStringItem{
					ID:   displayBranch,
					Text: fmt.Sprintf("%s (remote)", displayBranch),
				})
			}
		}
	}

	// If no branches found, provide a default
	if len(items) == 0 {
		items = append(items, fuzzy.BasicStringItem{
			ID:   "main",
			Text: "main (default branch)",
		})
	}

	return items
}
