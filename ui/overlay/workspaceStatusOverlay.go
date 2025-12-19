package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"claude-squad/session"
	"claude-squad/session/vc"
)

// WorkspaceInfo represents workspace data for display
type WorkspaceInfo struct {
	Path           string
	RepositoryRoot string
	Branch         string
	SessionTitle   string
	SessionStatus  session.Status
	IsWorktree     bool
	IsOrphaned     bool

	// VCS Status
	StagedCount    int
	ModifiedCount  int
	UntrackedCount int
	ConflictCount  int
	IsClean        bool

	// Timestamps
	LastChecked  time.Time
	LastActivity time.Time

	// Attention
	NeedsAttention  bool
	AttentionReason string
}

// WorkspaceSummary provides aggregated statistics
type WorkspaceSummary struct {
	TotalRepositories  int
	TotalWorkspaces    int
	TotalUncommitted   int
	TotalUntracked     int
	TotalStaged        int
	TotalConflicts     int
	WorkspacesWithWork int
	OrphanedWorkspaces int
}

// WorkspaceStatusOverlay displays workspace status across all sessions.
type WorkspaceStatusOverlay struct {
	BaseOverlay

	// State
	Dismissed bool
	OnDismiss func()

	// Data
	workspaces   []WorkspaceInfo
	summary      WorkspaceSummary
	byRepository map[string][]int // repository root -> indices in workspaces

	// Navigation
	selectedIdx  int
	scrollOffset int

	// View state
	showHelp       bool
	expandedRepos  map[string]bool // which repositories are expanded
	groupByRepo    bool            // group by repository or flat list
	statusMessage  string
	isRefreshing   bool
	lastError      error
	filterOrphaned bool // show only orphaned workspaces

	// Callbacks
	OnNavigateToSession func(sessionTitle string)
	OnRefresh           func()
}

// NewWorkspaceStatusOverlay creates a new workspace status overlay.
func NewWorkspaceStatusOverlay() *WorkspaceStatusOverlay {
	o := &WorkspaceStatusOverlay{
		Dismissed:     false,
		showHelp:      true,
		groupByRepo:   true,
		expandedRepos: make(map[string]bool),
		byRepository:  make(map[string][]int),
	}
	o.BaseOverlay.SetSize(80, 24)
	o.BaseOverlay.Focus()
	return o
}

// SetWorkspaces updates the workspace data.
func (o *WorkspaceStatusOverlay) SetWorkspaces(workspaces []WorkspaceInfo, summary WorkspaceSummary) {
	o.workspaces = workspaces
	o.summary = summary

	// Build repository index
	o.byRepository = make(map[string][]int)
	for i, ws := range workspaces {
		repoRoot := ws.RepositoryRoot
		if repoRoot == "" {
			repoRoot = ws.Path
		}
		o.byRepository[repoRoot] = append(o.byRepository[repoRoot], i)

		// Auto-expand repositories with issues
		if ws.NeedsAttention || ws.ConflictCount > 0 {
			o.expandedRepos[repoRoot] = true
		}
	}

	o.isRefreshing = false
}

// SetStatusMessage sets a status message to display.
func (o *WorkspaceStatusOverlay) SetStatusMessage(msg string) {
	o.statusMessage = msg
}

// SetRefreshing sets the refreshing state.
func (o *WorkspaceStatusOverlay) SetRefreshing(refreshing bool) {
	o.isRefreshing = refreshing
}

// SetSize updates the overlay dimensions.
func (o *WorkspaceStatusOverlay) SetSize(width, height int) {
	o.BaseOverlay.SetSize(width, height)
}

// HandleKeyPress handles key input.
func (o *WorkspaceStatusOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	// Handle common keys (Esc)
	if handled, shouldClose := o.BaseOverlay.HandleCommonKeys(msg); handled {
		if shouldClose {
			o.Dismissed = true
			if o.OnDismiss != nil {
				o.OnDismiss()
			}
			return true
		}
	}

	switch msg.String() {
	case "q":
		o.Dismissed = true
		if o.OnDismiss != nil {
			o.OnDismiss()
		}
		return true

	case "j", "down":
		o.navigateDown()

	case "k", "up":
		o.navigateUp()

	case "g", "home":
		o.selectedIdx = 0
		o.scrollOffset = 0

	case "G", "end":
		o.selectedIdx = len(o.workspaces) - 1
		o.ensureSelectedVisible()

	case "enter", " ":
		if o.groupByRepo {
			// Toggle expand/collapse for repository
			if ws := o.getSelectedWorkspace(); ws != nil {
				repoRoot := ws.RepositoryRoot
				if repoRoot == "" {
					repoRoot = ws.Path
				}
				o.expandedRepos[repoRoot] = !o.expandedRepos[repoRoot]
			}
		} else {
			// Navigate to session
			if ws := o.getSelectedWorkspace(); ws != nil && ws.SessionTitle != "" {
				if o.OnNavigateToSession != nil {
					o.OnNavigateToSession(ws.SessionTitle)
				}
				o.Dismissed = true
				if o.OnDismiss != nil {
					o.OnDismiss()
				}
				return true
			}
		}

	case "r":
		if o.OnRefresh != nil {
			o.isRefreshing = true
			o.statusMessage = "Refreshing..."
			o.OnRefresh()
		}

	case "?":
		o.showHelp = !o.showHelp

	case "o":
		o.filterOrphaned = !o.filterOrphaned
		o.selectedIdx = 0
		o.scrollOffset = 0

	case "tab":
		o.groupByRepo = !o.groupByRepo
		o.selectedIdx = 0
		o.scrollOffset = 0
	}

	return false
}

// View renders the overlay.
func (o *WorkspaceStatusOverlay) View() string {
	return o.Render()
}

// Render generates the overlay content.
func (o *WorkspaceStatusOverlay) Render() string {
	width := o.GetWidth()
	height := o.GetHeight()

	// Styles
	borderColor := lipgloss.Color("62")
	headerColor := lipgloss.Color("39")
	dimColor := lipgloss.Color("240")
	yellowColor := lipgloss.Color("208")
	redColor := lipgloss.Color("196")

	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width - 2).
		Height(height - 2)

	headerStyle := lipgloss.NewStyle().
		Foreground(headerColor).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(dimColor)

	// Build content
	var content strings.Builder

	// Title
	title := "Workspace Status"
	if o.isRefreshing {
		title += " ⟳"
	}
	content.WriteString(headerStyle.Render(title))
	content.WriteString("\n")

	// Summary line
	summaryParts := []string{
		fmt.Sprintf("%d repos", o.summary.TotalRepositories),
		fmt.Sprintf("%d workspaces", o.summary.TotalWorkspaces),
	}

	if o.summary.TotalUncommitted > 0 {
		summaryParts = append(summaryParts,
			lipgloss.NewStyle().Foreground(yellowColor).Render(
				fmt.Sprintf("%d uncommitted", o.summary.TotalUncommitted)))
	}

	if o.summary.TotalUntracked > 0 {
		summaryParts = append(summaryParts,
			lipgloss.NewStyle().Foreground(dimColor).Render(
				fmt.Sprintf("%d untracked", o.summary.TotalUntracked)))
	}

	if o.summary.TotalConflicts > 0 {
		summaryParts = append(summaryParts,
			lipgloss.NewStyle().Foreground(redColor).Render(
				fmt.Sprintf("%d conflicts", o.summary.TotalConflicts)))
	}

	if o.summary.OrphanedWorkspaces > 0 {
		summaryParts = append(summaryParts,
			lipgloss.NewStyle().Foreground(yellowColor).Render(
				fmt.Sprintf("%d orphaned", o.summary.OrphanedWorkspaces)))
	}

	content.WriteString(dimStyle.Render(strings.Join(summaryParts, " | ")))
	content.WriteString("\n")
	content.WriteString(strings.Repeat("─", width-6))
	content.WriteString("\n\n")

	// Workspace list
	visibleHeight := height - 10 // Account for header, footer, borders
	if o.showHelp {
		visibleHeight -= 4
	}

	startIdx := o.scrollOffset
	endIdx := startIdx + visibleHeight
	if endIdx > len(o.workspaces) {
		endIdx = len(o.workspaces)
	}

	if len(o.workspaces) == 0 {
		content.WriteString(dimStyle.Render("  No workspaces tracked"))
		content.WriteString("\n")
	} else {
		for i := startIdx; i < endIdx; i++ {
			ws := o.workspaces[i]

			// Skip orphaned if filter is off
			if o.filterOrphaned && !ws.IsOrphaned {
				continue
			}

			isSelected := i == o.selectedIdx

			line := o.renderWorkspaceLine(ws, width-8, isSelected)
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	// Status message
	if o.statusMessage != "" {
		content.WriteString("\n")
		content.WriteString(dimStyle.Render(o.statusMessage))
	}

	// Help footer
	if o.showHelp {
		content.WriteString("\n")
		content.WriteString(strings.Repeat("─", width-6))
		content.WriteString("\n")

		helpLines := []string{
			"[j/k] Navigate  [Enter] Expand/Go  [r] Refresh  [o] Orphaned",
			"[Tab] Toggle grouping  [?] Toggle help  [Esc/q] Close",
		}

		for _, line := range helpLines {
			content.WriteString(dimStyle.Render(line))
			content.WriteString("\n")
		}
	}

	return containerStyle.Render(content.String())
}

// renderWorkspaceLine renders a single workspace line.
func (o *WorkspaceStatusOverlay) renderWorkspaceLine(ws WorkspaceInfo, maxWidth int, selected bool) string {
	// Colors
	greenColor := lipgloss.Color("40")
	yellowColor := lipgloss.Color("208")
	redColor := lipgloss.Color("196")
	dimColor := lipgloss.Color("240")
	selectedBg := lipgloss.Color("62")
	selectedFg := lipgloss.Color("230")

	var parts []string

	// Selection indicator
	if selected {
		parts = append(parts, "▶")
	} else {
		parts = append(parts, " ")
	}

	// Worktree indicator
	if ws.IsWorktree {
		parts = append(parts, "├─")
	} else {
		parts = append(parts, "  ")
	}

	// Status indicator
	var statusIcon string
	var statusColor lipgloss.Color

	if ws.ConflictCount > 0 {
		statusIcon = "✗"
		statusColor = redColor
	} else if ws.IsClean {
		statusIcon = "✓"
		statusColor = greenColor
	} else if ws.NeedsAttention {
		statusIcon = "⚠"
		statusColor = yellowColor
	} else if ws.ModifiedCount > 0 || ws.StagedCount > 0 {
		statusIcon = "●"
		statusColor = yellowColor
	} else if ws.UntrackedCount > 0 {
		statusIcon = "○"
		statusColor = dimColor
	} else {
		statusIcon = " "
		statusColor = dimColor
	}

	parts = append(parts, lipgloss.NewStyle().Foreground(statusColor).Render(statusIcon))

	// Path (abbreviated)
	displayPath := abbreviatePath(ws.Path, 30)
	parts = append(parts, displayPath)

	// Branch
	if ws.Branch != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(dimColor).Render("("+ws.Branch+")"))
	}

	// Change counts
	var changeParts []string
	if ws.StagedCount > 0 {
		changeParts = append(changeParts,
			lipgloss.NewStyle().Foreground(greenColor).Render(fmt.Sprintf("+%d", ws.StagedCount)))
	}
	if ws.ModifiedCount > 0 {
		changeParts = append(changeParts,
			lipgloss.NewStyle().Foreground(yellowColor).Render(fmt.Sprintf("~%d", ws.ModifiedCount)))
	}
	if ws.UntrackedCount > 0 {
		changeParts = append(changeParts,
			lipgloss.NewStyle().Foreground(dimColor).Render(fmt.Sprintf("?%d", ws.UntrackedCount)))
	}
	if ws.ConflictCount > 0 {
		changeParts = append(changeParts,
			lipgloss.NewStyle().Foreground(redColor).Render(fmt.Sprintf("!%d", ws.ConflictCount)))
	}

	if len(changeParts) > 0 {
		parts = append(parts, strings.Join(changeParts, " "))
	}

	// Session info
	if ws.SessionTitle != "" {
		sessionInfo := fmt.Sprintf("[%s]", ws.SessionTitle)
		switch ws.SessionStatus {
		case session.Running:
			sessionInfo = lipgloss.NewStyle().Foreground(greenColor).Render("●") + " " + sessionInfo
		case session.Paused:
			sessionInfo = lipgloss.NewStyle().Foreground(dimColor).Render("⏸") + " " + sessionInfo
		default:
			sessionInfo = lipgloss.NewStyle().Foreground(dimColor).Render("○") + " " + sessionInfo
		}
		parts = append(parts, sessionInfo)
	} else if ws.IsOrphaned {
		parts = append(parts, lipgloss.NewStyle().Foreground(yellowColor).Render("(orphaned)"))
	}

	line := strings.Join(parts, " ")

	// Apply selection highlighting
	if selected {
		lineStyle := lipgloss.NewStyle().
			Background(selectedBg).
			Foreground(selectedFg).
			Width(maxWidth)
		line = lineStyle.Render(line)
	}

	return line
}

// Navigation helpers

func (o *WorkspaceStatusOverlay) navigateDown() {
	if o.selectedIdx < len(o.workspaces)-1 {
		o.selectedIdx++
		o.ensureSelectedVisible()
	}
}

func (o *WorkspaceStatusOverlay) navigateUp() {
	if o.selectedIdx > 0 {
		o.selectedIdx--
		o.ensureSelectedVisible()
	}
}

func (o *WorkspaceStatusOverlay) ensureSelectedVisible() {
	visibleHeight := o.GetHeight() - 10
	if o.showHelp {
		visibleHeight -= 4
	}

	if o.selectedIdx < o.scrollOffset {
		o.scrollOffset = o.selectedIdx
	} else if o.selectedIdx >= o.scrollOffset+visibleHeight {
		o.scrollOffset = o.selectedIdx - visibleHeight + 1
	}
}

func (o *WorkspaceStatusOverlay) getSelectedWorkspace() *WorkspaceInfo {
	if o.selectedIdx < 0 || o.selectedIdx >= len(o.workspaces) {
		return nil
	}
	return &o.workspaces[o.selectedIdx]
}

// Helper functions

func abbreviatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	// Try to show last component
	parts := strings.Split(path, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		if len(lastPart) <= maxLen-3 {
			return "..." + lastPart
		}
	}

	return path[:maxLen-3] + "..."
}

// FromVCSStatus converts VCS status to WorkspaceInfo.
func WorkspaceInfoFromVCS(path string, status *vc.VCSStatus, sessionTitle string, sessionStatus session.Status, isWorktree bool) WorkspaceInfo {
	info := WorkspaceInfo{
		Path:          path,
		Branch:        status.Branch,
		SessionTitle:  sessionTitle,
		SessionStatus: sessionStatus,
		IsWorktree:    isWorktree,
		IsClean:       status.IsClean,
	}

	info.StagedCount = len(status.StagedFiles)
	info.ModifiedCount = len(status.UnstagedFiles)
	info.UntrackedCount = len(status.UntrackedFiles)
	info.ConflictCount = len(status.ConflictFiles)

	if status.HasConflicts {
		info.NeedsAttention = true
		info.AttentionReason = "Merge conflicts"
	}

	return info
}
