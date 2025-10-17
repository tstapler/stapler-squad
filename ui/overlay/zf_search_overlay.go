package overlay

import (
	"claude-squad/log"
	"claude-squad/ui/fuzzy"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ZFSearchOverlay implements a ZF-inspired fuzzy search overlay with real-time updates
// This addresses the issues with the original fuzzy input overlay:
// 1. Eliminates the stuck/frozen overlay problem
// 2. Provides immediate visual feedback as user types
// 3. Uses ZF's filename priority and strict path matching
// 4. Implements progressive refinement with space-separated tokens
type ZFSearchOverlay struct {
	// Core components
	input      textinput.Model
	zfEngine   *fuzzy.SimpleFuzzyEngine
	spinner    spinner.Model
	ctx        context.Context
	cancelFunc context.CancelFunc

	// State
	title         string
	placeholder   string
	results       []fuzzy.ZFSearchResult
	selectedIndex int
	width         int
	height        int
	focused       bool
	loading       bool
	error         error
	directories   []string

	// Real-time update management
	lastQuery     string
	updatePending bool
	searchID      int64 // Track latest search to prevent race conditions

	// Visual styling
	titleStyle     lipgloss.Style
	inputStyle     lipgloss.Style
	resultStyle    lipgloss.Style
	selectedStyle  lipgloss.Style
	highlightStyle lipgloss.Style
	loadingStyle   lipgloss.Style
	errorStyle     lipgloss.Style
	matchTypeStyle map[fuzzy.ZFMatchType]lipgloss.Style

	// Callbacks
	onSelect func(item fuzzy.SearchItem)
	onCancel func()
}

// ZFSearchMsg represents internal messages for search updates
type ZFSearchMsg struct {
	Query   string
	Results []fuzzy.ZFSearchResult
	Error   error
}

// ZFSearchTriggerMsg represents a debounced search trigger
type ZFSearchTriggerMsg struct {
	Query   string
	SearchID int64
}

// NewZFSearchOverlay creates a new ZF-inspired search overlay
func NewZFSearchOverlay(title, placeholder string, directories []string) (*ZFSearchOverlay, error) {
	// Initialize simple fuzzy search engine (replaces SQLite/FTS5-based ZF engine)
	config := fuzzy.DefaultSimpleFuzzyConfig()
	zfEngine, err := fuzzy.NewSimpleFuzzyEngine(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create simple fuzzy search engine: %w", err)
	}

	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 200

	// Initialize spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00"))

	// Create cancelable context for managing search operations
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Define match type specific styles
	matchTypeStyles := map[fuzzy.ZFMatchType]lipgloss.Style{
		fuzzy.MatchFilename: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true), // Green for filename matches
		fuzzy.MatchPath:     lipgloss.NewStyle().Foreground(lipgloss.Color("#0080FF")).Bold(true), // Blue for path matches
		fuzzy.MatchFuzzy:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF00")),            // Yellow for fuzzy matches
	}

	overlay := &ZFSearchOverlay{
		input:       ti,
		zfEngine:    zfEngine,
		spinner:     sp,
		ctx:         ctx,
		cancelFunc:  cancelFunc,
		title:       title,
		placeholder: placeholder,
		directories: directories,
		results:     []fuzzy.ZFSearchResult{},
		width:       80,
		height:      20,
		focused:     true,

		// Styling
		titleStyle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			MarginBottom(1),

		inputStyle: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#0080FF")).
			Padding(0, 1).
			Width(70),

		resultStyle: lipgloss.NewStyle().
			Padding(0, 1),

		selectedStyle: lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#3C3C3C")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true),

		highlightStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")).
			Bold(true),

		loadingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")),

		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")),

		matchTypeStyle: matchTypeStyles,
	}

	// Index directories in background
	go overlay.indexDirectories()

	return overlay, nil
}

// indexDirectories indexes all specified directories in the background
func (zf *ZFSearchOverlay) indexDirectories() {
	for _, dir := range zf.directories {
		select {
		case <-zf.ctx.Done():
			return
		default:
			if err := zf.zfEngine.IndexDirectory(dir); err != nil {
				// Log error but continue with other directories
				log.WarningLog.Printf("Failed to index directory %s: %v", dir, err)
			}
		}
	}
}

// SetOnSelect sets the callback for item selection
func (zf *ZFSearchOverlay) SetOnSelect(callback func(fuzzy.SearchItem)) {
	zf.onSelect = callback
}

// SetOnCancel sets the callback for cancellation
func (zf *ZFSearchOverlay) SetOnCancel(callback func()) {
	zf.onCancel = callback
}

// SetSize sets the overlay dimensions
func (zf *ZFSearchOverlay) SetSize(width, height int) {
	zf.width = width
	zf.height = height

	// Adjust input width
	inputWidth := width - 10
	if inputWidth < 30 {
		inputWidth = 30
	}
	if inputWidth > 100 {
		inputWidth = 100
	}
	zf.inputStyle = zf.inputStyle.Width(inputWidth)
}

// Update handles all overlay updates and input
func (zf *ZFSearchOverlay) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return zf.handleKeyPress(msg)

	case ZFSearchTriggerMsg:
		// Handle debounced search trigger
		// Only process if this is the latest search (prevent race conditions)
		if msg.SearchID == zf.searchID {
			return zf.triggerSearch(msg.Query)
		}
		return nil

	case ZFSearchMsg:
		// Handle search results
		zf.loading = false
		zf.results = msg.Results
		zf.error = msg.Error
		zf.selectedIndex = 0 // Reset selection
		return nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		zf.spinner, cmd = zf.spinner.Update(msg)
		return cmd
	}

	// Update text input
	var inputCmd tea.Cmd
	zf.input, inputCmd = zf.input.Update(msg)
	cmds = append(cmds, inputCmd)

	// Check if query changed and trigger debounced search
	currentQuery := strings.TrimSpace(zf.input.Value())
	if currentQuery != zf.lastQuery {
		zf.lastQuery = currentQuery
		zf.searchID++ // Increment search ID to invalidate previous searches

		// Use tea.Tick for BubbleTea-compatible debouncing
		cmds = append(cmds, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
			return ZFSearchTriggerMsg{
				Query:    currentQuery,
				SearchID: zf.searchID,
			}
		}))
	}

	return tea.Batch(cmds...)
}

// handleKeyPress processes keyboard input
func (zf *ZFSearchOverlay) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEnter:
		return zf.handleSelection()

	case tea.KeyEsc:
		if zf.onCancel != nil {
			zf.onCancel()
		}
		return nil

	case tea.KeyUp:
		if len(zf.results) > 0 {
			if zf.selectedIndex > 0 {
				zf.selectedIndex--
			} else {
				zf.selectedIndex = len(zf.results) - 1
			}
		}
		return nil

	case tea.KeyDown:
		if len(zf.results) > 0 {
			if zf.selectedIndex < len(zf.results)-1 {
				zf.selectedIndex++
			} else {
				zf.selectedIndex = 0
			}
		}
		return nil

	case tea.KeyTab:
		return zf.handleAutoComplete()

	case tea.KeyCtrlC:
		if zf.onCancel != nil {
			zf.onCancel()
		}
		return nil
	}

	// Let text input handle other keys
	var cmd tea.Cmd
	zf.input, cmd = zf.input.Update(msg)
	return cmd
}

// handleSelection processes item selection
func (zf *ZFSearchOverlay) handleSelection() tea.Cmd {
	if len(zf.results) > 0 && zf.selectedIndex >= 0 && zf.selectedIndex < len(zf.results) {
		if zf.onSelect != nil {
			zf.onSelect(zf.results[zf.selectedIndex].Item)
		}
		return nil
	}

	// Handle raw input if no selection
	currentInput := strings.TrimSpace(zf.input.Value())
	if currentInput != "" && zf.onSelect != nil {
		// Create a basic item from raw input
		rawItem := &fuzzy.FileSearchItem{
			ID:       currentInput,
			FullPath: currentInput,
			Filename: filepath.Base(currentInput),
		}
		zf.onSelect(rawItem)
	}

	return nil
}

// handleAutoComplete implements smart path completion
func (zf *ZFSearchOverlay) handleAutoComplete() tea.Cmd {
	if len(zf.results) > 0 && zf.selectedIndex >= 0 && zf.selectedIndex < len(zf.results) {
		selectedPath := zf.results[zf.selectedIndex].Item.GetID()
		currentInput := strings.TrimSpace(zf.input.Value())

		// Smart completion logic
		completion := zf.getSmartCompletion(currentInput, selectedPath)
		zf.input.SetValue(completion)
	}
	return nil
}

// getSmartCompletion provides intelligent path completion
func (zf *ZFSearchOverlay) getSmartCompletion(current, target string) string {
	if current == "" {
		return target
	}

	// If current input is already contained in target, return target
	if strings.Contains(target, current) {
		return target
	}

	// For path-like inputs, try common path completion
	if strings.Contains(current, "/") && strings.Contains(target, "/") {
		currentParts := strings.Split(current, "/")
		targetParts := strings.Split(target, "/")

		// Find longest common prefix
		var commonParts []string
		for i := 0; i < len(currentParts) && i < len(targetParts); i++ {
			if currentParts[i] == targetParts[i] || currentParts[i] == "" {
				commonParts = append(commonParts, targetParts[i])
			} else if strings.HasPrefix(targetParts[i], currentParts[i]) {
				commonParts = append(commonParts, targetParts[i])
				break
			} else {
				break
			}
		}

		if len(commonParts) > 0 {
			// Add remaining parts
			remaining := targetParts[len(commonParts):]
			allParts := append(commonParts, remaining...)
			return strings.Join(allParts, "/")
		}
	}

	return target
}

// triggerSearch initiates a search operation
func (zf *ZFSearchOverlay) triggerSearch(query string) tea.Cmd {
	if query == "" {
		return func() tea.Msg {
			return ZFSearchMsg{
				Query:   query,
				Results: []fuzzy.ZFSearchResult{},
				Error:   nil,
			}
		}
	}

	zf.loading = true

	return func() tea.Msg {
		// Try ZF search first, with fallback to simple file search
		results, err := zf.zfEngine.Search(query, zf.directories)

		// If ZF search fails or returns no results, fall back to simple file search
		if err != nil || len(results) == 0 {
			// Fallback to simple file system search
			fallbackResults, fallbackErr := zf.performFallbackSearch(query)
			if fallbackErr == nil && len(fallbackResults) > 0 {
				results = fallbackResults
				err = nil
			}
		}

		return ZFSearchMsg{
			Query:   query,
			Results: results,
			Error:   err,
		}
	}
}

// performFallbackSearch performs a simple filesystem-based search when SQLite indexing fails
func (zf *ZFSearchOverlay) performFallbackSearch(query string) ([]fuzzy.ZFSearchResult, error) {
	if query == "" {
		return []fuzzy.ZFSearchResult{}, nil
	}

	var results []fuzzy.ZFSearchResult
	queryLower := strings.ToLower(query)
	maxResults := 50 // Limit fallback results for performance

	for _, directory := range zf.directories {
		if len(results) >= maxResults {
			break
		}

		err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Continue walking on errors
			}

			// Skip directories and hidden files
			if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			// Simple string matching (case-insensitive)
			filename := filepath.Base(path)
			if strings.Contains(strings.ToLower(filename), queryLower) ||
			   strings.Contains(strings.ToLower(path), queryLower) {

				// Create a basic ZFSearchResult
				result := fuzzy.ZFSearchResult{
					SearchResult: fuzzy.SearchResult{
						Item: &fuzzy.FileSearchItem{
							ID:       path,
							FullPath: path,
							Filename: filename,
						},
						Score:   0.5, // Basic score for fallback matches
						Matches: []int{}, // No character-level highlighting for fallback
					},
					MatchType: fuzzy.MatchFuzzy, // Mark as fuzzy match
				}
				results = append(results, result)

				// Early exit if we have enough results
				if len(results) >= maxResults {
					return filepath.SkipDir
				}
			}

			return nil
		})

		if err != nil {
			// Log error but continue with other directories
			log.WarningLog.Printf("Error walking directory %s for fallback search: %v", directory, err)
		}
	}

	return results, nil
}

// View renders the overlay
func (zf *ZFSearchOverlay) View() string {
	var sb strings.Builder

	// Title with search info
	titleText := zf.title
	if len(zf.results) > 0 {
		titleText = fmt.Sprintf("%s (%d results)", zf.title, len(zf.results))
	}
	sb.WriteString(zf.titleStyle.Render(titleText))
	sb.WriteString("\n")

	// Input field
	sb.WriteString(zf.inputStyle.Render(zf.input.View()))
	sb.WriteString("\n\n")

	// Loading indicator or error
	if zf.loading {
		sb.WriteString(zf.loadingStyle.Render(fmt.Sprintf("%s Searching...", zf.spinner.View())))
		sb.WriteString("\n")
	} else if zf.error != nil {
		sb.WriteString(zf.errorStyle.Render(fmt.Sprintf("Error: %v", zf.error)))
		sb.WriteString("\n")
	}

	// Results with ZF-style highlighting
	maxResults := zf.height - 6
	if len(zf.results) > 0 {
		visibleResults := zf.results
		if len(visibleResults) > maxResults {
			visibleResults = visibleResults[:maxResults]
		}

		for i, result := range visibleResults {
			line := zf.formatResult(result, i == zf.selectedIndex)
			sb.WriteString(line)
			sb.WriteString("\n")
		}

		// Show count if truncated
		if len(zf.results) > maxResults {
			moreText := fmt.Sprintf("... and %d more results", len(zf.results)-maxResults)
			sb.WriteString(zf.resultStyle.Render(moreText))
			sb.WriteString("\n")
		}
	} else if zf.input.Value() != "" && !zf.loading {
		sb.WriteString(zf.resultStyle.Render("No results found"))
		sb.WriteString("\n")
	}

	// Help text
	sb.WriteString("\n")
	helpText := "↑↓ navigate • Enter select • Tab complete • Esc cancel"
	sb.WriteString(zf.resultStyle.Render(helpText))

	return sb.String()
}

// formatResult formats a search result with ZF-style highlighting and match type indication
func (zf *ZFSearchOverlay) formatResult(result fuzzy.ZFSearchResult, selected bool) string {
	displayText := result.Item.GetDisplayText()

	// Apply character highlighting
	highlighted := zf.applyHighlighting(displayText, result.Matches)

	// Add match type indicator
	var matchIndicator string
	matchStyle := zf.matchTypeStyle[result.MatchType]
	switch result.MatchType {
	case fuzzy.MatchFilename:
		matchIndicator = matchStyle.Render("[F] ") // F for filename
	case fuzzy.MatchPath:
		matchIndicator = matchStyle.Render("[P] ") // P for path
	case fuzzy.MatchFuzzy:
		matchIndicator = matchStyle.Render("[~] ") // ~ for fuzzy
	}

	formattedText := matchIndicator + highlighted

	// Apply selection styling
	if selected {
		return zf.selectedStyle.Render(formattedText)
	} else {
		return zf.resultStyle.Render(formattedText)
	}
}

// applyHighlighting applies character-level highlighting to text
func (zf *ZFSearchOverlay) applyHighlighting(text string, matches []int) string {
	if len(matches) == 0 {
		return text
	}

	var result strings.Builder
	runes := []rune(text)
	matchSet := make(map[int]bool)

	// Create set of match positions
	for _, pos := range matches {
		if pos >= 0 && pos < len(runes) {
			matchSet[pos] = true
		}
	}

	// Build highlighted text
	for i, r := range runes {
		if matchSet[i] {
			result.WriteString(zf.highlightStyle.Render(string(r)))
		} else {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// Close cleans up resources
func (zf *ZFSearchOverlay) Close() error {
	if zf.cancelFunc != nil {
		zf.cancelFunc()
	}
	if zf.zfEngine != nil {
		return zf.zfEngine.Close()
	}
	return nil
}

// GetHeight returns the rendered height
func (zf *ZFSearchOverlay) GetHeight() int {
	baseHeight := 6 // Title + input + loading + help
	resultHeight := len(zf.results)
	maxResults := zf.height - baseHeight

	if resultHeight > maxResults {
		resultHeight = maxResults
	}

	return baseHeight + resultHeight
}