package app

import (
	"claude-squad/app/state"
	"claude-squad/config"
	"claude-squad/session"
	"claude-squad/ui"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

// BenchmarkLargeSessionNavigation tests navigation performance with many sessions
func BenchmarkLargeSessionNavigation(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Sessions_%d", count), func(b *testing.B) {
			h := setupBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate realistic navigation patterns
				// Navigate to middle of list
				for j := 0; j < count/4; j++ {
					h.list.Down()
				}

				// Navigate back up
				for j := 0; j < count/8; j++ {
					h.list.Up()
				}

				// Jump to end and back (using SetSelectedIdx)
				h.list.SetSelectedIdx(count - 1)
				h.list.SetSelectedIdx(0)
			}
		})
	}
}

// BenchmarkAttachDetachPerformance tests session attachment and detachment speed
func BenchmarkAttachDetachPerformance(b *testing.B) {
	sessionCounts := []int{10, 25, 50, 100}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("AttachDetach_%d", count), func(b *testing.B) {
			h := setupBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Get random instance
				instanceIndex := i % count
				h.list.SetSelectedIdx(instanceIndex)
				instance := h.list.GetSelectedInstance()

				if instance != nil {
					// Simulate attach operation timing
					start := time.Now()

					// Mock attach operations (without actually attaching for benchmark)
					h.tabbedWindow.UpdatePreview(instance)
					h.tabbedWindow.UpdateDiff(instance)
					h.menu.SetInstance(instance)

					// Track attach time
					attachTime := time.Since(start)
					b.ReportMetric(float64(attachTime.Nanoseconds()), "ns/attach")
				}
			}
		})
	}
}

// BenchmarkFilteringPerformance tests filtering and search performance
func BenchmarkFilteringPerformance(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Filter_%d", count), func(b *testing.B) {
			h := setupBenchmarkHome(b, count)

			// Add varied statuses for realistic filtering
			instances := h.list.GetInstances()
			statuses := []session.Status{session.Ready, session.Running, session.Paused, session.NeedsApproval}
			for i, instance := range instances {
				instance.Status = statuses[i%len(statuses)]
			}

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test paused filter toggle
				h.list.TogglePausedFilter()
				h.list.TogglePausedFilter()

				// Test search functionality
				h.list.SearchByTitle("test")
				h.list.ExitSearchMode()
			}
		})
	}
}

// BenchmarkCategoryOrganization tests category organization performance
func BenchmarkCategoryOrganization(b *testing.B) {
	sessionCounts := []int{100, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Categories_%d", count), func(b *testing.B) {
			h := setupBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				h.list.OrganizeByCategory()
			}
		})
	}
}

// BenchmarkRenderingPerformance tests UI rendering with many sessions
func BenchmarkRenderingPerformance(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500, 1000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Render_%d", count), func(b *testing.B) {
			h := setupBenchmarkHome(b, count)

			// Set realistic window size
			h.updateHandleWindowSizeEvent(struct {
				Width  int
				Height int
			}{Width: 120, Height: 40})

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Test full UI rendering
				_ = h.View()

				// Test list rendering specifically
				_ = h.list.String()
			}
		})
	}
}

// BenchmarkMemoryUsage tests memory efficiency with large session counts
func BenchmarkMemoryUsage(b *testing.B) {
	sessionCounts := []int{100, 500, 1000, 2000}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Memory_%d", count), func(b *testing.B) {
			h := setupBenchmarkHome(b, count)

			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				// Simulate realistic usage pattern
				for j := 0; j < 10; j++ {
					h.list.Down()
					h.instanceChanged()
				}

				// Force some operations that might allocate
				h.list.OrganizeByCategory()
				_ = h.View()
			}
		})
	}
}

// BenchmarkStartupPerformance tests application startup with existing sessions
func BenchmarkStartupPerformance(b *testing.B) {
	sessionCounts := []int{50, 100, 200, 500}

	for _, count := range sessionCounts {
		b.Run(fmt.Sprintf("Startup_%d", count), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()

				// Setup storage with existing sessions
				appConfig := config.DefaultConfig()
				appState := config.LoadState()
				storage, err := session.NewStorage(appState)
				if err != nil {
					b.Fatalf("Failed to create storage: %v", err)
				}

				// Pre-populate with sessions
				instances := make([]*session.Instance, count)
				for j := 0; j < count; j++ {
					instance, err := session.NewInstance(session.InstanceOptions{
						Title:    fmt.Sprintf("startup-session-%d", j),
						Path:     ".",
						Program:  "echo",
						Category: fmt.Sprintf("Category-%d", j%5),
					})
					if err != nil {
						b.Fatalf("Failed to create instance: %v", err)
					}
					instances[j] = instance
				}
				storage.SaveInstances(instances)

				b.StartTimer()

				// Simulate app startup
				ctx := context.Background()
				h := &home{
					ctx:          ctx,
					spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
					menu:         ui.NewMenu(),
					tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewVCPane()),
					errBox:       ui.NewErrBox(),
					storage:      storage,
					appConfig:    appConfig,
					program:      "echo",
					autoYes:      true,
					stateManager: state.NewManager(),
					appState:     appState,
				}
				h.list = ui.NewList(&h.spinner, true, appState)

				// Load instances (simulate startup)
				loadedInstances, err := storage.LoadInstances()
				if err != nil {
					b.Fatalf("Failed to load instances: %v", err)
				}
				for _, instance := range loadedInstances {
					h.list.AddInstance(instance)
				}

				h.updateHandleWindowSizeEvent(struct {
					Width  int
					Height int
				}{Width: 120, Height: 40})
			}
		})
	}
}

// setupBenchmarkHome creates a test home instance with specified number of sessions
func setupBenchmarkHome(b *testing.B, sessionCount int) *home {
	appConfig := config.DefaultConfig()
	appState := config.LoadState()

	storage, err := session.NewStorage(appState)
	if err != nil {
		b.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()
	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane(), ui.NewVCPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      "echo",
		autoYes:      true,
		stateManager: state.NewManager(),
		appState:     appState,
	}
	h.list = ui.NewList(&h.spinner, true, appState)

	// Create sessions with varied categories and realistic data
	categories := []string{"Development", "Testing", "Production", "Staging", "Research", "Bugfix", "Feature", "Hotfix", "Uncategorized"}
	programs := []string{"claude", "aider", "cursor", "codeium", "copilot"}

	for i := 0; i < sessionCount; i++ {
		category := categories[i%len(categories)]
		program := programs[i%len(programs)]

		instance, err := session.NewInstance(session.InstanceOptions{
			Title:    fmt.Sprintf("perf-test-session-%d", i),
			Path:     fmt.Sprintf("./test-path-%d", i%10), // Vary paths
			Program:  program,
			Category: category,
		})
		if err != nil {
			b.Fatalf("Failed to create instance: %v", err)
		}

		// Set varied statuses
		statuses := []session.Status{session.Ready, session.Running, session.Paused, session.NeedsApproval}
		instance.Status = statuses[i%len(statuses)]

		h.list.AddInstance(instance)
	}

	// Set window size for realistic rendering
	h.updateHandleWindowSizeEvent(struct {
		Width  int
		Height int
	}{Width: 120, Height: 40})

	return h
}

// BenchmarkRealtimeUpdates tests performance of real-time session updates
func BenchmarkRealtimeUpdates(b *testing.B) {
	h := setupBenchmarkHome(b, 100)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulate status updates across multiple sessions
		instances := h.list.GetInstances()
		for j := 0; j < len(instances) && j < 10; j++ {
			// Cycle through statuses
			switch instances[j].Status {
			case session.Ready:
				instances[j].Status = session.Running
			case session.Running:
				instances[j].Status = session.Paused
			case session.Paused:
				instances[j].Status = session.Ready
			}
		}

		// Update UI to reflect changes
		h.list.OrganizeByCategory()
		_ = h.View()
	}
}
