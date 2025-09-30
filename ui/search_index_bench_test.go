package ui

import (
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/sahilm/fuzzy"
)

// BenchmarkHybridSearchIndex benchmarks the new hybrid search index implementation
func BenchmarkHybridSearchIndex(b *testing.B) {
	sessionCounts := []int{10, 50, 100, 500, 1000, 2000}
	queries := []string{"project", "work", "api", "react", "backend", "frontend"}

	for _, count := range sessionCounts {
		for _, query := range queries {
			b.Run(fmt.Sprintf("hybrid_sessions_%d_query_%s", count, query), func(b *testing.B) {
				// Setup realistic session data
				instances := createRealisticInstances(count)

				// Create list with search index
				spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
				list := NewList(&spinner, false, nil)

				// Add instances to list (this triggers index building)
				for _, instance := range instances {
					finalizer := list.AddInstance(instance)
					finalizer()
				}

				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					// Use the optimized search through the List interface
					list.SearchByTitle(query)

					// Get results count (simulating real usage)
					_ = len(list.searchResults)

					// Reset search mode for next iteration
					list.ExitSearchMode()
				}
			})
		}
	}
}

// BenchmarkHybridVsOriginalSearch compares hybrid search vs original sahilm/fuzzy
func BenchmarkHybridVsOriginalSearch(b *testing.B) {
	instances := createRealisticInstances(1000)
	query := "backend"

	b.Run("hybrid_search_index", func(b *testing.B) {
		// Setup list with hybrid search index
		spinner := spinner.New(spinner.WithSpinner(spinner.MiniDot))
		list := NewList(&spinner, false, nil)

		for _, instance := range instances {
			finalizer := list.AddInstance(instance)
			finalizer()
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			list.SearchByTitle(query)
			_ = len(list.searchResults)
			list.ExitSearchMode()
		}
	})

	b.Run("original_sahilm_fuzzy", func(b *testing.B) {
		// Use the original SessionSearchSource approach
		for i := 0; i < b.N; i++ {
			searchSource := SessionSearchSource{sessions: instances}
			matches := fuzzy.FindFrom(query, searchSource)
			_ = len(matches)
		}
	})
}

// BenchmarkSearchIndexRebuild benchmarks index rebuild performance
func BenchmarkSearchIndexRebuild(b *testing.B) {
	sessionCounts := []int{100, 500, 1000, 2000, 5000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("rebuild_%d_sessions", count), func(b *testing.B) {
			instances := createRealisticInstances(count)
			searchIndex := NewSearchIndex()

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				searchIndex.RebuildIndex(instances)
			}
		})
	}
}

// BenchmarkCategoryOptimizedSearch benchmarks category-aware search optimizations
func BenchmarkCategoryOptimizedSearch(b *testing.B) {
	instances := createRealisticInstances(1000)
	searchIndex := NewSearchIndex()
	searchIndex.RebuildIndex(instances)

	categoryQueries := []string{"work", "personal", "development", "testing"}

	for _, query := range categoryQueries {
		b.Run(fmt.Sprintf("category_%s", query), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				results := searchIndex.Search(query, 100)
				_ = len(results)
			}
		})
	}
}

// BenchmarkProgramOptimizedSearch benchmarks program-aware search optimizations
func BenchmarkProgramOptimizedSearch(b *testing.B) {
	instances := createRealisticInstances(1000)
	searchIndex := NewSearchIndex()
	searchIndex.RebuildIndex(instances)

	programQueries := []string{"claude", "aider", "cursor", "vim"}

	for _, query := range programQueries {
		b.Run(fmt.Sprintf("program_%s", query), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				results := searchIndex.Search(query, 100)
				_ = len(results)
			}
		})
	}
}

// BenchmarkSearchIndexMemoryUsage benchmarks memory allocation patterns
func BenchmarkSearchIndexMemoryUsage(b *testing.B) {
	instances := createRealisticInstances(1000)
	queries := []string{"backend", "frontend", "api", "service", "work"}

	b.Run("hybrid_memory_allocation", func(b *testing.B) {
		b.ReportAllocs()

		searchIndex := NewSearchIndex()
		searchIndex.RebuildIndex(instances)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			for _, query := range queries {
				results := searchIndex.Search(query, 50)
				_ = len(results)
			}
		}
	})
}

// BenchmarkSearchLatencyByQueryLength benchmarks search performance by query length
func BenchmarkSearchLatencyByQueryLength(b *testing.B) {
	instances := createRealisticInstances(1000)
	searchIndex := NewSearchIndex()
	searchIndex.RebuildIndex(instances)

	queryLengthTests := []struct {
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

	for _, qt := range queryLengthTests {
		b.Run(qt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				results := searchIndex.Search(qt.query, 50)
				_ = len(results)
			}
		})
	}
}

// BenchmarkFullWorkflowWithIndex benchmarks the complete search workflow with indexing
func BenchmarkFullWorkflowWithIndex(b *testing.B) {
	sessionCounts := []int{100, 500, 1000, 2000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("full_workflow_indexed_%d", count), func(b *testing.B) {
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
				// Enter search mode with hybrid indexing
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

// BenchmarkIndexValidation benchmarks index validation and rebuild checks
func BenchmarkIndexValidation(b *testing.B) {
	instances := createRealisticInstances(1000)
	searchIndex := NewSearchIndex()

	b.Run("validation_check", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = searchIndex.IsIndexValid()
		}
	})

	b.Run("index_stats", func(b *testing.B) {
		searchIndex.RebuildIndex(instances)
		for i := 0; i < b.N; i++ {
			_ = searchIndex.GetStats()
		}
	})
}