package ui

import (
	"fmt"
	"testing"

	"github.com/sahilm/fuzzy"
)

// BenchmarkRealisticSearchComparison shows true performance when index is pre-built
func BenchmarkRealisticSearchComparison(b *testing.B) {
	sessionCounts := []int{100, 500, 1000, 2000}
	queries := []string{"backend", "work", "api", "react"}

	for _, count := range sessionCounts {
		for _, query := range queries {
			b.Run(fmt.Sprintf("sessions_%d_query_%s", count, query), func(b *testing.B) {
				instances := createRealisticInstances(count)

				// Test 1: Hybrid search with pre-built index
				b.Run("hybrid_prebuilt", func(b *testing.B) {
					searchIndex := NewSearchIndex()
					searchIndex.RebuildIndex(instances) // Pre-build index once

					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						results := searchIndex.Search(query, 50)
						_ = len(results)
					}
				})

				// Test 2: Original sahilm/fuzzy approach
				b.Run("original_fuzzy", func(b *testing.B) {
					searchSource := SessionSearchSource{sessions: instances}

					b.ResetTimer()

					for i := 0; i < b.N; i++ {
						matches := fuzzy.FindFrom(query, searchSource)
						_ = len(matches)
					}
				})
			})
		}
	}
}

// BenchmarkIndexOperations benchmarks different index operations separately
func BenchmarkIndexOperations(b *testing.B) {
	instances := createRealisticInstances(1000)
	searchIndex := NewSearchIndex()
	searchIndex.RebuildIndex(instances)

	b.Run("category_detection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("work", 50) // Should hit category optimization
			_ = len(results)
		}
	})

	b.Run("program_detection", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("claude", 50) // Should hit program optimization
			_ = len(results)
		}
	})

	b.Run("hybrid_fuzzy_search", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("backend service api", 50) // Should use hybrid approach
			_ = len(results)
		}
	})

	b.Run("closestmatch_prefilter", func(b *testing.B) {
		if searchIndex.closestMatch != nil {
			for i := 0; i < b.N; i++ {
				candidates := searchIndex.closestMatch.ClosestN("backend", 100)
				_ = len(candidates)
			}
		}
	})
}

// BenchmarkMemoryOptimizedSearch benchmarks memory usage patterns
func BenchmarkMemoryOptimizedSearch(b *testing.B) {
	instances := createRealisticInstances(1000)

	b.Run("hybrid_memory_usage", func(b *testing.B) {
		b.ReportAllocs()

		searchIndex := NewSearchIndex()
		searchIndex.RebuildIndex(instances) // One-time cost

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("backend", 50)
			_ = len(results)
		}
	})

	b.Run("original_memory_usage", func(b *testing.B) {
		b.ReportAllocs()

		searchSource := SessionSearchSource{sessions: instances}

		for i := 0; i < b.N; i++ {
			matches := fuzzy.FindFrom("backend", searchSource)
			_ = len(matches)
		}
	})
}

// BenchmarkScalabilityComparison shows performance scaling
func BenchmarkScalabilityComparison(b *testing.B) {
	sessionCounts := []int{10, 50, 100, 500, 1000, 2000, 5000}
	query := "backend api service"

	for _, count := range sessionCounts {
		instances := createRealisticInstances(count)

		b.Run(fmt.Sprintf("hybrid_%d", count), func(b *testing.B) {
			searchIndex := NewSearchIndex()
			searchIndex.RebuildIndex(instances)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				results := searchIndex.Search(query, 50)
				_ = len(results)
			}
		})

		b.Run(fmt.Sprintf("original_%d", count), func(b *testing.B) {
			searchSource := SessionSearchSource{sessions: instances}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				matches := fuzzy.FindFrom(query, searchSource)
				_ = len(matches)
			}
		})
	}
}

// BenchmarkOptimizationStrategies benchmarks different optimization strategies
func BenchmarkOptimizationStrategies(b *testing.B) {
	instances := createRealisticInstances(1000)
	searchIndex := NewSearchIndex()
	searchIndex.RebuildIndex(instances)

	// Test instant category matches
	b.Run("instant_category_work", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("work", 50)
			_ = len(results)
		}
	})

	b.Run("instant_category_personal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("personal", 50)
			_ = len(results)
		}
	})

	// Test instant program matches
	b.Run("instant_program_claude", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("claude", 50)
			_ = len(results)
		}
	})

	b.Run("instant_program_aider", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("aider", 50)
			_ = len(results)
		}
	})

	// Test hybrid fuzzy search for complex queries
	b.Run("complex_fuzzy_query", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := searchIndex.Search("backend react service", 50)
			_ = len(results)
		}
	})
}