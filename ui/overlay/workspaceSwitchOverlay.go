package overlay

import (
	"fmt"
	"strings"
	"time"

	"claude-squad/log"
	"claude-squad/session/vcs"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WorkspaceSwitchMode represents the current mode of the overlay
type WorkspaceSwitchMode int

const (
	// ModeSelectDestination is the main destination selection mode
	ModeSelectDestination WorkspaceSwitchMode = iota
	// ModeCreateBookmark is for creating a new bookmark/branch
	ModeCreateBookmark
	// ModeCreateWorktree is for creating a new worktree
	ModeCreateWorktree
	// ModeSelectDirectory is for selecting a directory to change to
	ModeSelectDirectory
)

// DestinationType represents the type of destination item
type DestinationType int

const (
	DestTypeBookmark DestinationType = iota
	DestTypeRevision
	DestTypeWorktree
	DestTypeAction // Special items like "Create new..."
)

// DestinationItem represents a selectable destination
type DestinationItem struct {
	Type        DestinationType
	Name        string
	Description string
	ID          string // Revision ID or path
	IsCurrent   bool
	Timestamp   time.Time
}

// WorkspaceSwitchOverlay provides an interface for switching workspaces
type WorkspaceSwitchOverlay struct {
	BaseOverlay

	// Session info
	sessionTitle     string
	repoPath         string
	currentBookmark  string
	currentRevision  string
	hasChanges       bool
	modifiedCount    int
	vcsType          string

	// Destination items
	destinations    []DestinationItem
	selectedIndex   int
	filteredIndices []int // Indices into destinations for filtered view
	filterQuery     string

	// Change handling strategy
	changeStrategy vcs.ChangeStrategy

	// UI mode
	mode        WorkspaceSwitchMode
	input       textinput.Model
	inputActive bool

	// Status
	message     string
	messageType string // "success", "error", "warning", ""
	isLoading   bool

	// Callbacks
	OnSwitch func(target string, switchType int, strategy vcs.ChangeStrategy, createIfMissing bool)
	OnCancel func()
}

// NewWorkspaceSwitchOverlay creates a new workspace switch overlay
func NewWorkspaceSwitchOverlay(sessionTitle string, repoPath string) *WorkspaceSwitchOverlay {
	ti := textinput.New()
	ti.Placeholder = "Enter name..."
	ti.CharLimit = 100
	ti.Width = 40

	overlay := &WorkspaceSwitchOverlay{
		sessionTitle:    sessionTitle,
		repoPath:        repoPath,
		selectedIndex:   0,
		changeStrategy:  vcs.KeepAsWIP,
		mode:            ModeSelectDestination,
		input:           ti,
		inputActive:     false,
		isLoading:       true,
		filteredIndices: []int{},
	}

	overlay.BaseOverlay.SetSize(80, 30)
	overlay.BaseOverlay.Focus()

	return overlay
}

// LoadTargets loads available switch targets asynchronously
func (w *WorkspaceSwitchOverlay) LoadTargets() {
	go func() {
		vcsClient, err := vcs.Detect(w.repoPath)
		if err != nil {
			w.message = fmt.Sprintf("Failed to detect VCS: %v", err)
			w.messageType = "error"
			w.isLoading = false
			return
		}

		w.vcsType = string(vcsClient.Type())

		// Get current state
		if bookmark, err := vcsClient.GetCurrentBookmark(); err == nil {
			w.currentBookmark = bookmark
		}
		if rev, err := vcsClient.GetCurrentRevision(); err == nil {
			w.currentRevision = rev.ShortID
		}
		if status, err := vcsClient.GetStatus(); err == nil {
			w.hasChanges = status.HasChanges
			w.modifiedCount = status.ModifiedFiles + status.AddedFiles + status.DeletedFiles
		}

		// Load bookmarks
		if bookmarks, err := vcsClient.ListBookmarks(); err == nil {
			for _, b := range bookmarks {
				if b.IsRemote {
					continue // Skip remote branches
				}
				w.destinations = append(w.destinations, DestinationItem{
					Type:        DestTypeBookmark,
					Name:        b.Name,
					Description: "bookmark",
					ID:          b.RevisionID,
					IsCurrent:   b.Name == w.currentBookmark,
				})
			}
		}

		// Load recent revisions (only if JJ - Git users prefer branches)
		if vcsClient.Type() == vcs.VCSTypeJJ {
			if revisions, err := vcsClient.ListRecentRevisions(5); err == nil {
				for _, r := range revisions {
					// Skip if this revision is already covered by a bookmark
					alreadyCovered := false
					for _, d := range w.destinations {
						if d.ID == r.ID || d.ID == r.ShortID {
							alreadyCovered = true
							break
						}
					}
					if alreadyCovered {
						continue
					}

					desc := r.Description
					if len(desc) > 30 {
						desc = desc[:27] + "..."
					}

					w.destinations = append(w.destinations, DestinationItem{
						Type:        DestTypeRevision,
						Name:        r.ShortID,
						Description: desc,
						ID:          r.ID,
						IsCurrent:   r.IsCurrent,
						Timestamp:   r.Timestamp,
					})
				}
			}
		}

		// Load worktrees
		if worktrees, err := vcsClient.ListWorktrees(); err == nil {
			for _, wt := range worktrees {
				if wt.IsCurrent {
					continue // Skip current worktree
				}
				w.destinations = append(w.destinations, DestinationItem{
					Type:        DestTypeWorktree,
					Name:        wt.Name,
					Description: wt.Path,
					ID:          wt.Path,
					IsCurrent:   false,
				})
			}
		}

		// Add action items
		w.destinations = append(w.destinations, DestinationItem{
			Type:        DestTypeAction,
			Name:        "[+] Create new bookmark...",
			Description: "action:create_bookmark",
		})
		w.destinations = append(w.destinations, DestinationItem{
			Type:        DestTypeAction,
			Name:        "[w] Create new worktree...",
			Description: "action:create_worktree",
		})
		w.destinations = append(w.destinations, DestinationItem{
			Type:        DestTypeAction,
			Name:        "[d] Change directory only...",
			Description: "action:change_directory",
		})

		// Initialize filtered indices to all items
		w.filteredIndices = make([]int, len(w.destinations))
		for i := range w.destinations {
			w.filteredIndices[i] = i
		}

		w.isLoading = false
		log.InfoLog.Printf("[WorkspaceSwitch] Loaded %d destinations for %s", len(w.destinations), w.sessionTitle)
	}()
}

// SetSize updates the overlay dimensions
func (w *WorkspaceSwitchOverlay) SetSize(width, height int) {
	w.BaseOverlay.SetSize(width, height)
}

// HandleKeyPress processes keyboard input
func (w *WorkspaceSwitchOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Clear transient messages
	if w.messageType != "error" {
		w.message = ""
		w.messageType = ""
	}

	// Handle input mode
	if w.inputActive {
		return w.handleInputKeys(msg)
	}

	// Handle common keys
	if handled, shouldClose := w.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose && w.OnCancel != nil {
			w.OnCancel()
		}
		return shouldClose
	}

	switch w.mode {
	case ModeSelectDestination:
		return w.handleDestinationKeys(msg)
	case ModeCreateBookmark, ModeCreateWorktree:
		return w.handleCreateKeys(msg)
	}

	return false
}

// handleDestinationKeys handles keys in destination selection mode
func (w *WorkspaceSwitchOverlay) handleDestinationKeys(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if w.selectedIndex > 0 {
			w.selectedIndex--
		}
		return false

	case tea.KeyDown, tea.KeyCtrlN:
		if w.selectedIndex < len(w.filteredIndices)-1 {
			w.selectedIndex++
		}
		return false

	case tea.KeyTab:
		// Cycle through change strategies
		w.changeStrategy = (w.changeStrategy + 1) % 3
		return false

	case tea.KeyEnter:
		return w.selectCurrentItem()

	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "n", "N":
			w.mode = ModeCreateBookmark
			w.inputActive = true
			w.input.Placeholder = "Enter bookmark name..."
			w.input.Focus()
			return false

		case "w", "W":
			w.mode = ModeCreateWorktree
			w.inputActive = true
			w.input.Placeholder = "Enter worktree path..."
			w.input.Focus()
			return false

		case "d", "D":
			// Emit directory change action
			if w.OnSwitch != nil {
				w.OnSwitch("", 0, w.changeStrategy, false) // switchType 0 = directory
			}
			return true

		case "/":
			// Activate filter mode
			w.inputActive = true
			w.input.Placeholder = "Filter..."
			w.input.Focus()
			return false

		case "1":
			w.changeStrategy = vcs.KeepAsWIP
			return false

		case "2":
			w.changeStrategy = vcs.BringAlong
			return false

		case "3":
			w.changeStrategy = vcs.Abandon
			return false
		}
	}

	return false
}

// selectCurrentItem handles selection of the current item
func (w *WorkspaceSwitchOverlay) selectCurrentItem() bool {
	if len(w.filteredIndices) == 0 || w.selectedIndex >= len(w.filteredIndices) {
		return false
	}

	item := w.destinations[w.filteredIndices[w.selectedIndex]]

	switch item.Type {
	case DestTypeAction:
		switch item.Description {
		case "action:create_bookmark":
			w.mode = ModeCreateBookmark
			w.inputActive = true
			w.input.Placeholder = "Enter bookmark name..."
			w.input.Focus()
			return false
		case "action:create_worktree":
			w.mode = ModeCreateWorktree
			w.inputActive = true
			w.input.Placeholder = "Enter worktree path..."
			w.input.Focus()
			return false
		case "action:change_directory":
			if w.OnSwitch != nil {
				w.OnSwitch("", 0, w.changeStrategy, false)
			}
			return true
		}

	case DestTypeBookmark:
		if w.OnSwitch != nil {
			w.OnSwitch(item.Name, 1, w.changeStrategy, false) // switchType 1 = revision
		}
		return true

	case DestTypeRevision:
		if w.OnSwitch != nil {
			w.OnSwitch(item.ID, 1, w.changeStrategy, false)
		}
		return true

	case DestTypeWorktree:
		if w.OnSwitch != nil {
			w.OnSwitch(item.ID, 2, w.changeStrategy, false) // switchType 2 = worktree
		}
		return true
	}

	return false
}

// handleCreateKeys handles keys in creation modes
func (w *WorkspaceSwitchOverlay) handleCreateKeys(msg tea.KeyMsg) bool {
	// Creation modes are handled via input
	return false
}

// handleInputKeys handles keys when input is active
func (w *WorkspaceSwitchOverlay) handleInputKeys(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		w.inputActive = false
		w.input.Blur()
		w.input.SetValue("")
		if w.mode != ModeSelectDestination {
			w.mode = ModeSelectDestination
		}
		return false

	case tea.KeyEnter:
		value := strings.TrimSpace(w.input.Value())

		// Handle filter mode (in destination selection)
		if w.mode == ModeSelectDestination {
			w.filterQuery = value
			w.applyFilter()
			w.inputActive = false
			w.input.Blur()
			w.input.SetValue("")
			return false
		}

		// Handle creation modes
		if value == "" {
			w.message = "Name cannot be empty"
			w.messageType = "error"
			return false
		}

		if w.OnSwitch != nil {
			switch w.mode {
			case ModeCreateBookmark:
				w.OnSwitch(value, 1, w.changeStrategy, true) // createIfMissing=true
			case ModeCreateWorktree:
				w.OnSwitch(value, 2, w.changeStrategy, true)
			}
		}

		w.inputActive = false
		w.input.Blur()
		w.input.SetValue("")
		return true

	default:
		var cmd tea.Cmd
		w.input, cmd = w.input.Update(msg)
		_ = cmd
		return false
	}
}

// applyFilter filters destinations based on the query
func (w *WorkspaceSwitchOverlay) applyFilter() {
	if w.filterQuery == "" {
		// Reset to show all
		w.filteredIndices = make([]int, len(w.destinations))
		for i := range w.destinations {
			w.filteredIndices[i] = i
		}
		return
	}

	query := strings.ToLower(w.filterQuery)
	w.filteredIndices = []int{}

	for i, dest := range w.destinations {
		name := strings.ToLower(dest.Name)
		desc := strings.ToLower(dest.Description)
		if strings.Contains(name, query) || strings.Contains(desc, query) {
			w.filteredIndices = append(w.filteredIndices, i)
		}
	}

	// Reset selection
	w.selectedIndex = 0
}

// Render renders the overlay
func (w *WorkspaceSwitchOverlay) Render() string {
	responsiveWidth := w.GetResponsiveWidth()
	hPadding, vPadding := w.GetResponsivePadding()

	// Styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(vPadding, hPadding).
		Width(responsiveWidth)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0")).
		Padding(0, 1)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7")).
		Padding(0, 1)

	currentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10"))

	actionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("14"))

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("11"))

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Italic(true)

	radioSelectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true)

	radioNormalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8"))

	var content strings.Builder

	// Title
	content.WriteString(titleStyle.Render("Switch Workspace"))
	content.WriteString("\n\n")

	// Session info
	content.WriteString(subtitleStyle.Render(fmt.Sprintf("Session: %s", w.sessionTitle)))
	content.WriteString("\n")
	current := w.currentBookmark
	if current == "" {
		current = w.currentRevision
	}
	if w.hasChanges {
		content.WriteString(subtitleStyle.Render(fmt.Sprintf("Current: %s ", current)))
		content.WriteString(warningStyle.Render(fmt.Sprintf("(%d files modified)", w.modifiedCount)))
	} else {
		content.WriteString(subtitleStyle.Render(fmt.Sprintf("Current: %s", current)))
	}
	content.WriteString("\n")
	content.WriteString(subtitleStyle.Render(fmt.Sprintf("VCS: %s", w.vcsType)))
	content.WriteString("\n\n")

	// Loading state
	if w.isLoading {
		content.WriteString("Loading destinations...\n")
		return style.Render(content.String())
	}

	// Destination list
	content.WriteString("─ Destination ─\n")
	maxVisible := w.GetMaxVisibleItems(1) - 10 // Reserve space for other UI elements
	if maxVisible < 5 {
		maxVisible = 5
	}

	start := 0
	if w.selectedIndex >= maxVisible {
		start = w.selectedIndex - maxVisible + 1
	}

	for i := start; i < len(w.filteredIndices) && i < start+maxVisible; i++ {
		dest := w.destinations[w.filteredIndices[i]]
		prefix := "  "
		if dest.IsCurrent {
			prefix = "* "
		}

		var itemText string
		switch dest.Type {
		case DestTypeAction:
			itemText = actionStyle.Render(dest.Name)
		case DestTypeBookmark:
			itemText = fmt.Sprintf("%s%s  %s", prefix, dest.Name, subtitleStyle.Render("bookmark"))
		case DestTypeRevision:
			itemText = fmt.Sprintf("%s%s  %s", prefix, dest.Name, subtitleStyle.Render(dest.Description))
		case DestTypeWorktree:
			itemText = fmt.Sprintf("%s%s  %s", prefix, dest.Name, subtitleStyle.Render(dest.Description))
		}

		if dest.IsCurrent {
			itemText = currentStyle.Render(itemText)
		}

		if i == w.selectedIndex {
			content.WriteString(selectedStyle.Render(itemText))
		} else {
			content.WriteString(normalStyle.Render(itemText))
		}
		content.WriteString("\n")
	}
	content.WriteString("\n")

	// Filter display
	if w.filterQuery != "" {
		content.WriteString(subtitleStyle.Render(fmt.Sprintf("Filter: %s", w.filterQuery)))
		content.WriteString("\n\n")
	}

	// Uncommitted changes handling (only show if there are changes)
	if w.hasChanges {
		content.WriteString("─ Handle uncommitted changes ─\n")
		strategies := []struct {
			label string
			value vcs.ChangeStrategy
		}{
			{"Keep as WIP revision", vcs.KeepAsWIP},
			{"Bring changes to destination", vcs.BringAlong},
			{"Abandon changes", vcs.Abandon},
		}
		for _, s := range strategies {
			if w.changeStrategy == s.value {
				content.WriteString(radioSelectedStyle.Render(fmt.Sprintf("(•) %s", s.label)))
			} else {
				content.WriteString(radioNormalStyle.Render(fmt.Sprintf("( ) %s", s.label)))
			}
			content.WriteString("\n")
		}
		content.WriteString("\n")
	}

	// Warning message
	if w.mode == ModeSelectDestination && !w.inputActive {
		content.WriteString(warningStyle.Render("⚠ Claude will restart with conversation preserved"))
		content.WriteString("\n\n")
	}

	// Input field (if active)
	if w.inputActive {
		content.WriteString("\n")
		content.WriteString(w.input.View())
		content.WriteString("\n\n")
	}

	// Status message
	if w.message != "" {
		msgStyle := subtitleStyle
		if w.messageType == "error" {
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		} else if w.messageType == "warning" {
			msgStyle = warningStyle
		} else if w.messageType == "success" {
			msgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		}
		content.WriteString(msgStyle.Render(w.message))
		content.WriteString("\n\n")
	}

	// Help text
	if w.inputActive {
		content.WriteString(helpStyle.Render("Enter: confirm • Esc: cancel"))
	} else {
		content.WriteString(helpStyle.Render("↑↓: navigate • Tab/1-3: change strategy • /: filter • Enter: switch • Esc: cancel"))
	}

	return style.Render(content.String())
}
