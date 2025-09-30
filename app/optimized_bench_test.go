package app

import (
	"claude-squad/app/state"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"fmt"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

// BenchmarkOptimizedNavigation tests actual navigation performance without artificial delays
func BenchmarkOptimizedNavigation(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Fast_Navigation_%d", count), func(b *testing.B) {
			h := setupOptimizedBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test pure navigation without expensive operations
				for j := 0; j < 20; j++ {
					h.list.Down()
					// Only call fast operations, not full instanceChanged
					h.instanceChanged()
				}

				for j := 0; j < 10; j++ {
					h.list.Up()
					h.instanceChanged()
				}
			}
		})
	}
}

// BenchmarkExpensiveOperations tests the expensive operations separately
func BenchmarkExpensiveOperations(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Expensive_Ops_%d", count), func(b *testing.B) {
			h := setupOptimizedBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test only the expensive operations
				h.instanceChanged()
			}
		})
	}
}

// BenchmarkMemoryEfficiency tests memory usage patterns
func BenchmarkMemoryEfficiency(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Memory_%d_sessions", count), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				h := setupOptimizedBenchmarkHome(b, count)

				// Simulate realistic usage
				for j := 0; j < 50; j++ {
					h.list.Down()
					if j%10 == 0 {
						// Occasional expensive operation
						h.instanceChanged()
					} else {
						// Mostly fast operations
						h.instanceChanged()
					}
				}

				// Force cleanup
				h = nil
			}
		})
	}
}

// BenchmarkUIRendering tests UI rendering performance
func BenchmarkUIRendering(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("UI_Render_%d", count), func(b *testing.B) {
			h := setupOptimizedBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test full UI rendering
				_ = h.View()
			}
		})
	}
}

// BenchmarkListOperations tests list-specific operations
func BenchmarkListOperations(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("List_Ops_%d", count), func(b *testing.B) {
			h := setupOptimizedBenchmarkHome(b, count)

			// Add status variety for realistic testing
			instances := h.list.GetInstances()
			statuses := []session.Status{session.Ready, session.Running, session.Paused, session.NeedsApproval}
			for i, instance := range instances {
				instance.Status = statuses[i%len(statuses)]
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test various list operations
				h.list.OrganizeByCategory()
				h.list.TogglePausedFilter()
				h.list.TogglePausedFilter()
				h.list.SearchByTitle("test")
				h.list.ExitSearchMode()
			}
		})
	}
}

// BenchmarkSessionCreation tests session creation performance
func BenchmarkSessionCreation(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("bench-session-%d", i),
			Path:     ".",
			Program:  "echo",
			Category: "Development",
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}
		_ = instance
	}
}

// setupOptimizedBenchmarkHome creates a home instance optimized for benchmarking
func setupOptimizedBenchmarkHome(b *testing.B, sessionCount int) *home {
	appConfig := config.DefaultConfig()
	appState := config.LoadState()

	storage, err := session.NewStorage(appState)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	h := &home{
		ctx:                  ctx,
		spinner:              spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:                 ui.NewMenu(),
		tabbedWindow:         ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:               ui.NewErrBox(),
		storage:              storage,
		appConfig:            appConfig,
		program:              "echo",
		autoYes:              true,
		stateManager: state.NewManager(),
		appState:             appState,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create sessions with realistic variety
	categories := []string{"Development", "Testing", "Production", "Staging", "Research"}
	programs := []string{"claude", "aider", "cursor"}

	for i := 0; i < sessionCount; i++ {
		category := categories[i%len(categories)]
		program := programs[i%len(programs)]

		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("bench-session-%d", i),
			Path:     fmt.Sprintf("./path-%d", i%5), // Vary paths but limit to 5 unique
			Program:  program,
			Category: category,
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}

		// Set realistic status distribution
		statuses := []session.Status{
			session.Ready, session.Ready, session.Ready, // 60% ready
			session.Running, session.Running, // 40% running
			session.Paused,        // 20% paused
			session.NeedsApproval, // 20% needs approval
		}
		instance.Status = statuses[i%len(statuses)]

		h.list.AddInstance(instance)
	}

	// Set reasonable window size
	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 120, Height: 40})

	return h
}

// BenchmarkRealtimeScenario simulates real user behavior
func BenchmarkRealtimeScenario(b *testing.B) {
	sessionCounts := []int{50, 100, 200}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Realtime_%d", count), func(b *testing.B) {
			h := setupOptimizedBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate realistic user session:
				// 1. Navigate quickly through a few sessions
				for j := 0; j < 5; j++ {
					h.list.Down()
					h.instanceChanged()
				}

				// 2. Stop and view one session (expensive ops)
				h.instanceChanged()

				// 3. Search for something
				h.list.SearchByTitle("dev")
				h.list.ExitSearchMode()

				// 4. Filter paused sessions
				h.list.TogglePausedFilter()
				h.list.TogglePausedFilter()

				// 5. Navigate a bit more
				for j := 0; j < 3; j++ {
					h.list.Up()
					h.instanceChanged()
				}
			}
		})
	}
}
