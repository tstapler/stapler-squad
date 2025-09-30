package ui

import (
	"claude-squad/session"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// OrganizationMode defines how sessions are organized in the UI
type OrganizationMode int

const (
	OrganizeByCategory OrganizationMode = iota
	OrganizeByRepository
	OrganizeFlat
)

// =============================================================================
// Messages - All possible state changes in the list component
// =============================================================================

// ListMessage is the interface that all list-specific messages implement
type ListMessage interface {
	IsListMessage()
}

// Navigation messages
type SelectNextMsg struct{}
type SelectPrevMsg struct{}
type SelectIndexMsg struct{ Index int }
type ScrollToIndexMsg struct{ Index int }

func (SelectNextMsg) IsListMessage()      {}
func (SelectPrevMsg) IsListMessage()      {}
func (SelectIndexMsg) IsListMessage()     {}
func (ScrollToIndexMsg) IsListMessage()   {}

// Data manipulation messages
type AddSessionMsg struct{ Session *session.Instance }
type RemoveSessionMsg struct{ Index int }
type UpdateSessionMsg struct{ Index int; Session *session.Instance }
type SessionsLoadedMsg struct{ Sessions []*session.Instance }

func (AddSessionMsg) IsListMessage()     {}
func (RemoveSessionMsg) IsListMessage()  {}
func (UpdateSessionMsg) IsListMessage()  {}
func (SessionsLoadedMsg) IsListMessage() {}

// UI state messages
type ToggleSearchMsg struct{}
type SetSearchQueryMsg struct{ Query string }
type TogglePausedFilterMsg struct{}
type ToggleGroupMsg struct{ GroupID string }
type ChangeOrganizationMsg struct{ Mode OrganizationMode }
type SetSizeMsg struct{ Width, Height int }

func (ToggleSearchMsg) IsListMessage()        {}
func (SetSearchQueryMsg) IsListMessage()      {}
func (TogglePausedFilterMsg) IsListMessage()  {}
func (ToggleGroupMsg) IsListMessage()         {}
func (ChangeOrganizationMsg) IsListMessage()  {}
func (SetSizeMsg) IsListMessage()             {}

// =============================================================================
// Models - Pure data structures
// =============================================================================

// SessionListModel holds the core business data
type SessionListModel struct {
	sessions         []*session.Instance
	sessionToIndex   map[*session.Instance]int // O(1) lookups
	repoGroups       map[string][]*session.Instance
	categoryGroups   map[string][]*session.Instance
	lastUpdated      time.Time
}

// ListViewState holds all UI-specific state
type ListViewState struct {
	selectedIndex      int
	scrollOffset       int
	searchQuery        string
	searchActive       bool
	hidePaused         bool
	organizationMode   OrganizationMode
	expandedGroups     map[string]bool
	width              int
	height             int
	lastStateChange    time.Time
}

// DisplayItem represents a single item in the rendered view
type DisplayItem struct {
	Session      *session.Instance
	GlobalIndex  int
	DisplayIndex int
	IsHeader     bool
	HeaderText   string
	HeaderLevel  int
	GroupID      string
	Selected     bool
}

// ListDisplayModel holds computed display data
type ListDisplayModel struct {
	visibleItems     []DisplayItem
	totalSessions    int
	filteredCount    int
	scrollIndicator  string
	canScrollUp      bool
	canScrollDown    bool
	maxVisibleItems  int
}

// =============================================================================
// Main Component - Follows BubbleTea Elm Architecture
// =============================================================================

// ListComponentElm is the new Elm-architecture list component
type ListComponentElm struct {
	model       SessionListModel
	viewState   ListViewState
	displayData ListDisplayModel
	services    *ListServices
}

// NewListComponentElm creates a new Elm-architecture list component
func NewListComponentElm(renderer *InstanceRenderer) *ListComponentElm {
	return &ListComponentElm{
		model: SessionListModel{
			sessions:       []*session.Instance{},
			sessionToIndex: make(map[*session.Instance]int),
			repoGroups:     make(map[string][]*session.Instance),
			categoryGroups: make(map[string][]*session.Instance),
			lastUpdated:    time.Now(),
		},
		viewState: ListViewState{
			selectedIndex:    0,
			scrollOffset:     0,
			searchQuery:      "",
			searchActive:     false,
			hidePaused:       false,
			organizationMode: OrganizeByCategory,
			expandedGroups:   make(map[string]bool),
			width:            80,
			height:           24,
			lastStateChange:  time.Now(),
		},
		displayData: ListDisplayModel{
			visibleItems:    []DisplayItem{},
			totalSessions:   0,
			filteredCount:   0,
			scrollIndicator: "",
		},
		services: NewListServices(renderer),
	}
}

// Update handles all state changes following the Elm architecture pattern
func (lc *ListComponentElm) Update(msg tea.Msg) (*ListComponentElm, tea.Cmd) {
	switch msg := msg.(type) {

	// Navigation messages
	case SelectNextMsg:
		return lc.handleSelectNext(), nil
	case SelectPrevMsg:
		return lc.handleSelectPrev(), nil
	case SelectIndexMsg:
		return lc.handleSelectIndex(msg.Index), nil
	case ScrollToIndexMsg:
		return lc.handleScrollToIndex(msg.Index), nil

	// Data messages
	case AddSessionMsg:
		return lc.handleAddSession(msg.Session), nil
	case RemoveSessionMsg:
		return lc.handleRemoveSession(msg.Index), nil
	case UpdateSessionMsg:
		return lc.handleUpdateSession(msg.Index, msg.Session), nil
	case SessionsLoadedMsg:
		return lc.handleSessionsLoaded(msg.Sessions), nil

	// UI state messages
	case ToggleSearchMsg:
		return lc.handleToggleSearch(), nil
	case SetSearchQueryMsg:
		return lc.handleSetSearchQuery(msg.Query), nil
	case TogglePausedFilterMsg:
		return lc.handleTogglePausedFilter(), nil
	case ToggleGroupMsg:
		return lc.handleToggleGroup(msg.GroupID), nil
	case ChangeOrganizationMsg:
		return lc.handleChangeOrganization(msg.Mode), nil
	case SetSizeMsg:
		return lc.handleSetSize(msg.Width, msg.Height), nil
	}

	// No changes needed for this message
	return lc, nil
}

// View renders the current state (pure function)
func (lc *ListComponentElm) View() string {
	// Ensure display data is computed
	lc.displayData = computeDisplayData(lc.model, lc.viewState, lc.services)

	// Build the view based on organization mode
	var content strings.Builder

	// Header with organization mode
	orgMode := "Categories"
	switch lc.viewState.organizationMode {
	case OrganizeByRepository:
		orgMode = "Repositories"
	case OrganizeFlat:
		orgMode = "Flat"
	}

	content.WriteString(fmt.Sprintf("Claude Squad Sessions (%s)", orgMode))
	if lc.viewState.searchActive {
		content.WriteString(fmt.Sprintf(" - Search: %s", lc.viewState.searchQuery))
	}
	if lc.viewState.hidePaused {
		content.WriteString(" - Hide Paused")
	}
	content.WriteString(lc.displayData.scrollIndicator)
	content.WriteString("\n\n")

	// Render display items
	if len(lc.displayData.visibleItems) == 0 {
		content.WriteString("No sessions available\n")
	} else {
		// Show a window of visible items based on scroll offset
		maxVisible := lc.displayData.maxVisibleItems
		start := lc.viewState.scrollOffset
		end := start + maxVisible
		if end > len(lc.displayData.visibleItems) {
			end = len(lc.displayData.visibleItems)
		}

		for i := start; i < end; i++ {
			item := lc.displayData.visibleItems[i]
			line := lc.renderDisplayItem(item)
			content.WriteString(line)
			content.WriteString("\n")
		}

		// Show scroll indicators
		if lc.displayData.canScrollUp {
			content.WriteString("... (more items above)\n")
		}
		if lc.displayData.canScrollDown {
			content.WriteString("... (more items below)\n")
		}
	}

	// Summary
	content.WriteString(fmt.Sprintf("\nShowing %d/%d sessions | Selected: %d",
		lc.displayData.filteredCount, lc.displayData.totalSessions, lc.viewState.selectedIndex))

	return content.String()
}

// renderDisplayItem renders a single display item (header or session)
func (lc *ListComponentElm) renderDisplayItem(item DisplayItem) string {
	if item.IsHeader {
		// Render group header
		indent := strings.Repeat("  ", item.HeaderLevel)
		icon := "▼"
		if !lc.viewState.expandedGroups[item.GroupID] {
			icon = "►"
		}
		marker := ""
		if item.Selected {
			marker = " <--"
		}
		return fmt.Sprintf("%s%s %s%s", indent, icon, item.HeaderText, marker)
	} else {
		// Render session item
		indent := strings.Repeat("  ", item.HeaderLevel)
		marker := ""
		if item.Selected {
			marker = " <-- SELECTED"
		}
		status := ""
		switch item.Session.Status {
		case session.Running:
			status = "[R]"
		case session.Ready:
			status = "[✓]"
		case session.Paused:
			status = "[⏸]"
		case session.NeedsApproval:
			status = "[!]"
		}
		return fmt.Sprintf("%s%d. %s %s %s%s", indent, item.GlobalIndex+1, status, item.Session.Title, item.Session.Branch, marker)
	}
}

// =============================================================================
// Pure State Update Functions
// =============================================================================

// Navigation handlers
func (lc *ListComponentElm) handleSelectNext() *ListComponentElm {
	newViewState := selectNext(lc.model, lc.viewState)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleSelectPrev() *ListComponentElm {
	newViewState := selectPrev(lc.model, lc.viewState)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleSelectIndex(index int) *ListComponentElm {
	newViewState := selectIndex(lc.model, lc.viewState, index)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleScrollToIndex(index int) *ListComponentElm {
	newViewState := scrollToIndex(lc.model, lc.viewState, index)
	return lc.withViewState(newViewState).recomputeDisplay()
}

// Data handlers
func (lc *ListComponentElm) handleAddSession(session *session.Instance) *ListComponentElm {
	newModel := addSession(lc.model, session, lc.services)
	return lc.withModel(newModel).recomputeDisplay()
}

func (lc *ListComponentElm) handleRemoveSession(index int) *ListComponentElm {
	newModel := removeSession(lc.model, index, lc.services)
	// May need to adjust selection if it's out of bounds
	newViewState := validateSelection(newModel, lc.viewState)
	return lc.withModel(newModel).withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleUpdateSession(index int, session *session.Instance) *ListComponentElm {
	newModel := updateSession(lc.model, index, session, lc.services)
	return lc.withModel(newModel).recomputeDisplay()
}

func (lc *ListComponentElm) handleSessionsLoaded(sessions []*session.Instance) *ListComponentElm {
	newModel := loadSessions(lc.model, sessions, lc.services)
	newViewState := validateSelection(newModel, lc.viewState)
	return lc.withModel(newModel).withViewState(newViewState).recomputeDisplay()
}

// UI state handlers
func (lc *ListComponentElm) handleToggleSearch() *ListComponentElm {
	newViewState := toggleSearch(lc.viewState)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleSetSearchQuery(query string) *ListComponentElm {
	newViewState := setSearchQuery(lc.viewState, query)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleTogglePausedFilter() *ListComponentElm {
	newViewState := togglePausedFilter(lc.viewState)
	// Reset selection to first visible item after filter change
	newViewState = resetSelectionToFirst(lc.model, newViewState)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleToggleGroup(groupID string) *ListComponentElm {
	newViewState := toggleGroup(lc.viewState, groupID)
	return lc.withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleChangeOrganization(mode OrganizationMode) *ListComponentElm {
	newViewState := changeOrganization(lc.viewState, mode)
	// Recompute groups when organization changes
	newModel := recomputeGroups(lc.model, mode, lc.services)
	return lc.withModel(newModel).withViewState(newViewState).recomputeDisplay()
}

func (lc *ListComponentElm) handleSetSize(width, height int) *ListComponentElm {
	newViewState := setSize(lc.viewState, width, height)
	return lc.withViewState(newViewState).recomputeDisplay()
}

// =============================================================================
// Helper Methods for Immutable Updates
// =============================================================================

// withModel returns a new component with updated model
func (lc *ListComponentElm) withModel(model SessionListModel) *ListComponentElm {
	return &ListComponentElm{
		model:       model,
		viewState:   lc.viewState,
		displayData: lc.displayData,
		services:    lc.services,
	}
}

// withViewState returns a new component with updated view state
func (lc *ListComponentElm) withViewState(viewState ListViewState) *ListComponentElm {
	return &ListComponentElm{
		model:       lc.model,
		viewState:   viewState,
		displayData: lc.displayData,
		services:    lc.services,
	}
}

// recomputeDisplay updates the display data based on current model and view state
func (lc *ListComponentElm) recomputeDisplay() *ListComponentElm {
	displayData := computeDisplayData(lc.model, lc.viewState, lc.services)
	return &ListComponentElm{
		model:       lc.model,
		viewState:   lc.viewState,
		displayData: displayData,
		services:    lc.services,
	}
}

// =============================================================================
// Pure Functions for State Transformations
// These will be implemented in the next phase
// =============================================================================

// Navigation functions
func selectNext(model SessionListModel, viewState ListViewState) ListViewState {
	visibleItems := getVisibleItemsElm(model, viewState)
	if len(visibleItems) == 0 {
		return viewState
	}

	// Find current selection in visible items
	currentVisibleIdx := findVisibleIndex(model, viewState)
	if currentVisibleIdx == -1 {
		// No valid selection, select first visible item
		return selectFirstVisible(model, viewState)
	}

	// Move to next visible item
	nextVisibleIdx := currentVisibleIdx + 1
	if nextVisibleIdx >= len(visibleItems) {
		// At end, stay at current position
		return viewState
	}

	// Find the global index of the next visible item
	nextItem := visibleItems[nextVisibleIdx]
	for i, session := range model.sessions {
		if session == nextItem {
			newViewState := viewState
			newViewState.selectedIndex = i
			newViewState.lastStateChange = time.Now()
			return ensureSelectionVisible(model, newViewState)
		}
	}

	return viewState
}

func selectPrev(model SessionListModel, viewState ListViewState) ListViewState {
	visibleItems := getVisibleItemsElm(model, viewState)
	if len(visibleItems) == 0 {
		return viewState
	}

	// Find current selection in visible items
	currentVisibleIdx := findVisibleIndex(model, viewState)
	if currentVisibleIdx == -1 {
		// No valid selection, select first visible item
		return selectFirstVisible(model, viewState)
	}

	// Move to previous visible item
	prevVisibleIdx := currentVisibleIdx - 1
	if prevVisibleIdx < 0 {
		// At beginning, stay at current position
		return viewState
	}

	// Find the global index of the previous visible item
	prevItem := visibleItems[prevVisibleIdx]
	for i, session := range model.sessions {
		if session == prevItem {
			newViewState := viewState
			newViewState.selectedIndex = i
			newViewState.lastStateChange = time.Now()
			return ensureSelectionVisible(model, newViewState)
		}
	}

	return viewState
}

func selectIndex(model SessionListModel, viewState ListViewState, index int) ListViewState {
	if index < 0 || index >= len(model.sessions) {
		return viewState
	}

	newViewState := viewState
	newViewState.selectedIndex = index
	newViewState.lastStateChange = time.Now()
	return ensureSelectionVisible(model, newViewState)
}

func scrollToIndex(model SessionListModel, viewState ListViewState, index int) ListViewState {
	visibleItems := getVisibleItemsElm(model, viewState)
	if len(visibleItems) == 0 {
		return viewState
	}

	// Find the visible index of the target item
	targetItem := model.sessions[index]
	for i, item := range visibleItems {
		if item == targetItem {
			newViewState := viewState
			newViewState.scrollOffset = i
			newViewState.lastStateChange = time.Now()
			return newViewState
		}
	}

	return viewState
}

// Data functions
func addSession(model SessionListModel, sess *session.Instance, services *ListServices) SessionListModel {
	newModel := model
	newModel.sessions = append(model.sessions, sess)

	// Rebuild session-to-index map
	newModel.sessionToIndex = make(map[*session.Instance]int, len(newModel.sessions))
	for i, s := range newModel.sessions {
		newModel.sessionToIndex[s] = i
	}

	// Update groups based on organization mode
	newModel = rebuildGroups(newModel, services)
	newModel.lastUpdated = time.Now()

	return newModel
}

func removeSession(model SessionListModel, index int, services *ListServices) SessionListModel {
	if index < 0 || index >= len(model.sessions) {
		return model
	}

	newModel := model
	// Remove session from slice
	newModel.sessions = append(model.sessions[:index], model.sessions[index+1:]...)

	// Rebuild session-to-index map
	newModel.sessionToIndex = make(map[*session.Instance]int, len(newModel.sessions))
	for i, s := range newModel.sessions {
		newModel.sessionToIndex[s] = i
	}

	// Update groups
	newModel = rebuildGroups(newModel, services)
	newModel.lastUpdated = time.Now()

	return newModel
}

func updateSession(model SessionListModel, index int, sess *session.Instance, services *ListServices) SessionListModel {
	if index < 0 || index >= len(model.sessions) {
		return model
	}

	newModel := model
	newModel.sessions = make([]*session.Instance, len(model.sessions))
	copy(newModel.sessions, model.sessions)
	newModel.sessions[index] = sess

	// Update session-to-index map
	newModel.sessionToIndex = make(map[*session.Instance]int, len(newModel.sessions))
	for i, s := range newModel.sessions {
		newModel.sessionToIndex[s] = i
	}

	// Update groups
	newModel = rebuildGroups(newModel, services)
	newModel.lastUpdated = time.Now()

	return newModel
}

func loadSessions(model SessionListModel, sessions []*session.Instance, services *ListServices) SessionListModel {
	newModel := model
	newModel.sessions = make([]*session.Instance, len(sessions))
	copy(newModel.sessions, sessions)

	// Build session-to-index map
	newModel.sessionToIndex = make(map[*session.Instance]int, len(sessions))
	for i, s := range sessions {
		newModel.sessionToIndex[s] = i
	}

	// Update groups
	newModel = rebuildGroups(newModel, services)
	newModel.lastUpdated = time.Now()

	return newModel
}

// UI state functions
func toggleSearch(viewState ListViewState) ListViewState {
	newViewState := viewState
	newViewState.searchActive = !viewState.searchActive
	if !newViewState.searchActive {
		// Clear search query when disabling search
		newViewState.searchQuery = ""
	}
	newViewState.lastStateChange = time.Now()
	return newViewState
}

func setSearchQuery(viewState ListViewState, query string) ListViewState {
	newViewState := viewState
	newViewState.searchQuery = query
	newViewState.searchActive = query != ""
	// Reset scroll when search changes
	newViewState.scrollOffset = 0
	newViewState.lastStateChange = time.Now()
	return newViewState
}

func togglePausedFilter(viewState ListViewState) ListViewState {
	newViewState := viewState
	newViewState.hidePaused = !viewState.hidePaused
	// Reset scroll when filter changes
	newViewState.scrollOffset = 0
	newViewState.lastStateChange = time.Now()
	return newViewState
}

func toggleGroup(viewState ListViewState, groupID string) ListViewState {
	newViewState := viewState

	// Copy the expanded groups map
	newViewState.expandedGroups = make(map[string]bool)
	for k, v := range viewState.expandedGroups {
		newViewState.expandedGroups[k] = v
	}

	// Toggle the specific group
	newViewState.expandedGroups[groupID] = !viewState.expandedGroups[groupID]
	newViewState.lastStateChange = time.Now()
	return newViewState
}

func changeOrganization(viewState ListViewState, mode OrganizationMode) ListViewState {
	newViewState := viewState
	newViewState.organizationMode = mode
	// Reset scroll when organization changes
	newViewState.scrollOffset = 0
	// Reset expanded groups when organization changes
	newViewState.expandedGroups = make(map[string]bool)
	newViewState.lastStateChange = time.Now()
	return newViewState
}

func setSize(viewState ListViewState, width, height int) ListViewState {
	newViewState := viewState
	newViewState.width = width
	newViewState.height = height
	newViewState.lastStateChange = time.Now()
	return newViewState
}

// Validation and computation functions
func validateSelection(model SessionListModel, viewState ListViewState) ListViewState {
	if len(model.sessions) == 0 {
		// No sessions, reset selection
		newViewState := viewState
		newViewState.selectedIndex = -1
		newViewState.scrollOffset = 0
		return newViewState
	}

	if viewState.selectedIndex < 0 || viewState.selectedIndex >= len(model.sessions) {
		// Selection out of bounds, select first visible item
		return selectFirstVisible(model, viewState)
	}

	// Check if selected item is visible with current filters
	selectedVisibleIdx := findVisibleIndex(model, viewState)
	if selectedVisibleIdx == -1 {
		// Selected item not visible, select first visible item
		return selectFirstVisible(model, viewState)
	}

	return viewState
}

func resetSelectionToFirst(model SessionListModel, viewState ListViewState) ListViewState {
	return selectFirstVisible(model, viewState)
}

func recomputeGroups(model SessionListModel, mode OrganizationMode, services *ListServices) SessionListModel {
	return rebuildGroups(model, services)
}

func rebuildGroups(model SessionListModel, services *ListServices) SessionListModel {
	newModel := model

	// Reset groups
	newModel.repoGroups = make(map[string][]*session.Instance)
	newModel.categoryGroups = make(map[string][]*session.Instance)

	// Group by repository
	for _, sess := range model.sessions {
		// Try to get repository name using repository service
		repoName := services.Repository.GetRepositoryName(sess)
		if repoName == "" {
			repoName = "Unknown Repository"
		}
		newModel.repoGroups[repoName] = append(newModel.repoGroups[repoName], sess)
	}

	// Group by category
	for _, sess := range model.sessions {
		category := sess.Category
		if category == "" {
			category = "Uncategorized"
		}
		newModel.categoryGroups[category] = append(newModel.categoryGroups[category], sess)
	}

	return newModel
}

func computeDisplayData(model SessionListModel, viewState ListViewState, services *ListServices) ListDisplayModel {
	visibleItems := getVisibleItemsElm(model, viewState)
	displayItems := make([]DisplayItem, 0)

	switch viewState.organizationMode {
	case OrganizeByRepository:
		displayItems = buildRepositoryHierarchy(model, viewState, visibleItems, services)
	case OrganizeByCategory:
		displayItems = buildCategoryHierarchy(model, viewState, visibleItems)
	case OrganizeFlat:
		displayItems = buildFlatList(model, viewState, visibleItems)
	default:
		displayItems = buildCategoryHierarchy(model, viewState, visibleItems)
	}

	// Calculate scroll indicators
	maxVisible := calculateMaxVisibleElm(viewState)
	scrollIndicator := ""
	if len(visibleItems) > maxVisible {
		start := viewState.scrollOffset + 1
		end := viewState.scrollOffset + maxVisible
		if end > len(visibleItems) {
			end = len(visibleItems)
		}
		scrollIndicator = fmt.Sprintf(" [%d-%d/%d]", start, end, len(visibleItems))
	}

	return ListDisplayModel{
		visibleItems:    displayItems,
		totalSessions:   len(model.sessions),
		filteredCount:   len(visibleItems),
		scrollIndicator: scrollIndicator,
		canScrollUp:     viewState.scrollOffset > 0,
		canScrollDown:   viewState.scrollOffset+maxVisible < len(visibleItems),
		maxVisibleItems: maxVisible,
	}
}

// buildRepositoryHierarchy creates display items organized by repository
func buildRepositoryHierarchy(model SessionListModel, viewState ListViewState, visibleItems []*session.Instance, services *ListServices) []DisplayItem {
	var displayItems []DisplayItem

	// Group visible items by repository
	repoItems := make(map[string][]*session.Instance)
	for _, item := range visibleItems {
		repoName := services.Repository.GetRepositoryName(item)
		repoItems[repoName] = append(repoItems[repoName], item)
	}

	// Sort repository names
	var repoNames []string
	for repoName := range repoItems {
		repoNames = append(repoNames, repoName)
	}
	// Simple sort for now - could be enhanced with natural sorting
	for i := 0; i < len(repoNames); i++ {
		for j := i + 1; j < len(repoNames); j++ {
			if repoNames[i] > repoNames[j] {
				repoNames[i], repoNames[j] = repoNames[j], repoNames[i]
			}
		}
	}

	// Build display items with repository headers
	for _, repoName := range repoNames {
		sessions := repoItems[repoName]
		if len(sessions) == 0 {
			continue
		}

		// Add repository header
		expanded := viewState.expandedGroups[repoName]
		displayItems = append(displayItems, DisplayItem{
			Session:      nil,
			GlobalIndex:  -1,
			DisplayIndex: len(displayItems),
			IsHeader:     true,
			HeaderText:   fmt.Sprintf("%s (%d)", repoName, len(sessions)),
			HeaderLevel:  0,
			GroupID:      repoName,
			Selected:     false,
		})

		// Add sessions if expanded
		if expanded {
			for _, sess := range sessions {
				globalIdx := model.sessionToIndex[sess]
				displayItems = append(displayItems, DisplayItem{
					Session:      sess,
					GlobalIndex:  globalIdx,
					DisplayIndex: len(displayItems),
					IsHeader:     false,
					HeaderText:   "",
					HeaderLevel:  1,
					GroupID:      repoName,
					Selected:     globalIdx == viewState.selectedIndex,
				})
			}
		}
	}

	return displayItems
}

// buildCategoryHierarchy creates display items organized by category (existing logic)
func buildCategoryHierarchy(model SessionListModel, viewState ListViewState, visibleItems []*session.Instance) []DisplayItem {
	var displayItems []DisplayItem

	// Group visible items by category
	categoryItems := make(map[string][]*session.Instance)
	for _, item := range visibleItems {
		category := item.Category
		if category == "" {
			category = "Uncategorized"
		}
		categoryItems[category] = append(categoryItems[category], item)
	}

	// Sort category names (Uncategorized last)
	var categoryNames []string
	for categoryName := range categoryItems {
		if categoryName != "Uncategorized" {
			categoryNames = append(categoryNames, categoryName)
		}
	}
	// Simple sort
	for i := 0; i < len(categoryNames); i++ {
		for j := i + 1; j < len(categoryNames); j++ {
			if categoryNames[i] > categoryNames[j] {
				categoryNames[i], categoryNames[j] = categoryNames[j], categoryNames[i]
			}
		}
	}
	// Add Uncategorized at the end
	if _, exists := categoryItems["Uncategorized"]; exists {
		categoryNames = append(categoryNames, "Uncategorized")
	}

	// Build display items
	for _, categoryName := range categoryNames {
		sessions := categoryItems[categoryName]
		if len(sessions) == 0 {
			continue
		}

		// Add category header
		expanded := viewState.expandedGroups[categoryName]
		displayItems = append(displayItems, DisplayItem{
			Session:      nil,
			GlobalIndex:  -1,
			DisplayIndex: len(displayItems),
			IsHeader:     true,
			HeaderText:   fmt.Sprintf("%s (%d)", categoryName, len(sessions)),
			HeaderLevel:  0,
			GroupID:      categoryName,
			Selected:     false,
		})

		// Add sessions if expanded
		if expanded {
			for _, sess := range sessions {
				globalIdx := model.sessionToIndex[sess]
				displayItems = append(displayItems, DisplayItem{
					Session:      sess,
					GlobalIndex:  globalIdx,
					DisplayIndex: len(displayItems),
					IsHeader:     false,
					HeaderText:   "",
					HeaderLevel:  1,
					GroupID:      categoryName,
					Selected:     globalIdx == viewState.selectedIndex,
				})
			}
		}
	}

	return displayItems
}

// buildFlatList creates display items in a flat list (no grouping)
func buildFlatList(model SessionListModel, viewState ListViewState, visibleItems []*session.Instance) []DisplayItem {
	var displayItems []DisplayItem

	for _, sess := range visibleItems {
		globalIdx := model.sessionToIndex[sess]
		displayItems = append(displayItems, DisplayItem{
			Session:      sess,
			GlobalIndex:  globalIdx,
			DisplayIndex: len(displayItems),
			IsHeader:     false,
			HeaderText:   "",
			HeaderLevel:  0,
			GroupID:      "",
			Selected:     globalIdx == viewState.selectedIndex,
		})
	}

	return displayItems
}

// =============================================================================
// Services and Dependencies
// =============================================================================

// RepositoryService provides cached repository name lookups and manages expensive git operations
type RepositoryService struct {
	cache    map[string]string // sessionTitle -> repoName
	lastUsed map[string]time.Time // sessionTitle -> lastAccessTime
	maxAge   time.Duration
}

// ListServices encapsulates all external dependencies for the list component
type ListServices struct {
	Repository *RepositoryService
	Renderer   *InstanceRenderer
	// Future services can be added here:
	// StateManager *StateManager
	// SearchIndex  *SearchService
	// Analytics    *AnalyticsService
}

// NewListServices creates a new service container with all dependencies
func NewListServices(renderer *InstanceRenderer) *ListServices {
	return &ListServices{
		Repository: NewRepositoryService(),
		Renderer:   renderer,
	}
}

// NewRepositoryService creates a new repository service with caching
func NewRepositoryService() *RepositoryService {
	return &RepositoryService{
		cache:    make(map[string]string),
		lastUsed: make(map[string]time.Time),
		maxAge:   5 * time.Minute, // Cache entries expire after 5 minutes
	}
}

// GetRepositoryName retrieves a cached repository name or computes it if not cached
func (rs *RepositoryService) GetRepositoryName(sess *session.Instance) string {
	cacheKey := sess.Title
	now := time.Now()

	// Check if we have a cached entry that's still valid
	if repoName, exists := rs.cache[cacheKey]; exists {
		if lastUsed, hasTime := rs.lastUsed[cacheKey]; hasTime {
			if now.Sub(lastUsed) < rs.maxAge {
				// Update last used time
				rs.lastUsed[cacheKey] = now
				return repoName
			}
		}
		// Entry expired, remove it
		delete(rs.cache, cacheKey)
		delete(rs.lastUsed, cacheKey)
	}

	// Compute repository name (expensive operation)
	repoName := rs.computeRepositoryName(sess)

	// Cache the result
	rs.cache[cacheKey] = repoName
	rs.lastUsed[cacheKey] = now

	return repoName
}

// computeRepositoryName performs the actual expensive repository name lookup
func (rs *RepositoryService) computeRepositoryName(sess *session.Instance) string {
	// Try to get repository name from git worktree
	if gitWorktree, err := sess.GetGitWorktree(); err == nil && gitWorktree != nil {
		if repoName := gitWorktree.GetRepoName(); repoName != "" {
			return repoName
		}

		// Fallback to repository path
		repoPath := gitWorktree.GetRepoPath()
		if repoPath != "" {
			// Extract directory name from path
			parts := strings.Split(strings.TrimSuffix(repoPath, "/"), "/")
			if len(parts) > 0 {
				return parts[len(parts)-1]
			}
		}
	}

	// Final fallback: use path or title
	if sess.Path != "" {
		parts := strings.Split(strings.TrimSuffix(sess.Path, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	if sess.Title != "" {
		// Extract first word as project identifier
		fields := strings.Fields(sess.Title)
		if len(fields) > 0 {
			return "project-" + strings.ToLower(fields[0])
		}
	}

	return "unknown-repo"
}

// CleanExpired removes expired entries from the cache
func (rs *RepositoryService) CleanExpired() {
	now := time.Now()
	for key, lastUsed := range rs.lastUsed {
		if now.Sub(lastUsed) >= rs.maxAge {
			delete(rs.cache, key)
			delete(rs.lastUsed, key)
		}
	}
}

// =============================================================================
// Helper Functions for Pure State Operations
// =============================================================================

// getVisibleItemsElm returns sessions that should be visible based on current filters
func getVisibleItemsElm(model SessionListModel, viewState ListViewState) []*session.Instance {
	var visible []*session.Instance

	// If in search mode, filter by search query
	if viewState.searchActive && viewState.searchQuery != "" {
		query := strings.ToLower(viewState.searchQuery)
		for _, sess := range model.sessions {
			title := strings.ToLower(sess.Title)
			if strings.Contains(title, query) {
				// Apply additional filters
				if viewState.hidePaused && sess.Status == session.Paused {
					continue
				}
				visible = append(visible, sess)
			}
		}
	} else {
		// Normal mode: apply filters
		for _, sess := range model.sessions {
			if viewState.hidePaused && sess.Status == session.Paused {
				continue
			}
			visible = append(visible, sess)
		}
	}

	return visible
}

// findVisibleIndex finds the index of the currently selected item in the visible items list
func findVisibleIndex(model SessionListModel, viewState ListViewState) int {
	if viewState.selectedIndex < 0 || viewState.selectedIndex >= len(model.sessions) {
		return -1
	}

	selectedSession := model.sessions[viewState.selectedIndex]
	visibleItems := getVisibleItemsElm(model, viewState)

	for i, item := range visibleItems {
		if item == selectedSession {
			return i
		}
	}

	return -1 // Selected item is not in visible items
}

// selectFirstVisible selects the first visible item
func selectFirstVisible(model SessionListModel, viewState ListViewState) ListViewState {
	visibleItems := getVisibleItemsElm(model, viewState)
	if len(visibleItems) == 0 {
		return viewState
	}

	// Find global index of first visible item
	firstVisibleItem := visibleItems[0]
	for i, session := range model.sessions {
		if session == firstVisibleItem {
			newViewState := viewState
			newViewState.selectedIndex = i
			newViewState.scrollOffset = 0
			newViewState.lastStateChange = time.Now()
			return newViewState
		}
	}

	return viewState
}

// ensureSelectionVisible adjusts scroll offset to ensure selected item is visible
func ensureSelectionVisible(model SessionListModel, viewState ListViewState) ListViewState {
	visibleItems := getVisibleItemsElm(model, viewState)
	if len(visibleItems) == 0 {
		return viewState
	}

	selectedVisibleIdx := findVisibleIndex(model, viewState)
	if selectedVisibleIdx == -1 {
		// Selected item not visible, reset to first visible
		return selectFirstVisible(model, viewState)
	}

	// Calculate max visible items (simplified version)
	maxVisible := calculateMaxVisibleElm(viewState)

	newViewState := viewState

	// If selected item is above visible window, scroll up
	if selectedVisibleIdx < viewState.scrollOffset {
		newViewState.scrollOffset = selectedVisibleIdx
	}

	// If selected item is below visible window, scroll down
	if selectedVisibleIdx >= viewState.scrollOffset+maxVisible {
		newViewState.scrollOffset = selectedVisibleIdx - maxVisible + 1
		if newViewState.scrollOffset < 0 {
			newViewState.scrollOffset = 0
		}
	}

	// Ensure scroll offset doesn't go beyond the list
	if newViewState.scrollOffset >= len(visibleItems) {
		newViewState.scrollOffset = len(visibleItems) - 1
		if newViewState.scrollOffset < 0 {
			newViewState.scrollOffset = 0
		}
	}

	return newViewState
}

// calculateMaxVisibleElm calculates max visible items (simplified)
func calculateMaxVisibleElm(viewState ListViewState) int {
	// Simple calculation - in real implementation this would be more sophisticated
	availableHeight := viewState.height - 8 // Account for title and padding
	linesPerItem := 3
	maxItems := availableHeight / linesPerItem
	if maxItems < 1 {
		maxItems = 1
	}
	if maxItems > 50 {
		maxItems = 50
	}
	return maxItems
}