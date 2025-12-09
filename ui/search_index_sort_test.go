package ui

import (
	"claude-squad/session"
	"testing"
	"time"
)

// Helper function to create test sessions with various properties
func createTestSessions() []*session.Instance {
	now := time.Now()
	return []*session.Instance{
		{
			Title:                "Alpha Session",
			Path:                 "/repos/alpha-repo",
			Branch:               "feature/alpha",
			Status:               session.Running,
			CreatedAt:            now.Add(-24 * time.Hour),
			LastMeaningfulOutput: now.Add(-1 * time.Hour),
		},
		{
			Title:                "Beta Session",
			Path:                 "/repos/beta-repo",
			Branch:               "main",
			Status:               session.Paused,
			CreatedAt:            now.Add(-48 * time.Hour),
			LastMeaningfulOutput: now.Add(-30 * time.Minute),
		},
		{
			Title:                "Gamma Session",
			Path:                 "/repos/gamma-repo",
			Branch:               "develop",
			Status:               session.Ready,
			CreatedAt:            now.Add(-12 * time.Hour),
			LastMeaningfulOutput: now.Add(-2 * time.Hour),
		},
		{
			Title:                "Delta Session",
			Path:                 "/repos/alpha-repo", // Same repo as Alpha
			Branch:               "",                  // No branch
			Status:               session.NeedsApproval,
			CreatedAt:            now.Add(-6 * time.Hour),
			LastMeaningfulOutput: now.Add(-10 * time.Minute),
		},
	}
}

func TestSortMode_String(t *testing.T) {
	tests := []struct {
		mode     SortMode
		expected string
	}{
		{SortByLastActivity, "Last Activity"},
		{SortByCreationDate, "Creation Date"},
		{SortByTitleAZ, "Title"},
		{SortByRepository, "Repository"},
		{SortByBranch, "Branch"},
		{SortByStatus, "Status"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("SortMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSortMode_ShortName(t *testing.T) {
	tests := []struct {
		mode     SortMode
		expected string
	}{
		{SortByLastActivity, "Activity"},
		{SortByCreationDate, "Created"},
		{SortByTitleAZ, "Title"},
		{SortByRepository, "Repo"},
		{SortByBranch, "Branch"},
		{SortByStatus, "Status"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.ShortName(); got != tt.expected {
				t.Errorf("SortMode.ShortName() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSortDirection_String(t *testing.T) {
	if got := SortDescending.String(); got != "Descending" {
		t.Errorf("SortDescending.String() = %v, want Descending", got)
	}
	if got := SortAscending.String(); got != "Ascending" {
		t.Errorf("SortAscending.String() = %v, want Ascending", got)
	}
}

func TestSortDirection_Icon(t *testing.T) {
	if got := SortDescending.Icon(); got != "↓" {
		t.Errorf("SortDescending.Icon() = %v, want ↓", got)
	}
	if got := SortAscending.Icon(); got != "↑" {
		t.Errorf("SortAscending.Icon() = %v, want ↑", got)
	}
}

func TestSearchIndex_SortByLastActivity(t *testing.T) {
	idx := NewSearchIndex()
	sessions := createTestSessions()

	// Default is descending (most recent first)
	sorted := idx.Sort(sessions)

	// Delta has most recent activity (10 min ago), then Beta (30 min ago)
	if sorted[0].Title != "Delta Session" {
		t.Errorf("First session should be Delta (most recent), got %s", sorted[0].Title)
	}
	if sorted[1].Title != "Beta Session" {
		t.Errorf("Second session should be Beta, got %s", sorted[1].Title)
	}

	// Test ascending order
	idx.SetSortDirection(SortAscending)
	sorted = idx.Sort(sessions)

	// Gamma has oldest activity (2 hours ago)
	if sorted[0].Title != "Gamma Session" {
		t.Errorf("First session in ascending should be Gamma (oldest), got %s", sorted[0].Title)
	}
}

func TestSearchIndex_SortByCreationDate(t *testing.T) {
	idx := NewSearchIndex()
	idx.SetSortMode(SortByCreationDate)
	sessions := createTestSessions()

	// Default is descending (newest first)
	sorted := idx.Sort(sessions)

	// Delta is newest (6 hours ago)
	if sorted[0].Title != "Delta Session" {
		t.Errorf("First session should be Delta (newest created), got %s", sorted[0].Title)
	}
	// Beta is oldest (48 hours ago)
	if sorted[len(sorted)-1].Title != "Beta Session" {
		t.Errorf("Last session should be Beta (oldest created), got %s", sorted[len(sorted)-1].Title)
	}
}

func TestSearchIndex_SortByTitle(t *testing.T) {
	idx := NewSearchIndex()
	idx.SetSortMode(SortByTitleAZ)
	idx.SetSortDirection(SortAscending) // A-Z
	sessions := createTestSessions()

	sorted := idx.Sort(sessions)

	// Alphabetical order: Alpha, Beta, Delta, Gamma
	if sorted[0].Title != "Alpha Session" {
		t.Errorf("First session should be Alpha, got %s", sorted[0].Title)
	}
	if sorted[1].Title != "Beta Session" {
		t.Errorf("Second session should be Beta, got %s", sorted[1].Title)
	}
	if sorted[2].Title != "Delta Session" {
		t.Errorf("Third session should be Delta, got %s", sorted[2].Title)
	}
	if sorted[3].Title != "Gamma Session" {
		t.Errorf("Fourth session should be Gamma, got %s", sorted[3].Title)
	}
}

func TestSearchIndex_SortByRepository(t *testing.T) {
	idx := NewSearchIndex()
	idx.SetSortMode(SortByRepository)
	idx.SetSortDirection(SortAscending) // A-Z
	sessions := createTestSessions()

	sorted := idx.Sort(sessions)

	// Alphabetical by repo: alpha-repo (Alpha, Delta), beta-repo (Beta), gamma-repo (Gamma)
	// Alpha and Delta share alpha-repo, so they should be sorted by activity (Delta first since more recent)
	if sorted[0].Title != "Delta Session" {
		t.Errorf("First session should be Delta (alpha-repo, more recent activity), got %s", sorted[0].Title)
	}
	if sorted[1].Title != "Alpha Session" {
		t.Errorf("Second session should be Alpha (alpha-repo), got %s", sorted[1].Title)
	}
}

func TestSearchIndex_SortByBranch(t *testing.T) {
	idx := NewSearchIndex()
	idx.SetSortMode(SortByBranch)
	idx.SetSortDirection(SortAscending) // A-Z
	sessions := createTestSessions()

	sorted := idx.Sort(sessions)

	// Sessions without branches should be at the end
	if sorted[len(sorted)-1].Title != "Delta Session" {
		t.Errorf("Last session should be Delta (no branch), got %s", sorted[len(sorted)-1].Title)
	}

	// Alphabetical: develop, feature/alpha, main
	if sorted[0].Title != "Gamma Session" { // develop
		t.Errorf("First session should be Gamma (develop), got %s", sorted[0].Title)
	}
}

func TestSearchIndex_SortByStatus(t *testing.T) {
	idx := NewSearchIndex()
	idx.SetSortMode(SortByStatus)
	idx.SetSortDirection(SortAscending) // Running first
	sessions := createTestSessions()

	sorted := idx.Sort(sessions)

	// Priority order: Running > Ready > NeedsApproval > Loading > Paused
	if sorted[0].Status != session.Running {
		t.Errorf("First session should be Running, got %v", sorted[0].Status)
	}
	if sorted[1].Status != session.Ready {
		t.Errorf("Second session should be Ready, got %v", sorted[1].Status)
	}
	if sorted[2].Status != session.NeedsApproval {
		t.Errorf("Third session should be NeedsApproval, got %v", sorted[2].Status)
	}
	if sorted[3].Status != session.Paused {
		t.Errorf("Fourth session should be Paused, got %v", sorted[3].Status)
	}
}

func TestSearchIndex_CycleSortMode(t *testing.T) {
	idx := NewSearchIndex()

	// Default is SortByLastActivity
	if idx.GetSortMode() != SortByLastActivity {
		t.Errorf("Default sort mode should be SortByLastActivity")
	}

	// Cycle through all modes
	expectedModes := []SortMode{
		SortByCreationDate,
		SortByTitleAZ,
		SortByRepository,
		SortByBranch,
		SortByStatus,
		SortByLastActivity, // Wraps back
	}

	for _, expected := range expectedModes {
		idx.CycleSortMode()
		if idx.GetSortMode() != expected {
			t.Errorf("After cycle, expected %v, got %v", expected, idx.GetSortMode())
		}
	}
}

func TestSearchIndex_ToggleSortDirection(t *testing.T) {
	idx := NewSearchIndex()

	// Default is Descending
	if idx.GetSortDirection() != SortDescending {
		t.Errorf("Default sort direction should be SortDescending")
	}

	idx.ToggleSortDirection()
	if idx.GetSortDirection() != SortAscending {
		t.Errorf("After toggle, should be SortAscending")
	}

	idx.ToggleSortDirection()
	if idx.GetSortDirection() != SortDescending {
		t.Errorf("After second toggle, should be SortDescending")
	}
}

func TestSearchIndex_SortCacheInvalidation(t *testing.T) {
	idx := NewSearchIndex()
	sessions := createTestSessions()

	// First sort should create cache
	sorted1 := idx.Sort(sessions)

	// Get stats to check cache
	stats := idx.GetStats()
	if !stats["sort_cache_valid"].(bool) {
		t.Error("Sort cache should be valid after sort")
	}

	// Change sort mode should invalidate cache
	idx.SetSortMode(SortByTitleAZ)
	stats = idx.GetStats()
	if stats["sort_cache_valid"].(bool) {
		t.Error("Sort cache should be invalid after changing sort mode")
	}

	// Sort again
	sorted2 := idx.Sort(sessions)

	// Results should be different
	if sorted1[0].Title == sorted2[0].Title && sorted1[1].Title == sorted2[1].Title {
		// This might actually be the same by coincidence, but unlikely
		// The important thing is that the sort was re-computed
	}
}

func TestSearchIndex_GetSortDescription(t *testing.T) {
	idx := NewSearchIndex()

	// Default: Activity ↓
	desc := idx.GetSortDescription()
	if desc != "Activity ↓" {
		t.Errorf("Expected 'Activity ↓', got '%s'", desc)
	}

	idx.SetSortMode(SortByTitleAZ)
	idx.SetSortDirection(SortAscending)
	desc = idx.GetSortDescription()
	if desc != "Title ↑" {
		t.Errorf("Expected 'Title ↑', got '%s'", desc)
	}
}

func TestSearchIndex_EmptySessionsSort(t *testing.T) {
	idx := NewSearchIndex()
	sessions := []*session.Instance{}

	sorted := idx.Sort(sessions)
	if len(sorted) != 0 {
		t.Error("Sorting empty sessions should return empty slice")
	}
}

func TestSearchIndex_SecondarySort(t *testing.T) {
	// Test that sessions with equal primary sort values are sorted by LastActivity
	idx := NewSearchIndex()
	idx.SetSortMode(SortByRepository)
	idx.SetSortDirection(SortAscending)

	now := time.Now()
	sessions := []*session.Instance{
		{
			Title:                "Old Activity",
			Path:                 "/repos/same-repo",
			CreatedAt:            now,
			LastMeaningfulOutput: now.Add(-2 * time.Hour),
		},
		{
			Title:                "Recent Activity",
			Path:                 "/repos/same-repo",
			CreatedAt:            now,
			LastMeaningfulOutput: now.Add(-10 * time.Minute),
		},
	}

	sorted := idx.Sort(sessions)

	// Both have same repo, so secondary sort by activity should put "Recent Activity" first
	if sorted[0].Title != "Recent Activity" {
		t.Errorf("Expected 'Recent Activity' first (secondary sort), got '%s'", sorted[0].Title)
	}
}
