package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// GitFileStatus represents the status of a file in git
type GitFileStatus struct {
	Path       string
	Staged     bool   // File is staged for commit
	Modified   bool   // File has unstaged changes
	Untracked  bool   // File is untracked
	StatusChar string // Git status character (M, A, D, ??, etc.)
}

// GitStatusOverlay provides a fugitive-style interactive git status interface
type GitStatusOverlay struct {
	// Git status data
	Files      []GitFileStatus
	BranchName string

	// Navigation state
	selectedIdx  int
	scrollOffset int

	// Overlay state
	width, height int
	Dismissed     bool

	// Callbacks for git operations
	OnStageFile   func(path string) error
	OnUnstageFile func(path string) error
	OnToggleFile  func(path string) error
	OnUnstageAll  func() error
	OnCommit      func() error
	OnCommitAmend func() error
	OnShowDiff    func(path string) error
	OnPush        func() error
	OnPull        func() error
	OnCancel      func()
	OnOpenFile    func(path string) error

	// Status message for feedback
	statusMessage string

	// Help visibility
	showHelp bool
}

// NewGitStatusOverlay creates a new git status overlay
func NewGitStatusOverlay() *GitStatusOverlay {
	return &GitStatusOverlay{
		Files:         []GitFileStatus{},
		selectedIdx:   0,
		scrollOffset:  0,
		Dismissed:     false,
		showHelp:      true,
		statusMessage: "Git Status - Press ? for help",
	}
}

// SetSize sets the dimensions of the overlay
func (g *GitStatusOverlay) SetSize(width, height int) {
	g.width = width
	g.height = height
}

// SetFiles updates the git status file list
func (g *GitStatusOverlay) SetFiles(files []GitFileStatus, branchName string) {
	g.Files = files
	g.BranchName = branchName

	// Reset selection if it's out of bounds
	if g.selectedIdx >= len(g.Files) {
		g.selectedIdx = 0
	}
}

// GetSelectedFile returns the currently selected file, or nil if none
func (g *GitStatusOverlay) GetSelectedFile() *GitFileStatus {
	if len(g.Files) == 0 || g.selectedIdx < 0 || g.selectedIdx >= len(g.Files) {
		return nil
	}
	return &g.Files[g.selectedIdx]
}

// HandleKeyPress processes key events with fugitive-style mappings
// Returns true if the overlay should be closed
func (g *GitStatusOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "esc":
		// Exit git mode
		g.Dismissed = true
		if g.OnCancel != nil {
			g.OnCancel()
		}
		return true

	case "?":
		// Toggle help
		g.showHelp = !g.showHelp
		return false

	case "j", "down":
		// Navigate down
		if len(g.Files) > 0 {
			g.selectedIdx = (g.selectedIdx + 1) % len(g.Files)
			g.ensureSelectedVisible()
		}
		return false

	case "k", "up":
		// Navigate up
		if len(g.Files) > 0 {
			g.selectedIdx = (g.selectedIdx - 1 + len(g.Files)) % len(g.Files)
			g.ensureSelectedVisible()
		}
		return false

	case "s":
		// Stage file
		if file := g.GetSelectedFile(); file != nil {
			if g.OnStageFile != nil {
				if err := g.OnStageFile(file.Path); err != nil {
					g.statusMessage = fmt.Sprintf("Error staging %s: %v", file.Path, err)
				} else {
					g.statusMessage = fmt.Sprintf("Staged %s", file.Path)
				}
			}
		}
		return false

	case "u":
		// Unstage file
		if file := g.GetSelectedFile(); file != nil {
			if g.OnUnstageFile != nil {
				if err := g.OnUnstageFile(file.Path); err != nil {
					g.statusMessage = fmt.Sprintf("Error unstaging %s: %v", file.Path, err)
				} else {
					g.statusMessage = fmt.Sprintf("Unstaged %s", file.Path)
				}
			}
		}
		return false

	case "-":
		// Toggle staging for file
		if file := g.GetSelectedFile(); file != nil {
			if g.OnToggleFile != nil {
				if err := g.OnToggleFile(file.Path); err != nil {
					g.statusMessage = fmt.Sprintf("Error toggling %s: %v", file.Path, err)
				} else {
					action := "staged"
					if file.Staged {
						action = "unstaged"
					}
					g.statusMessage = fmt.Sprintf("Toggled %s (%s)", file.Path, action)
				}
			}
		}
		return false

	case "U":
		// Unstage all
		if g.OnUnstageAll != nil {
			if err := g.OnUnstageAll(); err != nil {
				g.statusMessage = fmt.Sprintf("Error unstaging all: %v", err)
			} else {
				g.statusMessage = "Unstaged all files"
			}
		}
		return false

	case "cc":
		// Create commit
		if g.OnCommit != nil {
			if err := g.OnCommit(); err != nil {
				g.statusMessage = fmt.Sprintf("Error creating commit: %v", err)
			} else {
				g.statusMessage = "Creating commit..."
				return true // Close overlay to show commit interface
			}
		}
		return false

	case "ca":
		// Amend commit
		if g.OnCommitAmend != nil {
			if err := g.OnCommitAmend(); err != nil {
				g.statusMessage = fmt.Sprintf("Error amending commit: %v", err)
			} else {
				g.statusMessage = "Amending last commit..."
				return true // Close overlay to show commit interface
			}
		}
		return false

	case "dd":
		// Show diff for file
		if file := g.GetSelectedFile(); file != nil {
			if g.OnShowDiff != nil {
				if err := g.OnShowDiff(file.Path); err != nil {
					g.statusMessage = fmt.Sprintf("Error showing diff: %v", err)
				} else {
					g.statusMessage = fmt.Sprintf("Showing diff for %s", file.Path)
					return true // Close overlay to show diff
				}
			}
		}
		return false

	case "p":
		// Push
		if g.OnPush != nil {
			if err := g.OnPush(); err != nil {
				g.statusMessage = fmt.Sprintf("Error pushing: %v", err)
			} else {
				g.statusMessage = "Pushing changes..."
			}
		}
		return false

	case "P":
		// Pull
		if g.OnPull != nil {
			if err := g.OnPull(); err != nil {
				g.statusMessage = fmt.Sprintf("Error pulling: %v", err)
			} else {
				g.statusMessage = "Pulling changes..."
			}
		}
		return false

	case "enter":
		// Open file for editing
		if file := g.GetSelectedFile(); file != nil {
			if g.OnOpenFile != nil {
				if err := g.OnOpenFile(file.Path); err != nil {
					g.statusMessage = fmt.Sprintf("Error opening %s: %v", file.Path, err)
				} else {
					g.statusMessage = fmt.Sprintf("Opening %s", file.Path)
					return true // Close overlay
				}
			}
		}
		return false

	default:
		return false
	}
}

// ensureSelectedVisible adjusts scroll offset to keep selected item visible
func (g *GitStatusOverlay) ensureSelectedVisible() {
	if len(g.Files) == 0 {
		return
	}

	maxVisible := g.getMaxVisibleItems()

	// If selected is above visible area, scroll up
	if g.selectedIdx < g.scrollOffset {
		g.scrollOffset = g.selectedIdx
	}

	// If selected is below visible area, scroll down
	if g.selectedIdx >= g.scrollOffset+maxVisible {
		g.scrollOffset = g.selectedIdx - maxVisible + 1
		if g.scrollOffset < 0 {
			g.scrollOffset = 0
		}
	}
}

// getMaxVisibleItems calculates how many items can fit in the display
func (g *GitStatusOverlay) getMaxVisibleItems() int {
	// Account for header, help, status line, padding
	availableHeight := g.height - 8
	if g.showHelp {
		availableHeight -= 10 // Extra space for help
	}

	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// Render renders the git status overlay
func (g *GitStatusOverlay) Render() string {
	// Styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(g.width - 4)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true)

	branchStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Bold(true)

	stagedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("40")) // Green

	modifiedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("208")) // Orange

	untrackedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")) // Red

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230"))

	// Build header
	var content strings.Builder
	content.WriteString(titleStyle.Render("Git Status"))
	if g.BranchName != "" {
		content.WriteString(" on ")
		content.WriteString(branchStyle.Render(g.BranchName))
	}
	content.WriteString("\n\n")

	// Show files with scrolling
	if len(g.Files) == 0 {
		content.WriteString("No changes to display\n")
	} else {
		maxVisible := g.getMaxVisibleItems()
		end := g.scrollOffset + maxVisible
		if end > len(g.Files) {
			end = len(g.Files)
		}

		for i := g.scrollOffset; i < end; i++ {
			file := g.Files[i]

			// Determine file status and styling
			var statusStr string
			var fileStyle lipgloss.Style

			if file.Staged {
				statusStr = "●" // Staged
				fileStyle = stagedStyle
			} else if file.Modified {
				statusStr = "M" // Modified
				fileStyle = modifiedStyle
			} else if file.Untracked {
				statusStr = "?" // Untracked
				fileStyle = untrackedStyle
			} else {
				statusStr = " "
				fileStyle = lipgloss.NewStyle()
			}

			line := fmt.Sprintf(" %s %s", statusStr, file.Path)

			// Apply selection highlighting
			if i == g.selectedIdx {
				line = selectedStyle.Render(line)
			} else {
				line = fileStyle.Render(line)
			}

			content.WriteString(line)
			content.WriteString("\n")
		}

		// Show scroll indicator
		if len(g.Files) > maxVisible {
			content.WriteString(fmt.Sprintf("\\n[%d-%d/%d]",
				g.scrollOffset+1, end, len(g.Files)))
		}
	}

	// Status message
	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(g.statusMessage))

	// Help section
	if g.showHelp {
		content.WriteString("\n\n")
		content.WriteString(titleStyle.Render("Keybindings:"))
		content.WriteString("\n")
		helpItems := []string{
			"s - stage file",
			"u - unstage file",
			"- - toggle staging",
			"U - unstage all",
			"cc - commit",
			"ca - amend commit",
			"dd - show diff",
			"p - push",
			"P - pull",
			"↵ - open file",
			"? - toggle help",
			"esc - exit",
		}

		for _, item := range helpItems {
			content.WriteString(fmt.Sprintf("  %s\n", item))
		}
	}

	return style.Render(content.String())
}

// View satisfies the tea.Model interface
func (g *GitStatusOverlay) View() string {
	return g.Render()
}

// SetStatusMessage updates the status message displayed at the bottom
func (g *GitStatusOverlay) SetStatusMessage(msg string) {
	g.statusMessage = msg
}
