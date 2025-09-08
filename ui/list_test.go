package ui

import (
	"claude-squad/session"
	"claude-squad/session/git"
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
