package ui

import (
	"claude-squad/session"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/sahilm/fuzzy"
)

// BenchmarkFuzzySearchImplementation benchmarks the new sahilm/fuzzy implementation
func BenchmarkFuzzySearchImplementation(b *testing.B) {
	sessionCounts := []int{10, 50, 100, 500, 1000, 2000}
	queries := []string{"project", "work", "api", "react", "backend", "frontend"}

	for _, count := range sessionCounts {
		for _, query := range queries {
			b.Run(fmt.Sprintf("sessions_%d_query_%s", count, query), func(b *testing.B) {
				// Setup realistic session data
				instances := createRealisticInstances(count)

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					// Create search source
					searchSource := SessionSearchSource{sessions: instances}

					// Perform fuzzy search
					matches := fuzzy.FindFrom(query, searchSource)

					// Convert to results (simulating real usage)
					_ = len(matches)
				}
			})
		}
	}
}

// BenchmarkFuzzySearchVsStringContains compares fuzzy search vs old string contains approach
func BenchmarkFuzzySearchVsStringContains(b *testing.B) {
	instances := createRealisticInstances(1000)
	query := "project"

	b.Run("fuzzy_search", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			searchSource := SessionSearchSource{sessions: instances}
			matches := fuzzy.FindFrom(query, searchSource)
			_ = len(matches)
		}
	})

	b.Run("string_contains", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			matches := 0
			for _, instance := range instances {
				if instance != nil && instance.Title != "" {
					// Simulate old implementation with strings.Contains
					searchText := fmt.Sprintf("%s %s %s %s",
						instance.Title, instance.Category, instance.Program, instance.Branch)
					if len(searchText) > 0 {
						matches++
					}
				}
			}
			_ = matches
		}
	})
}

// BenchmarkSessionSearchSource benchmarks the source interface implementation
func BenchmarkSessionSearchSource(b *testing.B) {
	sessionCounts := []int{100, 500, 1000, 2000, 5000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("string_generation_%d", count), func(b *testing.B) {
			instances := createRealisticInstances(count)
			searchSource := SessionSearchSource{sessions: instances}

			b.ResetTimer()

			// Benchmark string generation for all sessions
			for i := 0; i < b.N; i++ {
				for j := 0; j < searchSource.Len(); j++ {
					_ = searchSource.String(j)
				}
			}
		})
	}
}

// BenchmarkFullSearchWorkflow benchmarks the complete search workflow
func BenchmarkFullSearchWorkflow(b *testing.B) {
	sessionCounts := []int{50, 100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("full_workflow_%d", count), func(b *testing.B) {
			// Setup
			spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := NewList(&spinner, false, nil)

			instances := createRealisticInstances(count)
			for _, instance := range instances {
				finalizer := list.AddInstance(instance)
				finalizer()
			}

			query := "backend"

			b.ResetTimer()

			// Benchmark complete search workflow as user would experience
			for i := 0; i < b.N; i++ {
				// Enter search mode
				list.SearchByTitle(query)

				// Get visible results (what user sees)
				visibleItems := list.getVisibleItems()
				_ = len(visibleItems)

				// Exit search mode (cleanup)
				list.ExitSearchMode()
			}
		})
	}
}

// createRealisticInstances creates session instances with realistic, varied data for benchmarking
func createRealisticInstances(count int) []*session.Instance {
	instances := make([]*session.Instance, count)

	// Realistic project patterns
	projectTypes := []string{"Backend API", "Frontend React", "Mobile App", "Database Service", "Auth Service", "Payment Gateway", "User Management", "Analytics Dashboard", "File Upload", "Email Service"}
	categories := []string{"Work", "Personal", "Development", "Testing", "Infrastructure", "Client Projects", "Open Source"}
	programs := []string{"claude", "aider", "cursor", "vim", "vscode", "intellij"}
	branches := []string{"main", "develop", "feature/auth", "feature/payments", "hotfix/security", "release/v2.0", "feature/ui-redesign", "bugfix/memory-leak"}
	workingDirs := []string{"src", "backend", "frontend", "api", "services", "components", "utils", "tests", "docs", "config"}

	for i := 0; i < count; i++ {
		projectType := projectTypes[i%len(projectTypes)]
		category := categories[i%len(categories)]
		program := programs[i%len(programs)]
		branch := branches[i%len(branches)]
		workingDir := workingDirs[i%len(workingDirs)]

		title := fmt.Sprintf("%s %d", projectType, i/len(projectTypes)+1)
		path := fmt.Sprintf("/home/user/projects/%s-%d",
			fmt.Sprintf("%s", projectType), i)

		instance := &session.Instance{
			Title:      title,
			Category:   category,
			Program:    program,
			Branch:     branch,
			Path:       path,
			WorkingDir: workingDir,
			Status:     session.Ready,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		instances[i] = instance
	}

	return instances
}

// BenchmarkSearchMemoryUsage benchmarks memory allocation patterns
func BenchmarkSearchMemoryUsage(b *testing.B) {
	instances := createRealisticInstances(1000)
	queries := []string{"backend", "frontend", "api", "service", "app"}

	b.Run("memory_allocation", func(b *testing.B) {
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			for _, query := range queries {
				searchSource := SessionSearchSource{sessions: instances}
				matches := fuzzy.FindFrom(query, searchSource)

				// Convert matches to session instances (real usage pattern)
				results := make([]*session.Instance, 0, len(matches))
				for _, match := range matches {
					if match.Index >= 0 && match.Index < len(instances) {
						results = append(results, instances[match.Index])
					}
				}
				_ = results
			}
		}
	})
}

// BenchmarkSearchQueryLatency benchmarks search latency for different query lengths
func BenchmarkSearchQueryLatency(b *testing.B) {
	instances := createRealisticInstances(500)

	queryTests := []struct {
		name  string
		query string
	}{
		{"short_1char", "a"},
		{"short_2char", "ap"},
		{"medium_5char", "backe"},
		{"medium_8char", "backend"},
		{"long_15char", "backend-api-ser"},
		{"very_long_25char", "backend-api-service-auth"},
	}

	for _, qt := range queryTests {
		b.Run(qt.name, func(b *testing.B) {
			searchSource := SessionSearchSource{sessions: instances}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				matches := fuzzy.FindFrom(qt.query, searchSource)
				_ = len(matches)
			}
		})
	}
}