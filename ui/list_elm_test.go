package ui

import (
	"claude-squad/session"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

func TestListComponentElmBasic(t *testing.T) {
	// Create a spinner for the renderer
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	// Create the Elm architecture component
	lc := NewListComponentElm(renderer)

	// Test initial state
	if len(lc.model.sessions) != 0 {
		t.Errorf("Expected empty sessions initially, got %d", len(lc.model.sessions))
	}

	if lc.viewState.selectedIndex != 0 {
		t.Errorf("Expected selectedIndex to be 0 initially, got %d", lc.viewState.selectedIndex)
	}

	if lc.viewState.organizationMode != OrganizeByCategory {
		t.Errorf("Expected organization mode to be OrganizeByCategory, got %v", lc.viewState.organizationMode)
	}
}

func TestListComponentElmAddSession(t *testing.T) {
	// Create renderer
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	// Create component
	lc := NewListComponentElm(renderer)

	// Create test session
	testSession := &session.Instance{
		Title:     "Test Session",
		Category:  "Work",
		Status:    session.Ready,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add session via message
	updatedLc, cmd := lc.Update(AddSessionMsg{Session: testSession})

	// Should have no command
	if cmd != nil {
		t.Errorf("Expected no command from AddSessionMsg, got %v", cmd)
	}

	// Should have 1 session
	if len(updatedLc.model.sessions) != 1 {
		t.Errorf("Expected 1 session after adding, got %d", len(updatedLc.model.sessions))
	}

	// Session should be in the list
	if updatedLc.model.sessions[0] != testSession {
		t.Errorf("Added session not found in model")
	}

	// Session should be in category groups
	workSessions, exists := updatedLc.model.categoryGroups["Work"]
	if !exists || len(workSessions) != 1 {
		t.Errorf("Session not properly categorized")
	}
}

func TestListComponentElmNavigation(t *testing.T) {
	// Setup component with test data
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	lc := NewListComponentElm(renderer)

	// Add multiple test sessions
	sessions := []*session.Instance{
		{Title: "Session 1", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Session 2", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Session 3", Status: session.Paused, CreatedAt: time.Now()},
	}

	// Load sessions
	updatedLc, _ := lc.Update(SessionsLoadedMsg{Sessions: sessions})

	// Test SelectNext
	nextLc, _ := updatedLc.Update(SelectNextMsg{})
	if nextLc.viewState.selectedIndex == updatedLc.viewState.selectedIndex {
		t.Errorf("SelectNext should change selection")
	}

	// Test SelectPrev
	prevLc, _ := nextLc.Update(SelectPrevMsg{})
	if prevLc.viewState.selectedIndex != updatedLc.viewState.selectedIndex {
		t.Errorf("SelectPrev should return to original selection")
	}
}

func TestListComponentElmFiltering(t *testing.T) {
	// Setup component
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	lc := NewListComponentElm(renderer)

	// Add sessions with different statuses
	sessions := []*session.Instance{
		{Title: "Active Session", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Paused Session", Status: session.Paused, CreatedAt: time.Now()},
		{Title: "Running Session", Status: session.Running, CreatedAt: time.Now()},
	}

	// Load sessions
	updatedLc, _ := lc.Update(SessionsLoadedMsg{Sessions: sessions})

	// Initially all sessions should be visible
	visibleItems := getVisibleItemsElm(updatedLc.model, updatedLc.viewState)
	if len(visibleItems) != 3 {
		t.Errorf("Expected 3 visible items initially, got %d", len(visibleItems))
	}

	// Toggle paused filter
	filteredLc, _ := updatedLc.Update(TogglePausedFilterMsg{})

	// Should hide the paused session
	visibleFiltered := getVisibleItemsElm(filteredLc.model, filteredLc.viewState)
	if len(visibleFiltered) != 2 {
		t.Errorf("Expected 2 visible items after filtering paused, got %d", len(visibleFiltered))
	}

	// Paused session should not be in visible items
	for _, item := range visibleFiltered {
		if item.Status == session.Paused {
			t.Errorf("Paused session should not be visible when hidePaused is true")
		}
	}
}

func TestListComponentElmSearch(t *testing.T) {
	// Setup component
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	lc := NewListComponentElm(renderer)

	// Add sessions with different titles
	sessions := []*session.Instance{
		{Title: "Feature Implementation", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Bug Fix Task", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Another Feature", Status: session.Ready, CreatedAt: time.Now()},
	}

	// Load sessions
	updatedLc, _ := lc.Update(SessionsLoadedMsg{Sessions: sessions})

	// Test search
	searchLc, _ := updatedLc.Update(SetSearchQueryMsg{Query: "feature"})

	// Should find 2 sessions with "feature" in title (case-insensitive)
	visibleSearched := getVisibleItemsElm(searchLc.model, searchLc.viewState)
	if len(visibleSearched) != 2 {
		t.Errorf("Expected 2 search results for 'feature', got %d", len(visibleSearched))
	}

	// Should be in search mode
	if !searchLc.viewState.searchActive {
		t.Errorf("Should be in search mode after setting query")
	}

	// Clear search
	clearedLc, _ := searchLc.Update(SetSearchQueryMsg{Query: ""})
	if clearedLc.viewState.searchActive {
		t.Errorf("Should not be in search mode after clearing query")
	}

	// All sessions should be visible again
	visibleCleared := getVisibleItemsElm(clearedLc.model, clearedLc.viewState)
	if len(visibleCleared) != 3 {
		t.Errorf("Expected 3 visible items after clearing search, got %d", len(visibleCleared))
	}
}

func TestListComponentElmView(t *testing.T) {
	// Setup component
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	lc := NewListComponentElm(renderer)

	// Test view rendering
	view := lc.View()
	if view == "" {
		t.Errorf("View should return non-empty string")
	}

	// Should contain session list header
	if !strings.Contains(view, "Claude Squad Sessions") {
		t.Errorf("View should contain session list header")
	}

	// Should show session count
	if !strings.Contains(view, "0/0 sessions") {
		t.Errorf("View should show session count")
	}
}

func TestListComponentElmRepositoryHierarchy(t *testing.T) {
	// Setup component
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	lc := NewListComponentElm(renderer)

	// Add sessions from different "repositories" (simulated with paths)
	sessions := []*session.Instance{
		{Title: "Main Feature", Path: "/path/to/repo1", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Bug Fix", Path: "/path/to/repo1", Status: session.Running, CreatedAt: time.Now()},
		{Title: "UI Update", Path: "/path/to/repo2", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "API Endpoint", Path: "/path/to/repo2", Status: session.Paused, CreatedAt: time.Now()},
	}

	// Load sessions
	updatedLc, _ := lc.Update(SessionsLoadedMsg{Sessions: sessions})

	// Change to repository organization
	repoLc, _ := updatedLc.Update(ChangeOrganizationMsg{Mode: OrganizeByRepository})

	// Should be in repository mode
	if repoLc.viewState.organizationMode != OrganizeByRepository {
		t.Errorf("Should be in repository organization mode")
	}

	// Check that repository groups are created
	if len(repoLc.model.repoGroups) == 0 {
		t.Errorf("Repository groups should be created")
	}

	// View should show repository organization
	view := repoLc.View()
	if !strings.Contains(view, "Repositories") {
		t.Errorf("View should indicate repository organization mode")
	}

	// Should show repository names in the view
	if !strings.Contains(view, "repo1") && !strings.Contains(view, "repo2") {
		t.Errorf("View should show repository names")
	}
}

func TestListComponentElmOrganizationModes(t *testing.T) {
	// Setup component
	s := spinner.New()
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         80,
		repoNameCache: make(map[string]string),
	}

	lc := NewListComponentElm(renderer)

	// Add test sessions
	sessions := []*session.Instance{
		{Title: "Work Task", Category: "Work", Path: "/repo1", Status: session.Ready, CreatedAt: time.Now()},
		{Title: "Personal Task", Category: "Personal", Path: "/repo2", Status: session.Ready, CreatedAt: time.Now()},
	}

	// Load sessions
	updatedLc, _ := lc.Update(SessionsLoadedMsg{Sessions: sessions})

	// Test default mode (category)
	if updatedLc.viewState.organizationMode != OrganizeByCategory {
		t.Errorf("Default organization mode should be OrganizeByCategory")
	}

	// Change to repository mode
	repoLc, _ := updatedLc.Update(ChangeOrganizationMsg{Mode: OrganizeByRepository})
	if repoLc.viewState.organizationMode != OrganizeByRepository {
		t.Errorf("Should change to repository organization mode")
	}

	// Change to flat mode
	flatLc, _ := repoLc.Update(ChangeOrganizationMsg{Mode: OrganizeFlat})
	if flatLc.viewState.organizationMode != OrganizeFlat {
		t.Errorf("Should change to flat organization mode")
	}

	// Flat mode view should not have group headers
	flatView := flatLc.View()
	if strings.Contains(flatView, "▼") || strings.Contains(flatView, "►") {
		t.Errorf("Flat mode should not show group expansion icons")
	}
}