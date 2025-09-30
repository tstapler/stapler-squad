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

var (
	AdditionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	DeletionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	HunkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#0ea5e9"))
)

// diffRequest represents a request to update diff content
type diffRequest struct {
	instance *session.Instance
}

// diffResult represents the result of a diff update
type diffResult struct {
	stats      string
	diff       string
	err        error
	instanceID string
}

type DiffPane struct {
	viewport viewport.Model
	diff     string
	stats    string
	width    int
	height   int

	// Async diff system
	mu               sync.RWMutex
	diffWorkerCtx    context.Context
	diffWorkerCancel context.CancelFunc
	diffRequestCh    chan diffRequest
	diffResultCh     chan diffResult

	// Content cache
	diffCache      map[string]cachedDiffContent
	lastInstanceID string

	// Debouncing
	debounceTimer   *time.Timer
	pendingInstance *session.Instance
}

// cachedDiffContent represents cached diff content with timestamp
type cachedDiffContent struct {
	stats     string
	diff      string
	timestamp time.Time
	isValid   bool
}

const (
	// Debounce delay to batch rapid navigation
	diffDebounceDelay = 150 * time.Millisecond
	// Cache TTL for diff content
	diffCacheTTL = 3 * time.Second
)

func NewDiffPane() *DiffPane {
	ctx, cancel := context.WithCancel(context.Background())
	d := &DiffPane{
		viewport:         viewport.New(0, 0),
		diffWorkerCtx:    ctx,
		diffWorkerCancel: cancel,
		diffRequestCh:    make(chan diffRequest, 10),
		diffResultCh:     make(chan diffResult, 10),
		diffCache:        make(map[string]cachedDiffContent),
	}

	// Start background diff worker
	go d.diffWorker()

	return d
}

func (d *DiffPane) SetSize(width, height int) {
	d.width = width
	d.height = height
	d.viewport.Width = width
	d.viewport.Height = height
	// Update viewport content if diff exists
	if d.diff != "" || d.stats != "" {
		d.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, d.stats, d.diff))
	}
}

func (d *DiffPane) SetDiff(instance *session.Instance) {
	centeredFallbackMessage := lipgloss.Place(
		d.width,
		d.height,
		lipgloss.Center,
		lipgloss.Center,
		"No changes",
	)

	if instance == nil || !instance.Started() {
		d.viewport.SetContent(centeredFallbackMessage)
		return
	}

	stats := instance.GetDiffStats()
	if stats == nil {
		// Show loading message if worktree is not ready
		centeredMessage := lipgloss.Place(
			d.width,
			d.height,
			lipgloss.Center,
			lipgloss.Center,
			"Setting up worktree...",
		)
		d.viewport.SetContent(centeredMessage)
		return
	}

	if stats.Error != nil {
		// Show error message
		centeredMessage := lipgloss.Place(
			d.width,
			d.height,
			lipgloss.Center,
			lipgloss.Center,
			fmt.Sprintf("Error: %v", stats.Error),
		)
		d.viewport.SetContent(centeredMessage)
		return
	}

	if stats.IsEmpty() {
		d.stats = ""
		d.diff = ""
		d.viewport.SetContent(centeredFallbackMessage)
	} else {
		additions := AdditionStyle.Render(fmt.Sprintf("%d additions(+)", stats.Added))
		deletions := DeletionStyle.Render(fmt.Sprintf("%d deletions(-)", stats.Removed))
		d.stats = lipgloss.JoinHorizontal(lipgloss.Center, additions, " ", deletions)
		d.diff = colorizeDiff(stats.Content)
		d.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, d.stats, d.diff))
	}
}

func (d *DiffPane) String() string {
	return d.viewport.View()
}

// ScrollUp scrolls the viewport up
func (d *DiffPane) ScrollUp() {
	d.viewport.LineUp(1)
}

// ScrollDown scrolls the viewport down
func (d *DiffPane) ScrollDown() {
	d.viewport.LineDown(1)
}

// Cleanup stops the background worker and cleans up resources
func (d *DiffPane) Cleanup() {
	if d.diffWorkerCancel != nil {
		d.diffWorkerCancel()
	}
	if d.debounceTimer != nil {
		d.debounceTimer.Stop()
	}
}

// diffWorker runs in a background goroutine to handle expensive git operations
func (d *DiffPane) diffWorker() {
	for {
		select {
		case <-d.diffWorkerCtx.Done():
			return
		case req := <-d.diffRequestCh:
			d.processDiffRequest(req)
		}
	}
}

// processDiffRequest handles a single diff request asynchronously
func (d *DiffPane) processDiffRequest(req diffRequest) {
	if req.instance == nil {
		d.diffResultCh <- diffResult{
			stats:      "",
			diff:       "",
			err:        nil,
			instanceID: "",
		}
		return
	}

	instanceID := d.getInstanceID(req.instance)

	// Check cache first
	if cached, ok := d.getCachedDiff(instanceID); ok {
		d.diffResultCh <- diffResult{
			stats:      cached.stats,
			diff:       cached.diff,
			err:        nil,
			instanceID: instanceID,
		}
		return
	}

	// Perform expensive git operation in background
	stats := req.instance.GetDiffStats()
	var statsStr, diffStr string

	if stats == nil {
		statsStr = ""
		diffStr = ""
	} else if stats.Error != nil {
		// Error case - don't cache errors
		d.diffResultCh <- diffResult{
			stats:      "",
			diff:       "",
			err:        stats.Error,
			instanceID: instanceID,
		}
		return
	} else if stats.IsEmpty() {
		statsStr = ""
		diffStr = ""
	} else {
		additions := AdditionStyle.Render(fmt.Sprintf("%d additions(+)", stats.Added))
		deletions := DeletionStyle.Render(fmt.Sprintf("%d deletions(-)", stats.Removed))
		statsStr = lipgloss.JoinHorizontal(lipgloss.Center, additions, " ", deletions)
		diffStr = colorizeDiff(stats.Content)
	}

	// Cache the result
	d.setCachedDiff(instanceID, statsStr, diffStr)

	// Send result back
	d.diffResultCh <- diffResult{
		stats:      statsStr,
		diff:       diffStr,
		err:        nil,
		instanceID: instanceID,
	}
}

// getInstanceID generates a cache key for an instance
func (d *DiffPane) getInstanceID(instance *session.Instance) string {
	if instance == nil {
		return ""
	}
	return fmt.Sprintf("%s-%s", instance.Title, instance.Branch)
}

// getCachedDiff retrieves cached diff content if valid
func (d *DiffPane) getCachedDiff(instanceID string) (cachedDiffContent, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	cached, ok := d.diffCache[instanceID]
	if !ok || !cached.isValid || time.Since(cached.timestamp) > diffCacheTTL {
		return cachedDiffContent{}, false
	}

	return cached, true
}

// setCachedDiff stores diff content in cache
func (d *DiffPane) setCachedDiff(instanceID, stats, diff string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.diffCache[instanceID] = cachedDiffContent{
		stats:     stats,
		diff:      diff,
		timestamp: time.Now(),
		isValid:   true,
	}
}

// UpdateDiffAsync requests a diff update asynchronously with debouncing
func (d *DiffPane) UpdateDiffAsync(instance *session.Instance) {
	// Cancel any existing debounce timer
	if d.debounceTimer != nil {
		d.debounceTimer.Stop()
	}

	// Store the pending instance
	d.pendingInstance = instance

	// Set up debounced execution
	d.debounceTimer = time.AfterFunc(diffDebounceDelay, func() {
		d.requestDiffUpdate(d.pendingInstance)
	})
}

// requestDiffUpdate sends a diff request to the worker
func (d *DiffPane) requestDiffUpdate(instance *session.Instance) {
	select {
	case d.diffRequestCh <- diffRequest{instance: instance}:
	default:
		// Channel is full, skip this request to prevent blocking
	}
}

// ProcessResults processes any pending results from the background worker
func (d *DiffPane) ProcessResults() error {
	for {
		select {
		case result := <-d.diffResultCh:
			if result.err != nil {
				return result.err
			}

			// Update diff state with the result
			if result.instanceID != "" {
				d.stats = result.stats
				d.diff = result.diff
				d.lastInstanceID = result.instanceID

				// Update viewport content
				d.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, d.stats, d.diff))
			}
		default:
			// No more results to process
			return nil
		}
	}
}

func colorizeDiff(diff string) string {
	var coloredOutput strings.Builder

	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		if len(line) > 0 {
			if strings.HasPrefix(line, "@@") {
				// Color hunk headers cyan
				coloredOutput.WriteString(HunkStyle.Render(line) + "\n")
			} else if line[0] == '+' && (len(line) == 1 || line[1] != '+') {
				// Color added lines green, excluding metadata like '+++'
				coloredOutput.WriteString(AdditionStyle.Render(line) + "\n")
			} else if line[0] == '-' && (len(line) == 1 || line[1] != '-') {
				// Color removed lines red, excluding metadata like '---'
				coloredOutput.WriteString(DeletionStyle.Render(line) + "\n")
			} else {
				// Print metadata and unchanged lines without color
				coloredOutput.WriteString(line + "\n")
			}
		} else {
			// Preserve empty lines
			coloredOutput.WriteString("\n")
		}
	}

	return coloredOutput.String()
}
