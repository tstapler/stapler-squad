package ui

import (
	"claude-squad/session"
	"claude-squad/session/git"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

func TestInstanceRendererWithDifferentStatuses(t *testing.T) {
	// Create a spinner for the renderer
	s := spinner.New()
	s.Spinner = spinner.Dot

	// Create renderer
	renderer := &InstanceRenderer{
		spinner:       &s,
		width:         100,
		repoNameCache: make(map[string]string),
	}

	// Setup a mock instance with minimal required fields
	instance := &session.Instance{
		Title:     "Test Instance",
		Branch:    "test-branch",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Helper to set instance to started state with a mock git worktree
	setupStartedInstance := func(i *session.Instance) {
		i.SetTmuxSession(nil) // Just needs to be non-nil for Started() check
		mockWorktree := git.NewGitWorktreeFromStorage(
			"/path/to/repo",
			"/path/to/worktree",
			i.Title,
			i.Branch,
			"abcdef1234567890",
		)
		// Set the git worktree using reflection since there's no public setter
		i.SetGitWorktree(mockWorktree)
	}

	// Test rendering for each status type
	tests := []struct {
		name            string
		status          session.Status
		setupFunc       func(*session.Instance)
		expectedContent []string // Substrings we expect to find in the output
	}{
		{
			name:   "Running Status",
			status: session.Running,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch"}, // Can't reliably test spinner character
		},
		{
			name:   "Ready Status",
			status: session.Ready,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch", readyIcon},
		},
		{
			name:   "Paused Status",
			status: session.Paused,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch", pausedIcon},
		},
		{
			name:   "NeedsApproval Status",
			status: session.NeedsApproval,
			setupFunc: func(i *session.Instance) {
				setupStartedInstance(i)
			},
			expectedContent: []string{"Test Instance", "test-branch", needsApprovalIcon},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Reset instance for each test
			instance.Status = test.status
			if test.setupFunc != nil {
				test.setupFunc(instance)
			}

			// Render the instance
			result := renderer.Render(instance, 1, false, true)

			// Check that expected content is in the result
			for _, expected := range test.expectedContent {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected rendered output to contain '%s', but got: %s", expected, result)
				}
			}

			// For NeedsApproval, specifically check that the icon is rendered with the right style
			if test.status == session.NeedsApproval {
				expectedStyledIcon := needsApprovalStyle.Render(needsApprovalIcon)
				if !strings.Contains(result, expectedStyledIcon) {
					t.Errorf("Expected NeedsApproval status to be rendered with needsApprovalStyle, but styled icon '%s' not found in: %s",
						expectedStyledIcon, result)
				}
			}
		})
	}
}

func TestOrganizeByCategory(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)

	// Create test instances with different categories
	instance1 := &session.Instance{
		Title:     "Test1",
		Category:  "Work",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	instance2 := &session.Instance{
		Title:     "Test2",
		Category:  "Personal",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	instance3 := &session.Instance{
		Title:     "Test3",
		Category:  "", // No category, should go to "Uncategorized"
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	instance4 := &session.Instance{
		Title:     "Test4",
		Category:  "Work", // Same category as instance1
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add instances to list
	list.AddInstance(instance1)
	list.AddInstance(instance2)
	list.AddInstance(instance3)
	list.AddInstance(instance4)

	// Explicitly organize by category
	list.OrganizeByCategory()

	// Verify that the list has the correct number of categories
	expectedCategoryCount := 3 // Work, Personal, Uncategorized
	if len(list.categoryGroups) != expectedCategoryCount {
		t.Errorf("Expected %d categories, got %d", expectedCategoryCount, len(list.categoryGroups))
	}

	// Verify "Work" category has 2 instances
	workInstances := list.categoryGroups["Work"]
	if len(workInstances) != 2 {
		t.Errorf("Expected Work category to have 2 instances, got %d", len(workInstances))
	}

	// Verify "Personal" category has 1 instance
	personalInstances := list.categoryGroups["Personal"]
	if len(personalInstances) != 1 {
		t.Errorf("Expected Personal category to have 1 instance, got %d", len(personalInstances))
	}

	// Verify "Uncategorized" category has 1 instance
	uncategorizedInstances := list.categoryGroups["Uncategorized"]
	if len(uncategorizedInstances) != 1 {
		t.Errorf("Expected Uncategorized category to have 1 instance, got %d", len(uncategorizedInstances))
	}
}

func TestCategoryExpansionCollapse(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)

	// Create test instances with different categories
	instance1 := &session.Instance{
		Title:     "Test1",
		Category:  "Work",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	instance2 := &session.Instance{
		Title:     "Test2",
		Category:  "Personal",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add instances to list
	list.AddInstance(instance1)
	list.AddInstance(instance2)
	list.OrganizeByCategory()

	// Verify default expansion state (should be expanded)
	if !list.groupExpanded["Work"] {
		t.Errorf("Expected Work category to be expanded by default")
	}
	if !list.groupExpanded["Personal"] {
		t.Errorf("Expected Personal category to be expanded by default")
	}

	// Test collapse functionality
	list.CollapseCategory("Work")
	if list.groupExpanded["Work"] {
		t.Errorf("Expected Work category to be collapsed after CollapseCategory")
	}

	// Test expand functionality
	list.ExpandCategory("Work")
	if !list.groupExpanded["Work"] {
		t.Errorf("Expected Work category to be expanded after ExpandCategory")
	}

	// Test toggle functionality
	list.ToggleCategory("Work")
	if list.groupExpanded["Work"] {
		t.Errorf("Expected Work category to be collapsed after first ToggleCategory")
	}

	list.ToggleCategory("Work")
	if !list.groupExpanded["Work"] {
		t.Errorf("Expected Work category to be expanded after second ToggleCategory")
	}
}

func TestSearchByTitle(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)

	// Create test instances with different titles
	instance1 := &session.Instance{
		Title:     "Feature implementation",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	instance2 := &session.Instance{
		Title:     "Bug fix task",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	instance3 := &session.Instance{
		Title:     "Another feature",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Add instances to list
	list.AddInstance(instance1)
	list.AddInstance(instance2)
	list.AddInstance(instance3)

	// Verify list is not in search mode by default
	if list.searchMode {
		t.Errorf("Expected list to not be in search mode by default")
	}

	// Test search by title - should find both instances with "feature"
	list.SearchByTitle("feature")

	// Verify list is now in search mode
	if !list.searchMode {
		t.Errorf("Expected list to be in search mode after search")
	}

	// Verify correct search results
	expectedResultsCount := 2
	if len(list.searchResults) != expectedResultsCount {
		t.Errorf("Expected %d search results, got %d", expectedResultsCount, len(list.searchResults))
	}

	// Test case-insensitive search
	list.ExitSearchMode()
	list.SearchByTitle("FEATURE")
	if len(list.searchResults) != expectedResultsCount {
		t.Errorf("Expected %d search results for case-insensitive search, got %d",
			expectedResultsCount, len(list.searchResults))
	}

	// Test search for non-existent term
	list.ExitSearchMode()
	list.SearchByTitle("nonexistent")
	if len(list.searchResults) != 0 {
		t.Errorf("Expected 0 search results for non-existent term, got %d", len(list.searchResults))
	}

	// Test exiting search mode
	list.ExitSearchMode()
	if list.searchMode {
		t.Errorf("Expected list to not be in search mode after ExitSearchMode")
	}
	if len(list.searchResults) != 0 {
		t.Errorf("Expected empty search results after ExitSearchMode, got %d results",
			len(list.searchResults))
	}

	// Test empty search query should exit search mode
	list.SearchByTitle("")
	if list.searchMode {
		t.Errorf("Expected list to not be in search mode with empty query")
	}
}

func TestScrollingAndViewport(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)

	// Set a reasonable size for testing (simulate a terminal window)
	list.SetSize(80, 20)

	// Create many test instances to test scrolling
	for i := 1; i <= 15; i++ {
		instance := &session.Instance{
			Title:     fmt.Sprintf("Test Instance %d", i),
			Category:  "Test",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		list.AddInstance(instance)
	}

	// Test initial state
	if list.scrollOffset != 0 {
		t.Errorf("Expected initial scroll offset to be 0, got %d", list.scrollOffset)
	}

	// Test calculateMaxVisibleItems returns reasonable value
	maxVisible := list.calculateMaxVisibleItems()
	if maxVisible < 1 {
		t.Errorf("Expected at least 1 visible item, got %d", maxVisible)
	}
	if maxVisible > 15 {
		t.Errorf("Expected reasonable max visible items for height 20, got %d", maxVisible)
	}

	// Test getVisibleWindow returns correct slice
	visibleWindow := list.getVisibleWindow()
	if len(visibleWindow) > maxVisible {
		t.Errorf("Expected visible window to respect max visible items (%d), got %d items",
			maxVisible, len(visibleWindow))
	}

	// Test ensureSelectedVisible with different selections
	// Select an item that should require scrolling
	list.SetSelectedInstance(10) // Select 11th item (0-indexed)
	list.ensureSelectedVisible()

	// The scroll offset should have adjusted to show the selected item
	selectedVisibleIdx := list.getVisibleIndex()
	if selectedVisibleIdx == -1 {
		t.Errorf("Selected item should be visible after ensureSelectedVisible")
	}

	// Verify scroll offset is reasonable
	if list.scrollOffset < 0 {
		t.Errorf("Scroll offset should not be negative, got %d", list.scrollOffset)
	}
	if list.scrollOffset > 15 {
		t.Errorf("Scroll offset should not exceed item count, got %d", list.scrollOffset)
	}
}

func TestItemNumbering(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)
	list.SetSize(80, 20)

	// Create test instances
	for i := 1; i <= 5; i++ {
		instance := &session.Instance{
			Title:     fmt.Sprintf("Instance %d", i),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		list.AddInstance(instance)
	}

	// Test that numbering starts from 1 and is sequential
	for i := 0; i < 5; i++ {
		instance := list.items[i]
		rendered := list.renderer.Render(instance, i+1, false, false)

		expectedNumber := fmt.Sprintf(" %d. ", i+1)
		if !strings.Contains(rendered, expectedNumber) {
			t.Errorf("Expected item %d to have number '%s' in rendered output, got: %s",
				i, expectedNumber, rendered)
		}
	}

	// Test search mode numbering uses global indices
	list.SearchByTitle("Instance")
	visibleWindow := list.getVisibleWindow()

	// In search mode, numbering should still reflect global position
	for i, item := range visibleWindow {
		globalIdx := list.findGlobalIndex(item)
		if globalIdx == -1 {
			t.Errorf("Could not find global index for search result %d", i)
			continue
		}

		rendered := list.renderer.Render(item, globalIdx+1, false, false)
		expectedNumber := fmt.Sprintf(" %d. ", globalIdx+1)
		if !strings.Contains(rendered, expectedNumber) {
			t.Errorf("Expected search result to show global number '%s', got: %s",
				expectedNumber, rendered)
		}
	}
}

func TestViewportConstraints(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)

	// Set specific dimensions
	width, height := 50, 15
	list.SetSize(width, height)

	// Create many instances to force scrolling
	for i := 1; i <= 20; i++ {
		instance := &session.Instance{
			Title:     fmt.Sprintf("Long Instance Title %d", i),
			Category:  fmt.Sprintf("Category %d", (i-1)/5+1), // 4 categories
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		list.AddInstance(instance)
	}

	// Render the list and check dimensions
	rendered := list.String()

	// Check that the rendered output doesn't exceed expected dimensions
	lines := strings.Split(rendered, "\n")
	if len(lines) > height+5 { // Allow some buffer for styling
		t.Errorf("Rendered list should not exceed height constraint of %d (plus buffer), got %d lines",
			height, len(lines))
	}

	// Check that lines don't exceed width (approximately, due to styling)
	for i, line := range lines {
		// Remove ANSI escape sequences for accurate length check
		cleanLine := stripAnsiCodes(line)
		if len(cleanLine) > width+10 { // Allow buffer for styling
			t.Errorf("Line %d exceeds width constraint of %d (plus buffer): length %d",
				i, width, len(cleanLine))
		}
	}
}

func TestFilteringWithScrolling(t *testing.T) {
	// Create test spinner for list
	s := spinner.New()

	// Create list with test spinner
	list := NewList(&s, false, nil)
	list.SetSize(80, 20)

	// Create mix of running and paused instances
	for i := 1; i <= 10; i++ {
		instance := &session.Instance{
			Title:     fmt.Sprintf("Instance %d", i),
			Status:    session.Running,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if i%3 == 0 {
			instance.Status = session.Paused
		}
		list.AddInstance(instance)
	}

	// Test that visible items changes when filter is applied
	allItems := list.getVisibleItems()
	initialCount := len(allItems)

	// Apply paused filter
	list.TogglePausedFilter()
	filteredItems := list.getVisibleItems()
	filteredCount := len(filteredItems)

	if filteredCount >= initialCount {
		t.Errorf("Expected filtering to reduce visible items from %d, got %d",
			initialCount, filteredCount)
	}

	// Test that scroll offset resets when filter is applied
	if list.scrollOffset != 0 {
		t.Errorf("Expected scroll offset to reset to 0 when filter applied, got %d",
			list.scrollOffset)
	}

	// Test that visible window respects the filter
	visibleWindow := list.getVisibleWindow()
	for i, item := range visibleWindow {
		if item.Status == session.Paused {
			t.Errorf("Paused item found in visible window at index %d when hidePaused is true", i)
		}
	}
}

// Helper function to strip ANSI escape codes for accurate length measurement
func stripAnsiCodes(s string) string {
	// Simple regex to remove common ANSI escape sequences
	// This is a basic implementation - for more robust stripping, consider using a library
	result := ""
	inEscape := false

	for i, r := range s {
		if r == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		result += string(r)
	}

	return result
}
