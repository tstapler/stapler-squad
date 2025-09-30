package ui

import (
	"claude-squad/session"
	"fmt"
	"testing"
	"time"

	"github.com/sahilm/fuzzy"
)

func TestFuzzySearchDebug(t *testing.T) {
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

	// Print what search strings are generated
	for i := 0; i < source.Len(); i++ {
		searchText := source.String(i)
		fmt.Printf("Session %d (%s): %s\n", i, testSessions[i].Title, searchText)
	}

	// Test specific queries and see what matches
	testQueries := []string{"react", "work", "blog", "auth"}

	for _, query := range testQueries {
		fmt.Printf("\nQuery: '%s'\n", query)
		matches := fuzzy.FindFrom(query, source)

		for _, match := range matches {
			session := testSessions[match.Index]
			fmt.Printf("  Match: %s (Index: %d, String: %s)\n",
				session.Title, match.Index, match.Str)
		}
	}
}