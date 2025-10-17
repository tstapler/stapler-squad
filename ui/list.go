package ui

import (
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui/debounce"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

const readyIcon = "● "
const pausedIcon = "⏸ "
const needsApprovalIcon = "❗ "

var readyStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var addedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#51bd73", Dark: "#51bd73"})

var removedLinesStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#de613e"))

var pausedStyle = lipgloss.NewStyle().
	Foreground(lipgloss.AdaptiveColor{Light: "#707070", Dark: "#B0B0B0"})

var needsApprovalStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffaa00"))

var titleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#6B6570", Dark: "#B0B0B0"})

var selectedTitleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var selectedDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#1a1a1a"})

var mainTitle = lipgloss.NewStyle().
	Background(lipgloss.Color("62")).
	Foreground(lipgloss.Color("230"))

var autoYesStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#dde4f0")).
	Foreground(lipgloss.Color("#1a1a1a"))

type List struct {
	items         []*session.Instance
	selectedIdx   int
	height, width int
	renderer      *InstanceRenderer
	autoyes       bool

	// map of repo name to number of instances using it. Used to display the repo name only if there are
	// multiple repos in play.
	repos map[string]int

	// Session organization fields
	categoryGroups map[string][]*session.Instance // Map of category name to instances in that category
	groupExpanded  map[string]bool                // Map of category name to expanded state
	searchMode     bool                           // Whether search mode is active
	searchQuery    string                         // Current search query
	searchResults  []*session.Instance            // Filtered search results
	hidePaused     bool                           // Whether to hide paused sessions
	reviewQueue    *session.ReviewQueue           // Review queue for tracking sessions needing attention

	// State management for persistence
	stateManager *config.State // Reference to state manager for persistence

	// Scrolling support
	scrollOffset int // Index of the first visible item

	// Performance optimization: track if categories need reorganization
	categoriesNeedUpdate bool

	// Performance optimization: O(1) instance lookups instead of O(n) searches
	instanceToIndex   map[*session.Instance]int // O(1) index lookups
	visibleItemsCache []*session.Instance       // Cache filtered items

	// Search index for optimized fuzzy search performance
	searchIndex *SearchIndex // Hybrid closestmatch + sahilm/fuzzy search index
	visibleCacheValid bool                      // Cache validity flag
	visibleIndexMap   map[*session.Instance]int // O(1) visible index lookups

	// Performance optimization: debounce state saving during navigation
	stateSaveTimer *time.Timer
	stateSaveDelay time.Duration

	// Dynamic element size tracking to prevent viewport overflow
	actualItemHeight       int    // Measured height of a rendered item in lines
	actualCategoryHeight   int    // Measured height of a category header in lines
	lastMeasuredContent    string // Cache of last measured content to detect changes
	sizeMeasurementValid   bool   // Whether our measurements are current
	lastLoggedViewport     string // Cache last logged viewport to avoid spam
	lastViewportLogTime    time.Time // Rate limit viewport logging

	// Cache viewport calculations to avoid excessive recalculation
	cachedMaxVisible     int  // Cached result of calculateMaxVisibleItems
	maxVisibleCacheValid bool // Whether the cached max visible is valid
	lastCachedHeight     int  // Height used for cached calculation
	lastCachedCategories int  // Category count used for cached calculation

	// Live search debouncing
	searchDebouncer *debounce.Debouncer // Debouncer for live search input

	// Streaming search state
	searchCancel    context.CancelFunc // Cancel function for active streaming search
	searchLoading   bool               // Whether search is currently in progress
	searchStage     string             // Current search stage for user feedback
}

// SessionSearchSource implements the fuzzy.Source interface for session instances
// This enables advanced fuzzy search across multiple session fields using sahilm/fuzzy
type SessionSearchSource struct {
	sessions []*session.Instance
}

// String returns the searchable text for the session at index i
// Fields are prioritized by importance: Title, Category, Program, Branch, Path, WorkingDir
func (s SessionSearchSource) String(i int) string {
	if i < 0 || i >= len(s.sessions) {
		return ""
	}

	instance := s.sessions[i]
	parts := make([]string, 0, 6)

	// Primary field - Title (highest priority)
	if instance.Title != "" {
		parts = append(parts, instance.Title)
	}

	// Secondary fields - Context and organization
	if instance.Category != "" {
		parts = append(parts, instance.Category)
	}

	if instance.Program != "" {
		parts = append(parts, instance.Program)
	}

	if instance.Branch != "" {
		parts = append(parts, instance.Branch)
	}

	// Path information - extract repo name and working directory
	if instance.Path != "" {
		repoName := filepath.Base(instance.Path)
		if repoName != "" && repoName != "." {
			parts = append(parts, repoName)
		}
	}

	if instance.WorkingDir != "" {
		parts = append(parts, instance.WorkingDir)
	}

	return strings.Join(parts, " ")
}

// Len returns the number of sessions in the source
func (s SessionSearchSource) Len() int {
	return len(s.sessions)
}

// SessionSearchItem wraps a session instance to implement the fuzzy.SearchItem interface
type SessionSearchItem struct {
	instance *session.Instance
}

// GetSearchText returns the text used for fuzzy matching
func (s SessionSearchItem) GetSearchText() string {
	parts := make([]string, 0, 6)

	if s.instance.Title != "" {
		parts = append(parts, s.instance.Title)
	}
	if s.instance.Category != "" {
		parts = append(parts, s.instance.Category)
	}
	if s.instance.Program != "" {
		parts = append(parts, s.instance.Program)
	}
	if s.instance.Branch != "" {
		parts = append(parts, s.instance.Branch)
	}
	if s.instance.Path != "" {
		parts = append(parts, filepath.Base(s.instance.Path))
	}
	if s.instance.WorkingDir != "" {
		parts = append(parts, s.instance.WorkingDir)
	}

	return strings.Join(parts, " ")
}

// GetDisplayText returns the text to display in the UI (just the title)
func (s SessionSearchItem) GetDisplayText() string {
	return s.instance.Title
}

// GetID returns a unique identifier for the item
func (s SessionSearchItem) GetID() string {
	return s.instance.Title + "|" + s.instance.Path
}

func NewList(spinner *spinner.Model, autoYes bool, stateManager *config.State) *List {
	l := &List{
		items:                []*session.Instance{},
		renderer:             &InstanceRenderer{spinner: spinner, repoNameCache: make(map[string]string)},
		repos:                make(map[string]int),
		autoyes:              autoYes,
		categoryGroups:       make(map[string][]*session.Instance),
		instanceToIndex:      make(map[*session.Instance]int),
		visibleItemsCache:    []*session.Instance{},
		visibleCacheValid:    false,
		visibleIndexMap:      make(map[*session.Instance]int),
		groupExpanded:        make(map[string]bool),
		searchMode:           false,
		searchResults:        []*session.Instance{},
		hidePaused:           false,
		stateManager:         stateManager,
		categoriesNeedUpdate: true,                   // Initialize as needing update
		stateSaveDelay:       200 * time.Millisecond, // Debounce state saves during navigation
		actualItemHeight:     4,                      // Default fallback
		actualCategoryHeight: 2,                      // Default fallback
		searchIndex:          NewSearchIndex(),       // Initialize optimized search index
		sizeMeasurementValid: false,                  // Needs initial measurement
		searchDebouncer:      debounce.New(200 * time.Millisecond), // Live search debouncing
	}

	// Load persisted UI state if available
	l.loadUIState()

	return l
}

// rebuildInstanceIndex rebuilds the instanceToIndex map for O(1) lookups
func (l *List) rebuildInstanceIndex() {
	l.instanceToIndex = make(map[*session.Instance]int, len(l.items))
	for idx, instance := range l.items {
		l.instanceToIndex[instance] = idx
	}
}

// invalidateVisibleCache marks the visible items cache as invalid
func (l *List) invalidateVisibleCache() {
	l.visibleCacheValid = false
	l.visibleItemsCache = nil
	// Clear the visible index map as well
	for k := range l.visibleIndexMap {
		delete(l.visibleIndexMap, k)
	}
}

// IsVisibleCacheValid returns whether the visible cache is currently valid (debug helper)
func (l *List) IsVisibleCacheValid() bool {
	return l.visibleCacheValid
}

// GetVisibleItems returns the currently visible items (public accessor for debugging)
func (l *List) GetVisibleItems() []*session.Instance {
	return l.getVisibleItems()
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
	// Invalidate viewport cache if height changed
	if l.height != height {
		l.maxVisibleCacheValid = false
	}
	l.width = width
	l.height = height
	l.renderer.setWidth(width)
}

// SetSessionPreviewSize sets the height and width for the tmux sessions. This makes the stdout line have the correct
// width and height.
func (l *List) SetSessionPreviewSize(width, height int) (err error) {
	for i, item := range l.items {
		if !item.Started() || item.Paused() {
			continue
		}

		if innerErr := item.SetPreviewSize(width, height); innerErr != nil {
			// Log PTY initialization errors but don't propagate them to avoid breaking the UI
			// These errors are common when sessions are starting up or recovering
			log.WarningLog.Printf("could not set preview size for instance %d (%s): %v", i, item.Title, innerErr)
			// Don't accumulate these errors to prevent UI disruption
			continue
		}
	}
	return
}

func (l *List) NumInstances() int {
	return len(l.items)
}

// InstanceRenderer handles rendering of session.Instance objects
type InstanceRenderer struct {
	spinner *spinner.Model
	width   int
	// Performance optimization: cache repository names to avoid repeated git operations
	repoNameCache map[string]string // Map of instance title to repo name
}

func (r *InstanceRenderer) setWidth(width int) {
	r.width = AdjustPreviewWidth(width)
}

// ɹ and ɻ are other options.
const branchIcon = "Ꮧ"

func (r *InstanceRenderer) Render(i *session.Instance, idx int, selected bool, hasMultipleRepos bool) string {
	prefix := fmt.Sprintf(" %d. ", idx)
	if idx >= 10 {
		prefix = prefix[:len(prefix)-1]
	}
	titleS := selectedTitleStyle
	descS := selectedDescStyle
	if !selected {
		titleS = titleStyle
		descS = listDescStyle
	}

	// add spinner next to title if it's running
	var join string

	// Use instance-type-aware status icon (handles managed vs external)
	statusIcon := i.GetStatusIconForType()

	if i.IsManaged {
		// Managed instances use the standard status rendering
		switch i.Status {
		case session.Running:
			join = fmt.Sprintf("%s ", r.spinner.View())
		case session.Ready:
			join = readyStyle.Render(readyIcon)
		case session.Paused:
			join = pausedStyle.Render(pausedIcon)
		case session.NeedsApproval:
			join = needsApprovalStyle.Render(needsApprovalIcon)
		default:
		}
	} else {
		// External instances use the eye icon with a distinctive color
		externalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		join = externalStyle.Render(statusIcon + " ")
	}

	// Add review queue badge if session needs attention
	if i.NeedsReview() {
		reviewItem, _ := i.GetReviewItem()
		if reviewItem != nil {
			badge := reviewItem.Priority.Emoji()
			join = join + " " + badge
		}
	}

	// Cut the title if it's too long
	titleText := i.Title
	widthAvail := r.width - 3 - len(prefix) - 1
	if widthAvail > 0 && widthAvail < len(titleText) && len(titleText) >= widthAvail-3 {
		titleText = titleText[:widthAvail-3] + "..."
	}
	title := titleS.Render(lipgloss.JoinHorizontal(
		lipgloss.Left,
		lipgloss.Place(r.width-3, 1, lipgloss.Left, lipgloss.Center, fmt.Sprintf("%s %s", prefix, titleText)),
		" ",
		join,
	))

	stat := i.GetDiffStats()

	var diff string
	var addedDiff, removedDiff string
	if stat == nil || stat.Error != nil || stat.IsEmpty() {
		// Don't show diff stats if there's an error or if they don't exist
		addedDiff = ""
		removedDiff = ""
		diff = ""
	} else {
		addedDiff = fmt.Sprintf("+%d", stat.Added)
		removedDiff = fmt.Sprintf("-%d ", stat.Removed)
		diff = lipgloss.JoinHorizontal(
			lipgloss.Center,
			addedLinesStyle.Background(descS.GetBackground()).Render(addedDiff),
			lipgloss.Style{}.Background(descS.GetBackground()).Foreground(descS.GetForeground()).Render(","),
			removedLinesStyle.Background(descS.GetBackground()).Render(removedDiff),
		)
	}

	remainingWidth := r.width
	remainingWidth -= len(prefix)
	remainingWidth -= len(branchIcon)

	diffWidth := len(addedDiff) + len(removedDiff)
	if diffWidth > 0 {
		diffWidth += 1
	}

	// Use fixed width for diff stats to avoid layout issues
	remainingWidth -= diffWidth

	branch := i.Branch
	if i.Started() {
		// Skip repo name retrieval for paused instances
		if !i.Paused() {
			// Check cache first to avoid expensive git operations
			var repoName string
			if r.repoNameCache != nil {
				if cachedName, exists := r.repoNameCache[i.Title]; exists {
					repoName = cachedName
				} else {
					// Only call expensive git operation if not cached
					name, err := i.RepoName()
					if err != nil {
						// Silently skip repo name for directory sessions without git worktrees
						// This is expected behavior and not an error
						if !strings.Contains(err.Error(), "gitWorktree is nil") {
							log.WarningLog.Printf("could not get repo name in instance renderer: %v", err)
						}
					} else {
						repoName = name
						// Cache the result for future renders
						r.repoNameCache[i.Title] = repoName
					}
				}
			} else {
				// Fallback to direct call if cache not initialized
				name, err := i.RepoName()
				if err != nil {
					// Silently skip repo name for directory sessions without git worktrees
					if !strings.Contains(err.Error(), "gitWorktree is nil") {
						log.WarningLog.Printf("could not get repo name in instance renderer: %v", err)
					}
				} else {
					repoName = name
				}
			}

			if repoName != "" {
				branch += fmt.Sprintf(" (%s)", repoName)
			}
		}
	}
	// Don't show branch if there's no space for it. Or show ellipsis if it's too long.
	if remainingWidth < 0 {
		branch = ""
	} else if remainingWidth < len(branch) {
		if remainingWidth < 3 {
			branch = ""
		} else {
			// We know the remainingWidth is at least 4 and branch is longer than that, so this is safe.
			branch = branch[:remainingWidth-3] + "..."
		}
	}
	remainingWidth -= len(branch)

	// Add spaces to fill the remaining width.
	spaces := ""
	if remainingWidth > 0 {
		spaces = strings.Repeat(" ", remainingWidth)
	}

	branchLine := fmt.Sprintf("%s %s-%s%s%s", strings.Repeat(" ", len(prefix)), branchIcon, branch, spaces, diff)

	// join title and subtitle
	text := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		descS.Render(branchLine),
	)

	return text
}

// Define styles for category headers
var categoryHeaderStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Bold(true).
	Foreground(lipgloss.Color("#ffffff")).
	Background(lipgloss.Color("#555555"))

var categoryHeaderSelectedStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Bold(true).
	Foreground(lipgloss.Color("#ffffff")).
	Background(lipgloss.Color("#007acc"))

var expandedIconStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffffff"))

var collapsedIconStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffffff"))

// Performance optimization: pre-computed styles to avoid repeated allocations
var noSessionsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

func (l *List) String() string {
	// Build dynamic title with filter status
	titleText := " Instances"
	var filters []string

	// Add search filter info with progress indicator
	if l.searchMode && l.searchQuery != "" {
		searchText := fmt.Sprintf("🔍 %s", l.searchQuery)
		if l.searchLoading && l.searchStage != "" {
			searchText += fmt.Sprintf(" (%s...)", l.searchStage)
		}
		filters = append(filters, searchText)
	}

	// Add paused filter info
	if l.hidePaused {
		filters = append(filters, "Active Only")
	}

	// Construct title with filters
	if len(filters) > 0 {
		titleText += fmt.Sprintf(" (%s)", strings.Join(filters, " | "))
	}
	titleText += " "

	const autoYesText = " auto-yes "
	const expandedIcon = "▼ "
	const collapsedIcon = "► "

	// Always ensure categories are organized correctly
	l.OrganizeByCategory()

	// Ensure selected item is visible (update scroll offset if needed)
	l.ensureSelectedVisible()

	// Write the title.
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("\n")

	// Write title line with scroll indicators
	// Use the actual list width, not adjusted preview width for proper layout
	titleWidth := l.width
	scrollIndicator := l.getScrollIndicator()
	if !l.autoyes {
		titleWithScroll := titleText + scrollIndicator
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleWithScroll)))
	} else {
		titleWithScroll := titleText + scrollIndicator
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleWithScroll))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n")
	b.WriteString("\n")

	// Get the visible window of items to render
	visibleWindow := l.getVisibleWindow()

	// Render items with scrolling support
	l.renderVisibleItems(&b, visibleWindow)

	// Return the rendered content - viewport calculations ensure proper sizing
	content := b.String()

	return lipgloss.NewStyle().
		Width(l.width).
		Render(content)
}

// Down selects the next item in the list.
func (l *List) Down() {
	visibleItems := l.getVisibleItems()
	if len(visibleItems) == 0 {
		return
	}

	// Find current position in visible items
	currentVisibleIdx := l.getVisibleIndex()
	if currentVisibleIdx < len(visibleItems)-1 {
		// Move to next visible item
		nextItem := visibleItems[currentVisibleIdx+1]
		// Find its global index
		for i, item := range l.items {
			if item == nextItem {
				l.selectedIdx = i
				l.saveSelectedIndex() // Save selection change
				break
			}
		}
	}

	// Ensure the selected item is visible after navigation
	l.ensureSelectedVisible()
}

// Kill terminates the selected instance in the list.
func (l *List) Kill() {
	if len(l.items) == 0 {
		return
	}

	// Ensure selectedIdx is within bounds
	if l.selectedIdx < 0 || l.selectedIdx >= len(l.items) {
		return
	}

	targetInstance := l.items[l.selectedIdx]

	// Kill the tmux session
	if err := targetInstance.Kill(); err != nil {
		log.ErrorLog.Printf("could not kill instance: %v", err)
	}

	// If you delete the last one in the list, select the previous one.
	if l.selectedIdx == len(l.items)-1 {
		defer l.Up()
	}

	// Unregister the reponame if the instance is not paused
	if !targetInstance.Paused() {
		repoName, err := targetInstance.RepoName()
		if err != nil {
			log.WarningLog.Printf("could not get repo name: %v", err)
		} else {
			l.rmRepo(repoName)
		}
	}

	// Since there's items after this, the selectedIdx can stay the same.
	l.items = append(l.items[:l.selectedIdx], l.items[l.selectedIdx+1:]...)

	// Rebuild index and invalidate cache due to list modification
	l.rebuildInstanceIndex()
	l.invalidateVisibleCache()
	l.searchIndex.MarkNeedsRebuild() // Mark search index for rebuild

	// Mark categories as needing update after item removal
	l.categoriesNeedUpdate = true
}

func (l *List) Attach() (chan struct{}, error) {
	targetInstance := l.items[l.selectedIdx]
	return targetInstance.Attach()
}

// Up selects the previous item in the list.
func (l *List) Up() {
	visibleItems := l.getVisibleItems()
	if len(visibleItems) == 0 {
		return
	}

	// Find current position in visible items
	currentVisibleIdx := l.getVisibleIndex()
	if currentVisibleIdx > 0 {
		// Move to previous visible item
		prevItem := visibleItems[currentVisibleIdx-1]
		// Find its global index
		for i, item := range l.items {
			if item == prevItem {
				l.selectedIdx = i
				l.saveSelectedIndex() // Save selection change
				break
			}
		}
	}

	// Ensure the selected item is visible after navigation
	l.ensureSelectedVisible()
}

func (l *List) addRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		l.repos[repo] = 0
	}
	l.repos[repo]++
}

func (l *List) rmRepo(repo string) {
	if _, ok := l.repos[repo]; !ok {
		if log.ErrorLog != nil {
			log.ErrorLog.Printf("repo %s not found", repo)
		}
		return
	}
	l.repos[repo]--
	if l.repos[repo] == 0 {
		delete(l.repos, repo)
	}
}

// AddInstance adds a new instance to the list. It returns a finalizer function that should be called when the instance
// is started. If the instance was restored from storage or is paused, you can call the finalizer immediately.
// When creating a new one and entering the name, you want to call the finalizer once the name is done.
func (l *List) AddInstance(instance *session.Instance) (finalize func()) {
	l.items = append(l.items, instance)

	// Rebuild index and invalidate cache due to list modification
	l.rebuildInstanceIndex()
	l.invalidateVisibleCache()
	l.searchIndex.MarkNeedsRebuild() // Mark search index for rebuild

	// Add to the appropriate category group
	category := instance.Category
	if category == "" {
		category = "Uncategorized"
	}

	// Initialize the category if it doesn't exist
	if _, exists := l.categoryGroups[category]; !exists {
		l.categoryGroups[category] = []*session.Instance{}

		// Initialize expansion state if it doesn't exist
		if _, expanded := l.groupExpanded[category]; !expanded {
			// Default to expanded for new categories
			l.groupExpanded[category] = true
		}
	}

	// Add instance to its category group
	l.categoryGroups[category] = append(l.categoryGroups[category], instance)

	// Mark categories as needing update
	l.categoriesNeedUpdate = true

	// The finalizer registers the repo name once the instance is started.
	return func() {
		// Skip repo registration for paused instances or not started instances
		if !instance.Started() || instance.Paused() {
			return
		}

		// Check if the gitWorktree is initialized before trying to get repo name
		if gitWorktree, err := instance.GetGitWorktree(); err != nil || gitWorktree == nil {
			// Don't try to log here - it might be nil during testing
			return
		}

		repoName, err := instance.RepoName()
		if err != nil {
			// Use a nil check to avoid crashes during testing
			if log.WarningLog != nil {
				log.WarningLog.Printf("could not get repo name in finalizer: %v", err)
			}
			return
		}

		l.addRepo(repoName)
	}
}

// GetSelectedInstance returns the currently selected instance
func (l *List) GetSelectedInstance() *session.Instance {
	if len(l.items) == 0 || l.selectedIdx < 0 {
		return nil
	}

	// Ensure selectedIdx is within bounds
	if l.selectedIdx >= len(l.items) {
		// Find first visible item or reset to 0
		visibleItems := l.getVisibleItems()
		if len(visibleItems) > 0 {
			// Find the global index of the first visible item
			for i, item := range l.items {
				if item == visibleItems[0] {
					l.selectedIdx = i
					break
				}
			}
		} else {
			return nil
		}
	}

	return l.items[l.selectedIdx]
}

// SetSelectedInstance sets the selected index. Noop if the index is out of bounds.
func (l *List) SetSelectedInstance(idx int) {
	if idx < 0 || idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
	l.ensureSelectedVisible() // Ensure new selection is visible
	l.saveSelectedIndex()     // Save selection change
}

// SetSelectedIdx sets the selected index without triggering save (for loading state)
func (l *List) SetSelectedIdx(idx int) {
	if idx >= len(l.items) {
		return
	}
	l.selectedIdx = idx
}

// GetInstances returns all instances in the list
func (l *List) GetInstances() []*session.Instance {
	return l.items
}

// OrganizeByCategory organizes sessions into category groups
// Managed and external instances are separated into distinct top-level sections:
// - "Squad Sessions" for managed instances
// - "External Claude" for externally discovered instances
func (l *List) OrganizeByCategory() {
	// Only reorganize if needed (performance optimization)
	if !l.categoriesNeedUpdate {
		return
	}

	// Reset category groups
	l.categoryGroups = make(map[string][]*session.Instance)

	// Group instances by category (supporting nested categories)
	for _, instance := range l.items {
		// Skip paused sessions if hidePaused is true
		if l.hidePaused && instance.Status == session.Paused {
			continue
		}

		categoryPath := instance.GetCategoryPath()

		// Determine the category to use for grouping
		var category string
		if len(categoryPath) == 1 {
			// Simple category (e.g., "Work" or "Uncategorized")
			category = categoryPath[0]
		} else {
			// Nested category - use the full path (e.g., "Work/Frontend")
			category = strings.Join(categoryPath, "/")
		}

		// Prepend top-level section based on instance type
		// This creates two-tier hierarchy: "Squad Sessions/Work" or "External Claude/Uncategorized"
		if instance.IsManaged {
			// Managed instances go under "Squad Sessions"
			if category == "Uncategorized" {
				category = "Squad Sessions"
			} else {
				category = "Squad Sessions/" + category
			}
		} else {
			// External instances go under "External Claude"
			if category == "Uncategorized" {
				category = "External Claude"
			} else {
				category = "External Claude/" + category
			}
		}

		// Initialize the category if it doesn't exist
		if _, exists := l.categoryGroups[category]; !exists {
			l.categoryGroups[category] = []*session.Instance{}

			// Initialize expansion state if it doesn't exist
			if _, expanded := l.groupExpanded[category]; !expanded {
				// Default to expanded for new categories
				l.groupExpanded[category] = true
			}
		}

		// Add instance to its category group
		l.categoryGroups[category] = append(l.categoryGroups[category], instance)
	}

	// Mark categories as updated
	l.categoriesNeedUpdate = false
	// Invalidate viewport cache since category count may have changed
	l.maxVisibleCacheValid = false
}

// TogglePausedFilter toggles whether paused sessions are hidden
func (l *List) TogglePausedFilter() {
	l.hidePaused = !l.hidePaused
	// Invalidate cache due to filter change
	l.invalidateVisibleCache()
	// Mark categories as needing update when filter changes
	l.categoriesNeedUpdate = true
	// Re-organize to apply the filter
	l.OrganizeByCategory()

	// Reset scroll position when filter changes
	l.scrollOffset = 0

	// Ensure selection is valid for the new filtered view
	visibleItems := l.getVisibleItems()
	if len(visibleItems) == 0 {
		l.selectedIdx = -1 // No valid selection when no visible items
		return
	}

	// If current selection is no longer visible, select the first visible item
	if l.getVisibleIndex() == -1 {
		// Find the global index of the first visible item
		for i, item := range l.items {
			if item == visibleItems[0] {
				l.selectedIdx = i
				break
			}
		}
	}

	// Ensure the selected item is visible after filter change
	l.ensureSelectedVisible()

	// Persist the state change
	l.saveUIState()
}

// getVisibleItems returns the items that should be visible based on current filters
func (l *List) getVisibleItems() []*session.Instance {
	// Return cached result if valid
	if l.visibleCacheValid && l.visibleItemsCache != nil {
		return l.visibleItemsCache
	}

	// Rebuild cache
	var visible []*session.Instance

	// If in search mode, return search results (already filtered)
	if l.searchMode && len(l.searchResults) > 0 {
		for _, item := range l.searchResults {
			if l.hidePaused && item.Status == session.Paused {
				continue
			}
			visible = append(visible, item)
		}
	} else {
		// Normal mode: filter items based on hidePaused
		for _, item := range l.items {
			if l.hidePaused && item.Status == session.Paused {
				continue
			}
			visible = append(visible, item)
		}
	}

	// Cache the result and build visible index map for O(1) lookups
	l.visibleItemsCache = visible
	l.visibleCacheValid = true

	// Build visible index map for O(1) getVisibleIndex lookups
	for i, item := range visible {
		l.visibleIndexMap[item] = i
	}

	return visible
}

// getVisibleIndex returns the index of the currently selected item in the visible items list
// Performance optimized: O(1) using visible index map instead of O(n) scanning
func (l *List) getVisibleIndex() int {
	if l.selectedIdx < 0 || l.selectedIdx >= len(l.items) {
		return -1
	}

	selectedItem := l.items[l.selectedIdx]

	// Check if selected item should be visible based on current filters
	if l.hidePaused && selectedItem.Status == session.Paused {
		return -1 // Selected item is filtered out
	}

	// Ensure visible items cache and index map are built
	l.getVisibleItems()

	// O(1) lookup in visible index map
	if visibleIdx, exists := l.visibleIndexMap[selectedItem]; exists {
		return visibleIdx
	}

	return -1 // Selected item not in visible items
}

// UI State Persistence Methods

// loadUIState loads the persisted UI state from the state manager
func (l *List) loadUIState() {
	if l.stateManager == nil {
		return
	}

	uiState := l.stateManager.GetUIState()

	// Load filter state
	l.hidePaused = uiState.HidePaused

	// Load search state
	l.searchMode = uiState.SearchMode
	l.searchQuery = uiState.SearchQuery

	// Load category expansion states
	for category, expanded := range uiState.CategoryExpanded {
		l.groupExpanded[category] = expanded
	}

	// Note: selectedIdx will be restored in the app layer since it needs to validate against current items
	log.InfoLog.Printf("Loaded UI state: hidePaused=%v, searchMode=%v, categories=%d",
		l.hidePaused, l.searchMode, len(uiState.CategoryExpanded))
}

// saveUIState persists the current UI state to the state manager
func (l *List) saveUIState() {
	if l.stateManager == nil {
		return
	}

	// Save individual state changes to avoid overwriting concurrent changes
	if err := l.stateManager.SetHidePaused(l.hidePaused); err != nil {
		log.ErrorLog.Printf("Failed to save hidePaused state: %v", err)
	}

	if err := l.stateManager.SetSearchMode(l.searchMode, l.searchQuery); err != nil {
		log.ErrorLog.Printf("Failed to save search state: %v", err)
	}

	// Save category expansion states
	for category, expanded := range l.groupExpanded {
		if err := l.stateManager.SetCategoryExpanded(category, expanded); err != nil {
			log.ErrorLog.Printf("Failed to save category expansion for %s: %v", category, err)
		}
	}
}

// saveSelectedIndex saves just the selected index with debouncing
func (l *List) saveSelectedIndex() {
	if l.stateManager == nil {
		return
	}

	// Cancel any existing timer
	if l.stateSaveTimer != nil {
		l.stateSaveTimer.Stop()
	}

	// Start new debounced save timer
	l.stateSaveTimer = time.AfterFunc(l.stateSaveDelay, func() {
		if err := l.stateManager.SetSelectedIndex(l.selectedIdx); err != nil {
			log.ErrorLog.Printf("Failed to save selected index: %v", err)
		}
	})
}

// ToggleCategory toggles the expanded state of a category
func (l *List) ToggleCategory(category string) {
	if _, exists := l.groupExpanded[category]; exists {
		wasExpanded := l.groupExpanded[category]
		l.groupExpanded[category] = !wasExpanded

		// If we're collapsing a category with the currently selected instance,
		// or if we're expanding a category and we're on the header,
		// handle selection appropriately
		if len(l.items) > 0 && l.selectedIdx >= 0 && l.selectedIdx < len(l.items) {
			selectedInstance := l.items[l.selectedIdx]
			selectedCategory := selectedInstance.Category
			if selectedCategory == "" {
				selectedCategory = "Uncategorized"
			}

			if selectedCategory == category {
				// Find the instances in this category
				if instances, ok := l.categoryGroups[category]; ok && len(instances) > 0 {
					// Get the first instance (header) of this category
					firstInstance := instances[0]

					// If we're collapsing, always select the header
					if wasExpanded {
						for i, instance := range l.items {
							if instance == firstInstance {
								l.selectedIdx = i
								break
							}
						}
					}

					// If we're expanding and we're already on the header, the header stays selected
					// This is handled implicitly since we don't change selectedIdx
				}
			}
		}

		// Persist the state change
		l.saveUIState()
	}
}

// ExpandCategory expands a category
func (l *List) ExpandCategory(category string) {
	l.groupExpanded[category] = true
}

// CollapseCategory collapses a category
func (l *List) CollapseCategory(category string) {
	wasExpanded := l.groupExpanded[category]
	l.groupExpanded[category] = false

	// If we're collapsing a category with the currently selected instance,
	// move selection to the first instance of the category
	if wasExpanded && len(l.items) > 0 && l.selectedIdx >= 0 && l.selectedIdx < len(l.items) {
		selectedInstance := l.items[l.selectedIdx]
		selectedCategory := selectedInstance.Category
		if selectedCategory == "" {
			selectedCategory = "Uncategorized"
		}

		if selectedCategory == category {
			// Find the first instance of this category
			if instances, ok := l.categoryGroups[category]; ok && len(instances) > 0 {
				firstInstance := instances[0]
				for i, instance := range l.items {
					if instance == firstInstance {
						l.selectedIdx = i
						break
					}
				}
			}
		}
	}
}

// SearchByTitle enables search mode and filters instances using advanced fuzzy search
// This searches across all session fields: title, category, program, branch, path, and working directory
func (l *List) SearchByTitle(query string) {
	if query == "" {
		// Exit search mode if query is empty
		l.searchMode = false
		l.searchResults = nil
		l.invalidateVisibleCache() // Invalidate cache when exiting search
		l.scrollOffset = 0         // Reset scroll when exiting search
		l.ensureSelectedVisible()
		l.saveUIState()
		return
	}

	// Enter search mode
	l.searchMode = true
	l.searchQuery = query
	l.invalidateVisibleCache() // Invalidate cache when search changes
	l.scrollOffset = 0         // Reset scroll when entering search

	// Ensure search index is up-to-date
	if !l.searchIndex.IsIndexValid() {
		l.searchIndex.RebuildIndex(l.items)
	}

	// Perform optimized hybrid fuzzy search
	// This uses closestmatch for fast pre-filtering + sahilm/fuzzy for high-quality ranking
	l.searchResults = l.searchIndex.Search(query, len(l.items)) // No limit, get all matches

	// Ensure the selected item is visible in search results
	l.ensureSelectedVisible()

	// Persist the state change
	l.saveUIState()
}

// ExitSearchMode exits search mode
func (l *List) ExitSearchMode() {
	// Cancel any active streaming search
	if l.searchCancel != nil {
		l.searchCancel()
		l.searchCancel = nil
	}

	l.searchMode = false
	l.searchQuery = ""
	l.searchResults = nil
	l.searchLoading = false
	l.searchStage = ""
	l.scrollOffset = 0 // Reset scroll when exiting search

	// Ensure the selected item is visible after exiting search
	l.ensureSelectedVisible()

	// Persist the state change
	l.saveUIState()
}

// SearchByTitleLive performs live search with debouncing to avoid excessive updates
// This method should be called on every keystroke for instant search feedback
func (l *List) SearchByTitleLive(query string) {
	// If query is empty, exit search mode immediately
	if query == "" {
		l.searchDebouncer.Cancel()
		l.ExitSearchMode()
		return
	}

	// Use debouncer to avoid excessive search operations during fast typing
	l.searchDebouncer.Trigger(func() {
		l.SearchByTitleStreaming(query)
	})
}

// SearchByTitleStreaming performs streaming search with parallel processing and intermediate results
func (l *List) SearchByTitleStreaming(query string) {
	// Cancel any existing search
	if l.searchCancel != nil {
		l.searchCancel()
	}

	if query == "" {
		// Exit search mode if query is empty
		l.searchMode = false
		l.searchResults = nil
		l.searchLoading = false
		l.searchStage = ""
		l.invalidateVisibleCache()
		l.scrollOffset = 0
		l.ensureSelectedVisible()
		l.saveUIState()
		return
	}

	// Enter search mode
	l.searchMode = true
	l.searchQuery = query
	l.searchLoading = true
	l.searchStage = "initializing"
	l.invalidateVisibleCache()
	l.scrollOffset = 0

	// Ensure search index is up-to-date
	if !l.searchIndex.IsIndexValid() {
		l.searchIndex.RebuildIndex(l.items)
	}

	// Create context for this search
	ctx, cancel := context.WithCancel(context.Background())
	l.searchCancel = cancel

	// Start streaming search
	resultStream := l.searchIndex.SearchStream(ctx, query, len(l.items))

	go func() {
		defer func() {
			l.searchLoading = false
			l.searchStage = ""
		}()

		for result := range resultStream {
			select {
			case <-ctx.Done():
				return
			default:
				// Update search results with latest data
				l.searchResults = result.Instances
				l.searchStage = result.Stage
				l.searchLoading = !result.Complete

				// Update UI cache
				l.invalidateVisibleCache()
				l.ensureSelectedVisible()

				// Save state if search is complete
				if result.Complete {
					l.saveUIState()
				}

				// Note: In a real TUI app, we'd need to trigger a screen refresh here
				// This would typically be done by sending a custom message to the BubbleTea update loop
			}
		}
	}()
}

// getSortedCategories returns a sorted list of category names with nested hierarchy
// Parent categories come before their children, and "Uncategorized" is always at the end
func (l *List) getSortedCategories() []string {
	categories := make([]string, 0, len(l.categoryGroups))
	for category := range l.categoryGroups {
		categories = append(categories, category)
	}

	// Separate parent and nested categories
	parents := make([]string, 0)
	nested := make([]string, 0)

	for _, category := range categories {
		if strings.Contains(category, "/") {
			nested = append(nested, category)
		} else {
			parents = append(parents, category)
		}
	}

	// Sort parents alphabetically (except "Uncategorized")
	sort.Slice(parents, func(i, j int) bool {
		if parents[i] == "Uncategorized" {
			return false
		}
		if parents[j] == "Uncategorized" {
			return true
		}
		return parents[i] < parents[j]
	})

	// Sort nested categories by parent, then by child
	sort.Slice(nested, func(i, j int) bool {
		partsI := strings.Split(nested[i], "/")
		partsJ := strings.Split(nested[j], "/")

		// First compare parent categories
		if partsI[0] != partsJ[0] {
			return partsI[0] < partsJ[0]
		}

		// If same parent, compare child categories
		return partsI[1] < partsJ[1]
	})

	// Build final order: parents first, then their nested children
	result := make([]string, 0, len(categories))
	uncategorized := ""

	for _, parent := range parents {
		if parent == "Uncategorized" {
			uncategorized = parent
			continue
		}

		// Add parent category if it has instances
		result = append(result, parent)

		// Add nested categories for this parent
		for _, category := range nested {
			if strings.HasPrefix(category, parent+"/") {
				result = append(result, category)
			}
		}
	}

	// Add "Uncategorized" at the end if it exists
	if uncategorized != "" {
		result = append(result, uncategorized)
	}

	return result
}

// isHeaderSelected returns true if the current selection is on a category header
func (l *List) isHeaderSelected() bool {
	// If no selection or no items, can't be on header
	if l.selectedIdx < 0 || l.selectedIdx >= len(l.items) || len(l.items) == 0 {
		return false
	}

	// Get the current category
	selectedInstance := l.items[l.selectedIdx]
	selectedCategory := selectedInstance.Category
	if selectedCategory == "" {
		selectedCategory = "Uncategorized"
	}

	// Check if this instance is the first in its category AND the category is collapsed
	if instances, ok := l.categoryGroups[selectedCategory]; ok && len(instances) > 0 {
		firstInstance := instances[0]
		if selectedInstance == firstInstance {
			// The instance is visually treated as a header when the category is collapsed
			return !l.groupExpanded[selectedCategory]
		}
	}

	return false
}

// measureActualContentSize measures the actual rendered size of content elements
func (l *List) measureActualContentSize() {
	if len(l.items) == 0 {
		return
	}

	// Measure actual item height by rendering a sample item
	sampleItem := l.items[0]
	renderedItem := l.renderer.Render(sampleItem, 1, false, true)
	l.actualItemHeight = strings.Count(renderedItem, "\n") + 1 // +1 for the content itself

	// Add spacing between items (the "\n\n" we add between items)
	l.actualItemHeight += 2

	// Measure actual category header height
	categoryHeader := categoryHeaderStyle.Render("▼ Sample Category (1)")
	l.actualCategoryHeight = strings.Count(categoryHeader, "\n") + 1 // +1 for the content itself
	l.actualCategoryHeight += 1 // Add spacing after header

	l.sizeMeasurementValid = true

	log.InfoLog.Printf("Measured content sizes: item=%d lines, category=%d lines",
		l.actualItemHeight, l.actualCategoryHeight)
}

// invalidateContentSizeMeasurement marks size measurements as invalid when content changes
func (l *List) invalidateContentSizeMeasurement() {
	l.sizeMeasurementValid = false
}

// calculateMaxVisibleItems calculates how many items can fit in the available screen height
func (l *List) calculateMaxVisibleItems() int {
	// Check if we can use cached result
	currentCategories := len(l.categoryGroups)
	if l.maxVisibleCacheValid && l.lastCachedHeight == l.height && l.lastCachedCategories == currentCategories {
		return l.cachedMaxVisible
	}

	// Calculate fresh result
	// Be much more conservative to prevent overflow
	// Account for title area (4 lines) and generous padding
	titleLines := 4
	padding := 6 // Extra conservative padding for category headers and spacing
	availableHeight := l.height - titleLines - padding

	// Each session item takes 4 lines (title + branch + 2 spacing lines)
	// Based on actual rendering: lipgloss.JoinVertical + "\n\n" between items
	linesPerItem := 4

	// Reserve space for potential category headers
	// This is a fixed conservative estimate rather than dynamic calculation
	categoryHeaderReserve := 8 // Reserve space for up to 4 category headers
	availableHeight -= categoryHeaderReserve

	// Be very conservative with the calculation to prevent overflow
	maxItems := availableHeight / linesPerItem
	if maxItems < 1 {
		maxItems = 1
	}

	// Much lower cap to prevent content going beyond visible area
	if maxItems > 20 {
		maxItems = 20
	}

	// Cache the result
	l.cachedMaxVisible = maxItems
	l.maxVisibleCacheValid = true
	l.lastCachedHeight = l.height
	l.lastCachedCategories = currentCategories

	// Rate-limited debug logging to understand viewport calculations (avoid spam)
	currentViewport := fmt.Sprintf("height=%d, available=%d, maxItems=%d, categories=%d",
		l.height, availableHeight, maxItems, currentCategories)

	// Only log if viewport changed OR it's been more than 5 seconds since last log
	now := time.Now()
	if l.lastLoggedViewport != currentViewport || now.Sub(l.lastViewportLogTime) > 5*time.Second {
		log.InfoLog.Printf("Viewport calculation: %s", currentViewport)
		l.lastLoggedViewport = currentViewport
		l.lastViewportLogTime = now
	}

	return maxItems
}

// ensureSelectedVisible adjusts scroll offset to ensure the selected item is visible
func (l *List) ensureSelectedVisible() {
	visibleItems := l.getVisibleItems()
	if len(visibleItems) == 0 {
		return
	}

	// Find the position of the selected item in the visible items list
	selectedVisibleIdx := l.getVisibleIndex()
	if selectedVisibleIdx == -1 {
		// Selected item is not in visible items, reset scroll
		l.scrollOffset = 0
		return
	}

	maxVisible := l.calculateMaxVisibleItems()

	// If selected item is above the visible window, scroll up
	if selectedVisibleIdx < l.scrollOffset {
		l.scrollOffset = selectedVisibleIdx
	}

	// If selected item is below the visible window, scroll down
	if selectedVisibleIdx >= l.scrollOffset+maxVisible {
		l.scrollOffset = selectedVisibleIdx - maxVisible + 1
		if l.scrollOffset < 0 {
			l.scrollOffset = 0
		}
	}

	// Ensure scroll offset doesn't go beyond the list
	if l.scrollOffset >= len(visibleItems) {
		l.scrollOffset = len(visibleItems) - 1
		if l.scrollOffset < 0 {
			l.scrollOffset = 0
		}
	}
}

// getVisibleWindow returns the slice of visible items that should be rendered
func (l *List) getVisibleWindow() []*session.Instance {
	visibleItems := l.getVisibleItems()
	if len(visibleItems) == 0 {
		return nil
	}

	maxVisible := l.calculateMaxVisibleItems()
	start := l.scrollOffset
	end := start + maxVisible

	if start >= len(visibleItems) {
		start = len(visibleItems) - 1
		if start < 0 {
			start = 0
		}
	}

	if end > len(visibleItems) {
		end = len(visibleItems)
	}

	if start >= end {
		return visibleItems[start : start+1]
	}

	return visibleItems[start:end]
}

// getScrollIndicator returns a string indicating scroll position and filter status
func (l *List) getScrollIndicator() string {
	visibleItems := l.getVisibleItems()
	totalAllItems := len(l.items)

	// Show filter status even if no scrolling needed
	if len(visibleItems) == 0 {
		if totalAllItems > 0 {
			return " [0/" + fmt.Sprintf("%d]", totalAllItems)
		}
		return ""
	}

	maxVisible := l.calculateMaxVisibleItems()

	// If filtering is active, always show counts
	if l.searchMode || l.hidePaused {
		if len(visibleItems) <= maxVisible {
			// All visible items fit, but show filter status
			if len(visibleItems) < totalAllItems {
				return fmt.Sprintf(" [%d/%d]", len(visibleItems), totalAllItems)
			}
		} else {
			// Scrolling needed with filtering
			visibleStart := l.scrollOffset + 1
			visibleEnd := l.scrollOffset + maxVisible
			if visibleEnd > len(visibleItems) {
				visibleEnd = len(visibleItems)
			}
			return fmt.Sprintf(" [%d-%d/%d of %d]", visibleStart, visibleEnd, len(visibleItems), totalAllItems)
		}
	}

	// Normal scrolling without filters
	if len(visibleItems) <= maxVisible {
		return "" // All items fit, no indicator needed
	}

	visibleStart := l.scrollOffset + 1
	visibleEnd := l.scrollOffset + maxVisible
	if visibleEnd > len(visibleItems) {
		visibleEnd = len(visibleItems)
	}

	return fmt.Sprintf(" [%d-%d/%d]", visibleStart, visibleEnd, len(visibleItems))
}

// renderVisibleItems renders only the items in the visible window
func (l *List) renderVisibleItems(b *strings.Builder, visibleWindow []*session.Instance) {
	if len(visibleWindow) == 0 {
		// Use pre-computed style to avoid repeated lipgloss.NewStyle() calls
		b.WriteString(noSessionsStyle.Render("No sessions available"))
		return
	}

	// In search mode, render search results from visible window
	if l.searchMode {
		for i, item := range visibleWindow {
			// visibleWindow already contains only search results, no need to double-check
			globalIdx := l.findGlobalIndex(item)
			if globalIdx >= 0 {
				// Use the actual global index + 1 for numbering, not scroll offset
				displayNumber := globalIdx + 1
				b.WriteString(l.renderer.Render(item, displayNumber, globalIdx == l.selectedIdx, true))
				if i != len(visibleWindow)-1 {
					b.WriteString("\n\n")
				}
			}
		}
		return
	}

	// Normal mode: render visible items with category awareness
	// Create a map to track which categories we need to show (items visible in window)
	categoriesInWindow := make(map[string][]*session.Instance)
	for _, item := range visibleWindow {
		category := item.Category
		if category == "" {
			category = "Uncategorized"
		}
		categoriesInWindow[category] = append(categoriesInWindow[category], item)
	}

	// Get only categories that have items in the visible window
	categories := make([]string, 0, len(categoriesInWindow))
	for category := range categoriesInWindow {
		// Only include categories that have items in the visible window
		if len(categoriesInWindow[category]) > 0 {
			categories = append(categories, category)
		}
	}
	sort.Slice(categories, func(i, j int) bool {
		if categories[i] == "Uncategorized" {
			return false
		}
		if categories[j] == "Uncategorized" {
			return true
		}
		return categories[i] < categories[j]
	})

	// Render categories and their visible items
	firstCategory := true
	renderedCount := 0

	for _, category := range categories {
		instances := categoriesInWindow[category]
		// Get the total instances in this category (not just visible window)
		totalInstances := l.categoryGroups[category]

		// Skip categories that have no instances at all
		if len(totalInstances) == 0 {
			continue
		}

		// Add spacing between categories
		if !firstCategory {
			b.WriteString("\n")
		} else {
			firstCategory = false
		}

		// Check if this category contains the selected instance
		isSelectedCategory := false
		if l.selectedIdx >= 0 && l.selectedIdx < len(l.items) {
			selectedInstance := l.items[l.selectedIdx]
			selectedCategory := selectedInstance.Category
			if selectedCategory == "" {
				selectedCategory = "Uncategorized"
			}
			if selectedCategory == category {
				isSelectedCategory = true
			}
		}

		// Always show category headers for categories that exist
		{
			// Render category header
			icon := "▼ "
			iconStyle := expandedIconStyle
			if !l.groupExpanded[category] {
				icon = "► "
				iconStyle = collapsedIconStyle
			}

			// Get total count for category (not just visible)
			totalInCategory := len(l.categoryGroups[category])

			// Handle nested category display with indentation
			displayName := category
			indent := ""
			if strings.Contains(category, "/") {
				// Nested category - show only the child part with indentation
				parts := strings.Split(category, "/")
				if len(parts) >= 2 {
					displayName = parts[len(parts)-1] // Last part (child category)
					indent = "  "                     // Indent nested categories
				}
			}

			categoryHeader := fmt.Sprintf("%s%s%s (%d)", indent, iconStyle.Render(icon), displayName, totalInCategory)

			// Use selected style if this is the selected category header
			isHeaderSelected := isSelectedCategory && !l.groupExpanded[category]
			if isHeaderSelected {
				b.WriteString(categoryHeaderSelectedStyle.Render(categoryHeader))
			} else {
				b.WriteString(categoryHeaderStyle.Render(categoryHeader))
			}
			b.WriteString("\n")
		}

		// Render instances if category is expanded
		if l.groupExpanded[category] {
			for i, item := range instances {
				globalIdx := l.findGlobalIndex(item)
				if globalIdx >= 0 {
					// Use the actual global index + 1 for numbering
					displayNumber := globalIdx + 1
					b.WriteString(l.renderer.Render(item, displayNumber, globalIdx == l.selectedIdx, true))
					if i != len(instances)-1 || renderedCount < len(visibleWindow)-1 {
						b.WriteString("\n\n")
					}
					renderedCount++
				}
			}
		}
	}
}

// findGlobalIndex finds the global index of an instance in the items list
func (l *List) findGlobalIndex(target *session.Instance) int {
	if idx, exists := l.instanceToIndex[target]; exists {
		return idx
	}
	return -1
}

// ClearAllFilters clears all active filters and returns to the default view
func (l *List) ClearAllFilters() {
	// Clear search mode
	l.searchMode = false
	l.searchQuery = ""
	l.searchResults = nil

	// Reset paused filter to default (show all)
	l.hidePaused = false

	// Reset scroll position
	l.scrollOffset = 0

	// Re-organize with new filter settings
	l.OrganizeByCategory()

	// Ensure selected item is visible
	l.ensureSelectedVisible()

	// Persist all state changes
	l.saveUIState()

	log.InfoLog.Printf("Cleared all filters and search")
}

// GetSearchState returns the current search mode and query
func (l *List) GetSearchState() (bool, string) {
	return l.searchMode, l.searchQuery
}

// GetSearchStatus returns the current search loading state and stage
func (l *List) GetSearchStatus() (loading bool, stage string) {
	return l.searchLoading, l.searchStage
}

// SetReviewQueue sets the review queue for the list
func (l *List) SetReviewQueue(queue *session.ReviewQueue) {
	l.reviewQueue = queue
}

// GetReviewQueue returns the review queue
func (l *List) GetReviewQueue() *session.ReviewQueue {
	return l.reviewQueue
}
