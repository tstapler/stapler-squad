package omnibar

import (
	"claude-squad/config"
	"claude-squad/github"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui/overlay"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// OmnibarCallbacks contains the callbacks for the omnibar overlay
type OmnibarCallbacks struct {
	OnComplete func(session.InstanceOptions)
	OnCancel   func()
}

// OmnibarOverlay is the intelligent session creation interface
type OmnibarOverlay struct {
	overlay.BaseOverlay

	// Input state
	omnibarInput     textinput.Model
	sessionNameInput textinput.Model

	// Detection state
	currentDetection   *DetectionResult
	currentValidation  *ValidationResult
	lastDetectionInput string
	lastDetectionTime  time.Time

	// Focus state
	focusedField string // "omnibar", "name", "program"
	programInput textinput.Model

	// Configuration
	program          string
	generatePRPrompt bool

	// UI state
	error        string
	warning      string
	cloningStatus string
	isLoading    bool

	// Styles
	titleStyle       lipgloss.Style
	inputLabelStyle  lipgloss.Style
	detectionStyle   lipgloss.Style
	errorStyle       lipgloss.Style
	warningStyle     lipgloss.Style
	successStyle     lipgloss.Style
	helpStyle        lipgloss.Style
	buttonStyle      lipgloss.Style
	selectedBtnStyle lipgloss.Style

	// Callbacks
	onComplete func(session.InstanceOptions)
	onCancel   func()
}

// NewOmnibarOverlay creates a new omnibar overlay
func NewOmnibarOverlay(callbacks OmnibarCallbacks) *OmnibarOverlay {
	if callbacks.OnComplete == nil {
		panic("OmnibarOverlay requires OnComplete callback")
	}

	// Load default program
	cfg := config.LoadConfig()

	// Create omnibar input
	omniInput := textinput.New()
	omniInput.Placeholder = "Enter path, GitHub URL, or owner/repo..."
	omniInput.Focus()
	omniInput.CharLimit = 500
	omniInput.Width = 50

	// Create session name input
	nameInput := textinput.New()
	nameInput.Placeholder = "Session name (auto-generated)"
	nameInput.CharLimit = 100
	nameInput.Width = 40

	// Create program input
	progInput := textinput.New()
	progInput.Placeholder = "Program"
	progInput.SetValue(cfg.DefaultProgram)
	progInput.CharLimit = 200
	progInput.Width = 40

	// Create styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginBottom(1)

	inputLabelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#AAAAAA"))

	detectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF00")).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000"))

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFCC00"))

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF00"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666")).
		Italic(true)

	buttonStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#555555")).
		Padding(0, 2)

	selectedBtnStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF")).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 2)

	o := &OmnibarOverlay{
		omnibarInput:     omniInput,
		sessionNameInput: nameInput,
		programInput:     progInput,
		focusedField:     "omnibar",
		program:          cfg.DefaultProgram,
		generatePRPrompt: true, // Default to generating PR prompts

		titleStyle:       titleStyle,
		inputLabelStyle:  inputLabelStyle,
		detectionStyle:   detectionStyle,
		errorStyle:       errorStyle,
		warningStyle:     warningStyle,
		successStyle:     successStyle,
		helpStyle:        helpStyle,
		buttonStyle:      buttonStyle,
		selectedBtnStyle: selectedBtnStyle,

		onComplete: callbacks.OnComplete,
		onCancel:   callbacks.OnCancel,
	}

	o.BaseOverlay.SetSize(70, 25)
	o.BaseOverlay.Focus()

	return o
}

// SetSize sets the overlay size
func (o *OmnibarOverlay) SetSize(width, height int) {
	o.BaseOverlay.SetSize(width, height)

	responsiveWidth := o.GetResponsiveWidth() - 10
	if responsiveWidth < 30 {
		responsiveWidth = 30
	}

	o.omnibarInput.Width = responsiveWidth
	o.sessionNameInput.Width = responsiveWidth - 10
	o.programInput.Width = responsiveWidth - 10
}

// Focus focuses the overlay
func (o *OmnibarOverlay) Focus() {
	o.BaseOverlay.Focus()
	o.omnibarInput.Focus()
	o.focusedField = "omnibar"
}

// Blur blurs the overlay
func (o *OmnibarOverlay) Blur() {
	o.BaseOverlay.Blur()
	o.omnibarInput.Blur()
	o.sessionNameInput.Blur()
	o.programInput.Blur()
}

// Update handles messages
func (o *OmnibarOverlay) Update(msg tea.Msg) tea.Cmd {
	if !o.IsFocused() {
		return nil
	}

	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		// Handle escape
		if keyMsg.Type == tea.KeyEscape {
			if o.onCancel != nil {
				o.onCancel()
			}
			return nil
		}

		// Handle Tab navigation
		if keyMsg.Type == tea.KeyTab || keyMsg.Type == tea.KeyShiftTab {
			o.cycleField(keyMsg.Type == tea.KeyShiftTab)
			return nil
		}

		// Handle Enter - submit
		if keyMsg.Type == tea.KeyEnter {
			o.handleSubmit()
			return nil
		}

		// Forward to focused input
		var cmd tea.Cmd
		switch o.focusedField {
		case "omnibar":
			o.omnibarInput, cmd = o.omnibarInput.Update(msg)
			o.detectInput()
		case "name":
			o.sessionNameInput, cmd = o.sessionNameInput.Update(msg)
		case "program":
			o.programInput, cmd = o.programInput.Update(msg)
		}
		return cmd
	}

	return nil
}

// cycleField cycles through input fields
func (o *OmnibarOverlay) cycleField(reverse bool) {
	fields := []string{"omnibar", "name", "program"}
	currentIndex := 0
	for i, f := range fields {
		if f == o.focusedField {
			currentIndex = i
			break
		}
	}

	if reverse {
		currentIndex = (currentIndex - 1 + len(fields)) % len(fields)
	} else {
		currentIndex = (currentIndex + 1) % len(fields)
	}

	o.focusedField = fields[currentIndex]

	// Update focus state
	o.omnibarInput.Blur()
	o.sessionNameInput.Blur()
	o.programInput.Blur()

	switch o.focusedField {
	case "omnibar":
		o.omnibarInput.Focus()
	case "name":
		o.sessionNameInput.Focus()
	case "program":
		o.programInput.Focus()
	}
}

// detectInput runs detection on the current input
func (o *OmnibarOverlay) detectInput() {
	input := strings.TrimSpace(o.omnibarInput.Value())

	// Skip if input hasn't changed
	if input == o.lastDetectionInput {
		return
	}

	o.lastDetectionInput = input
	o.lastDetectionTime = time.Now()
	o.error = ""
	o.warning = ""

	if input == "" {
		o.currentDetection = nil
		o.currentValidation = nil
		return
	}

	// Run detection
	o.currentDetection = Detect(input)

	// Update suggested session name if we have a detection
	if o.currentDetection != nil && o.currentDetection.Type != InputTypeUnknown {
		if o.sessionNameInput.Value() == "" || o.sessionNameInput.Value() == o.currentDetection.SuggestedName {
			o.sessionNameInput.SetValue(o.currentDetection.SuggestedName)
		}

		// Run validation
		o.currentValidation = Validate(o.currentDetection)

		if o.currentValidation != nil {
			if !o.currentValidation.Valid {
				o.error = o.currentValidation.ErrorMessage
			}
			if len(o.currentValidation.Warnings) > 0 {
				o.warning = o.currentValidation.Warnings[0]
			}
		}
	}
}

// handleSubmit handles form submission
func (o *OmnibarOverlay) handleSubmit() {
	input := strings.TrimSpace(o.omnibarInput.Value())
	if input == "" {
		o.error = "Please enter a path or GitHub URL"
		return
	}

	if o.currentDetection == nil || o.currentDetection.Type == InputTypeUnknown {
		o.error = "Unable to detect input type"
		return
	}

	if o.currentValidation != nil && !o.currentValidation.Valid {
		// Already have an error displayed
		return
	}

	// Get session name
	sessionName := strings.TrimSpace(o.sessionNameInput.Value())
	if sessionName == "" {
		sessionName = o.currentDetection.SuggestedName
	}
	if sessionName == "" {
		o.error = "Session name is required"
		return
	}

	// Get program
	program := strings.TrimSpace(o.programInput.Value())
	if program == "" {
		program = config.LoadConfig().DefaultProgram
	}

	// Build session options based on detection type
	opts := session.InstanceOptions{
		Title:   sessionName,
		Program: program,
	}

	switch o.currentDetection.Type {
	case InputTypeLocalPath:
		o.handleLocalPath(&opts)

	case InputTypePathWithBranch:
		o.handlePathWithBranch(&opts)

	case InputTypeGitHubPR, InputTypeGitHubBranch, InputTypeGitHubRepo, InputTypeGitHubShorthand:
		o.handleGitHub(&opts)
	}

	if o.error != "" {
		return
	}

	log.DebugLog.Printf("OmnibarOverlay: Creating session with options: %+v", opts)
	o.onComplete(opts)
}

// handleLocalPath handles local path session creation
func (o *OmnibarOverlay) handleLocalPath(opts *session.InstanceOptions) {
	path := o.currentDetection.LocalPath

	// Expand path
	expandedPath, err := overlay.ExpandPath(path)
	if err != nil {
		o.error = fmt.Sprintf("Invalid path: %v", err)
		return
	}

	opts.Path = expandedPath
	opts.SessionType = session.SessionTypeDirectory
}

// handlePathWithBranch handles path+branch session creation
func (o *OmnibarOverlay) handlePathWithBranch(opts *session.InstanceOptions) {
	path := o.currentDetection.LocalPath
	branch := o.currentDetection.Branch

	// Expand path
	expandedPath, err := overlay.ExpandPath(path)
	if err != nil {
		o.error = fmt.Sprintf("Invalid path: %v", err)
		return
	}

	opts.Path = expandedPath

	// Check if it's a git repo for worktree creation
	if o.currentValidation != nil && o.currentValidation.IsGitRepo {
		opts.SessionType = session.SessionTypeNewWorktree
		// For new worktrees, store branch info in the title suffix or use existing worktree path
		// The actual branch creation is handled during session instantiation
		// Update the title to include branch info
		if branch != "" && !strings.Contains(opts.Title, branch) {
			opts.Title = opts.Title + "-" + branch
		}
	} else {
		opts.SessionType = session.SessionTypeDirectory
	}
}

// handleGitHub handles GitHub-based session creation
func (o *OmnibarOverlay) handleGitHub(opts *session.InstanceOptions) {
	ref := o.currentDetection.GitHubRef
	if ref == nil {
		o.error = "Invalid GitHub reference"
		return
	}

	o.cloningStatus = "Cloning repository..."
	o.isLoading = true

	// Clone or get the repository
	cloneOpts := github.CloneOptions{
		Owner:  ref.Owner,
		Repo:   ref.Repo,
		Branch: ref.Branch,
	}

	result, err := github.GetOrCloneRepository(cloneOpts)
	o.isLoading = false
	o.cloningStatus = ""

	if err != nil {
		o.error = fmt.Sprintf("Failed to clone repository: %v", err)
		return
	}

	opts.Path = result.Path
	opts.SessionType = session.SessionTypeDirectory

	// Add GitHub metadata
	opts.GitHubOwner = ref.Owner
	opts.GitHubRepo = ref.Repo
	opts.GitHubSourceRef = ref.OriginalURL
	opts.ClonedRepoPath = result.Path

	// Handle PR-specific metadata
	if ref.Type == github.RefTypePR {
		opts.GitHubPRNumber = ref.PRNumber
		opts.GitHubPRURL = ref.HTMLURL()

		// Generate PR context prompt if requested
		if o.generatePRPrompt {
			prInfo, err := github.GetPRInfo(ref.Owner, ref.Repo, ref.PRNumber)
			if err == nil {
				opts.Prompt = github.GeneratePRPrompt(prInfo, true)
			}
		}
	}
}

// View renders the overlay
func (o *OmnibarOverlay) View() string {
	if !o.IsFocused() {
		return ""
	}

	var sb strings.Builder

	// Title
	sb.WriteString(o.titleStyle.Render("🚀 Quick Session Creation"))
	sb.WriteString("\n\n")

	// Omnibar input
	omnibarLabel := "📍 Location:"
	if o.focusedField == "omnibar" {
		omnibarLabel = "► " + omnibarLabel
	}
	sb.WriteString(o.inputLabelStyle.Render(omnibarLabel))
	sb.WriteString("\n")
	sb.WriteString(o.omnibarInput.View())
	sb.WriteString("\n")

	// Detection indicator
	if o.currentDetection != nil && o.currentDetection.Type != InputTypeUnknown {
		icon := o.currentDetection.Type.Icon()
		typeName := o.currentDetection.Type.String()
		sb.WriteString(o.detectionStyle.Render(fmt.Sprintf("%s Detected: %s", icon, typeName)))
		sb.WriteString("\n")

		// Show additional context for GitHub refs
		if o.currentDetection.GitHubRef != nil {
			ref := o.currentDetection.GitHubRef
			contextInfo := o.helpStyle.Render(fmt.Sprintf("  %s", ref.DisplayName()))
			sb.WriteString(contextInfo)
			sb.WriteString("\n")
		}
	}

	// Warning
	if o.warning != "" {
		sb.WriteString(o.warningStyle.Render("⚠️ " + o.warning))
		sb.WriteString("\n")
	}

	// Cloning status
	if o.cloningStatus != "" {
		sb.WriteString(o.warningStyle.Render("⏳ " + o.cloningStatus))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Session name input
	nameLabel := "📛 Session Name:"
	if o.focusedField == "name" {
		nameLabel = "► " + nameLabel
	}
	sb.WriteString(o.inputLabelStyle.Render(nameLabel))
	sb.WriteString("\n")
	sb.WriteString(o.sessionNameInput.View())
	sb.WriteString("\n\n")

	// Program input
	progLabel := "🤖 Program:"
	if o.focusedField == "program" {
		progLabel = "► " + progLabel
	}
	sb.WriteString(o.inputLabelStyle.Render(progLabel))
	sb.WriteString("\n")

	// Show friendly program name
	progDisplay := filepath.Base(o.programInput.Value())
	if strings.Contains(progDisplay, "claude") {
		progDisplay = "claude"
	}
	sb.WriteString(o.programInput.View())
	sb.WriteString("\n")

	// Error message
	if o.error != "" {
		sb.WriteString("\n")
		sb.WriteString(o.errorStyle.Render("❌ " + o.error))
		sb.WriteString("\n")
	}

	// Help text
	sb.WriteString("\n")
	helpText := "Tab: Next field • Enter: Create • Esc: Cancel"
	sb.WriteString(o.helpStyle.Render(helpText))
	sb.WriteString("\n")

	// Examples
	sb.WriteString(o.helpStyle.Render("Examples: ~/projects/myapp • owner/repo • https://github.com/o/r/pull/123"))

	// Wrap in a box
	contentWidth := o.GetResponsiveWidth()
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 2).
		Width(contentWidth)

	return boxStyle.Render(sb.String())
}

// getDisplayPath converts absolute path to display-friendly format
func getDisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
