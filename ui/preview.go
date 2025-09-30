package ui

import (
	"claude-squad/session"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var previewPaneStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

// previewRequest represents a request to update preview content
type previewRequest struct {
	instance    *session.Instance
	fullHistory bool
}

// previewResult represents the result of a preview update
type previewResult struct {
	content    string
	err        error
	instanceID string // for cache invalidation
}

type PreviewPane struct {
	width  int
	height int

	previewState previewState
	isScrolling  bool
	viewport     viewport.Model

	// Async preview system
	mu                  sync.RWMutex
	previewWorkerCtx    context.Context
	previewWorkerCancel context.CancelFunc
	previewRequestCh    chan previewRequest
	previewResultCh     chan previewResult

	// Content cache
	contentCache   map[string]cachedContent
	lastInstanceID string

	// Debouncing
	debounceTimer   *time.Timer
	pendingInstance *session.Instance
}

type previewState struct {
	// fallback is true if the preview pane is displaying fallback text
	fallback bool
	// text is the text displayed in the preview pane
	text string
}

// cachedContent represents cached preview content with timestamp
type cachedContent struct {
	content   string
	timestamp time.Time
	isValid   bool
}

const (
	// Debounce delay to batch rapid navigation
	previewDebounceDelay = 150 * time.Millisecond
	// Cache TTL for preview content
	previewCacheTTL = 2 * time.Second
)

func NewPreviewPane() *PreviewPane {
	ctx, cancel := context.WithCancel(context.Background())
	p := &PreviewPane{
		viewport:            viewport.New(0, 0),
		previewWorkerCtx:    ctx,
		previewWorkerCancel: cancel,
		previewRequestCh:    make(chan previewRequest, 10),
		previewResultCh:     make(chan previewResult, 10),
		contentCache:        make(map[string]cachedContent),
	}

	// Start background preview worker
	go p.previewWorker()

	return p
}

// Cleanup stops the background worker and cleans up resources
func (p *PreviewPane) Cleanup() {
	if p.previewWorkerCancel != nil {
		p.previewWorkerCancel()
	}
	if p.debounceTimer != nil {
		p.debounceTimer.Stop()
	}
}

func (p *PreviewPane) SetSize(width, maxHeight int) {
	p.width = width
	p.height = maxHeight
	p.viewport.Width = width
	p.viewport.Height = maxHeight
}

// setFallbackState sets the preview state with fallback text and a message
func (p *PreviewPane) setFallbackState(message string) {
	p.previewState = previewState{
		fallback: true,
		text:     lipgloss.JoinVertical(lipgloss.Center, FallBackText, "", message),
	}
}

// previewWorker runs in a background goroutine to handle expensive tmux operations
func (p *PreviewPane) previewWorker() {
	for {
		select {
		case <-p.previewWorkerCtx.Done():
			return
		case req := <-p.previewRequestCh:
			p.processPreviewRequest(req)
		}
	}
}

// processPreviewRequest handles a single preview request asynchronously
func (p *PreviewPane) processPreviewRequest(req previewRequest) {
	if req.instance == nil {
		p.previewResultCh <- previewResult{
			content:    "",
			err:        nil,
			instanceID: "",
		}
		return
	}

	instanceID := p.getInstanceID(req.instance)

	// Check cache first
	if cached, ok := p.getCachedContent(instanceID, req.fullHistory); ok {
		p.previewResultCh <- previewResult{
			content:    cached,
			err:        nil,
			instanceID: instanceID,
		}
		return
	}

	// Perform expensive tmux operation in background
	var content string
	var err error

	if req.fullHistory {
		content, err = req.instance.PreviewFullHistory()
	} else {
		content, err = req.instance.Preview()
	}

	// Cache the result
	p.setCachedContent(instanceID, content, req.fullHistory)

	// Send result back
	p.previewResultCh <- previewResult{
		content:    content,
		err:        err,
		instanceID: instanceID,
	}
}

// getInstanceID generates a cache key for an instance
func (p *PreviewPane) getInstanceID(instance *session.Instance) string {
	if instance == nil {
		return ""
	}
	return fmt.Sprintf("%s-%s", instance.Title, instance.Branch)
}

// getCachedContent retrieves cached content if valid
func (p *PreviewPane) getCachedContent(instanceID string, fullHistory bool) (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := instanceID
	if fullHistory {
		key += "-full"
	}

	cached, ok := p.contentCache[key]
	if !ok || !cached.isValid || time.Since(cached.timestamp) > previewCacheTTL {
		return "", false
	}

	return cached.content, true
}

// setCachedContent stores content in cache
func (p *PreviewPane) setCachedContent(instanceID, content string, fullHistory bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := instanceID
	if fullHistory {
		key += "-full"
	}

	p.contentCache[key] = cachedContent{
		content:   content,
		timestamp: time.Now(),
		isValid:   true,
	}
}

// invalidateCache invalidates cached content for an instance
func (p *PreviewPane) invalidateCache(instanceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.contentCache, instanceID)
	delete(p.contentCache, instanceID+"-full")
}

// UpdateContentAsync requests a preview update asynchronously with debouncing
func (p *PreviewPane) UpdateContentAsync(instance *session.Instance) {
	// Cancel any existing debounce timer
	if p.debounceTimer != nil {
		p.debounceTimer.Stop()
	}

	// Store the pending instance
	p.pendingInstance = instance

	// Set up debounced execution
	p.debounceTimer = time.AfterFunc(previewDebounceDelay, func() {
		p.requestPreviewUpdate(p.pendingInstance, false)
	})
}

// requestPreviewUpdate sends a preview request to the worker
func (p *PreviewPane) requestPreviewUpdate(instance *session.Instance, fullHistory bool) {
	select {
	case p.previewRequestCh <- previewRequest{
		instance:    instance,
		fullHistory: fullHistory,
	}:
	default:
		// Channel is full, skip this request to prevent blocking
	}
}

// ProcessResults processes any pending results from the background worker
func (p *PreviewPane) ProcessResults() error {
	for {
		select {
		case result := <-p.previewResultCh:
			if result.err != nil {
				return result.err
			}

			// Update preview state with the result
			if result.instanceID != "" {
				p.previewState = previewState{
					fallback: false,
					text:     result.content,
				}
				p.lastInstanceID = result.instanceID
			}
		default:
			// No more results to process
			return nil
		}
	}
}

// Updates the preview pane content with the tmux pane content (LEGACY - kept for compatibility)
func (p *PreviewPane) UpdateContent(instance *session.Instance) error {
	switch {
	case instance == nil:
		p.setFallbackState("No agents running yet. Spin up a new instance with 'n' to get started!")
		return nil
	case instance.Status == session.Paused:
		p.setFallbackState(lipgloss.JoinVertical(lipgloss.Center,
			"Session is paused. Press 'r' to resume.",
			"",
			lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{
					Light: "#FFD700",
					Dark:  "#FFD700",
				}).
				Render(fmt.Sprintf(
					"The instance can be checked out at '%s' (copied to your clipboard)",
					instance.Branch,
				)),
		))
		return nil
	}

	var content string
	var err error

	// If in scroll mode but haven't captured content yet, do it now
	if p.isScrolling && p.viewport.Height > 0 && len(p.viewport.View()) == 0 {
		// Capture full pane content including scrollback history using capture-pane -p -S -
		content, err = instance.PreviewFullHistory()
		if err != nil {
			return err
		}

		// Set content in the viewport
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}).
			Render("ESC to exit scroll mode")

		p.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, content, footer))
	} else if !p.isScrolling {
		// In normal mode, use the usual preview
		content, err = instance.Preview()
		if err != nil {
			return err
		}

		// Always update the preview state with content, even if empty
		// This ensures that newly created instances will display their content immediately
		if len(content) == 0 && !instance.Started() {
			p.setFallbackState("Please enter a name for the instance.")
		} else {
			// Update the preview state with the current content
			p.previewState = previewState{
				fallback: false,
				text:     content,
			}
		}
	}

	return nil
}

// Returns the preview pane content as a string.
func (p *PreviewPane) String() string {
	if p.width == 0 || p.height == 0 {
		return strings.Repeat("\n", p.height)
	}

	if p.previewState.fallback {
		// Calculate available height for fallback text
		availableHeight := p.height - 3 - 4 // 2 for borders, 1 for margin, 1 for padding

		// Count the number of lines in the fallback text
		fallbackLines := len(strings.Split(p.previewState.text, "\n"))

		// Calculate padding needed above and below to center the content
		totalPadding := availableHeight - fallbackLines
		topPadding := 0
		bottomPadding := 0
		if totalPadding > 0 {
			topPadding = totalPadding / 2
			bottomPadding = totalPadding - topPadding // accounts for odd numbers
		}

		// Build the centered content
		var lines []string
		if topPadding > 0 {
			lines = append(lines, strings.Repeat("\n", topPadding))
		}
		lines = append(lines, p.previewState.text)
		if bottomPadding > 0 {
			lines = append(lines, strings.Repeat("\n", bottomPadding))
		}

		// Center both vertically and horizontally
		return previewPaneStyle.
			Width(p.width).
			Align(lipgloss.Center).
			Render(strings.Join(lines, ""))
	}

	// If in copy mode, use the viewport to display scrollable content
	if p.isScrolling {
		return p.viewport.View()
	}

	// Normal mode display
	// Calculate available height accounting for border and margin
	availableHeight := p.height - 1 //  1 for ellipsis

	lines := strings.Split(p.previewState.text, "\n")

	// Truncate if we have more lines than available height
	if availableHeight > 0 {
		if len(lines) > availableHeight {
			lines = lines[:availableHeight]
			lines = append(lines, "...")
		} else {
			// Pad with empty lines to fill available height
			padding := availableHeight - len(lines)
			lines = append(lines, make([]string, padding)...)
		}
	}

	content := strings.Join(lines, "\n")
	rendered := previewPaneStyle.Width(p.width).Render(content)
	return rendered
}

// ScrollUp scrolls up in the viewport
func (p *PreviewPane) ScrollUp(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if !p.isScrolling {
		// Entering scroll mode - request full history asynchronously
		p.requestPreviewUpdate(instance, true)

		// Set a loading state while we wait for content
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}).
			Render("ESC to exit scroll mode")

		loadingContent := "Loading full history..."
		contentWithFooter := lipgloss.JoinVertical(lipgloss.Left, loadingContent, footer)
		p.viewport.SetContent(contentWithFooter)

		p.isScrolling = true
		return nil
	}

	// Already in scroll mode, just scroll the viewport
	p.viewport.LineUp(1)
	return nil
}

// ScrollDown scrolls down in the viewport
func (p *PreviewPane) ScrollDown(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if !p.isScrolling {
		// Entering scroll mode - request full history asynchronously
		p.requestPreviewUpdate(instance, true)

		// Set a loading state while we wait for content
		footer := lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#808080", Dark: "#808080"}).
			Render("ESC to exit scroll mode")

		loadingContent := "Loading full history..."
		contentWithFooter := lipgloss.JoinVertical(lipgloss.Left, loadingContent, footer)
		p.viewport.SetContent(contentWithFooter)

		p.isScrolling = true
		return nil
	}

	// Already in scroll mode, just scroll the viewport
	p.viewport.LineDown(1)
	return nil
}

// ResetToNormalMode exits scroll mode and returns to normal mode
func (p *PreviewPane) ResetToNormalMode(instance *session.Instance) error {
	if instance == nil || instance.Status == session.Paused {
		return nil
	}

	if p.isScrolling {
		p.isScrolling = false
		// Reset viewport
		p.viewport.SetContent("")
		p.viewport.GotoTop()

		// Request fresh content asynchronously
		p.requestPreviewUpdate(instance, false)
	}

	return nil
}
