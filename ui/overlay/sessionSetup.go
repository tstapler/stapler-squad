package overlay

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui/fuzzy"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionSetupStep represents the current step in the session setup wizard
type SessionSetupStep int

const (
	StepBasics SessionSetupStep = iota // Name + Program in one step
	StepLocation                      // Location choice (streamlined)
	StepAdvanced                      // Category + Tags (optional, combined)
	StepConfirm                       // Final confirmation
	
	// Legacy steps still referenced in complex flows
	StepRepository
	StepDirectory
	StepWorktree
	StepBranch
)

// SessionSetupOverlay is a multi-step modal for configuring a new session
type SessionSetupOverlay struct {
	// Core state
	step              SessionSetupStep
	width             int
	height            int
	focused           bool
	error             string
	
	// Session configuration being built
	sessionName       string
	repoPath          string
	workingDir        string
	program           string
	branch            string
	existingWorktree  string
	category          string
	tags              []string
	
	// Step-specific states
	locationChoice    string // "current", "different", "existing"
	branchChoice      string // "new", "existing"
	showAdvanced      bool   // Whether to show advanced options
	
	// Combined input state for basics step
	basicsFocused     string // "name" or "program"
	
	// UI components for different steps
	nameInput         *TextInputOverlay
	programInput      *TextInputOverlay
	categoryInput     *TextInputOverlay
	categorySelector  *FuzzyInputOverlay
	repoSelector      *FuzzyInputOverlay
	dirBrowser        *FuzzyInputOverlay
	worktreeSelector  *FuzzyInputOverlay
	branchSelector    *FuzzyInputOverlay
	
	// UI Styles
	titleStyle        lipgloss.Style
	stepStyle         lipgloss.Style
	contentStyle      lipgloss.Style
	errorStyle        lipgloss.Style
	buttonStyle       lipgloss.Style
	selectedStyle     lipgloss.Style
	infoStyle         lipgloss.Style
	
	// Callback when complete
	onComplete        func(session.InstanceOptions)
	onCancel          func()
}

// NewSessionSetupOverlay creates a new session setup wizard overlay
func NewSessionSetupOverlay() *SessionSetupOverlay {
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
	
	// Create an empty session setup overlay
	return &SessionSetupOverlay{
		step:          StepBasics,
		width:         60,
		height:        20,
		focused:       true,
		error:         "",
		
		sessionName:   "",
		repoPath:      ".",  // Default to current directory
		workingDir:    "",   // Default to repository root
		program:       defaultProgram,
		branch:        "",   // Will be generated based on session name
		category:      "",   // Default to no category
		tags:          []string{},
		
		locationChoice: "current",
		branchChoice:   "new",
		showAdvanced:   false,
		basicsFocused:  "name",
		
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
	}
}

// SetSize sets the size of the overlay
func (s *SessionSetupOverlay) SetSize(width, height int) {
	s.width = width
	s.height = height
	
	// Update component sizes
	if s.nameInput != nil {
		s.nameInput.SetSize(width-20, 5)
	}
	
	if s.programInput != nil {
		s.programInput.SetSize(width-20, 5)
	}
	
	if s.categoryInput != nil {
		s.categoryInput.SetSize(width-20, 5)
	}
	
	if s.categorySelector != nil {
		s.categorySelector.SetSize(width-10, height-10)
	}
	
	if s.repoSelector != nil {
		s.repoSelector.SetSize(width-10, height-10)
	}
	
	if s.dirBrowser != nil {
		s.dirBrowser.SetSize(width-10, height-10)
	}
	
	if s.worktreeSelector != nil {
		s.worktreeSelector.SetSize(width-10, height-10)
	}
	
	if s.branchSelector != nil {
		s.branchSelector.SetSize(width-10, height-10)
	}
}

// SetOnComplete sets the callback function when the session setup is complete
func (s *SessionSetupOverlay) SetOnComplete(callback func(session.InstanceOptions)) {
	s.onComplete = callback
}

// SetOnCancel sets the callback function when setup is cancelled
func (s *SessionSetupOverlay) SetOnCancel(callback func()) {
	s.onCancel = callback
}

// Focus gives focus to the overlay
func (s *SessionSetupOverlay) Focus() {
	s.focused = true
	
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
		if s.dirBrowser != nil {
			s.dirBrowser.Focus()
		}
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
	s.focused = false
	
	// Blur all inputs
	// TextInputOverlay handles focus internally
	
	if s.categorySelector != nil {
		s.categorySelector.Blur()
	}
	
	if s.repoSelector != nil {
		s.repoSelector.Blur()
	}
	
	if s.dirBrowser != nil {
		s.dirBrowser.Blur()
	}
	
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
			// Skip advanced step if not needed, go directly to confirm
			s.step = StepConfirm
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
		// After selecting a repository, select directory within it
		if s.repoPath == "" {
			s.error = "Please select a repository"
			return
		}
		s.step = StepDirectory
		s.initDirectoryBrowser()
	
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
		if s.branchChoice == "new" {
			// Using new branch, go to confirm
			s.step = StepConfirm
		} else {
			// Need to select an existing branch
			s.initBranchSelector()
		}
	
	case StepConfirm:
		// Complete the setup
		if s.onComplete != nil {
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
			
			instance := session.InstanceOptions{
				Title:            s.sessionName,
				Path:             repoPath,
				WorkingDir:       s.workingDir,
				Program:          s.program,
				ExistingWorktree: existingWorktree,
				Category:         s.category,
				Tags:             s.tags,
			}
			
			s.onComplete(instance)
		}
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
		// Branch selection depends on the location choice
		if s.locationChoice == "different" {
			s.step = StepDirectory
			if s.dirBrowser != nil {
				s.dirBrowser.Focus()
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
		s.repoSelector = NewFuzzyInputOverlay("Select Repository", "Search repositories")
		
		// TODO: Load recent repositories from config
		// For now, use a placeholder example
		home, _ := os.UserHomeDir()
		homeDisplayText := "Home Directory (~)"
		
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: ".", Text: "Current Repository"},
			fuzzy.BasicStringItem{ID: home, Text: homeDisplayText},
			fuzzy.BasicStringItem{ID: filepath.Join(home, "projects"), Text: "~/projects"},
			fuzzy.BasicStringItem{ID: filepath.Join(home, "Documents"), Text: "~/Documents"},
		}
		s.repoSelector.SetItems(items)
		
		// Set up selection callback
		s.repoSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			path := item.GetID()
			
			// Expand the path (~ to home directory)
			expandedPath, err := ExpandPath(path)
			if err == nil {
				path = expandedPath
			}
			
			// Check if path exists
			if !PathExists(path) {
				s.error = "Path does not exist: " + path
				return
			}
			
			// Check if path is a directory
			if !IsDirectory(path) {
				s.error = "Not a directory: " + path
				return
			}
			
			s.repoPath = path
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

// initDirectoryBrowser initializes the directory browser component
func (s *SessionSetupOverlay) initDirectoryBrowser() {
	if s.dirBrowser == nil {
		s.dirBrowser = NewFuzzyInputOverlay("Select Directory", "Search directories")
		
		// Expand the repository path first
		expandedRepoPath, err := ExpandPath(s.repoPath)
		if err != nil {
			s.error = "Error expanding path: " + err.Error()
			return
		}
		
		// List subdirectories in the repository
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "", Text: "<Repository Root>"},
		}
		
		// Check if the path exists
		if !PathExists(expandedRepoPath) {
			s.error = "Repository path does not exist: " + expandedRepoPath
			// Still include the repository root as an option
		} else {
			// Add subdirectories
			dirs, err := getSubdirectories(expandedRepoPath)
			if err == nil {
				for _, dir := range dirs {
					// Use relative path as ID and display text
					relPath, err := filepath.Rel(expandedRepoPath, dir)
					if err == nil {
						items = append(items, fuzzy.BasicStringItem{
							ID:   relPath,
							Text: relPath + "/",
						})
					}
				}
			}
		}
		
		s.dirBrowser.SetItems(items)
		
		// Set up selection callback
		s.dirBrowser.SetOnSelect(func(item fuzzy.SearchItem) {
			s.workingDir = item.GetID()
			
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
		})
		
		s.dirBrowser.SetOnCancel(func() {
			s.prevStep()
		})
		
		s.dirBrowser.SetSize(s.width-10, s.height-10)
		s.dirBrowser.Focus()
	}
}

// getSubdirectories returns a list of subdirectories in the given path
func getSubdirectories(path string) ([]string, error) {
	var dirs []string
	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		// Skip the root directory itself
		if p == path {
			return nil
		}
		
		// Skip hidden directories (starting with .)
		if info.IsDir() && strings.HasPrefix(filepath.Base(p), ".") {
			return filepath.SkipDir
		}
		
		// Add directory to the list
		if info.IsDir() {
			dirs = append(dirs, p)
			
			// Limit depth to avoid excessive recursion
			// If we're already 2 levels deep, skip deeper directories
			relPath, err := filepath.Rel(path, p)
			if err == nil && strings.Count(relPath, string(os.PathSeparator)) >= 2 {
				return filepath.SkipDir
			}
		}
		
		return nil
	})
	
	return dirs, err
}

// initWorktreeSelector initializes the worktree selector component
func (s *SessionSetupOverlay) initWorktreeSelector() {
	if s.worktreeSelector == nil {
		s.worktreeSelector = NewFuzzyInputOverlay("Select Worktree", "Search worktrees")
		
		// TODO: Implement actual worktree loading from git
		// For now, create some example worktrees in the user's home directory
		home, _ := os.UserHomeDir()
		
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: filepath.Join(home, "worktrees", "feature-branch"), Text: "feature-branch (~/worktrees/feature-branch)"},
			fuzzy.BasicStringItem{ID: filepath.Join(home, "worktrees", "bugfix-branch"), Text: "bugfix-branch (~/worktrees/bugfix-branch)"},
			fuzzy.BasicStringItem{ID: "~/git/project/worktree1", Text: "Custom Worktree Path (~/git/project/worktree1)"},
		}
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
		
		// TODO: Implement actual branch loading from git based on the repository path
		// For now, use placeholder example branches
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "main", Text: "main (default branch)"},
			fuzzy.BasicStringItem{ID: "develop", Text: "develop"},
			fuzzy.BasicStringItem{ID: "feature/login", Text: "feature/login"},
		}
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
		switch msg.Type {
		case tea.KeyEsc:
			// Cancel the setup
			if s.onCancel != nil {
				s.onCancel()
			}
			return nil
			
		case tea.KeyEnter:
			// Handle Enter for different steps
			if s.step == StepBasics || s.step == StepAdvanced {
				s.nextStep()
				return nil
			} else if s.step == StepLocation {
				s.nextStep()
				return nil
			} else if s.step == StepBranch && s.branchChoice == "new" {
				s.nextStep()
				return nil
			} else if s.step == StepConfirm {
				s.nextStep()
				return nil
			}
			
		case tea.KeyTab:
			// Tab has special meaning in some steps
			if s.step == StepBasics {
				// Toggle between name and program input
				if s.basicsFocused == "name" {
					s.basicsFocused = "program"
				} else {
					s.basicsFocused = "name"
				}
				return nil
			} else if s.step == StepLocation {
				// Cycle through location choices
				switch s.locationChoice {
				case "current":
					s.locationChoice = "different"
				case "different":
					s.locationChoice = "existing"
				case "existing":
					s.locationChoice = "current"
				}
				return nil
			} else if s.step == StepBranch {
				// Toggle branch choice
				if s.branchChoice == "new" {
					s.branchChoice = "existing"
				} else {
					s.branchChoice = "new"
				}
				return nil
			}
		
		case tea.KeyUp:
			// Up arrow navigation
			if s.step == StepBasics {
				// Toggle between name and program input (up goes to name)
				s.basicsFocused = "name"
				return nil
			} else if s.step == StepLocation {
				// Cycle backward through location choices
				switch s.locationChoice {
				case "current":
					s.locationChoice = "existing"
				case "different":
					s.locationChoice = "current"
				case "existing":
					s.locationChoice = "different"
				}
				return nil
			} else if s.step == StepBranch {
				// Toggle to new branch
				s.branchChoice = "new"
				return nil
			}
		
		case tea.KeyDown:
			// Down arrow navigation
			if s.step == StepBasics {
				// Toggle between name and program input (down goes to program)
				s.basicsFocused = "program"
				return nil
			} else if s.step == StepLocation {
				// Cycle forward through location choices
				switch s.locationChoice {
				case "current":
					s.locationChoice = "different"
				case "different":
					s.locationChoice = "existing"
				case "existing":
					s.locationChoice = "current"
				}
				return nil
			} else if s.step == StepBranch {
				// Toggle to existing branch
				s.branchChoice = "existing"
				return nil
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
		if s.dirBrowser != nil {
			_ = s.dirBrowser.Update(msg)
		}
		
	case StepWorktree:
		if s.worktreeSelector != nil {
			_ = s.worktreeSelector.Update(msg)
		}
		
	case StepBranch:
		if s.branchSelector != nil && s.branchChoice == "existing" {
			_ = s.branchSelector.Update(msg)
		}
	}
	
	return cmd
}

// View renders the overlay based on the current step
func (s *SessionSetupOverlay) View() string {
	if !s.focused {
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
		if s.dirBrowser != nil {
			sb.WriteString(s.dirBrowser.View())
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
		sb.WriteString("Branch strategy:\n\n")
		
		newStyle := s.buttonStyle
		existingStyle := s.buttonStyle
		
		// Highlight selected choice
		if s.branchChoice == "new" {
			newStyle = s.selectedStyle
		} else {
			existingStyle = s.selectedStyle
		}
		
		// Render the choices
		sb.WriteString(newStyle.Render("Create New Branch"))
		sb.WriteString(existingStyle.Render("Use Existing Branch"))
		sb.WriteString("\n\n")
		
		// Show branch selector if existing branch is selected
		if s.branchChoice == "existing" {
			if s.branchSelector != nil {
				sb.WriteString(s.branchSelector.View())
			} else {
				sb.WriteString("Loading branches...")
			}
		} else {
			sb.WriteString(s.infoStyle.Render("A new branch will be created based on your session name"))
		}
	
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
	
	for _, opt := range options {
		isSelected := s.locationChoice == opt.key
		
		var style lipgloss.Style
		if isSelected {
			style = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#00FFFF")).
				Padding(0, 1).
				MarginBottom(1).
				Background(lipgloss.Color("#1a1a2e"))
		} else {
			style = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#555555")).
				Padding(0, 1).
				MarginBottom(1)
		}
		
		content := fmt.Sprintf("%s %s\n%s", opt.icon, opt.title, opt.desc)
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