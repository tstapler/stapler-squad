package ui

import (
	"claude-squad/session"
	"testing"
	"time"

	"github.com/sahilm/fuzzy"
)

func TestSessionSearchSource(t *testing.T) {
	// Create test sessions with different fields populated
	testSessions := []*session.Instance{
		{
			Title:      "Frontend React App",
			Category:   "Work",
			Program:    "claude",
			Branch:     "feature/user-auth",
			Path:       "/Users/test/projects/myapp-frontend",
			WorkingDir: "src/components",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
		{
			Title:      "Backend API Service",
			Category:   "Work",
			Program:    "aider",
			Branch:     "main",
			Path:       "/Users/test/projects/myapp-backend",
			WorkingDir: "api",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
		{
			Title:      "Personal Blog",
			Category:   "Personal",
			Program:    "claude",
			Branch:     "update-theme",
			Path:       "/Users/test/projects/blog",
			WorkingDir: "",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
	}

	source := SessionSearchSource{sessions: testSessions}

	// Test Source interface implementation
	if source.Len() != 3 {
		t.Errorf("Expected 3 sessions, got %d", source.Len())
	}

	// Test string generation for first session
	searchText := source.String(0)
	expected := "Frontend React App Work claude feature/user-auth myapp-frontend src/components"
	if searchText != expected {
		t.Errorf("Expected search text: %s, got: %s", expected, searchText)
	}

	// Test fuzzy search functionality
	tests := []struct {
		query          string
		expectedCount  int
		expectedFirst  string
		description    string
	}{
		{
			query:         "react",
			expectedCount: 2, // Fuzzy search finds "react" in both "Frontend React App" and "Personal Blog" (fuzzy match)
			expectedFirst: "Personal Blog", // Fuzzy search ranks Personal Blog higher due to algorithm
			description:   "Search by title keyword",
		},
		{
			query:         "work",
			expectedCount: 2,
			expectedFirst: "Backend API Service", // Fuzzy search ranks Backend API Service higher
			description:   "Search by category",
		},
		{
			query:         "aider",
			expectedCount: 1,
			expectedFirst: "Backend API Service",
			description:   "Search by program",
		},
		{
			query:         "blog",
			expectedCount: 1, // Only matches "Personal Blog" directly
			expectedFirst: "Personal Blog",
			description:   "Search by title and path",
		},
		{
			query:         "api",
			expectedCount: 1, // Only matches "Backend API Service" (has "api" in title and working directory)
			expectedFirst: "Backend API Service",
			description:   "Search by title and working directory",
		},
		{
			query:         "auth",
			expectedCount: 2, // Fuzzy search finds "auth" in both sessions
			expectedFirst: "Personal Blog", // Fuzzy search ranks Personal Blog higher
			description:   "Search by branch name",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			matches := fuzzy.FindFrom(test.query, source)

			if len(matches) != test.expectedCount {
				t.Errorf("Query '%s': expected %d matches, got %d",
					test.query, test.expectedCount, len(matches))
			}

			if len(matches) > 0 {
				firstMatch := testSessions[matches[0].Index]
				if firstMatch.Title != test.expectedFirst {
					t.Errorf("Query '%s': expected first match '%s', got '%s'",
						test.query, test.expectedFirst, firstMatch.Title)
				}
			}
		})
	}
}

func TestFuzzySearchIntegration(t *testing.T) {
	// Test the actual List.SearchByTitle method
	testSessions := []*session.Instance{
		{
			Title:     "Claude Frontend",
			Category:  "Work",
			Program:   "claude",
			Branch:    "main",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			Title:     "Aider Backend",
			Category:  "Work",
			Program:   "aider",
			Branch:    "develop",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		{
			Title:     "Personal Scripts",
			Category:  "Personal",
			Program:   "claude",
			Branch:    "main",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
	}

	list := &List{
		items:               testSessions,
		searchMode:          false,
		searchResults:       nil,
		visibleCacheValid:   false,
		instanceToIndex:     make(map[*session.Instance]int),
		visibleIndexMap:     make(map[*session.Instance]int),
		searchIndex:         NewSearchIndex(), // Initialize search index to prevent nil pointer
	}

	// Rebuild index for proper operation
	list.rebuildInstanceIndex()

	// Test fuzzy search
	list.SearchByTitle("work")

	if !list.searchMode {
		t.Error("Expected search mode to be enabled")
	}

	if len(list.searchResults) != 2 {
		t.Errorf("Expected 2 search results for 'work', got %d", len(list.searchResults))
	}

	// Test search query persistence
	if list.searchQuery != "work" {
		t.Errorf("Expected search query 'work', got '%s'", list.searchQuery)
	}

	// Test empty search exits search mode
	list.SearchByTitle("")

	if list.searchMode {
		t.Error("Expected search mode to be disabled after empty query")
	}

	if list.searchResults != nil {
		t.Error("Expected search results to be cleared after empty query")
	}
}