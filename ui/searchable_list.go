package ui

import (
	"claude-squad/session"
	"claude-squad/ui/fuzzy"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// SessionSearchItem implements fuzzy.SearchItem for session instances
type SessionSearchItem struct {
	Instance *session.Instance
}

// GetSearchText returns the text used for fuzzy matching
func (s SessionSearchItem) GetSearchText() string {
	// Search across multiple fields to maximize findability
	searchTexts := []string{
		s.Instance.Title,
		s.Instance.Branch,
		s.Instance.Category,
	}

	// Add tags if available
	if s.Instance.Tags != nil && len(s.Instance.Tags) > 0 {
		searchTexts = append(searchTexts, strings.Join(s.Instance.Tags, " "))
	}

	// Combine all searchable text
	return strings.Join(searchTexts, " ")
}

// GetDisplayText returns the text to display in the UI
func (s SessionSearchItem) GetDisplayText() string {
	return s.Instance.Title
}

// GetID returns a unique identifier for the item
func (s SessionSearchItem) GetID() string {
	return s.Instance.Title
}

// SearchableList extends List with search capability
type SearchableList struct {
	*List
	searcher        *fuzzy.FuzzySearcher
	searchMode      bool
	searchQuery     string
	searchResults   []fuzzy.SearchResult
	categories      map[string][]*session.Instance  // Map of category to instances
	expandedCategories map[string]bool             // Track which categories are expanded
	tagFilter       *TagFilter                     // Tag-based filtering
}

// NewSearchableList creates a new searchable list component
func NewSearchableList(spinner *spinner.Model, autoYes bool) *SearchableList {
	baseList := NewList(spinner, autoYes)
	fuzzyConfig := fuzzy.DefaultConfig()
	
	return &SearchableList{
		List:              baseList,
		searcher:          fuzzy.NewFuzzySearcher(fuzzyConfig),
		searchMode:        false,
		searchQuery:       "",
		categories:        make(map[string][]*session.Instance),
		expandedCategories: make(map[string]bool),
		tagFilter:         NewTagFilter(),
	}
}

// EnterSearchMode activates search mode
func (l *SearchableList) EnterSearchMode() {
	l.searchMode = true
	l.searchQuery = ""
	l.updateSearchItems()
}

// ExitSearchMode deactivates search mode
func (l *SearchableList) ExitSearchMode() {
	l.searchMode = false
	l.searchQuery = ""
}

// IsInSearchMode returns whether the list is in search mode
func (l *SearchableList) IsInSearchMode() bool {
	return l.searchMode
}

// updateSearchItems updates the searcher with the current items
func (l *SearchableList) updateSearchItems() {
	items := make([]fuzzy.SearchItem, len(l.items))
	for i, instance := range l.items {
		items[i] = SessionSearchItem{Instance: instance}
	}
	l.searcher.SetItems(items)
}

// UpdateSearchQuery updates the search query and triggers a search
func (l *SearchableList) UpdateSearchQuery(query string) {
	l.searchQuery = query
	l.searcher.SetQuery(query, func() {
		l.searchResults = l.searcher.GetResults()
	})
}

// GetInstanceByCategory returns all instances grouped by category
func (l *SearchableList) GetInstanceByCategory() map[string][]*session.Instance {
	categories := make(map[string][]*session.Instance)
	
	// Default category for uncategorized items
	defaultCategory := "Uncategorized"
	
	for _, instance := range l.items {
		category := instance.Category
		if category == "" {
			category = defaultCategory
		}
		
		if _, exists := categories[category]; !exists {
			categories[category] = []*session.Instance{}
		}
		
		categories[category] = append(categories[category], instance)
	}
	
	return categories
}

// ToggleCategoryExpanded toggles the expanded state of a category
func (l *SearchableList) ToggleCategoryExpanded(category string) {
	if _, exists := l.expandedCategories[category]; !exists {
		l.expandedCategories[category] = true // Default to expanded
	} else {
		l.expandedCategories[category] = !l.expandedCategories[category]
	}
}

// IsCategoryExpanded checks if a category is expanded
func (l *SearchableList) IsCategoryExpanded(category string) bool {
	if expanded, exists := l.expandedCategories[category]; exists {
		return expanded
	}
	return true // Default to expanded
}

// ApplyTagFilter applies tag filtering to the instance list
func (l *SearchableList) ApplyTagFilter() {
	// Update available tags based on current instances
	l.tagFilter.UpdateAvailableTags(l.items)
}

// FilterInstancesByTag filters instances by the active tag
func (l *SearchableList) FilterInstancesByTag(instances []*session.Instance) []*session.Instance {
	return l.tagFilter.ApplyFilter(instances)
}

// SetActiveTag sets the active tag filter
func (l *SearchableList) SetActiveTag(tag string) {
	l.tagFilter.SetActiveTag(tag)
}

// ClearTagFilter clears the active tag filter
func (l *SearchableList) ClearTagFilter() {
	l.tagFilter.ClearTagFilter()
}

// String renders the SearchableList component
func (l *SearchableList) String() string {
	if l.searchMode {
		return l.renderSearchMode()
	}
	return l.renderCategorizedList()
}

// renderSearchMode renders the list in search mode with results
func (l *SearchableList) renderSearchMode() string {
	const titleText = " Search Results "
	
	// Write the title and search query
	var b strings.Builder
	b.WriteString("\n\n")
	
	// Create search header
	titleWidth := AdjustPreviewWidth(l.width) + 2
	searchHeader := lipgloss.Place(
		titleWidth, 1, lipgloss.Left, lipgloss.Bottom, 
		mainTitle.Render(titleText + fmt.Sprintf("(%d)", len(l.searchResults))))
	
	b.WriteString(searchHeader)
	b.WriteString("\n")
	
	queryStyle := lipgloss.NewStyle().Padding(1, 2).Foreground(lipgloss.Color("#888888"))
	b.WriteString(queryStyle.Render("Query: " + l.searchQuery))
	
	b.WriteString("\n\n")
	
	// Show loading indicator or results
	if l.searcher.IsLoading() {
		loadingStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		b.WriteString(loadingStyle.Render(" Searching..."))
	} else if len(l.searchResults) == 0 {
		noResultsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
		b.WriteString(noResultsStyle.Render(" No results found"))
	} else {
		// Render search results with highlighting
		for i, result := range l.searchResults {
			item := result.Item.(SessionSearchItem)
			instance := item.Instance
			
			selected := false
			if i == l.selectedIdx {
				selected = true
			}
			
			// Get highlighted text for result
			b.WriteString(l.renderer.RenderWithHighlights(instance, i+1, selected, true, result.Matches))
			
			if i != len(l.searchResults)-1 {
				b.WriteString("\n\n")
			}
		}
	}
	
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// renderCategorizedList renders the list with sessions grouped by category
func (l *SearchableList) renderCategorizedList() string {
	const titleText = " Instances "
	const autoYesText = " auto-yes "
	const expandedIcon = "▼ "
	const collapsedIcon = "► "

	// First apply tag filtering if active (used when rendering categories)
	
	// Get instances grouped by category
	categories := l.GetInstanceByCategory()
	l.categories = categories
	
	// Write the title.
	var b strings.Builder
	b.WriteString("\n\n")

	// Write title line
	titleWidth := AdjustPreviewWidth(l.width) + 2
	if !l.autoyes {
		b.WriteString(lipgloss.Place(
			titleWidth, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText)))
	} else {
		title := lipgloss.Place(
			titleWidth/2, 1, lipgloss.Left, lipgloss.Bottom, mainTitle.Render(titleText))
		autoYes := lipgloss.Place(
			titleWidth-(titleWidth/2), 1, lipgloss.Right, lipgloss.Bottom, autoYesStyle.Render(autoYesText))
		b.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top, title, autoYes))
	}

	b.WriteString("\n\n")
	
	// Render tag filter if there are tags available
	l.ApplyTagFilter()
	tagFilterView := l.tagFilter.View()
	if tagFilterView != "" {
		b.WriteString(tagFilterView)
		b.WriteString("\n\n")
	}
	
	// Track the global index for selection highlighting
	globalIdx := 0
	
	// Sort categories for consistent display order
	categoryNames := make([]string, 0, len(categories))
	for cat := range categories {
		categoryNames = append(categoryNames, cat)
	}
	// Sort categories alphabetically for consistent display
	sort.Strings(categoryNames)
	
	// Render each category
	for _, category := range categoryNames {
		// Apply tag filtering to the instances in this category
		instances := categories[category]
		filteredInstances := l.FilterInstancesByTag(instances)
		
		if len(filteredInstances) == 0 {
			continue
		}
		
		// Render category header with expand/collapse icon
		isExpanded := l.IsCategoryExpanded(category)
		categoryIcon := collapsedIcon
		if isExpanded {
			categoryIcon = expandedIcon
		}
		
		// Truncate category name if it's too long (max 30 chars)
		displayCategory := category
		if len(category) > 30 {
			displayCategory = category[:27] + "..."
		}
		
		categoryStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#000080", Dark: "#87CEFA"}).
			Padding(0, 0, 0, 1)
		
		b.WriteString(categoryStyle.Render(categoryIcon + displayCategory + 
			fmt.Sprintf(" (%d)", len(filteredInstances))))
		b.WriteString("\n")
		
		// Only render instances if category is expanded
		if isExpanded {
			for idx, instance := range filteredInstances {
				selected := globalIdx == l.selectedIdx
				renderedInstance := l.renderer.Render(instance, globalIdx+1, selected, true)
				b.WriteString(renderedInstance)
				
				if idx != len(filteredInstances)-1 || category != categoryNames[len(categoryNames)-1] {
					b.WriteString("\n\n")
				}
				
				globalIdx++
			}
		} else if category != categoryNames[len(categoryNames)-1] {
			b.WriteString("\n")
		}
	}
	
	return lipgloss.Place(l.width, l.height, lipgloss.Left, lipgloss.Top, b.String())
}

// AddInstance overrides the List method to update search items and expanded categories
func (l *SearchableList) AddInstance(instance *session.Instance) func() {
	finalizer := l.List.AddInstance(instance)
	
	// Default to expand category and update search items
	if instance.Category != "" {
		l.expandedCategories[instance.Category] = true
	}
	
	l.updateSearchItems()
	
	return func() {
		finalizer()
		l.updateSearchItems()
	}
}

// HandleSearchKeyPress handles keyboard input in search mode
func (l *SearchableList) HandleSearchKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyEsc:
		l.ExitSearchMode()
		return true
	case tea.KeyBackspace:
		if len(l.searchQuery) > 0 {
			l.searchQuery = l.searchQuery[:len(l.searchQuery)-1]
			l.UpdateSearchQuery(l.searchQuery)
		}
	case tea.KeySpace:
		l.searchQuery += " "
		l.UpdateSearchQuery(l.searchQuery)
	case tea.KeyRunes:
		l.searchQuery += string(msg.Runes)
		l.UpdateSearchQuery(l.searchQuery)
	case tea.KeyEnter:
		// If results are available, select the current one
		if len(l.searchResults) > 0 && l.selectedIdx < len(l.searchResults) {
			l.ExitSearchMode()
			return true
		}
	}
	return false
}

// Note: RenderWithHighlights is implemented in renderer.go