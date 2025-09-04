package ui

import (
	"claude-squad/config"
	"claude-squad/log"
	"claude-squad/session"
	"errors"
	"fmt"
	"sort"
	"strings"

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
	Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#888888"})

var needsApprovalStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#ffaa00"))

var titleStyle = lipgloss.NewStyle().
	Padding(1, 1, 0, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#dddddd"})

var listDescStyle = lipgloss.NewStyle().
	Padding(0, 1, 1, 1).
	Foreground(lipgloss.AdaptiveColor{Light: "#A49FA5", Dark: "#777777"})

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

	// State management for persistence
	stateManager *config.State // Reference to state manager for persistence

	// Scrolling support
	scrollOffset int // Index of the first visible item
}

func NewList(spinner *spinner.Model, autoYes bool, stateManager *config.State) *List {
	l := &List{
		items:          []*session.Instance{},
		renderer:       &InstanceRenderer{spinner: spinner},
		repos:          make(map[string]int),
		autoyes:        autoYes,
		categoryGroups: make(map[string][]*session.Instance),
		groupExpanded:  make(map[string]bool),
		searchMode:     false,
		searchResults:  []*session.Instance{},
		hidePaused:     false,
		stateManager:   stateManager,
	}

	// Load persisted UI state if available
	l.loadUIState()

	return l
}

// SetSize sets the height and width of the list.
func (l *List) SetSize(width, height int) {
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
			err = errors.Join(
				err, fmt.Errorf("could not set preview size for instance %d: %v", i, innerErr))
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
			repoName, err := i.RepoName()
			if err != nil {
				// Log at warning level but don't break rendering
				log.WarningLog.Printf("could not get repo name in instance renderer: %v", err)
			} else {
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

func (l *List) String() string {
	// Build dynamic title with filter status
	titleText := " Instances"
	var filters []string

	// Add search filter info
	if l.searchMode && l.searchQuery != "" {
		filters = append(filters, fmt.Sprintf("🔍 %s", l.searchQuery))
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
	titleWidth := AdjustPreviewWidth(l.width) + 2
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

	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
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
func (l *List) OrganizeByCategory() {
	// Reset category groups
	l.categoryGroups = make(map[string][]*session.Instance)

	// Group instances by category
	for _, instance := range l.items {
		// Skip paused sessions if hidePaused is true
		if l.hidePaused && instance.Status == session.Paused {
			continue
		}

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
	}
}

// TogglePausedFilter toggles whether paused sessions are hidden
func (l *List) TogglePausedFilter() {
	l.hidePaused = !l.hidePaused
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
	// If in search mode, return search results (already filtered)
	if l.searchMode && len(l.searchResults) > 0 {
		var visible []*session.Instance
		for _, item := range l.searchResults {
			if l.hidePaused && item.Status == session.Paused {
				continue
			}
			visible = append(visible, item)
		}
		return visible
	}

	// Normal mode: filter items based on hidePaused
	var visible []*session.Instance
	for _, item := range l.items {
		if l.hidePaused && item.Status == session.Paused {
			continue
		}
		visible = append(visible, item)
	}
	return visible
}

// getVisibleIndex returns the index of the currently selected item in the visible items list
func (l *List) getVisibleIndex() int {
	if l.selectedIdx < 0 || l.selectedIdx >= len(l.items) {
		return -1
	}

	selectedItem := l.items[l.selectedIdx]
	visibleItems := l.getVisibleItems()

	for i, item := range visibleItems {
		if item == selectedItem {
			return i
		}
	}

	return -1
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

// saveSelectedIndex saves just the selected index
func (l *List) saveSelectedIndex() {
	if l.stateManager == nil {
		return
	}

	if err := l.stateManager.SetSelectedIndex(l.selectedIdx); err != nil {
		log.ErrorLog.Printf("Failed to save selected index: %v", err)
	}
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

// SearchByTitle enables search mode and filters instances by title
func (l *List) SearchByTitle(query string) {
	if query == "" {
		// Exit search mode if query is empty
		l.searchMode = false
		l.searchResults = nil
		l.scrollOffset = 0 // Reset scroll when exiting search
		l.ensureSelectedVisible()
		l.saveUIState()
		return
	}

	// Enter search mode
	l.searchMode = true
	l.searchQuery = query
	l.scrollOffset = 0 // Reset scroll when entering search

	// Convert query to lowercase for case-insensitive matching
	query = strings.ToLower(query)

	// Filter instances by title
	l.searchResults = make([]*session.Instance, 0)
	for _, instance := range l.items {
		title := strings.ToLower(instance.Title)
		if strings.Contains(title, query) {
			l.searchResults = append(l.searchResults, instance)
		}
	}

	// Ensure the selected item is visible in search results
	l.ensureSelectedVisible()

	// Persist the state change
	l.saveUIState()
}

// ExitSearchMode exits search mode
func (l *List) ExitSearchMode() {
	l.searchMode = false
	l.searchQuery = ""
	l.searchResults = nil
	l.scrollOffset = 0 // Reset scroll when exiting search

	// Ensure the selected item is visible after exiting search
	l.ensureSelectedVisible()

	// Persist the state change
	l.saveUIState()
}

// getSortedCategories returns a sorted list of category names
// with "Uncategorized" always at the end
func (l *List) getSortedCategories() []string {
	categories := make([]string, 0, len(l.categoryGroups))
	for category := range l.categoryGroups {
		categories = append(categories, category)
	}

	// Sort categories alphabetically with "Uncategorized" at the end
	sort.Slice(categories, func(i, j int) bool {
		if categories[i] == "Uncategorized" {
			return false
		}
		if categories[j] == "Uncategorized" {
			return true
		}
		return categories[i] < categories[j]
	})

	return categories
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

// calculateMaxVisibleItems calculates how many items can fit in the available screen height
func (l *List) calculateMaxVisibleItems() int {
	// Account for title area (4 lines) and some padding
	titleLines := 4
	padding := 2
	availableHeight := l.height - titleLines - padding

	// Each session item takes 3 lines (title + branch + spacing)
	linesPerItem := 3

	// Estimate available space conservatively
	// We'll refine this during actual rendering
	maxItems := availableHeight / linesPerItem
	if maxItems < 1 {
		maxItems = 1
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
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("No sessions available"))
		return
	}

	// In search mode, render search results from visible window
	if l.searchMode {
		for i, item := range visibleWindow {
			// visibleWindow already contains only search results, no need to double-check
			globalIdx := l.findGlobalIndex(item)
			if globalIdx >= 0 {
				b.WriteString(l.renderer.Render(item, l.scrollOffset+i+1, globalIdx == l.selectedIdx, true))
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

	// Get ALL existing categories from categoryGroups, not just those in window
	categories := make([]string, 0, len(l.categoryGroups))
	for category := range l.categoryGroups {
		categories = append(categories, category)
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
			categoryHeader := fmt.Sprintf("%s%s (%d)", iconStyle.Render(icon), category, totalInCategory)

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
					b.WriteString(l.renderer.Render(item, renderedCount+1, globalIdx == l.selectedIdx, true))
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
	for idx, instance := range l.items {
		if instance == target {
			return idx
		}
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
