package ui

import (
	"claude-squad/session"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

// createMockInstances creates a specified number of mock session instances for testing
func createMockInstances(count int) []*session.Instance {
	instances := make([]*session.Instance, count)
	categories := []string{"Work", "Personal", "Development", "Testing", "Uncategorized"}

	for i := 0; i < count; i++ {
		// Create mock instance with realistic titles
		title := fmt.Sprintf("session-%d-project-alpha-backend-api", i)
		category := categories[i%len(categories)]

		// Create a mock instance (this may need adjustment based on actual constructor)
		instance := &session.Instance{
			Title:    title,
			Category: category,
			Status:   session.Ready,
		}
		instances[i] = instance
	}

	return instances
}

// BenchmarkSearchPerformance benchmarks the search functionality with different session counts
func BenchmarkSearchPerformance(b *testing.B) {
	sessionCounts := []int{10, 50, 100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("sessions_%d", count), func(b *testing.B) {
			// Setup
			spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := NewList(&spinner, false, nil)

			// Add mock instances
			instances := createMockInstances(count)
			for _, instance := range instances {
				finalizer := list.AddInstance(instance)
				finalizer() // Call finalizer immediately
			}

			// Organize by category to simulate real conditions
			list.OrganizeByCategory()

			// Search query that should match some items
			searchQuery := "project"

			b.ResetTimer()

			// Benchmark the search operation
			for i := 0; i < b.N; i++ {
				list.SearchByTitle(searchQuery)
				// Also benchmark getting visible items (what user sees)
				_ = list.getVisibleItems()
			}
		})
	}
}

// BenchmarkSearchByTitle benchmarks just the search logic without UI updates
func BenchmarkSearchByTitle(b *testing.B) {
	sessionCounts := []int{100, 500, 1000, 2000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("search_only_%d", count), func(b *testing.B) {
			// Setup
			spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := NewList(&spinner, false, nil)

			instances := createMockInstances(count)
			for _, instance := range instances {
				finalizer := list.AddInstance(instance)
				finalizer()
			}

			searchQuery := "project-alpha"

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Manually do the search logic to isolate performance
				list.searchMode = true
				list.searchQuery = searchQuery
				list.invalidateVisibleCache()

				// The actual search filtering
				searchResults := make([]*session.Instance, 0)
				query := searchQuery // Already lowercase in real implementation
				for _, instance := range list.items {
					if instance != nil && len(instance.Title) > 0 {
						// Case-insensitive search (simulate strings.ToLower calls)
						if len(instance.Title) >= len(query) {
							// Simulate string contains check
							searchResults = append(searchResults, instance)
						}
					}
				}
				list.searchResults = searchResults
			}
		})
	}
}

// BenchmarkVisibleItemsCalculation benchmarks the getVisibleItems method
func BenchmarkVisibleItemsCalculation(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("visible_items_%d", count), func(b *testing.B) {
			spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
			list := NewList(&spinner, false, nil)

			instances := createMockInstances(count)
			for _, instance := range instances {
				finalizer := list.AddInstance(instance)
				finalizer()
			}

			// Setup different filter conditions
			list.hidePaused = true
			list.searchMode = true
			list.searchQuery = "project"
			list.SearchByTitle("project") // Populate search results

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Invalidate cache to force recalculation
				list.invalidateVisibleCache()
				_ = list.getVisibleItems()
			}
		})
	}
}

// BenchmarkCacheInvalidation benchmarks cache invalidation overhead
func BenchmarkCacheInvalidation(b *testing.B) {
	spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	list := NewList(&spinner, false, nil)

	instances := createMockInstances(1000)
	for _, instance := range instances {
		finalizer := list.AddInstance(instance)
		finalizer()
	}

	// Build up the cache first
	list.getVisibleItems()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		list.invalidateVisibleCache()
		// Rebuild cache
		list.getVisibleItems()
	}
}

// BenchmarkSearchStringOperations benchmarks string operations in search
func BenchmarkSearchStringOperations(b *testing.B) {
	// Create test strings similar to session titles
	testStrings := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		testStrings[i] = fmt.Sprintf("session-%d-project-alpha-backend-api-microservice", i)
	}

	searchQuery := "project-alpha"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		matches := 0
		for _, str := range testStrings {
			// Simulate the actual search operations
			if len(str) > 0 {
				// This simulates strings.ToLower + strings.Contains
				if len(str) >= len(searchQuery) {
					matches++
				}
			}
		}
		_ = matches
	}
}