package ui

import (
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/session/vc"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var (
	vcTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("62"))

	vcBranchStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	vcStagedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("40")) // Green

	vcUnstagedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208")) // Orange

	vcUntrackedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196")) // Red

	vcConflictStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	vcSelectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("62")).
			Foreground(lipgloss.Color("230"))

	vcHeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Bold(true)

	vcHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// vcRequest represents a request to update VC status
type vcRequest struct {
	instance *session.Instance
}

// vcResult represents the result of a VC status update
type vcResult struct {
	status     *vc.VCSStatus
	provider   vc.VCSProvider
	err        error
	instanceID string
}

// VCPane displays version control status for a session
type VCPane struct {
	width  int
	height int

	// Current state
	status      *vc.VCSStatus
	provider    vc.VCSProvider
	selectedIdx int
	scrollOffset int

	// Async update system
	mu             sync.RWMutex
	vcWorkerCtx    context.Context
	vcWorkerCancel context.CancelFunc
	vcRequestCh    chan vcRequest
	vcResultCh     chan vcResult

	// Content cache
	contentCache   map[string]cachedVCContent
	lastInstanceID string

	// Debouncing
	debounceTimer   *time.Timer
	pendingInstance *session.Instance

	// Viewport for scrolling long file lists
	viewport viewport.Model

	// Show help
	showHelp bool

	// Command palette
	commandPalette *VCCommandPalette

	// Callbacks
	OnOpenTerminal func(workDir string, cmd string) error
	OnShowFileDiff func(path string) error
	OnStageFile    func(path string) error
	OnUnstageFile  func(path string) error
	OnCommit       func(message string) error
}

type cachedVCContent struct {
	status    *vc.VCSStatus
	provider  vc.VCSProvider
	timestamp time.Time
	isValid   bool
}

const (
	vcDebounceDelay = 150 * time.Millisecond
	vcCacheTTL      = 2 * time.Second
)

// NewVCPane creates a new VC pane
func NewVCPane() *VCPane {
	ctx, cancel := context.WithCancel(context.Background())
	p := &VCPane{
		viewport:       viewport.New(0, 0),
		vcWorkerCtx:    ctx,
		vcWorkerCancel: cancel,
		vcRequestCh:    make(chan vcRequest, 10),
		vcResultCh:     make(chan vcResult, 10),
		contentCache:   make(map[string]cachedVCContent),
		showHelp:       true,
	}

	// Start background worker
	go p.vcWorker()

	return p
}

// Cleanup stops the background worker
func (v *VCPane) Cleanup() {
	if v.vcWorkerCancel != nil {
		v.vcWorkerCancel()
	}
	if v.debounceTimer != nil {
		v.debounceTimer.Stop()
	}
}

// SetSize sets the dimensions of the pane
func (v *VCPane) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.Width = width
	v.viewport.Height = height - 10 // Reserve space for header and help
}

// vcWorker runs in background to handle VCS operations
func (v *VCPane) vcWorker() {
	for {
		select {
		case <-v.vcWorkerCtx.Done():
			return
		case req := <-v.vcRequestCh:
			v.processVCRequest(req)
		}
	}
}

// processVCRequest handles a single VC status request
func (v *VCPane) processVCRequest(req vcRequest) {
	instanceName := "nil"
	if req.instance != nil {
		instanceName = req.instance.Title
	}
	log.DebugLog.Printf("[VC] Worker processing request for instance: %s", instanceName)

	if req.instance == nil {
		v.vcResultCh <- vcResult{
			status:     nil,
			provider:   nil,
			err:        nil,
			instanceID: "",
		}
		return
	}

	instanceID := v.getInstanceID(req.instance)

	// Check cache
	if cached, ok := v.getCachedContent(instanceID); ok {
		log.DebugLog.Printf("[VC] Cache hit for instance: %s", instanceName)
		v.vcResultCh <- vcResult{
			status:     cached.status,
			provider:   cached.provider,
			err:        nil,
			instanceID: instanceID,
		}
		return
	}

	log.DebugLog.Printf("[VC] Cache miss for instance: %s, fetching VCS status", instanceName)

	// Get working directory
	workDir := req.instance.GetWorkingDirectory()
	if workDir == "" {
		v.vcResultCh <- vcResult{
			status:     nil,
			provider:   nil,
			err:        fmt.Errorf("no working directory for instance"),
			instanceID: instanceID,
		}
		return
	}

	// Create VCS provider
	provider, err := vc.NewProvider(workDir)
	if err != nil {
		// No VCS found - not an error, just no VCS in this directory
		log.DebugLog.Printf("[VC] No VCS found for instance: %s in %s", instanceName, workDir)
		v.vcResultCh <- vcResult{
			status:     nil,
			provider:   nil,
			err:        nil,
			instanceID: instanceID,
		}
		return
	}

	// Get status
	status, err := provider.GetStatus()
	if err != nil {
		log.ErrorLog.Printf("[VC] Error getting status for instance: %s: %v", instanceName, err)
		v.vcResultCh <- vcResult{
			status:     nil,
			provider:   provider,
			err:        err,
			instanceID: instanceID,
		}
		return
	}

	// Cache the result
	v.setCachedContent(instanceID, status, provider)

	v.vcResultCh <- vcResult{
		status:     status,
		provider:   provider,
		err:        nil,
		instanceID: instanceID,
	}
}

// getInstanceID generates a cache key for an instance
func (v *VCPane) getInstanceID(instance *session.Instance) string {
	if instance == nil {
		return ""
	}
	return fmt.Sprintf("%s-%s", instance.Title, instance.Branch)
}

// getCachedContent retrieves cached content if valid
func (v *VCPane) getCachedContent(instanceID string) (*cachedVCContent, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	cached, ok := v.contentCache[instanceID]
	if !ok || !cached.isValid || time.Since(cached.timestamp) > vcCacheTTL {
		return nil, false
	}

	return &cached, true
}

// setCachedContent stores content in cache
func (v *VCPane) setCachedContent(instanceID string, status *vc.VCSStatus, provider vc.VCSProvider) {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.contentCache[instanceID] = cachedVCContent{
		status:    status,
		provider:  provider,
		timestamp: time.Now(),
		isValid:   true,
	}
}

// InvalidateCache invalidates cached content for an instance
func (v *VCPane) InvalidateCache(instanceID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.contentCache, instanceID)
}

// UpdateVCAsync requests a VC status update asynchronously with debouncing
func (v *VCPane) UpdateVCAsync(instance *session.Instance) {
	instanceName := "nil"
	if instance != nil {
		instanceName = instance.Title
	}
	log.DebugLog.Printf("[VC] UpdateVCAsync called for instance: %s", instanceName)

	// Cancel any existing debounce timer
	if v.debounceTimer != nil {
		v.debounceTimer.Stop()
	}

	// Store the pending instance
	v.pendingInstance = instance

	// Set up debounced execution
	v.debounceTimer = time.AfterFunc(vcDebounceDelay, func() {
		log.DebugLog.Printf("[VC] Debounce timer fired for instance: %s", instanceName)
		v.requestVCUpdate(v.pendingInstance)
	})
}

// requestVCUpdate sends a request to the worker
func (v *VCPane) requestVCUpdate(instance *session.Instance) {
	instanceName := "nil"
	if instance != nil {
		instanceName = instance.Title
	}

	select {
	case v.vcRequestCh <- vcRequest{instance: instance}:
		log.DebugLog.Printf("[VC] Request queued for instance: %s", instanceName)
	default:
		log.WarningLog.Printf("[VC] Channel full, dropping request for instance: %s", instanceName)
	}
}

// ProcessResults processes pending results from the background worker
func (v *VCPane) ProcessResults() error {
	resultCount := 0
	for {
		select {
		case result := <-v.vcResultCh:
			resultCount++
			log.DebugLog.Printf("[VC] ProcessResults: received result #%d, instanceID: '%s'",
				resultCount, result.instanceID)

			if result.err != nil {
				log.ErrorLog.Printf("[VC] ProcessResults: error in result: %v", result.err)
				// Don't return error, just log it - allows UI to show "no VCS"
			}

			v.status = result.status
			v.provider = result.provider
			v.lastInstanceID = result.instanceID

			// Reset selection if out of bounds
			if v.status != nil {
				totalFiles := v.status.TotalChanges()
				if v.selectedIdx >= totalFiles {
					v.selectedIdx = 0
				}
			}
		default:
			return nil
		}
	}
}

// GetSelectedFile returns the currently selected file
func (v *VCPane) GetSelectedFile() *vc.FileChange {
	if v.status == nil {
		return nil
	}

	files := v.status.AllChangedFiles()
	if v.selectedIdx < 0 || v.selectedIdx >= len(files) {
		return nil
	}

	return &files[v.selectedIdx]
}

// SelectNext moves selection down
func (v *VCPane) SelectNext() {
	if v.status == nil {
		return
	}
	totalFiles := v.status.TotalChanges()
	if totalFiles > 0 {
		v.selectedIdx = (v.selectedIdx + 1) % totalFiles
		v.ensureSelectedVisible()
	}
}

// SelectPrev moves selection up
func (v *VCPane) SelectPrev() {
	if v.status == nil {
		return
	}
	totalFiles := v.status.TotalChanges()
	if totalFiles > 0 {
		v.selectedIdx = (v.selectedIdx - 1 + totalFiles) % totalFiles
		v.ensureSelectedVisible()
	}
}

// ensureSelectedVisible adjusts scroll offset to keep selected item visible
func (v *VCPane) ensureSelectedVisible() {
	maxVisible := v.getMaxVisibleItems()

	if v.selectedIdx < v.scrollOffset {
		v.scrollOffset = v.selectedIdx
	}

	if v.selectedIdx >= v.scrollOffset+maxVisible {
		v.scrollOffset = v.selectedIdx - maxVisible + 1
		if v.scrollOffset < 0 {
			v.scrollOffset = 0
		}
	}
}

// getMaxVisibleItems calculates how many items can fit
func (v *VCPane) getMaxVisibleItems() int {
	// Account for header, help, section headers
	availableHeight := v.height - 12
	if v.showHelp {
		availableHeight -= 6
	}
	if availableHeight < 1 {
		return 1
	}
	return availableHeight
}

// ScrollUp scrolls the file list up
func (v *VCPane) ScrollUp() {
	v.SelectPrev()
}

// ScrollDown scrolls the file list down
func (v *VCPane) ScrollDown() {
	v.SelectNext()
}

// ToggleHelp toggles help visibility
func (v *VCPane) ToggleHelp() {
	v.showHelp = !v.showHelp
}

// StageSelected stages the selected file
func (v *VCPane) StageSelected() error {
	file := v.GetSelectedFile()
	if file == nil || v.provider == nil {
		return nil
	}
	err := v.provider.Stage(file.Path)
	if err == nil {
		// Invalidate cache to refresh
		v.InvalidateCache(v.lastInstanceID)
	}
	return err
}

// UnstageSelected unstages the selected file
func (v *VCPane) UnstageSelected() error {
	file := v.GetSelectedFile()
	if file == nil || v.provider == nil {
		return nil
	}
	err := v.provider.Unstage(file.Path)
	if err == nil {
		v.InvalidateCache(v.lastInstanceID)
	}
	return err
}

// StageAll stages all files
func (v *VCPane) StageAll() error {
	if v.provider == nil {
		return nil
	}
	err := v.provider.StageAll()
	if err == nil {
		v.InvalidateCache(v.lastInstanceID)
	}
	return err
}

// UnstageAll unstages all files
func (v *VCPane) UnstageAll() error {
	if v.provider == nil {
		return nil
	}
	err := v.provider.UnstageAll()
	if err == nil {
		v.InvalidateCache(v.lastInstanceID)
	}
	return err
}

// GetProvider returns the current VCS provider
func (v *VCPane) GetProvider() vc.VCSProvider {
	return v.provider
}

// StageSelectedFile stages the selected file and triggers refresh
func (v *VCPane) StageSelectedFile(instance *session.Instance) error {
	if err := v.StageSelected(); err != nil {
		return err
	}
	// Trigger async refresh to update status
	v.UpdateVCAsync(instance)
	return nil
}

// UnstageSelectedFile unstages the selected file and triggers refresh
func (v *VCPane) UnstageSelectedFile(instance *session.Instance) error {
	if err := v.UnstageSelected(); err != nil {
		return err
	}
	v.UpdateVCAsync(instance)
	return nil
}

// StageAll stages all files - wrapper to match handler signature
func (v *VCPane) StageAllFiles(instance *session.Instance) error {
	if err := v.StageAll(); err != nil {
		return err
	}
	v.UpdateVCAsync(instance)
	return nil
}

// UnstageAllFiles unstages all files - wrapper to match handler signature
func (v *VCPane) UnstageAllFiles(instance *session.Instance) error {
	if err := v.UnstageAll(); err != nil {
		return err
	}
	v.UpdateVCAsync(instance)
	return nil
}

// GetInteractiveCommand returns the appropriate interactive command for the VCS
func (v *VCPane) GetInteractiveCommand(instance *session.Instance) string {
	if v.provider != nil {
		return v.provider.GetInteractiveCommand()
	}
	// Default: return empty to let caller decide
	return ""
}

// GetStatus returns the current VCS status
func (v *VCPane) GetStatus() *vc.VCSStatus {
	return v.status
}

// String renders the pane content
func (v *VCPane) String() string {
	if v.width == 0 || v.height == 0 {
		return ""
	}

	var content strings.Builder

	// Handle no VCS case
	if v.status == nil {
		return v.renderNoVCS()
	}

	// Header
	content.WriteString(vcTitleStyle.Render("VC Status"))
	content.WriteString(" ")
	content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("(" + v.status.Type.String() + ")"))
	content.WriteString("\n")

	// Branch info
	if v.status.Branch != "" {
		content.WriteString("Branch: ")
		content.WriteString(vcBranchStyle.Render(v.status.Branch))
		if aheadBehind := v.status.AheadBehindString(); aheadBehind != "" {
			content.WriteString(" ")
			content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(aheadBehind))
		}
		content.WriteString("\n")
	}

	// Head commit
	if v.status.HeadCommit != "" {
		content.WriteString("HEAD: ")
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(v.status.HeadCommit))
		if v.status.Description != "" {
			desc := v.status.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			content.WriteString(" ")
			content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render(desc))
		}
		content.WriteString("\n")
	}

	content.WriteString("\n")

	// File sections
	if v.status.IsClean {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("40")).Render("Working directory clean"))
		content.WriteString("\n")
	} else {
		fileIndex := 0

		// Conflicts
		if len(v.status.ConflictFiles) > 0 {
			content.WriteString(vcHeaderStyle.Render("Conflicts:"))
			content.WriteString("\n")
			for _, file := range v.status.ConflictFiles {
				content.WriteString(v.renderFileLine(file, fileIndex))
				content.WriteString("\n")
				fileIndex++
			}
			content.WriteString("\n")
		}

		// Staged
		if len(v.status.StagedFiles) > 0 {
			content.WriteString(vcHeaderStyle.Render("Staged:"))
			content.WriteString("\n")
			for _, file := range v.status.StagedFiles {
				content.WriteString(v.renderFileLine(file, fileIndex))
				content.WriteString("\n")
				fileIndex++
			}
			content.WriteString("\n")
		}

		// Unstaged
		if len(v.status.UnstagedFiles) > 0 {
			content.WriteString(vcHeaderStyle.Render("Unstaged:"))
			content.WriteString("\n")
			for _, file := range v.status.UnstagedFiles {
				content.WriteString(v.renderFileLine(file, fileIndex))
				content.WriteString("\n")
				fileIndex++
			}
			content.WriteString("\n")
		}

		// Untracked
		if len(v.status.UntrackedFiles) > 0 {
			content.WriteString(vcHeaderStyle.Render("Untracked:"))
			content.WriteString("\n")
			for _, file := range v.status.UntrackedFiles {
				content.WriteString(v.renderFileLine(file, fileIndex))
				content.WriteString("\n")
				fileIndex++
			}
		}
	}

	// Help section
	if v.showHelp {
		content.WriteString("\n")
		content.WriteString(v.renderHelp())
	}

	return content.String()
}

// renderFileLine renders a single file line with selection highlighting
func (v *VCPane) renderFileLine(file vc.FileChange, index int) string {
	var style lipgloss.Style

	// Determine base style based on file status
	switch {
	case file.Status == vc.FileConflict:
		style = vcConflictStyle
	case file.IsStaged:
		style = vcStagedStyle
	case file.Status == vc.FileUntracked:
		style = vcUntrackedStyle
	default:
		style = vcUnstagedStyle
	}

	// Build the line
	prefix := "  "
	if index == v.selectedIdx {
		prefix = "> "
	}

	statusChar := file.Status.String()
	if file.IsStaged {
		statusChar = "●"
	}

	line := fmt.Sprintf("%s%s %s", prefix, statusChar, file.Path)

	// Apply selection highlighting
	if index == v.selectedIdx {
		return vcSelectedStyle.Render(line)
	}

	return style.Render(line)
}

// renderNoVCS renders the no-VCS fallback view
func (v *VCPane) renderNoVCS() string {
	var content strings.Builder

	content.WriteString(vcTitleStyle.Render("VC Status"))
	content.WriteString("\n\n")

	content.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Render("No version control system detected.\n"))
	content.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Render("This session is not in a Git or Jujutsu repository.\n"))

	return content.String()
}

// renderHelp renders the help section
func (v *VCPane) renderHelp() string {
	var help strings.Builder

	help.WriteString(vcHelpStyle.Render("───────────────────────────────────"))
	help.WriteString("\n")

	helpItems := []string{
		"j/k - navigate",
		"s - stage",
		"u - unstage",
		"S - stage all",
		"U - unstage all",
		"t - terminal",
		": - commands",
		"? - toggle help",
	}

	help.WriteString(vcHelpStyle.Render(strings.Join(helpItems, " | ")))

	return help.String()
}

// Command Palette Integration

// ShowCommandPalette shows the command palette overlay
func (v *VCPane) ShowCommandPalette() {
	if v.commandPalette == nil {
		v.commandPalette = NewVCCommandPalette()
	}

	// Set VCS type for appropriate commands
	if v.status != nil {
		v.commandPalette.SetVCSType(v.status.Type)
	}

	v.commandPalette.SetSize(v.width-4, v.height-4)
	v.commandPalette.Show()
}

// HideCommandPalette hides the command palette
func (v *VCPane) HideCommandPalette() {
	if v.commandPalette != nil {
		v.commandPalette.Hide()
	}
}

// IsCommandPaletteVisible returns true if the command palette is showing
func (v *VCPane) IsCommandPaletteVisible() bool {
	return v.commandPalette != nil && v.commandPalette.IsVisible()
}

// GetCommandPalette returns the command palette for external handling
func (v *VCPane) GetCommandPalette() *VCCommandPalette {
	return v.commandPalette
}

// SetCommandPaletteCallbacks sets the command execution callbacks
func (v *VCPane) SetCommandPaletteCallbacks(onSelect func(cmd VCCommand), onCancel func()) {
	if v.commandPalette == nil {
		v.commandPalette = NewVCCommandPalette()
	}
	v.commandPalette.OnSelect = onSelect
	v.commandPalette.OnCancel = onCancel
}
