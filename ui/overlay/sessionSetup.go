package overlay

import (
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui/fuzzy"
	"fmt"
	"strings"
	
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionSetupStep represents the current step in the session setup wizard
type SessionSetupStep int

const (
	StepName SessionSetupStep = iota
	StepLocation
	StepProgram
	StepRepository
	StepDirectory
	StepWorktree
	StepBranch
	StepConfirm
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
	
	// Step-specific states
	locationChoice    string // "current", "different", "existing"
	branchChoice      string // "new", "existing"
	
	// UI components for different steps
	nameInput         *TextInputOverlay
	programInput      *TextInputOverlay
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
	
	// Create an empty session setup overlay
	return &SessionSetupOverlay{
		step:          StepName,
		width:         60,
		height:        20,
		focused:       true,
		error:         "",
		
		sessionName:   "",
		repoPath:      ".",  // Default to current directory
		workingDir:    "",   // Default to repository root
		program:       defaultProgram,
		branch:        "",   // Will be generated based on session name
		
		locationChoice: "current",
		branchChoice:   "new",
		
		nameInput:     nameInput,
		programInput:  programInput,
		
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
	case StepName:
		if s.nameInput != nil {
			// TextInputOverlay will handle focus internally
		}
		break
	case StepProgram:
		if s.programInput != nil {
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
	// Handle step transitions based on location and branch choices
	switch s.step {
	case StepName:
		s.sessionName = s.nameInput.GetValue()
		if s.sessionName == "" {
			s.error = "Session name cannot be empty"
			return
		}
		s.step = StepProgram
		// TextInputOverlay doesn't have explicit Focus method
	
	case StepProgram:
		s.program = s.programInput.GetValue()
		if s.program == "" {
			s.program = config.LoadConfig().DefaultProgram
		}
		s.step = StepLocation
	
	case StepLocation:
		if s.locationChoice == "current" {
			// Using current directory, skip to confirm
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
			options := session.InstanceOptions{
				Title:            s.sessionName,
				Path:             s.repoPath,
				WorkingDir:       s.workingDir,
				Program:          s.program,
				ExistingWorktree: s.existingWorktree,
			}
			s.onComplete(options)
		}
	}
	
	// Clear any error when advancing steps
	s.error = ""
}

// prevStep goes back to the previous step in the wizard
func (s *SessionSetupOverlay) prevStep() {
	switch s.step {
	case StepName:
		// First step, cancel the setup
		if s.onCancel != nil {
			s.onCancel()
		}
	
	case StepProgram:
		s.step = StepName
		// TextInputOverlay doesn't have explicit Focus method
	
	case StepLocation:
		s.step = StepProgram
		// TextInputOverlay doesn't have explicit Focus method
	
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
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: ".", Text: "Current Repository"},
			fuzzy.BasicStringItem{ID: "~/projects/myproject", Text: "My Project"},
			fuzzy.BasicStringItem{ID: "~/projects/another-project", Text: "Another Project"},
		}
		s.repoSelector.SetItems(items)
		
		// Set up selection callback
		s.repoSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			s.repoPath = item.GetID()
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
		
		// TODO: Implement actual directory browser with async loading
		// For now, use placeholder example directories
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "", Text: "<Repository Root>"},
			fuzzy.BasicStringItem{ID: "src", Text: "src/"},
			fuzzy.BasicStringItem{ID: "src/main", Text: "src/main/"},
			fuzzy.BasicStringItem{ID: "src/test", Text: "src/test/"},
		}
		s.dirBrowser.SetItems(items)
		
		// Set up selection callback
		s.dirBrowser.SetOnSelect(func(item fuzzy.SearchItem) {
			s.workingDir = item.GetID()
			s.nextStep()
		})
		
		s.dirBrowser.SetOnCancel(func() {
			s.prevStep()
		})
		
		s.dirBrowser.SetSize(s.width-10, s.height-10)
		s.dirBrowser.Focus()
	}
}

// initWorktreeSelector initializes the worktree selector component
func (s *SessionSetupOverlay) initWorktreeSelector() {
	if s.worktreeSelector == nil {
		s.worktreeSelector = NewFuzzyInputOverlay("Select Worktree", "Search worktrees")
		
		// TODO: Implement actual worktree loading from git
		// For now, use placeholder example worktrees
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "/path/to/worktree1", Text: "feature/auth (Created 2 days ago)"},
			fuzzy.BasicStringItem{ID: "/path/to/worktree2", Text: "bugfix/login (Created 1 week ago)"},
		}
		s.worktreeSelector.SetItems(items)
		
		// Set up selection callback
		s.worktreeSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			s.existingWorktree = item.GetID()
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
		
		// TODO: Implement actual branch loading from git
		// For now, use placeholder example branches
		items := []fuzzy.SearchItem{
			fuzzy.BasicStringItem{ID: "main", Text: "main"},
			fuzzy.BasicStringItem{ID: "develop", Text: "develop"},
			fuzzy.BasicStringItem{ID: "feature/login", Text: "feature/login"},
		}
		s.branchSelector.SetItems(items)
		
		// Set up selection callback
		s.branchSelector.SetOnSelect(func(item fuzzy.SearchItem) {
			s.branch = item.GetID()
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
			// Special case for selection steps
			if s.step == StepName || s.step == StepProgram {
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
			if s.step == StepLocation {
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
		}
	}
	
	// Forward messages to the appropriate component for the current step
	var cmd tea.Cmd
	switch s.step {
	case StepName:
		if msg, ok := msg.(tea.KeyMsg); ok && s.nameInput != nil {
			s.nameInput.HandleKeyPress(msg)
		}
		
	case StepProgram:
		if msg, ok := msg.(tea.KeyMsg); ok && s.programInput != nil {
			s.programInput.HandleKeyPress(msg)
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
	
	// Title
	sb.WriteString(s.titleStyle.Render("New Session Setup"))
	sb.WriteString("\n")
	
	// Step indicator
	stepText := fmt.Sprintf("Step %d of %d: ", int(s.step)+1, int(StepConfirm)+1)
	switch s.step {
	case StepName:
		stepText += "Session Name"
	case StepProgram:
		stepText += "Choose Program"
	case StepLocation:
		stepText += "Location Type"
	case StepRepository:
		stepText += "Select Repository"
	case StepDirectory:
		stepText += "Select Directory"
	case StepWorktree:
		stepText += "Select Worktree"
	case StepBranch:
		stepText += "Branch Strategy"
	case StepConfirm:
		stepText += "Confirm"
	}
	sb.WriteString(s.stepStyle.Render(stepText))
	sb.WriteString("\n\n")
	
	// Content based on current step
	switch s.step {
	case StepName:
		sb.WriteString(s.nameInput.View())
		sb.WriteString("\n")
		sb.WriteString(s.infoStyle.Render("Enter a descriptive name for your session"))
		
	case StepProgram:
		sb.WriteString(s.programInput.View())
		sb.WriteString("\n")
		sb.WriteString(s.infoStyle.Render("Enter the program to run (e.g. claude, aider, gemini)"))
	
	case StepLocation:
		sb.WriteString("Where do you want to create the session?\n\n")
		
		currentStyle := s.buttonStyle
		differentStyle := s.buttonStyle
		existingStyle := s.buttonStyle
		
		// Highlight selected choice
		switch s.locationChoice {
		case "current":
			currentStyle = s.selectedStyle
		case "different":
			differentStyle = s.selectedStyle
		case "existing":
			existingStyle = s.selectedStyle
		}
		
		// Render the choices
		sb.WriteString(currentStyle.Render("Current Repository"))
		sb.WriteString(differentStyle.Render("Different Repository"))
		sb.WriteString(existingStyle.Render("Existing Worktree"))
		sb.WriteString("\n\n")
		
		// Help text
		sb.WriteString(s.infoStyle.Render("Press Tab to toggle between options, Enter to select"))
	
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
		// Summary of session settings
		sb.WriteString("Session Settings Summary:\n\n")
		sb.WriteString(fmt.Sprintf("Name: %s\n", s.sessionName))
		sb.WriteString(fmt.Sprintf("Program: %s\n", s.program))
		
		if s.locationChoice == "current" {
			sb.WriteString("Location: Current Repository\n")
			if s.workingDir != "" {
				sb.WriteString(fmt.Sprintf("Working Directory: %s\n", s.workingDir))
			}
		} else if s.locationChoice == "different" {
			sb.WriteString(fmt.Sprintf("Repository: %s\n", s.repoPath))
			if s.workingDir != "" {
				sb.WriteString(fmt.Sprintf("Working Directory: %s\n", s.workingDir))
			}
			if s.branchChoice == "new" {
				sb.WriteString("Branch: New branch will be created\n")
			} else {
				sb.WriteString(fmt.Sprintf("Branch: %s\n", s.branch))
			}
		} else { // "existing"
			sb.WriteString(fmt.Sprintf("Using Existing Worktree: %s\n", s.existingWorktree))
		}
		
		sb.WriteString("\nPress Enter to create session or Esc to cancel")
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